//go:build darwin

// The Objective-C half of the macOS host.
//
// This is a separate file rather than a cgo preamble, and that is forced rather
// than chosen: cgo emits the preamble into BOTH its main generated C file and
// _cgo_export.c, so any DEFINITION in a preamble is compiled twice and the link
// fails with duplicate symbols. Declarations may live in the preamble;
// @implementation blocks and function bodies may not.
//
// What genuinely requires Objective-C is protocol conformance — AppKit and
// WebKit expose no C API, and serving assets and receiving calls each need a
// class conforming to a protocol. Wails' WailsContext conforms to four
// (WKURLSchemeHandler, WKScriptMessageHandler, WKNavigationDelegate,
// WKUIDelegate); this needs two.

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#include <stdlib.h>

// Declares hostServeAsset and hostHandleCall, both implemented in Go.
#include "_cgo_export.h"

static WKWebView *gWebView = nil;

// The scheme assets are served over. It must not be http or https: WebKit
// reserves those and registering a handler for one throws. Wails registers
// "wails" through the identical call, and the app's 'self'-only CSP works
// against it today — which is the evidence that a custom scheme does not
// produce an origin the CSP refuses.
static NSString *const kScheme = @"drmd";

// One class, both protocols.
@interface DrmdBridge : NSObject <WKURLSchemeHandler, WKScriptMessageHandler>
@end

@implementation DrmdBridge

- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)task {
  NSString *path = task.request.URL.path;
  if (path.length == 0 || [path isEqualToString:@"/"]) {
    path = @"/index.html";
  }

  int length = 0;
  char *mimeType = NULL;
  void *body = hostServeAsset((char *)path.UTF8String, &length, &mimeType);
  if (body == NULL) {
    [task didFailWithError:[NSError errorWithDomain:@"drmd" code:404 userInfo:nil]];
    return;
  }

  NSData *data = [NSData dataWithBytes:body length:length];
  NSURLResponse *response =
      [[NSURLResponse alloc] initWithURL:task.request.URL
                                MIMEType:[NSString stringWithUTF8String:mimeType]
                   expectedContentLength:length
                        textEncodingName:@"utf-8"];
  [task didReceiveResponse:response];
  [task didReceiveData:data];
  [task didFinish];

  // Go allocated both through C.CBytes / C.CString; this side owns them now.
  free(body);
  free(mimeType);
}

- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)task {
}

- (void)userContentController:(WKUserContentController *)controller
      didReceiveScriptMessage:(WKScriptMessage *)message {
  if (![message.body isKindOfClass:[NSDictionary class]]) return;
  NSDictionary *body = (NSDictionary *)message.body;

  NSData *argsData = [NSJSONSerialization dataWithJSONObject:(body[@"args"] ?: @[])
                                                     options:0
                                                       error:nil];
  NSString *args = [[NSString alloc] initWithData:argsData encoding:NSUTF8StringEncoding];

  hostHandleCall([body[@"id"] intValue], (char *)[body[@"method"] UTF8String],
                 (char *)args.UTF8String);
}

@end

// WKWebView handles drops itself and would try to navigate to a dropped file.
// Subclassing lets the host take the drop first: file URLs go to Go, and
// anything else falls through to the webview's own handling so text drags into
// the editor still work.
@interface DrmdWebView : WKWebView
@end

@implementation DrmdWebView

- (NSDragOperation)draggingEntered:(id<NSDraggingInfo>)sender {
  return [self fileURLsFrom:sender].count > 0 ? NSDragOperationCopy
                                              : [super draggingEntered:sender];
}

- (NSDragOperation)draggingUpdated:(id<NSDraggingInfo>)sender {
  return [self fileURLsFrom:sender].count > 0 ? NSDragOperationCopy
                                              : [super draggingUpdated:sender];
}

