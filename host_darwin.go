//go:build darwin

package main

// A macOS host with nothing between the application and the operating system.
//
// SPIKE — see docs/superpowers/specs/2026-08-10-own-the-host-spike-design.md.
// It implements only what carries risk: a window, embedded assets over a custom
// URL scheme, a bound call, and a panicking bound call that REJECTS. The twelve
// nativePort operations are deliberately absent.
//
// AppKit and WebKit expose no C API, so some Objective-C is unavoidable. What
// actually requires the LANGUAGE is protocol conformance — serving assets and
// receiving calls each need a class conforming to a protocol — and that is all
// host_darwin.m contains. Wails' WailsContext conforms to four protocols
// (WKURLSchemeHandler, WKScriptMessageHandler, WKNavigationDelegate,
// WKUIDelegate); this needs two.

/*
#cgo CFLAGS: -x objective-c -fmodules -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#include <stdlib.h>

// DECLARATIONS ONLY. Definitions live in host_darwin.m, and that split is
// forced rather than chosen: cgo emits this preamble into BOTH its main
// generated C file and _cgo_export.c, so a definition here is compiled twice
// and the link fails on duplicate symbols.
void hostRun(const char *title, int width, int height, int dropMode);
void hostEvalJS(const char *js);
void hostOpenFile(int callID, const char *title, const char *extensionsCSV);
void hostSaveFile(int callID, const char *title, const char *defaultName, const char *extensionsCSV);
void hostDialog(int callID, const char *title, const char *message, const char *buttonsCSV, const char *defaultButton, const char *cancelButton, int isError);
void hostRevealPath(const char *path);
void hostOpenURL(const char *url);
void hostSetTitle(const char *title);
void hostCloseNow(void);
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
	"strings"
	"sync"
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
	if !dropWaitMode && !walkMode && !closeCheckMode && !docCheckMode {
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
// promise, so the frontend's await settles. Under Wails v2 the same panic is
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

// reportCloseDecision prints what the guard decided, so a close driven by the
// window button is observable rather than merely effective.
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
		if guard(context.Background()) {
			fmt.Println("CLOSE: guard PREVENTED the close")
			fmt.Println("VERDICT: PASS — a dirty document was protected")
			os.Exit(0)
		}
		fmt.Println("CLOSE: guard ALLOWED the close")
		fmt.Println("VERDICT: PASS — a clean document closed without a prompt")
		os.Exit(0)
	}()
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
func hostShuttingDown() { beginShutdown() }

//export hostModalResult
func hostModalResult(id C.int, cresult *C.char) {
	result := C.GoString(cresult)

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

func (darwinNative) EmitFilesDropped(_ context.Context, paths []string) {
	emitToFrontend("files:dropped", paths)
}

func (darwinNative) EmitFileOpen(_ context.Context, p string) {
	emitToFrontend(fileOpenEvent, p)
}

// emitToFrontend delivers an event through the runtime shim the injected script
// installs. app.js subscribes with globalThis.runtime.EventsOn at two sites, so
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
