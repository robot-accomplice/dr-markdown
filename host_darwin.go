//go:build darwin

package main

// A macOS host with nothing between the application and the operating system.
//
// Began as a spike; the decision it produced is recorded in
// docs/decisions/2026-08-10-host-replacement.md.
// It implements only what carries risk: a window, embedded assets over a custom
// URL scheme, a bound call, and a panicking bound call that REJECTS. The twelve
// nativePort operations are deliberately absent.
//
// AppKit and WebKit expose no C API, so some Objective-C is unavoidable. What
// actually requires the LANGUAGE is protocol conformance — serving assets and
// receiving calls each need a class conforming to a protocol — and that is all
// host_darwin.m contains. The framework this replaced used one context object
// conforming to four protocols
// (WKURLSchemeHandler, WKScriptMessageHandler, WKNavigationDelegate,
// WKUIDelegate); this needs three. WKNavigationDelegate joined them when a
// mermaid diagram link was found to navigate the main frame away from the app,
// handing the bridge to a remote origin (#145).

/*
#cgo CFLAGS: -x objective-c -fmodules -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#include <stdlib.h>

// DECLARATIONS ONLY. Definitions live in host_darwin.m, and that split is
// forced rather than chosen: cgo emits this preamble into BOTH its main
// generated C file and _cgo_export.c, so a definition here is compiled twice
// and the link fails on duplicate symbols.
void hostRun(const char *title, int width, int height, int dropMode);
void hostTerminateApproved(void);
void hostTerminateNow(void);
void hostEvalJS(const char *js);
void hostOpenFile(int callID, const char *title, const char *extensionsCSV);
void hostSaveFile(int callID, const char *title, const char *defaultName, const char *extensionsCSV);
void hostDialog(int callID, const char *title, const char *message, const char *buttonsCSV, const char *defaultButton, const char *cancelButton, int isError);
void hostRevealPath(const char *path);
void hostOpenURL(const char *url);
void hostSetTitle(const char *title);
void hostCloseNow(void);
char *hostMenuJSON(void);
int hostIsDefaultMarkdownHandler(void);
int hostSetDefaultMarkdownHandler(void);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// darwinHost implements hostPort over AppKit and WebKit directly.
type darwinHost struct{}

func (darwinHost) Native() nativePort { return darwinNative{} }

func (darwinHost) Run(cfg hostConfig) error {
	spikeAssets = cfg.Assets

	// Exactly one bindable object, matching what the application passes today.
	for _, bound := range cfg.Bind {
		if a, ok := bound.(*App); ok {
			setBoundApp(a)
		}
	}

	// A run that never reports is a failure, not a wait. The injected script can
	// die at PARSE time — one bad escape kills the bridge, the runtime shim and
	// the gate runner together — and because bridge.js degrades when the host is
	// absent, the window still opens and looks healthy. Without this the harness
	// sits there forever looking busy.
	//
	// NOT armed in drop mode, where the gates are deliberately skipped and the
	// window must stay open indefinitely waiting for a person to drag a file
	// onto it. A watchdog that closes the window out from under the operator
	// reports its own timeout as the result.
	if gateMode {
		go func() {
			select {
			case <-time.After(45 * time.Second):
				fmt.Println("GATES: nothing reported in 45s.")
				fmt.Println("VERDICT: FAIL (no report — suspect a parse error in the injected script)")
				os.Exit(1)
			case <-hostDone:
			}
		}()
	}

	// Held for the C callbacks, which cannot carry a Go receiver.
	setLifecycle(cfg)

	// The navigation gate must be able to FAIL rather than merely hang. Without
	// the delegate the page simply leaves and nothing calls back, so a gate that
	// only reports on refusal would time out — which reads as a broken harness,
	// not as a verdict, and a verdict is the whole point.
	// The quit gate needs a deadline for the same reason, learned the hard way:
	// when the guard's dialog could not be delivered, the gate did not fail — it
	// simply sat there, and the application could be neither quit nor closed. A
	// harness that hangs reads as a broken harness rather than as a verdict, and
	// the defect hid behind that.
	// The close gate gets the same deadline as the quit gate, for the same reason
	// and by the same lesson: a guard whose dialog cannot be delivered leaves the
	// harness sitting there, and a harness that sits there reads as broken rather
	// than as a verdict. This one has never hung — it is here so it cannot start.
	if closeCheckMode {
		go func() {
			time.Sleep(quitCheckDeadline)
			fmt.Println("CLOSE: no verdict within", quitCheckDeadline)
			fmt.Println("VERDICT: FAIL — the guard never answered. Either it was never " +
				"reached, or its dialog could not be delivered")
			os.Exit(1)
		}()
	}

	if quitCheckMode {
		go func() {
			time.Sleep(quitCheckDeadline)
			fmt.Println("QUIT: no verdict within", quitCheckDeadline)
			fmt.Println("VERDICT: FAIL — the guard never answered. Either it was never " +
				"reached, or its dialog could not be delivered and the application is wedged")
			os.Exit(1)
		}()
	}

	if navCheckMode {
		go func() {
			time.Sleep(navCheckDeadline)
			fmt.Println("NAV: no refusal within", navCheckDeadline)
			fmt.Println("VERDICT: FAIL — the main frame navigated off the app's own scheme, " +
				"so a document can hand a remote origin the native bindings")
			os.Exit(1)
		}()
	}

	// Lifecycle callbacks the application supplied. OnStartup must run before
	// the frontend can ask for anything, and it is what subscribes to file
	// drops — a host that never calls it leaves drag-and-drop silently dead.
	if cfg.OnStartup != nil {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-hostDone
			cancel()
		}()
		go cfg.OnStartup(ctx)
	}

	title := C.CString(cfg.Title)
	defer C.free(unsafe.Pointer(title))

	// Blocks in the AppKit event loop until the window closes.
	mode := C.int(0)
	if dropWaitMode {
		mode = 1
	}
	if walkMode {
		mode = 2
	}
	if docCheckMode {
		mode = 5
	}
	if menuCheckMode {
		mode = 6
	}
	if gateMode {
		mode = 7
	}
	if quitCheckMode {
		mode = 8
		if quitDirty {
			mode = 9
		}
	}
	if navCheckMode {
		mode = 10
	}
	if closeCheckMode {
		mode = 3
		if closeDirty {
			mode = 4
		}
	}
	C.hostRun(title, C.int(cfg.Width), C.int(cfg.Height), mode)
	return nil
}

// boundApp is the application object the host binds. Held at package scope
// because calls arrive from C and cannot carry a Go receiver.
var (
	boundAppMu  sync.Mutex
	boundAppVal *App
)

func setBoundApp(a *App) {
	boundAppMu.Lock()
	defer boundAppMu.Unlock()
	boundAppVal = a
}

func boundApp() *App {
	boundAppMu.Lock()
	defer boundAppMu.Unlock()
	return boundAppVal
}

// spikeAssets is the embedded frontend, held at package scope because the
// scheme handler is reached from C and cannot carry a Go receiver.
var spikeAssets fs.FS

//export hostServeAsset
func hostServeAsset(cpath *C.char, outLen *C.int, outMime **C.char) unsafe.Pointer {
	requested := C.GoString(cpath)

	// The walk script is SERVED rather than embedded in an Objective-C string
	// literal. Escaping a script this size through ObjC has already cost a round:
	// \n in a literal becomes a real newline, which is a SyntaxError that
	// silently kills the entire injected script.
	//
	// It is served ONLY during a harness run. These three paths were reachable on
	// the app's own scheme in every shipped build — including __docfixture.md,
	// which is a bare os.ReadFile of a path taken from the command line. Nothing
	// document-reachable was found that could ask for them, but a released
	// artifact should not carry a file-read endpoint at all, and gating it costs
	// one condition.
	if harnessRun() {
		if body := serveHarnessAsset(requested, outLen, outMime); body != nil {
			return body
		}
	}

	return serveEmbeddedAsset(requested, outLen, outMime)
}

// harnessRun reports whether any verification mode was asked for on the command
// line. Every one of them is set from argv, so this is false for a user launch.
func harnessRun() bool {
	return dropWaitMode || walkMode || docCheckMode || menuCheckMode ||
		closeCheckMode || gateMode || quitCheckMode || navCheckMode
}

func serveHarnessAsset(requested string, outLen *C.int, outMime **C.char) unsafe.Pointer {
	if requested == "/__docfixture.md" {
		body, err := os.ReadFile(docFixturePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "doc fixture %s: %v\n", docFixturePath, err)
			return nil
		}
		*outLen = C.int(len(body))
		*outMime = C.CString("text/markdown")
		return C.CBytes(body)
	}

	if requested == "/__doc.js" {
		body := []byte(compositeDocJS)
		*outLen = C.int(len(body))
		*outMime = C.CString("text/javascript")
		return C.CBytes(body)
	}

	if requested == "/__walk.js" {
		body := []byte(walkModuleJS)
		*outLen = C.int(len(body))
		*outMime = C.CString("text/javascript")
		return C.CBytes(body)
	}
	return nil
}

func serveEmbeddedAsset(requested string, outLen *C.int, outMime **C.char) unsafe.Pointer {

	name := path.Join("frontend/dist", strings.TrimPrefix(requested, "/"))

	body, err := fs.ReadFile(spikeAssets, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ASSET MISS %s\n", name)
		return nil
	}
	fmt.Fprintf(os.Stderr, "ASSET  ok  %s (%d bytes, %s)\n", name, len(body), contentType(name))

	*outLen = C.int(len(body))
	*outMime = C.CString(contentType(name))
	return C.CBytes(body)
}

// contentType maps an extension to a MIME type.
//
// `.mjs` is spelled out rather than left to the system table: the vendored Crepe
// and Mermaid bundles are ESM, and a module served as anything other than a
// JavaScript type is refused by the LOADER with a console error rather than by
// the request — which looks like a broken app, not a broken MIME type.
func contentType(name string) string {
	switch path.Ext(name) {
	case ".mjs", ".js":
		return "text/javascript"
	case ".css":
		return "text/css"
	case ".html":
		return "text/html"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	}
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

//export hostHandleCall
func hostHandleCall(id C.int, cmethod *C.char, cargs *C.char) {
	method := C.GoString(cmethod)
	args := C.GoString(cargs)

	// Off the main thread immediately. This runs on AppKit's main thread, and
	// doing real work here freezes the window — the defect a naive synchronous
	// bridge ships with, which nobody notices until a save is slow.
	//
	// The result is delivered through orDone so a call still in flight when the
	// window closes exits instead of resolving a promise into a torn-down
	// webview. There is no frontend left to tell, and saying so to nobody is
	// how a process ends up held open by a goroutine.
	go func() {
		result := make(chan callResult, 1)
		go func() {
			ok, payload := dispatchCall(boundApp(), method, args)
			result <- callResult{ok: ok, payload: payload}
		}()

		if r, alive := <-orDone(hostDone, result); alive {
			resolveCall(int(id), r.ok, r.payload)
		}
	}()
}

// callResult carries a dispatched call's answer back to the goroutine that is
// waiting on the host's lifetime as well as on the work.
type callResult struct {
	ok      bool
	payload string
}

// resolveCall settles the frontend promise. hostEvalJS dispatches to the main
// thread itself, so this is safe from a goroutine.
func resolveCall(id int, ok bool, payload string) {
	js := fmt.Sprintf("globalThis.__drmdResolve(%d, %t, %s)", id, ok, payload)
	cjs := C.CString(js)
	defer C.free(unsafe.Pointer(cjs))
	C.hostEvalJS(cjs)
}

// decodeArgs unmarshals the frontend's JSON argument array into typed targets.
//
// It refuses a mismatched count rather than leaving a parameter at its zero
// value. A save that arrived with a path and no content would otherwise write
// an empty file over the user's document and report success.
func decodeArgs(args []json.RawMessage, targets ...any) error {
	if len(args) != len(targets) {
		return fmt.Errorf("expected %d arguments, got %d", len(targets), len(args))
	}
	for i, target := range targets {
		if err := json.Unmarshal(args[i], target); err != nil {
			return fmt.Errorf("argument %d: %w", i, err)
		}
	}
	return nil
}

// dispatchCall routes one frontend call to the generated typed dispatcher.
//
// `payload` is a JSON value ready to interpolate into the resolver. The
// deferred recover is the point of owning the host: a panic becomes a REJECTED
// promise, so the frontend's await settles. Under the framework this replaced
// the same panic is
// recovered upstream and the callback discarded — the promise never settles and
// that operation is dead until restart (#61), with no downstream fix.
func dispatchCall(app *App, method, argsJSON string) (ok bool, payload string) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
			payload = mustJSON(fmt.Sprintf("panic in %s: %v", method, r))
		}
	}()

	// The gate runner's own methods. Not bound methods, and deliberately not in
	// app.go — they exist to exercise the host, not the application.
	switch method {
	case "Ping":
		if strings.Contains(argsJSON, "__emit_file_open") {
			go darwinNative{}.EmitFileOpen(context.Background(), "/tmp/from-go.md")
		}
		if strings.Contains(argsJSON, "__simulate_drop") {
			payload := C.CString(`["/tmp/dropped-a.md"]`)
			defer C.free(unsafe.Pointer(payload))
			hostFileDrop(payload)
		}
		return true, mustJSON("pong:" + argsJSON)
	case "Boom":
		panic("deliberate panic from a bound method")
	case "__gates":
		reportGates(argsJSON)
		return true, mustJSON("reported")
	case "__realdrop":
		reportRealDrop(argsJSON)
		return true, mustJSON("reported")
	case "__walk":
		reportWalk(argsJSON)
		return true, mustJSON("reported")
	case "__doc":
		reportComposite(argsJSON)
		return true, mustJSON("reported")
	case "__closenow":
		hostRequestClose()
		return true, mustJSON("reported")
	case "__quitnow":
		// The REAL gesture, not a stand-in: this asks AppKit to terminate, so
		// the guard is reached the way Cmd-Q reaches it or not at all.
		C.hostTerminateNow()
		return true, mustJSON("reported")
	}

	var args []json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false, mustJSON("bad argument payload: " + err.Error())
	}

	result, err, handled := dispatchBound(app, method, args)
	if !handled {
		return false, mustJSON("no such method: " + method)
	}
	if err != nil {
		// Logged as well as returned: the frontend gets the message, but a bound
		// method failing during an automated run is otherwise only visible as a
		// rejected promise nobody printed.
		fmt.Fprintf(os.Stderr, "BOUND %s failed: %v\n", method, err)
		return false, mustJSON(err.Error())
	}

	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return false, mustJSON("could not encode result: " + marshalErr.Error())
	}
	return true, string(encoded)
}

func mustJSON(v string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `"encoding failed"`
	}
	return string(b)
}

// darwinNative implements the twelve nativePort operations against AppKit
// directly. No framework, no third-party dependency — NSOpenPanel, NSSavePanel,
// NSAlert and NSWorkspace.
//
// A cancelled dialog returns an empty string and a nil error. That is not an
// error condition: every call site treats "" as "the user chose nothing", and
// turning it into an error would make cancelling a save look like a failed one.
type darwinNative struct{}

// Modal calls are ASYNCHRONOUS and answered over a channel.
//
// The obvious implementation dispatch_syncs to the main thread and returns the
// answer directly. That holds a cgo call open for as long as the dialog is on
// screen, pinning an OS thread and leaving the goroutine unpreemptable — and it
// makes the context.Context every one of these methods takes impossible to
// honour. Parking on a channel costs nothing and makes cancellation expressible.
var (
	modalMu      sync.Mutex
	modalNextID  int
	modalPending = map[int]chan string{}
)

// beginModal registers a pending call. The channel is BUFFERED so a late answer
// — one that arrives after the context was cancelled — never blocks the main
// thread on send.
func beginModal() (C.int, chan string) {
	closeObsMu.Lock()
	closeObs.prompts++
	closeObsMu.Unlock()

	modalMu.Lock()
	defer modalMu.Unlock()
	modalNextID++
	ch := make(chan string, 1)
	modalPending[modalNextID] = ch
	return C.int(modalNextID), ch
}

//export hostFileDrop
func hostFileDrop(cjson *C.char) {
	var paths []string
	if err := json.Unmarshal([]byte(C.GoString(cjson)), &paths); err != nil {
		fmt.Fprintf(os.Stderr, "file drop: %v\n", err)
		return
	}

	handler := currentDropHandler()
	if handler == nil {
		// Not an error: the OS can deliver a drop before startup has subscribed.
		// Dropping it silently is still worth saying out loud, because the
		// symptom is a drag that does nothing and looks like a broken app.
		fmt.Fprintf(os.Stderr, "file drop with no subscriber, discarded: %v\n", paths)
		return
	}

	// Off AppKit's main thread: the handler emits to the frontend, and doing
	// that inside the drag operation would block the drag from completing.
	go handler(paths)
}

// dropWaitMode holds the window open for a real drag instead of running the
// automated gates.
var dropWaitMode bool

// walkMode drives the whole UI surface instead of the gates.
var walkMode bool

// navCheckMode exercises the navigation delegate.
//
// It exists because that delegate is the second layer under #145 and no browser
// test can reach it: chromedp drives the page, not WKWebView's policy callback.
// Leaving it "verified by reading" is the shape of defect the review already
// caught once — a check that could not have found anything, finding nothing.
var navCheckMode bool

// Long enough for the frontend to boot and drive the navigation, short enough
// that a failing gate reports rather than looking stuck.
const navCheckDeadline = 25 * time.Second

// Long enough for a person to read a dialog and answer it, short enough that a
// wedged application reports rather than sitting there.
const quitCheckDeadline = 90 * time.Second

// quitCheckMode exercises the close guard on the QUIT path.
//
// It is a separate mode from closeCheckMode because it is a separate entry
// point, and conflating them is how the defect it exists to catch survived:
// -close/-close-dirty drive windowShouldClose:, and nothing drove terminate:.
var quitCheckMode bool

// quitDirty makes the quit gate exercise the case that matters. It is separate
// from the mode because the CLEAN quit is the half that can be verified without
// a human answering a dialog.
var quitDirty bool

// closeCheckMode exercises the close guard.
//
// It is its own mode because the guard cannot be checked from inside the walk:
// a CLEAN close ends the process the walk is reporting from, and a DIRTY one
// raises a prompt that only a person can answer.
var closeCheckMode bool

// closeDirty makes the close check run against UNSAVED work.
var closeDirty bool

// docCheckMode runs a structured document through the editor.
var docCheckMode bool

// menuCheckMode verifies the menu bar, which nothing else can see.
var menuCheckMode bool

// gateMode runs the host gates. NOT the default: with no flags this binary must
// be the application a user launches.
var gateMode bool

// Lifecycle callbacks the application supplied, reachable from C.
var (
	lifecycleMu   sync.Mutex
	onBeforeClose func(context.Context) bool
	onFileOpen    func(string)
)

func setLifecycle(cfg hostConfig) {
	lifecycleMu.Lock()
	defer lifecycleMu.Unlock()
	onBeforeClose = cfg.OnBeforeClose
	onFileOpen = cfg.OnFileOpen
	setNavigationBlockHandler(cfg.OnNavigationBlocked)
}

// hostRequestTerminate runs the close guard for a QUIT rather than a window
// close, and answers AppKit once it knows.
//
// Same two-phase shape as hostRequestClose and for the same reason: the guard
// prompts, and that dialog needs the main thread applicationShouldTerminate: is
// running on. The difference is only in how the answer is delivered —
// replyToApplicationShouldTerminate: rather than closing the window.
//
//export hostRequestTerminate
func hostRequestTerminate() {
	if quitCheckMode {
		// Reports and exits, like the close check. Falling through would ask the
		// guard twice for one quit.
		reportQuitDecision()
		return
	}
	lifecycleMu.Lock()
	guard := onBeforeClose
	lifecycleMu.Unlock()

	go func() {
		if guard != nil && guard(context.Background()) {
			// The user cancelled. Nothing to tell AppKit: the terminate was
			// already refused, so the application simply carries on with its
			// documents intact.
			return
		}
		C.hostTerminateApproved()
	}()
}

//export hostRequestClose
func hostRequestClose() {
	if closeCheckMode {
		// Reports and exits. Returning here matters: falling through would run
		// the guard a SECOND time, and a guard that prompts twice for one close
		// is worse than one that does not prompt at all.
		reportCloseDecision()
		return
	}
	lifecycleMu.Lock()
	guard := onBeforeClose
	lifecycleMu.Unlock()

	// Off the main thread: the guard prompts, and that dialog needs the main
	// thread the caller is currently occupying.
	go func() {
		if guard != nil && guard(context.Background()) {
			// The user cancelled. The window stays open and nothing else happens.
			return
		}
		C.hostCloseNow()
	}()
}

// What the close check observed while the guard ran. Written from the modal
// path, read once by reportCloseDecision.
var (
	closeObsMu sync.Mutex
	closeObs   closeObservation
)

// reportCloseDecision runs the close guard and judges what it did.
//
// It used to print PASS on both outcomes, which made -close-dirty incapable of
// failing — including in the case it exists to catch (GitHub #100). The
// judgement is judgeClose in closecheck.go, unit-tested there; this function
// only gathers the observation and reports it.
// reportQuitDecision runs the close guard on the QUIT path and judges what it
// did, reusing judgeClose so the two entry points are held to one standard.
//
// It is a separate gate from reportCloseDecision because it is a separate entry
// point into the same guard, and the whole defect was that only one of them was
// wired up. A gate that cannot distinguish them cannot have caught it.
func reportQuitDecision() {
	quitReported.Store(true)
	lifecycleMu.Lock()
	guard := onBeforeClose
	lifecycleMu.Unlock()

	if guard == nil {
		fmt.Println("QUIT: no guard registered — Cmd-Q would discard unsaved work silently")
		fmt.Println("VERDICT: FAIL")
		os.Exit(1)
	}
	go func() {
		prevented := guard(context.Background())

		closeObsMu.Lock()
		obs := closeObs
		closeObsMu.Unlock()
		obs.dirty = quitDirty
		obs.prevented = prevented

		fmt.Printf("QUIT: gesture=terminate: mode=%s guard=%s prompts=%d answer=%q\n",
			modeName(quitDirty), verb(prevented), obs.prompts, obs.answer)

		ok, why := judgeClose(obs)
		if !ok {
			fmt.Println("VERDICT: FAIL —", why)
			os.Exit(1)
		}
		fmt.Println("VERDICT: PASS —", why)
		os.Exit(0)
	}()
}

func reportCloseDecision() {
	lifecycleMu.Lock()
	guard := onBeforeClose
	lifecycleMu.Unlock()

	if guard == nil {
		fmt.Println("CLOSE: no guard registered — unsaved work would be lost silently")
		fmt.Println("VERDICT: FAIL")
		os.Exit(1)
	}
	go func() {
		prevented := guard(context.Background())

		closeObsMu.Lock()
		obs := closeObs
		closeObsMu.Unlock()
		obs.dirty = closeDirty
		obs.prevented = prevented

		fmt.Printf("CLOSE: mode=%s guard=%s prompts=%d answer=%q\n",
			modeName(closeDirty), verb(prevented), obs.prompts, obs.answer)

		ok, why := judgeClose(obs)
		if !ok {
			fmt.Println("VERDICT: FAIL —", why)
			os.Exit(1)
		}
		fmt.Println("VERDICT: PASS —", why)
		os.Exit(0)
	}()
}

func modeName(dirty bool) string {
	if dirty {
		return "dirty"
	}
	return "clean"
}

//export hostFileOpened
func hostFileOpened(cpath *C.char) {
	lifecycleMu.Lock()
	open := onFileOpen
	lifecycleMu.Unlock()

	if open == nil {
		return
	}
	// The application decides whether to emit now or hold the path until the
	// frontend asks: at launch the file arrives BEFORE the webview exists, which
	// is the defect behind #53.
	go open(C.GoString(cpath))
}

//export hostShuttingDown
func hostShuttingDown() {
	// A gate that cannot fail is decoration. Without applicationShouldTerminate:
	// the app terminates without ever consulting the guard, which is the DEFECT
	// this mode exists to catch — and it would look like a pass, because the
	// process simply exits 0 and prints no verdict at all. Reaching termination
	// with nothing reported is therefore the failure.
	if quitCheckMode && !quitReported.Load() {
		fmt.Println("QUIT: terminated without consulting the close guard")
		fmt.Println("VERDICT: FAIL — applicationShouldTerminate: never ran, so Cmd-Q discards unsaved work")
		os.Exit(1)
	}
	beginShutdown()
}

// quitReported records that the quit gate got as far as judging something.
var quitReported atomic.Bool

//export hostModalResult
func hostModalResult(id C.int, cresult *C.char) {
	result := C.GoString(cresult)

	closeObsMu.Lock()
	closeObs.answer = result
	closeObsMu.Unlock()

	modalMu.Lock()
	ch, ok := modalPending[int(id)]
	delete(modalPending, int(id))
	modalMu.Unlock()

	if ok {
		ch <- result
	}
}

// awaitModal parks until the dialog is answered, the caller cancels, or the
// host goes away. It never blocks indefinitely on any of the three.
func awaitModal(ctx context.Context, id C.int, ch chan string) (string, error) {
	answer, ok := <-orDone(or(hostDone, ctx.Done()), ch)
	if ok {
		return answer, nil
	}

	// The channel did not deliver, so nothing will ever read it. Drop the
	// registration or the map grows by one entry per abandoned dialog.
	forgetModal(id)
	if hostClosing() {
		return "", errHostClosed
	}
	return "", ctx.Err()
}

// forgetModal drops a pending registration. Safe to call for an id already
// delivered — hostModalResult removes its own entry.
func forgetModal(id C.int) {
	modalMu.Lock()
	delete(modalPending, int(id))
	modalMu.Unlock()
}

func cstr(s string) (*C.char, func()) {
	c := C.CString(s)
	return c, func() { C.free(unsafe.Pointer(c)) }
}

func (darwinNative) OpenMarkdownFile(ctx context.Context) (string, error) {
	id, ch := beginModal()
	title, freeTitle := cstr("Open Markdown Document")
	defer freeTitle()
	exts, freeExts := cstr("md,markdown")
	defer freeExts()

	C.hostOpenFile(id, title, exts)
	return awaitModal(ctx, id, ch)
}

func (darwinNative) SaveMarkdownFile(ctx context.Context, defaultFilename string) (string, error) {
	id, ch := beginModal()
	title, freeTitle := cstr("Save Markdown Document")
	defer freeTitle()
	name, freeName := cstr(defaultFilename)
	defer freeName()
	exts, freeExts := cstr("md")
	defer freeExts()

	C.hostSaveFile(id, title, name, exts)
	return awaitModal(ctx, id, ch)
}

func (darwinNative) SelectImageFile(ctx context.Context) (string, error) {
	id, ch := beginModal()
	title, freeTitle := cstr("Import Image")
	defer freeTitle()
	exts, freeExts := cstr("png,jpg,jpeg,gif,webp,svg")
	defer freeExts()

	C.hostOpenFile(id, title, exts)
	return awaitModal(ctx, id, ch)
}

func (darwinNative) ConfirmOverwriteChanged(ctx context.Context, p string) (string, error) {
	return showDialog(ctx, dialog{
		title: "File Changed on Disk",
		message: filepath.Base(p) + " has been modified by another program since you opened it.\n\n" +
			"Saving now replaces those changes with your version.",
		buttons: "Overwrite,Cancel",
		// Cancel is BOTH default and escape: overwriting discards someone
		// else's writes, so the keyboard must never do it by accident.
		defaultButton: "Cancel",
		cancelButton:  "Cancel",
	})
}

func (darwinNative) ConfirmUnsaved(ctx context.Context) (string, error) {
	return showDialog(ctx, dialog{
		title:         "Unsaved Changes",
		message:       "Save changes before closing?",
		buttons:       "Save,Don't Save,Cancel",
		defaultButton: "Save",
		cancelButton:  "Cancel",
	})
}

func (darwinNative) ShowError(ctx context.Context, title string, message string) {
	// Returns nothing, so nothing waits on it — but it still goes through the
	// same path rather than a second mechanism, so there is one way a dialog
	// reaches the screen.
	// Nothing waits on the answer, but the goroutine still has to be able to
	// exit: showDialog's own wait is bounded by hostDone, so a dialog left
	// unanswered at shutdown releases it rather than parking forever.
	go func() {
		_, _ = showDialog(ctx, dialog{
			title: title, message: message, buttons: "OK",
			defaultButton: "OK", cancelButton: "OK", isError: true,
		})
	}()
}

func showDialog(ctx context.Context, d dialog) (string, error) {
	id, ch := beginModal()
	ct, freeTitle := cstr(d.title)
	defer freeTitle()
	cm, freeMsg := cstr(d.message)
	defer freeMsg()
	cb, freeButtons := cstr(d.buttons)
	defer freeButtons()
	cd, freeDefault := cstr(d.defaultButton)
	defer freeDefault()
	cc, freeCancel := cstr(d.cancelButton)
	defer freeCancel()

	errFlag := C.int(0)
	if d.isError {
		errFlag = 1
	}
	C.hostDialog(id, ct, cm, cb, cd, cc, errFlag)
	return awaitModal(ctx, id, ch)
}

// dialog carries what the APPLICATION decided about a prompt, including which
// button is safe. AppKit defaults to the first button added, and the
// destructive choice is conventionally listed first — so a host that does not
// carry defaultButton across makes Return the destructive answer.
type dialog struct {
	title         string
	message       string
	buttons       string
	defaultButton string
	cancelButton  string
	isError       bool
}

func (darwinNative) RevealPath(_ context.Context, p string) error {
	cp, free := cstr(p)
	defer free()
	C.hostRevealPath(cp)
	return nil
}

func (darwinNative) OpenExternalURL(_ context.Context, url string) error {
	cu, free := cstr(url)
	defer free()
	C.hostOpenURL(cu)
	return nil
}

// Both Launch Services calls are local database operations — no UI comes from
// this process (the system consent dialog belongs to the OS), so neither needs
// the main queue.
func (darwinNative) IsDefaultMarkdownHandler(_ context.Context) (bool, error) {
	return C.hostIsDefaultMarkdownHandler() == 1, nil
}

func (darwinNative) SetDefaultMarkdownHandler(_ context.Context) error {
	status := C.hostSetDefaultMarkdownHandler()
	if status == -1 {
		// -1 is the host's own sentinel for "no bundle identifier", not a
		// Launch Services status — say what is actually wrong.
		return fmt.Errorf("not running from an application bundle")
	}
	if status != 0 {
		return fmt.Errorf("Launch Services returned status %d", int(status))
	}
	return nil
}

func (darwinNative) SetTitle(_ context.Context, title string) {
	ct, free := cstr(title)
	defer free()
	C.hostSetTitle(ct)
}

// SubscribeFileDrop registers the handler the window's drag machinery calls.
//
// Guarded because the two ends run on different threads: startup subscribes on
// a goroutine while AppKit delivers drops on the main thread.
func (darwinNative) SubscribeFileDrop(_ context.Context, onDrop func(paths []string)) {
	dropMu.Lock()
	defer dropMu.Unlock()
	dropHandler = onDrop
}

func currentDropHandler() func(paths []string) {
	dropMu.Lock()
	defer dropMu.Unlock()
	return dropHandler
}

var (
	dropMu      sync.Mutex
	dropHandler func(paths []string)
)

// hostReportBlockedNavigation records a main-frame navigation the host refused.
//
// The refusal itself happens in Objective-C, at the navigation delegate, because
// that is where WebKit asks. It is reported back across the boundary rather than
// logged there so it lands in the same event trail as every other refusal, and
// so a build with no console attached still has the record — which is the whole
// premise of internal/eventlog.
//
//export hostReportBlockedNavigation
func hostReportBlockedNavigation(curl *C.char) {
	if navCheckMode {
		fmt.Printf("NAV: refused %s\n", C.GoString(curl))
		fmt.Println("VERDICT: PASS — the host refused a main-frame navigation off its own scheme")
		os.Exit(0)
	}
	handler := currentNavigationBlockHandler()
	if handler == nil {
		// Not an error: the host can refuse a navigation before startup has
		// subscribed. Say it out loud anyway rather than dropping it silently.
		fmt.Fprintf(os.Stderr, "blocked navigation with no subscriber, discarded: %s\n", C.GoString(curl))
		return
	}
	handler(C.GoString(curl))
}

// hostDefaultHandlerMenuState backs validateMenuItem: for the default-handler
// menu item. AppKit calls it synchronously on the main thread at
// menu-validation time; the Launch Services query is a local database lookup,
// so this never blocks.
//
// A query failure fails OPEN to offering: the click path re-runs the decision
// and reports the error, so a transient failure costs an enabled item that
// explains itself rather than a silently dead one.
//
//export hostDefaultHandlerMenuState
func hostDefaultHandlerMenuState() C.int {
	isDefault, err := darwinNative{}.IsDefaultMarkdownHandler(context.Background())
	if err != nil {
		return C.int(defaultHandlerOffer)
	}
	return C.int(defaultHandlerMenuState(isDefault, executablePath()))
}

func setNavigationBlockHandler(onBlocked func(url string)) {
	navigationMu.Lock()
	defer navigationMu.Unlock()
	navigationBlockHandler = onBlocked
}

func currentNavigationBlockHandler() func(url string) {
	navigationMu.Lock()
	defer navigationMu.Unlock()
	return navigationBlockHandler
}

var (
	navigationMu           sync.Mutex
	navigationBlockHandler func(url string)
)

func (darwinNative) EmitFilesDropped(_ context.Context, paths []string) {
	emitToFrontend("files:dropped", paths)
}

func (darwinNative) EmitFileOpen(_ context.Context, p string) {
	emitToFrontend(fileOpenEvent, p)
}

// emitToFrontend delivers an event through the runtime shim the injected script
// installs. app.js subscribes with globalThis.drmd.events.on at two sites, so
// a host providing only the bound methods leaves both dead.
func emitToFrontend(name string, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "emit %s: %v\n", name, err)
		return
	}
	js := fmt.Sprintf("globalThis.__drmdEmit(%s, %s)", mustJSON(name), encoded)
	cjs, free := cstr(js)
	defer free()
	C.hostEvalJS(cjs)
}

// reportGates prints the self-run gate verdicts and exits.
//
// The gates report themselves rather than being read out of a Web Inspector
// console by hand. A measurement that depends on a person typing expressions
// correctly is a measurement nobody repeats, and this one has to be repeatable
// to be worth anything.
func reportGates(argsJSON string) {
	var results []map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &results); err != nil || len(results) == 0 {
		fmt.Printf("GATES: unreadable payload %q: %v\n", argsJSON, err)
		os.Exit(1)
	}

	pretty, _ := json.MarshalIndent(results[0], "", "  ")
	fmt.Printf("GATES:\n%s\n", pretty)

	ok := results[0]["gate1_app_ready"] == true &&
		strings.HasPrefix(str(results[0]["gate2_ping"]), "pong:") &&
		strings.HasPrefix(str(results[0]["gate3_boom"]), "REJECTED:") &&
		strings.HasPrefix(str(results[0]["gate3b_survived"]), "pong:") &&
		results[0]["gate4_event_received"] == "/tmp/from-go.md" &&
		len(str2slice(results[0]["gate5_drop_delivered"])) == 1 &&
		results[0]["gate6_saved_bytes"] == "ROUND TRIP EXACT"
	if !ok {
		fmt.Println("VERDICT: FAIL")
		os.Exit(1)
	}
	fmt.Println("VERDICT: PASS")
	os.Exit(0)
}

// str2slice reads the dropped-paths array the frontend received back.
// reportRealDrop prints what a genuine mouse drag delivered, end to end: AppKit
// gave the paths to Go, Go emitted the event, and the frontend received it.
// Gate 5 enters at the AppKit callback and so proves everything EXCEPT the drag
// itself; this is the only check that covers the drag.
func reportRealDrop(argsJSON string) {
	var payload [][]string
	if err := json.Unmarshal([]byte(argsJSON), &payload); err != nil || len(payload) == 0 {
		fmt.Printf("REAL DROP: unreadable payload %q: %v\n", argsJSON, err)
		os.Exit(1)
	}

	fmt.Printf("REAL DROP — the frontend received %d path(s):\n", len(payload[0]))
	for _, p := range payload[0] {
		if _, err := os.Stat(p); err != nil {
			fmt.Printf("  %s  — DOES NOT EXIST: %v\n", p, err)
			fmt.Println("VERDICT: FAIL")
			os.Exit(1)
		}
		fmt.Printf("  %s  (exists)\n", p)
	}
	fmt.Println("VERDICT: PASS — AppKit -> Go -> frontend, with a real drag")
	os.Exit(0)
}

// reportWalk prints one line per checked surface and fails on any miss.
func reportWalk(argsJSON string) {
	var payload [][]struct {
		Name   string `json:"name"`
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &payload); err != nil || len(payload) == 0 {
		fmt.Printf("WALK: unreadable payload: %v\n%s\n", err, argsJSON)
		os.Exit(1)
	}

	checks := payload[0]
	failed := 0
	fmt.Printf("UI WALK — %d checks\n\n", len(checks))
	for _, c := range checks {
		if c.OK {
			fmt.Printf("  ok    %s\n", c.Name)
			continue
		}
		failed++
		fmt.Printf("  FAIL  %s\n          %s\n", c.Name, c.Detail)
	}

	fmt.Printf("\n%d of %d passed\n", len(checks)-failed, len(checks))
	if failed > 0 {
		fmt.Println("WALK VERDICT: FAIL")
		os.Exit(1)
	}
	fmt.Println("WALK VERDICT: PASS")
	os.Exit(0)
}

func str2slice(v any) []any {
	s, _ := v.([]any)
	return s
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// reportComposite prints a line-by-line comparison of a structured document
// before and after the editor, so what changes is visible rather than summarised.
func reportComposite(argsJSON string) {
	var payload []struct {
		Input  string `json:"input"`
		Output string `json:"output"`
		Second string `json:"second"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &payload); err != nil || len(payload) == 0 {
		fmt.Printf("DOC: unreadable payload: %v\n", err)
		os.Exit(1)
	}

	in := strings.Split(payload[0].Input, "\n")
	out := strings.Split(payload[0].Output, "\n")

	stable := payload[0].Output == payload[0].Second
	if payload[0].Input == payload[0].Output {
		fmt.Printf("DOC: byte-identical across %d lines, stable=%v\n", len(in), stable)
		fmt.Println("VERDICT: PASS — safe as a .canonical.md fixture")
		os.Exit(0)
	}
	if !stable {
		fmt.Println("DOC: the document is NOT STABLE — a second pass changed it again.")
		fmt.Println("That is worse than a rewrite: the file would keep changing on every save.")
	}

	fmt.Printf("DOC: %d lines in, %d lines out, stable=%v — differences follow\n\n", len(in), len(out), stable)
	for i := 0; i < len(in) || i < len(out); i++ {
		var a, b string
		if i < len(in) {
			a = in[i]
		}
		if i < len(out) {
			b = out[i]
		}
		if a == b {
			continue
		}
		fmt.Printf("  line %2d\n    in : %q\n    out: %q\n", i+1, a, b)
	}

	fmt.Println("\nVERDICT: DIFFERS (see above; some rewriting is known and tracked)")
	os.Exit(1)
}

// hostReportMenu checks the installed menu bar and exits.
//
// A Cocoa app has NO menu unless it builds one, and the Edit menu's key
// equivalents are what deliver Cmd-C, Cmd-V, Cmd-X and Cmd-A to the first
// responder. The first version of this host shipped with mainMenu=NIL, which
// means an editor with no copy and no paste — invisible to every other gate,
// because the frontend is perfectly healthy and the keystrokes simply never
// arrive.
//
//export hostReportMenu
func hostReportMenu() {
	raw := C.hostMenuJSON()
	defer C.free(unsafe.Pointer(raw))

	var menus []struct {
		Title string `json:"title"`
		Items []struct {
			Title     string `json:"title"`
			Key       string `json:"key"`
			Shift     int    `json:"shift"`
			HasAction int    `json:"hasAction"`
			JS        string `json:"js"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(C.GoString(raw)), &menus); err != nil {
		fmt.Printf("MENU: unreadable: %v\n", err)
		os.Exit(1)
	}

	// title is empty for the application menu: AppKit always shows the process
	// name there and ignores whatever it is called.
	required := []struct{ menu, item, key string }{
		{"", "Quit", "q"},
		// The default-handler offer (spec: docs/decisions/2026-08-25-default-markdown-handler.md).
		// Presence and action only: its enabled/checked state depends on the
		// machine's current default, so a gate running on the maintainer's Mac
		// sees a different state than one running anywhere else.
		{"", "Set as Default Markdown Application", ""},
		{"File", "New", "n"},
		{"File", "Open", "o"},
		{"File", "Save", "s"},
		{"Edit", "Cut", "x"},
		{"Edit", "Copy", "c"},
		{"Edit", "Paste", "v"},
		{"Edit", "Select All", "a"},
		// Find lives on the menu for a reason beyond discoverability: a menu
		// item's key equivalent is matched before web content sees the event, so
		// claiming Cmd-F here is what stops it reaching the page unhandled (#132).
		{"Edit", "Find\u2026", "f"},
		{"Edit", "Find Next", "g"},
		{"View", "Formatted", "1"},
		{"View", "Raw", "2"},
		// The only route to Reveal in Finder since the contextual bar was
		// removed (#85). No key equivalent: it is not frequent enough to spend
		// one, and every unshifted letter is already taken by the editor.
		{"View", "Reveal Image in Finder", ""},
	}

	failed := 0
	fmt.Printf("MENU: %d menus\n", len(menus))
	for _, m := range menus {
		fmt.Printf("  %-8s %d items\n", "["+m.Title+"]", len(m.Items))
	}
	fmt.Println()

	for _, want := range required {
		found := false
		for _, m := range menus {
			if m.Title != want.menu {
				continue
			}
			for _, item := range m.Items {
				if strings.HasPrefix(item.Title, want.item) && item.Key == want.key && item.HasAction == 1 {
					found = true
				}
			}
		}
		if !found {
			fmt.Printf("  MISSING  %s > %s (cmd-%s)\n", want.menu, want.item, want.key)
			failed++
			continue
		}
		fmt.Printf("  ok       %s > %s (cmd-%s)\n", want.menu, want.item, want.key)
	}

	// A menu key equivalent BEATS the webview, so any key the frontend already
	// binds is a shortcut the menu would silently steal. Three were taken on the
	// first attempt -- Cmd-B (bold), Shift-Cmd-S (split) and Cmd-W (close tab) --
	// and none of them would have failed a test or logged anything. They would
	// simply have stopped working.
	taken, err := frontendShortcuts()
	if err != nil {
		fmt.Printf("\ncould not read the frontend shortcuts: %v\n", err)
		os.Exit(1)
	}
	for _, m := range menus {
		for _, item := range m.Items {
			if item.Key == "" {
				continue
			}
			combo := item.Key
			if item.Shift == 1 {
				combo = "shift+" + item.Key
			}
			owner, clash := taken[combo]
			if !clash {
				continue
			}
			// Sharing a key is fine when the menu runs the SAME handler. Compared
			// against the JavaScript the item executes, not its title: "New" and
			// "newDocument" have no useful textual relationship, and matching on
			// titles would either miss real clashes or exempt them by accident.
			if owner != "" && strings.Contains(item.JS, owner) {
				continue
			}
			fmt.Printf("  CLASH    %s > %s takes cmd-%s, which the editor binds to %s\n",
				m.Title, item.Title, combo, owner)
			failed++
		}
	}

	if failed > 0 {
		fmt.Printf("\nVERDICT: FAIL (%d problems)\n", failed)
		os.Exit(1)
	}
	fmt.Println("\nVERDICT: PASS")
	os.Exit(0)
}

// frontendShortcuts reads the keys app.js binds, so the menu is checked against
// what the application actually does rather than against a list kept by hand.
func frontendShortcuts() (map[string]string, error) {
	src, err := os.ReadFile("frontend/dist/src/app.js")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	re := regexp.MustCompile(`key === '([a-z0-9` + "`" + `])'( && e\.shiftKey)?\)\s*\{\s*\n\s*e\.preventDefault\(\)\s*\n\s*([A-Za-z]+)`)
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		combo := m[1]
		if m[2] != "" {
			combo = "shift+" + m[1]
		}
		out[combo] = m[3]
	}
	return out, nil
}