- (BOOL)performDragOperation:(id<NSDraggingInfo>)sender {
  NSArray<NSURL *> *urls = [self fileURLsFrom:sender];
  if (urls.count == 0) {
    return [super performDragOperation:sender];
  }

  NSMutableArray<NSString *> *paths = [NSMutableArray arrayWithCapacity:urls.count];
  for (NSURL *url in urls) [paths addObject:url.path];

  NSData *json = [NSJSONSerialization dataWithJSONObject:paths options:0 error:nil];
  NSString *encoded = [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
  hostFileDrop((char *)encoded.UTF8String);
  return YES;
}

- (NSArray<NSURL *> *)fileURLsFrom:(id<NSDraggingInfo>)sender {
  NSArray *urls = [sender.draggingPasteboard
      readObjectsForClasses:@[ [NSURL class] ]
                    options:@{NSPasteboardURLReadingFileURLsOnlyKey : @YES}];
  return urls ?: @[];
}

@end

@interface DrmdDelegate : NSObject <NSApplicationDelegate>
@end

// Close is a TWO-PHASE operation, and it has to be.
//
// beforeClose asks the user whether to save, and that dialog needs the main
// thread to display. windowShouldClose: already runs ON the main thread, so
// answering it synchronously would deadlock: the guard waits for a dialog that
// cannot appear until the guard returns.
//
// So the window refuses the first close, the decision is taken off-thread, and
// the window is closed again from Go once it is allowed.
@interface DrmdWindowDelegate : NSObject <NSWindowDelegate>
@property(nonatomic) BOOL closeApproved;
@end

static DrmdWindowDelegate *gWindowDelegate = nil;
static NSWindow *gWindow = nil;

@implementation DrmdWindowDelegate

- (BOOL)windowShouldClose:(NSWindow *)sender {
  if (self.closeApproved) return YES;
  hostRequestClose();
  return NO;
}

@end

void hostCloseNow(void) {
  dispatch_async(dispatch_get_main_queue(), ^{
    gWindowDelegate.closeApproved = YES;
    [gWindow close];
  });
}

@implementation DrmdDelegate
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)app {
  return YES;
}

// Files the OS routes to the application: a double-click in Finder, or `open`
// against a document this app is registered for.
//
// Only fires for a BUNDLED application. A bare binary is not registered for any
// document type, so this path cannot be exercised until the app is packaged —
// which is stated rather than left to look tested.
- (void)application:(NSApplication *)app openURLs:(NSArray<NSURL *> *)urls {
  for (NSURL *url in urls) {
    if (url.isFileURL) hostFileOpened((char *)url.path.UTF8String);
  }
}

// Tells Go the host is going away, BEFORE AppKit tears the webview down.
// Without it, an in-flight bound call resolves into a dead WKWebView and any
// goroutine parked on a dialog waits for an answer that can no longer come.
- (void)applicationWillTerminate:(NSNotification *)note {
  hostShuttingDown();
}
@end

void hostRun(const char *title, int width, int height, int dropMode) {
  @autoreleasepool {
    NSApplication *app = [NSApplication sharedApplication];
    [app setActivationPolicy:NSApplicationActivationPolicyRegular];
    app.delegate = [[DrmdDelegate alloc] init];

    DrmdBridge *bridgeObj = [[DrmdBridge alloc] init];

    WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];
    [config setURLSchemeHandler:bridgeObj forURLScheme:kScheme];
    [config.userContentController addScriptMessageHandler:bridgeObj name:@"drmd"];
    // Web Inspector. The gate verdicts are read from its console.
    [config.preferences setValue:@YES forKey:@"developerExtrasEnabled"];

    // The bound surface is an explicit two-method object, NOT a proxy over every
    // name. bridge.js degrades when a binding is absent — `wails()?.X() ?? missing(X)`
    // — and that degradation is exactly what lets the whole frontend boot against
    // a host implementing nothing. A proxy would make every method look present
    // and route it to a dispatcher with no answer.
    NSString *js =
        [NSString stringWithFormat:@"globalThis.__drmdDropMode = %@; globalThis.__drmdWalkMode = %@; globalThis.__drmdCloseMode = %@; globalThis.__drmdCloseDirty = %@; globalThis.__drmdDocMode = %@;",
                                    dropMode == 1 ? @"true" : @"false",
                                    dropMode == 2 ? @"true" : @"false",
                                    (dropMode == 3 || dropMode == 4) ? @"true" : @"false",
                                    dropMode == 4 ? @"true" : @"false",
                                    dropMode == 5 ? @"true" : @"false"];
    js = [js stringByAppendingString:
        @"(() => {"
        @"let nextId = 1; const pending = new Map();"
        @"globalThis.__drmdResolve = (id, ok, payload) => {"
        @"  const p = pending.get(id); if (!p) return; pending.delete(id);"
        @"  ok ? p.resolve(payload) : p.reject(new Error(payload));"
        @"};"
        @"const call = (method, args) => new Promise((resolve, reject) => {"
        @"  const id = nextId++; pending.set(id, { resolve, reject });"
        @"  window.webkit.messageHandlers.drmd.postMessage({ id, method, args });"
        @"});"
        // The full bound surface, taken from app.go. It is NOT minimal, and it
        // cannot be: bridge.js degrades when go.main.App is ABSENT, but several
        // entries are written `wails()?.Method(x)` rather than
        // `wails()?.Method?.(x)`, so once the object exists a missing method is
        // a TypeError rather than a fallback. A partial host breaks boot.
        @"const NAMES = ['OpenDocument','SaveDocument','SaveDocumentAs','SyncDocuments',"
        @"  'SetDirty','UpdateContent','ListFontFamilies','LoadPreferences','SavePreferences',"
        @"  'OpenRecentDocument','ImportImage','ImportDroppedImage','LoadImageAsset',"
        @"  'OpenExternalURL','RecordClientEvent','RevealImageAsset','ResolveUnsavedChanges',"
        @"  'FrontendReady','Ping','Boom'];"
        @"const App = {};"
        @"for (const n of NAMES) App[n] = (...args) => call(n, args);"
        @"globalThis.go = { main: { App } };"
        // app.js subscribes through globalThis.runtime.EventsOn at two sites
        // (files:dropped and file:open), so a host must supply that surface too.
        // It is not part of go.main.App and a host that provides only the bound
        // methods leaves both subscriptions silently dead.
        @"const listeners = new Map();"
        @"globalThis.runtime = { EventsOn: (name, handler) => {"
        @"  if (!listeners.has(name)) listeners.set(name, []);"
        @"  listeners.get(name).push(handler);"
        @"} };"
        @"globalThis.__drmdEmit = (name, payload) => {"
        @"  for (const h of listeners.get(name) ?? []) h(payload);"
        @"};"
        // The gates run themselves and report through the same channel. A hang
        // is the failure mode gate 3 exists to detect, so every gate is raced
        // against a timeout — without that, the failure we are testing for
        // would present as a test that never finishes.
        @"const timed = (p, label) => Promise.race([p,"
        @"  new Promise((r) => setTimeout(() => r(label + ' TIMED OUT (hung)'), 5000))]);"
        // Capture boot failures. A shell that renders is not a shell that
        // booted: index.html carries static markup, so the window can look
        // right while app.js never finishes.
        @"const errors = [];"
        // boot() CATCHES its own failure into console.error, so neither the
        // error event nor unhandledrejection fires. Patching console.error at
        // document-start is the only way to see it — the same trap this project
        // recorded when a ReferenceError hid behind console.warn for two rounds
        // of debugging.
        @"const realError = console.error.bind(console);"
        @"console.error = (...a) => { errors.push(a.map(String).join(' ')); realError(...a); };"
        @"window.addEventListener('error', (e) =>"
        @"  errors.push(String(e.message) + ' @ ' + String(e.filename) + ':' + e.lineno));"
        @"window.addEventListener('unhandledrejection', (e) =>"
        @"  errors.push('unhandled rejection: ' + String(e.reason)));"
        @"const runGates = async () => {"
        @"  const out = {};"
        @"  for (let i = 0; i < 150 && !globalThis.__app?.ready; i++)"
        @"    await new Promise((r) => setTimeout(r, 100));"
        @"  out.gate1_app_ready = globalThis.__app?.ready === true;"
        @"  out.diag_app_exists = typeof globalThis.__app;"
        @"  out.diag_errors = errors.slice(0, 8);"
        @"  out.gate2_ping = await timed("
        @"    globalThis.go.main.App.Ping('hello').catch((e) => 'THREW: ' + e.message), 'ping');"
        @"  out.gate3_boom = await timed("
        @"    globalThis.go.main.App.Boom()"
        @"      .then(() => 'RESOLVED — WRONG, a panic must not resolve')"
        @"      .catch((e) => 'REJECTED: ' + e.message), 'boom');"
        @"  out.gate3b_survived = await timed("
        @"    globalThis.go.main.App.Ping('still here').catch((e) => 'THREW: ' + e.message), 'ping2');"
        // Gate 4: the Go -> frontend event channel. app.js subscribes through
        // globalThis.runtime.EventsOn for files:dropped and file:open, and a host
        // that implements only bound methods leaves both silently dead — no
        // error, just a file opened from Finder that never appears.
        @"  const got = new Promise((r) => globalThis.runtime.EventsOn('file:open', r));"
        @"  globalThis.go.main.App.Ping('__emit_file_open');"
        @"  out.gate4_event_received = await timed(got, 'event');"
        // Gate 5: the file-drop path from AppKit's callback through the
        // subscriber to the frontend. The OS delivery itself needs a real drag
        // and a person; everything downstream of it is exercised here.
        @"  const dropped = new Promise((r) => globalThis.runtime.EventsOn('files:dropped', r));"
        @"  globalThis.go.main.App.Ping('__simulate_drop');"
        @"  out.gate5_drop_delivered = await timed(dropped, 'drop');"
        // Gate 6: an actual document round trip. The other gates prove the host
        // works; this proves the APPLICATION does — the editor, the fidelity
        // layer, the typed dispatch, Go's atomic write, and the bytes on disk.
        // A table is used deliberately: its delimiter row is the construct the
        // editor is known to respell, so a byte-identical round trip here is
        // the same claim the shipping app makes.
        @"  const path = '/tmp/drmd-spike-roundtrip.md';"
        // Double-escaped ON PURPOSE. Objective-C turns \n into a real newline, and a
        // raw line break inside a JavaScript string literal is a SyntaxError that
        // kills the ENTIRE injected script — no bridge, no runtime shim, no gate
        // runner. The app still opens, because bridge.js degrades when the host is
        // absent, so it presents as a healthy window that reports nothing at all.
        @"  const fixture = 'Intro paragraph.\\n\\n| a | b |\\n| --- | --- |\\n| 1 | 2 |\\n';"
        @"  try {"
        // Layout metrics. A clipped empty state with a scrollbar means the page
        // is either taller than its viewport or scrolled; these say which.
        @"  const se = document.scrollingElement;"
        @"  const region = document.querySelector('#document-region');"
        @"  const empty = document.querySelector('#empty-state');"
        @"  out.layout = {"
        @"    innerHeight: window.innerHeight,"
        @"    clientHeight: se.clientHeight,"
        @"    scrollHeight: se.scrollHeight,"
        @"    scrollTop: se.scrollTop,"
        @"    dpr: window.devicePixelRatio,"
        @"    bodyOverflowY: getComputedStyle(document.body).overflowY,"
        @"    regionClient: region.clientHeight,"
        @"    regionScroll: region.scrollHeight,"
        @"    regionScrollTop: region.scrollTop,"
        @"    emptyHeight: empty.getBoundingClientRect().height,"
        @"    emptyTop: empty.getBoundingClientRect().top,"
        @"    alignSelf: getComputedStyle(empty).alignSelf,"
        @"  };"
        @"    out.gate6_stage = 'setMarkdown';"
        @"    await timed(globalThis.__app.setMarkdown(fixture), 'setMarkdown');"
        @"    out.gate6_stage = 'serialize';"
        @"    const serialized = globalThis.__app.getEditorMarkdown();"
        @"    out.gate6_stage = 'save';"
        @"    await timed(globalThis.go.main.App.SaveDocument(path, serialized), 'save');"
        @"    out.gate6_stage = 'reopen';"
        @"    const reopened = await timed(globalThis.go.main.App.OpenRecentDocument(path), 'reopen');"
        @"    out.gate6_stage = 'done';"
        // Compared against what the EDITOR produced, not against the fixture.
        // The editor respells a table's delimiter row — the standing
        // re-serialization blocker, tracked against the application — and a host
        // gate that asserts byte-identity to the input would be re-reporting
        // that known defect as a host failure. What this must prove is narrower
        // and is the host's actual responsibility: the bytes Go was handed are
        // the bytes that came back off disk.
        @"    out.gate6_editor_respelled = serialized !== fixture;"
        @"    out.gate6_saved_bytes = reopened && reopened.content === serialized"
        @"      ? 'ROUND TRIP EXACT'"
        @"      : 'DIFFERS: ' + JSON.stringify(reopened);"
        @"  } catch (e) { out.gate6_saved_bytes = 'THREW: ' + e.message; }"
        @"  window.webkit.messageHandlers.drmd.postMessage("
        @"    { id: 0, method: '__gates', args: [out] });"
        @"};"
        // In drop mode the gates are skipped: the window has to stay open and
        // idle so a real drag can be performed on it.
        @"globalThis.__drmdReportRealDrop = (paths) =>"
        @"  window.webkit.messageHandlers.drmd.postMessage({ id: 0, method: '__realdrop', args: [paths] });"
        @"globalThis.runtime.EventsOn('files:dropped', (paths) => {"
        @"  if (globalThis.__drmdDropMode) globalThis.__drmdReportRealDrop(paths);"
        @"});"
        @"window.addEventListener('load', () => {"
        @"  if (globalThis.__drmdWalkMode) { import('drmd://app/__walk.js').catch((e) =>"
        @"    window.webkit.messageHandlers.drmd.postMessage({ id: 0, method: '__walk',"
        @"      args: [[{ name: 'walk module loaded', ok: false, detail: String(e && e.message) }]] })); }"
        @"  else if (globalThis.__drmdCloseMode) {"
        // Triggered once the app is READY rather than on a timer: the guard
        // reads dirty state, and asking before boot has populated it would
        // measure an empty session.
        @"    (async () => {"
        @"      for (let i = 0; i < 200 && !globalThis.__app?.ready; i++)"
        @"        await new Promise((r) => setTimeout(r, 50));"
        // A dirty buffer when asked for, so the guard is exercised on the case
        // that matters. A guard only ever tested against a clean document
        // proves nothing about whether it protects anything.
        @"      if (globalThis.__drmdCloseDirty) {"
        @"        await globalThis.__app.setMarkdown('# unsaved work\\n');"
        @"        globalThis.__app.debugSimulateEdit('# unsaved work\\n');"
        @"        await new Promise((r) => setTimeout(r, 250));"
        @"      }"
        @"      window.webkit.messageHandlers.drmd.postMessage({ id: 0, method: '__closenow', args: [] });"
        @"    })();"
        @"  }"
        @"  else if (globalThis.__drmdDocMode) { import('drmd://app/__doc.js'); }"
        @"  else if (!globalThis.__drmdDropMode) { runGates(); }"
        @"});"
        @"})()"];
    // The content world is load-bearing, and getting it wrong fails silently.
    // The three-argument initialiser does NOT reliably place the script in the
    // page's world, and a script in a separate world has its own globalThis: the
    // bridge is invisible to bridge.js, and the page's window.__app is invisible
    // to the gate runner. Nothing errors — both sides simply see an empty room.
    [config.userContentController
        addUserScript:[[WKUserScript alloc] initWithSource:js
                                            injectionTime:WKUserScriptInjectionTimeAtDocumentStart
                                         forMainFrameOnly:YES
                                           inContentWorld:[WKContentWorld pageWorld]]];

    NSRect frame = NSMakeRect(0, 0, width, height);
    NSWindow *window = [[NSWindow alloc]
        initWithContentRect:frame
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable |
                             NSWindowStyleMaskResizable | NSWindowStyleMaskMiniaturizable)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    window.title = [NSString stringWithUTF8String:title];

    gWebView = [[DrmdWebView alloc] initWithFrame:frame configuration:config];
    [gWebView registerForDraggedTypes:@[ NSPasteboardTypeFileURL ]];
    gWebView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    window.contentView = gWebView;

    NSURL *start =
        [NSURL URLWithString:[NSString stringWithFormat:@"%@://app/index.html", kScheme]];
    [gWebView loadRequest:[NSURLRequest requestWithURL:start]];

    gWindow = window;
    gWindowDelegate = [[DrmdWindowDelegate alloc] init];
    window.delegate = gWindowDelegate;

    [window center];
    [window makeKeyAndOrderFront:nil];
    [app activateIgnoringOtherApps:YES];
    [app run];
  }
}

// hostRunBare starts the AppKit event loop for the modal walkthrough. The run
// loop is required at all because dispatch_async to the main queue is only
// serviced once one is going.
//
// It creates a small ANCHOR WINDOW rather than running windowless. A modal with
// no parent window is placed by AppKit wherever it likes, which on a multi-
// monitor desk means dialogs opening on a screen the operator is not looking
// at — they then answer the wrong prompt, or appear not to answer at all, and
// the walkthrough reports a defect that is really a misplaced window.
// hostRunBare starts the AppKit event loop for the modal walkthrough. A run
// loop is required at all because dispatch_async to the main queue is only
// serviced once one is going.
//
// NO WINDOW. An earlier version put an instruction window on screen to solve
// dialogs opening on the wrong monitor, and it cost two rounds: at floating
// level it sat above the modal and took its key status, and at normal level it
// still held focus, so Return never reached the dialog. The dialog is the thing
// under test, and nothing else may compete with it for the keyboard.
//
// Placement is left to AppKit. In the real application there is always a window
// for a dialog to centre on; only this harness lacks one, so misplacement here
// is an artefact of the harness rather than a property of the host.
void hostRunBare(void) {
  NSApplication *app = [NSApplication sharedApplication];
  [app setActivationPolicy:NSApplicationActivationPolicyRegular];
  [app activateIgnoringOtherApps:YES];
  [app run];
}

#pragma mark - Native operations

// Every operation dispatches ASYNC to the main thread and reports back through
// hostModalResult, which hands the answer to a waiting goroutine over a channel.
//
// dispatch_sync would be simpler and is wrong: it holds the cgo call open for
// as long as the dialog is on screen — potentially minutes — pinning an OS
// thread and leaving the goroutine unpreemptable. Worse, it makes the
// context.Context every nativePort method already takes impossible to honour.
// With a channel the goroutine parks for free and a cancelled context can
// actually abandon the wait.

static void reportModal(int callID, NSString *value) {
  char *copy = strdup(value == nil ? "" : value.UTF8String);
  hostModalResult(callID, copy);
  free(copy);
}

void hostOpenFile(int callID, const char *title, const char *extensionsCSV) {
  NSString *heading = [NSString stringWithUTF8String:title];
  NSString *exts = [NSString stringWithUTF8String:extensionsCSV];

  dispatch_async(dispatch_get_main_queue(), ^{
    // Without this a panel can open behind whatever the operator is actually
    // looking at, and a dialog nobody can see reads as a dialog that never
    // returned.
    [NSApp activateIgnoringOtherApps:YES];
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    panel.title = heading;
    panel.canChooseFiles = YES;
    panel.canChooseDirectories = NO;
    panel.allowsMultipleSelection = NO;
    if (exts.length > 0) {
      panel.allowedFileTypes = [exts componentsSeparatedByString:@","];
    }
    reportModal(callID, [panel runModal] == NSModalResponseOK ? panel.URL.path : @"");
  });
}

void hostSaveFile(int callID, const char *title, const char *defaultName,
                  const char *extensionsCSV) {
  NSString *heading = [NSString stringWithUTF8String:title];
  NSString *name = [NSString stringWithUTF8String:defaultName];
  NSString *exts = [NSString stringWithUTF8String:extensionsCSV];

  dispatch_async(dispatch_get_main_queue(), ^{
    [NSApp activateIgnoringOtherApps:YES];
    NSSavePanel *panel = [NSSavePanel savePanel];
    panel.title = heading;
    if (name.length > 0) panel.nameFieldStringValue = name;
    if (exts.length > 0) {
      panel.allowedFileTypes = [exts componentsSeparatedByString:@","];
    }
    reportModal(callID, [panel runModal] == NSModalResponseOK ? panel.URL.path : @"");
  });
}

// Reports the TITLE of the button pressed, matching what the application
// already expects — callers compare against "Save", "Overwrite" and so on, so
// returning an index would move that decision into this file.
void hostDialog(int callID, const char *title, const char *message,
                const char *buttonsCSV, const char *defaultButton,
                const char *cancelButton, int isError) {
  NSString *heading = [NSString stringWithUTF8String:title];
  NSString *body = [NSString stringWithUTF8String:message];
  NSString *csv = [NSString stringWithUTF8String:buttonsCSV];
  NSString *defaultTitle = [NSString stringWithUTF8String:defaultButton];
  NSString *cancelTitle = [NSString stringWithUTF8String:cancelButton];

  dispatch_async(dispatch_get_main_queue(), ^{
    [NSApp activateIgnoringOtherApps:YES];
    NSAlert *alert = [[NSAlert alloc] init];
    alert.messageText = heading;
    alert.informativeText = body;
    alert.alertStyle = isError ? NSAlertStyleCritical : NSAlertStyleWarning;

    NSArray<NSString *> *titles =
        csv.length > 0 ? [csv componentsSeparatedByString:@","] : @[ @"OK" ];

    // ADD THE DEFAULT BUTTON FIRST, rather than adding in the caller's order and
    // then moving the default with keyEquivalent.
    //
    // Setting keyEquivalent looks like the obvious fix and does not work: NSAlert
    // lays its buttons out lazily and re-asserts "\r" on whichever button was
    // added first, so the assignment is silently undone. Escape survived because
    // NSAlert honours a button titled Cancel; Return did not. That is the bug the
    // modal walkthrough caught, and it made Return overwrite the user's file.
    //
    // Adding the safe button first is also the correct macOS presentation: the
    // rightmost button is the default, and a destructive confirm should not have
    // the destructive action there.
    NSMutableArray<NSString *> *ordered = [NSMutableArray arrayWithCapacity:titles.count];
    if (defaultTitle.length > 0 && [titles containsObject:defaultTitle]) {
      [ordered addObject:defaultTitle];
    }
    for (NSString *t in titles) {
      if (![ordered containsObject:t]) [ordered addObject:t];
    }

    NSMutableArray<NSButton *> *buttons = [NSMutableArray arrayWithCapacity:ordered.count];
    for (NSString *t in ordered) [buttons addObject:[alert addButtonWithTitle:t]];

    // Assign AFTER every button exists. NSAlert hands the first button "\r" as
    // it is added, so assigning during the loop can be undone by a later add.
    for (NSUInteger i = 0; i < ordered.count; i++) {
      NSString *t = ordered[i];
      if (defaultTitle.length > 0 && [t isEqualToString:defaultTitle]) {
        buttons[i].keyEquivalent = @"\r";
      } else if (cancelTitle.length > 0 && [t isEqualToString:cancelTitle]) {
        buttons[i].keyEquivalent = @"\033";
      } else if (defaultTitle.length > 0) {
        buttons[i].keyEquivalent = @"";
      }
    }

    // Report what AppKit ACTUALLY ended up with. Three rounds were spent
    // reasoning about what NSAlert does to key equivalents; measuring is
    // cheaper and it is the only thing admissible about a running framework.
    for (NSUInteger i = 0; i < ordered.count; i++) {
      const char *eq = buttons[i].keyEquivalent.length == 0
                           ? "(none)"
                           : ([buttons[i].keyEquivalent isEqualToString:@"\r"]
                                  ? "RETURN"
                                  : ([buttons[i].keyEquivalent isEqualToString:@"\033"] ? "ESCAPE"
                                                                                         : "other"));
      fprintf(stderr, "BUTTON %lu %-12s keyEquivalent=%s\n", (unsigned long)i,
              ordered[i].UTF8String, eq);
    }

    // Index into the ORDER BUTTONS WERE ADDED, which is no longer the caller's
    // order. Reporting titles[index] here would return the wrong button's name.
    NSInteger index = [alert runModal] - NSAlertFirstButtonReturn;
    reportModal(callID, (index >= 0 && index < (NSInteger)ordered.count) ? ordered[index] : @"");
  });
}

// The rest return nothing, so they are pure fire-and-forget. No channel, no
// waiting goroutine, nothing to cancel.

void hostRevealPath(const char *path) {
  NSString *p = [NSString stringWithUTF8String:path];
  dispatch_async(dispatch_get_main_queue(), ^{
    [[NSWorkspace sharedWorkspace] selectFile:p inFileViewerRootedAtPath:@""];
  });
}

void hostOpenURL(const char *url) {
  NSString *u = [NSString stringWithUTF8String:url];
  dispatch_async(dispatch_get_main_queue(), ^{
    [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:u]];
  });
}

void hostSetTitle(const char *title) {
  NSString *t = [NSString stringWithUTF8String:title];
  dispatch_async(dispatch_get_main_queue(), ^{
    gWebView.window.title = t;
  });
}

// Safe from any goroutine: WKWebView is main-thread-only, so calling it from a
// background goroutine would be undefined behaviour rather than an error.
void hostEvalJS(const char *js) {
  NSString *source = [NSString stringWithUTF8String:js];

  // Scheduled in COMMON run-loop modes, not through dispatch_async.
  //
  // dispatch_async to the main queue is not serviced while a modal session is
  // running, so any open dialog freezes every pending reply to the frontend —
  // a call made while an alert is up never settles, and the alert is often
  // itself the report of the failure the caller is waiting on. Measured: an
  // error dialog from one call left an unrelated call hung until it was
  // dismissed.
  CFRunLoopPerformBlock(CFRunLoopGetMain(), kCFRunLoopCommonModes, ^{
    // gWebView is nil once the window has gone. Messaging nil is a no-op in
    // Objective-C rather than a crash, but checking says so deliberately.
    if (gWebView == nil) return;
    [gWebView evaluateJavaScript:source completionHandler:nil];
  });
  CFRunLoopWakeUp(CFRunLoopGetMain());
}
