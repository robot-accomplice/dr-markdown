# Host Boundary Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Put every host-specific dependency behind one Go interface and one JavaScript transport module, so replacing Wails becomes "write one adapter" and its cost can be measured rather than estimated.

**Architecture:** A hand-written `hostPort` Go interface, defined from what the application needs and composing the existing `nativePort`. On the JavaScript side, a bridge module generated from `app.go` by a `go/ast` parser, calling a small hand-written `transport.js` that is the only module naming a host. No runtime behaviour changes.

**Tech Stack:** Go 1.x (see `go.mod`), Wails v2.13.0, vendored Milkdown Crepe ESM, chromedp for frontend tests. **No Node toolchain at any stage.**

## Global Constraints

- **No runtime behaviour change.** The app behaves identically: same documents, same dialogs, same events, same failure handling.
- **Every pre-existing test stays green AND unchanged**, except the three deletions in Task 7, which are justified there.
- **No Node toolchain.** Not for building, not for testing. `node --check` is a syntax gate only.
- **No Claude/Anthropic attribution** in commits or PRs.
- **Modified gitflow:** branch → PR to `develop` → PR to `main`. Pin credentials with `export GH_TOKEN=$(gh auth token --user robot-accomplice)`.
- **The local gate, taken from `.github/workflows`, not from memory:**
  `gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1`, plus
  `node --check` on every `frontend/dist/src/*.js` and `architext validate .`.
- `e2e/` is the only coverage of the frontend. `newTestBrowser` FAILS rather than skips when no Chrome is found. **Never set `DRMD_SKIP_E2E` in CI.**
- Architext data under `docs/architext/data/**` must be updated when architecture or trust boundaries change (Task 8).
- Design spec: `docs/superpowers/specs/2026-08-10-host-boundary-design.md`.

---

## File Structure

**Created:**

| path | responsibility |
| --- | --- |
| `tools/genbridge/main.go` | Parse `app.go` with `go/ast`; emit the JS bridge; fail if a bound method lacks its panic guard |
| `tools/genbridge/generate.go` | Pure functions: AST → method list → JS source. No I/O, so it is unit-testable |
| `tools/genbridge/generate_test.go` | Unit tests for the pure functions |
| `frontend/dist/src/host/transport.js` | The ONLY JS module naming a host. Resolves `globalThis.go.main.App` and `globalThis.runtime` |
| `frontend/dist/src/host/bridge.generated.js` | Generated, **tracked in git**. One line per bound method and per event |
| `host.go` | The `hostPort` interface and `appHost`, composing `nativePort` |
| `host_wails.go` | The only Wails-aware Go file besides `main.go` |
| `e2e/host_boundary_test.go` | Asserts the boundary holds: only `main.go` and `host_wails.go` name Wails |
| `genbridge_test.go` | Regenerate-and-diff gate at repo root (needs `app.go` in the same package dir) |

**Modified:**

| path | change |
| --- | --- |
| `app.go` | `nativePort.SubscribeFileDrop` gains a callback; new `EmitFilesDropped`; `App` gains `host` |
| `main.go` | Shrinks to constructing the Wails host and running it |
| `frontend/dist/src/bridge.js` | Becomes a thin re-export of the generated bridge, preserving its public shape |
| `frontend/dist/src/app.js` | Event subscriptions move from `globalThis.runtime` to the bridge |

---

### Task 1: Split `SubscribeFileDrop` into its two directions

`wailsNative.SubscribeFileDrop` currently subscribes to a native event **and** emits a frontend event inside the same function (`app.go:627-629`). Those cross the boundary in opposite directions and are two different mechanisms under any other host.

**Files:**
- Modify: `app.go` (the `nativePort` interface, `wailsNative.SubscribeFileDrop`, `App.startup`)
- Modify: `app_m5_test.go` (the `fakeNative` double)
- Test: `app_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `nativePort.SubscribeFileDrop(ctx context.Context, onDrop func(paths []string))` and `nativePort.EmitFilesDropped(ctx context.Context, paths []string)`.

- [ ] **Step 1: Write the failing test**

Add to `app_test.go`:

```go
// The two directions are separate mechanisms under any host: one receives from
// the OS, the other sends to the webview. Fusing them hid a host dependency
// inside what looked like a subscription.
func TestFileDropSubscriptionAndEmissionAreSeparate(t *testing.T) {
	native := &fakeNative{}
	app := newAppWithDependencies(appDependencies{native: native})
	app.ctx = context.Background()

	app.startup(app.ctx)

	if native.dropHandler == nil {
		t.Fatal("startup did not subscribe to file drops")
	}
	native.dropHandler([]string{"/tmp/a.md", "/tmp/b.md"})

	if got := native.emittedDrops; len(got) != 1 || got[0][0] != "/tmp/a.md" {
		t.Errorf("dropped paths were not emitted to the frontend: %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestFileDropSubscriptionAndEmissionAreSeparate -v`
Expected: FAIL to compile — `native.dropHandler` and `native.emittedDrops` undefined.

- [ ] **Step 3: Extend the fake**

In `app_m5_test.go`, add fields to `fakeNative`:

```go
	dropHandler  func(paths []string)
	emittedDrops [][]string
```

Replace the existing `SubscribeFileDrop` method and add the emitter:

```go
func (f *fakeNative) SubscribeFileDrop(_ context.Context, onDrop func(paths []string)) {
	f.fileDropSubscribed = true
	f.dropHandler = onDrop
}

func (f *fakeNative) EmitFilesDropped(_ context.Context, paths []string) {
	f.emittedDrops = append(f.emittedDrops, paths)
}
```

- [ ] **Step 4: Change the interface and the real adapter**

In `app.go`, in `type nativePort interface`, replace `SubscribeFileDrop(context.Context)` with:

```go
	SubscribeFileDrop(context.Context, func(paths []string))
	EmitFilesDropped(context.Context, []string)
```

Replace `wailsNative.SubscribeFileDrop` with:

```go
func (wailsNative) SubscribeFileDrop(ctx context.Context, onDrop func(paths []string)) {
	runtime.OnFileDrop(ctx, func(_, _ int, paths []string) { onDrop(paths) })
}

func (wailsNative) EmitFilesDropped(ctx context.Context, paths []string) {
	runtime.EventsEmit(ctx, "files:dropped", paths)
}
```

In `App.startup`, change the subscription call to:

```go
	a.native.SubscribeFileDrop(ctx, func(paths []string) {
		a.native.EmitFilesDropped(ctx, paths)
	})
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test . -run TestFileDropSubscriptionAndEmissionAreSeparate -v`
Expected: PASS

- [ ] **Step 6: Run the whole non-e2e suite unchanged**

Run: `go test . ./internal/... -count=1`
Expected: all packages ok. Any pre-existing test that had to change is a constraint violation — stop and report.

- [ ] **Step 7: Commit**

```bash
git add app.go app_m5_test.go app_test.go
git commit -m "refactor: file drop subscription and emission are opposite directions"
```

---

### Task 2: `tools/genbridge` parses `app.go` and returns the bound surface

Pure parsing only. No JS emitted yet, no consumers.

**Files:**
- Create: `tools/genbridge/generate.go`
- Create: `tools/genbridge/generate_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type Method struct { Name string; Params []string; Guarded bool }` and `func ParseApp(src string) ([]Method, error)`. `Params` holds parameter *names* in declaration order. `Guarded` reports whether the body's first statement is `defer a.reportPanic("<Name>")`.

- [ ] **Step 1: Write the failing test**

Create `tools/genbridge/generate_test.go`:

```go
package main

import "testing"

// The regex detector this replaces matched 17 of 18 methods, because
// UpdateContent is written on one line — it reported a method clean by never
// looking at it. The fixture below includes that exact shape.
func TestParseAppFindsEveryExportedMethodIncludingOneLiners(t *testing.T) {
	src := `package main

func (a *App) SaveDocument(path, content string) error {
	defer a.reportPanic("SaveDocument")
	return nil
}

func (a *App) UpdateContent(content string) { defer a.reportPanic("UpdateContent"); }

func (a *App) unexported() {}

func (b *Other) NotOurs() {}
`
	got, err := ParseApp(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 bound methods, got %d: %+v", len(got), got)
	}
	if got[0].Name != "SaveDocument" || len(got[0].Params) != 2 ||
		got[0].Params[0] != "path" || got[0].Params[1] != "content" {
		t.Errorf("SaveDocument parsed wrong: %+v", got[0])
	}
	if got[1].Name != "UpdateContent" || !got[1].Guarded {
		t.Errorf("one-line method parsed wrong: %+v", got[1])
	}
}

func TestParseAppReportsAnUnguardedMethod(t *testing.T) {
	src := `package main

func (a *App) Risky(x string) error { return nil }
`
	got, err := ParseApp(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Guarded {
		t.Fatalf("an unguarded method must be reported as unguarded: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tools/genbridge/ -v`
Expected: FAIL to compile — `ParseApp` undefined.

- [ ] **Step 3: Write the implementation**

Create `tools/genbridge/generate.go`:

```go
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
)

// Method is one Wails-bound method on *App.
type Method struct {
	Name    string
	Params  []string
	Guarded bool
}

// ParseApp returns the exported methods on *App, in declaration order.
//
// It uses go/ast rather than a regular expression on purpose. Every regex
// detector this replaces was wrong at least once: one matched 17 of 18 methods
// because a method was written on a single line, and reported the method it
// never looked at as clean.
func ParseApp(src string) ([]Method, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "app.go", src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse app.go: %w", err)
	}

	var out []Method
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		ident, ok := star.X.(*ast.Ident)
		if !ok || ident.Name != "App" {
			continue
		}
		if !fn.Name.IsExported() {
			continue
		}

		m := Method{Name: fn.Name.Name}
		for _, field := range fn.Type.Params.List {
			for _, name := range field.Names {
				m.Params = append(m.Params, name.Name)
			}
		}
		m.Guarded = firstStatementGuards(fn, m.Name)
		out = append(out, m)
	}
	return out, nil
}

// firstStatementGuards reports whether the method's FIRST statement is
// `defer a.reportPanic("<name>")`. Position matters: a guard placed after other
// statements does not cover them.
func firstStatementGuards(fn *ast.FuncDecl, name string) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	def, ok := fn.Body.List[0].(*ast.DeferStmt)
	if !ok {
		return false
	}
	sel, ok := def.Call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "reportPanic" || len(def.Call.Args) != 1 {
		return false
	}
	lit, ok := def.Call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	got, err := strconv.Unquote(lit.Value)
	return err == nil && got == name
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./tools/genbridge/ -v`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/genbridge/
git commit -m "feat: parse the bound surface from app.go with go/ast"
```

---

### Task 3: `genbridge` emits the JavaScript bridge

**Files:**
- Modify: `tools/genbridge/generate.go`
- Modify: `tools/genbridge/generate_test.go`

**Interfaces:**
- Consumes: `Method`, `ParseApp` from Task 2.
- Produces: `func EmitJS(methods []Method, events []string) string`. `events` is the list of frontend event names; the caller passes `[]string{"file:open", "files:dropped"}`.

- [ ] **Step 1: Write the failing test**

Append to `tools/genbridge/generate_test.go`:

```go
import "strings"

func TestEmitJSProducesCamelCaseCallsThroughTheTransport(t *testing.T) {
	js := EmitJS([]Method{
		{Name: "SaveDocument", Params: []string{"path", "content"}, Guarded: true},
		{Name: "FrontendReady", Guarded: true},
	}, []string{"file:open"})

	for _, want := range []string{
		"// Code generated by tools/genbridge. DO NOT EDIT.",
		"import { transport } from './transport.js'",
		"saveDocument: (path, content) => transport.call('SaveDocument', path, content),",
		"frontendReady: () => transport.call('FrontendReady'),",
		"onFileOpen: (handler) => transport.on('file:open', handler),",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("generated bridge is missing:\n  %s\ngot:\n%s", want, js)
		}
	}
	if strings.Contains(js, "globalThis.go") || strings.Contains(js, "wails") {
		t.Error("the generated bridge must name no host; that belongs in transport.js")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tools/genbridge/ -run TestEmitJS -v`
Expected: FAIL to compile — `EmitJS` undefined.

- [ ] **Step 3: Write the implementation**

Append to `tools/genbridge/generate.go`:

```go
import (
	"strings"
	"unicode"
)

// EmitJS renders the bridge module. It contains no host knowledge at all: every
// call goes through transport.js, which is the single module a host swap
// touches.
func EmitJS(methods []Method, events []string) string {
	var b strings.Builder
	b.WriteString("// Code generated by tools/genbridge. DO NOT EDIT.\n")
	b.WriteString("// Regenerate with: go run ./tools/genbridge\n")
	b.WriteString("//\n")
	b.WriteString("// This file is tracked in git on purpose. The Wails-generated binding is\n")
	b.WriteString("// gitignored, and a stale copy on a build machine once turned SyncDocuments\n")
	b.WriteString("// into a silent no-op, leaving Go with no documents at all.\n\n")
	b.WriteString("import { transport } from './transport.js'\n\n")
	b.WriteString("export const bridge = {\n")
	b.WriteString("  available: () => transport.available(),\n")

	for _, m := range methods {
		params := strings.Join(m.Params, ", ")
		args := "'" + m.Name + "'"
		if params != "" {
			args += ", " + params
		}
		fmt.Fprintf(&b, "  %s: (%s) => transport.call(%s),\n", lowerFirst(m.Name), params, args)
	}
	for _, e := range events {
		fmt.Fprintf(&b, "  %s: (handler) => transport.on('%s', handler),\n", eventHandlerName(e), e)
	}

	b.WriteString("}\n")
	return b.String()
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// eventHandlerName turns "file:open" into "onFileOpen".
func eventHandlerName(event string) string {
	out := "on"
	for _, part := range strings.FieldsFunc(event, func(r rune) bool { return r == ':' || r == '-' }) {
		if part == "" {
			continue
		}
		r := []rune(part)
		r[0] = unicode.ToUpper(r[0])
		out += string(r)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./tools/genbridge/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add tools/genbridge/
git commit -m "feat: emit a host-free bridge module from the parsed surface"
```

---

### Task 4: The `genbridge` command, `transport.js`, and the generated file

**Files:**
- Create: `tools/genbridge/main.go`
- Create: `frontend/dist/src/host/transport.js`
- Create: `frontend/dist/src/host/bridge.generated.js` (by running the tool)

**Interfaces:**
- Consumes: `ParseApp`, `EmitJS` from Tasks 2 and 3.
- Produces: the command `go run ./tools/genbridge`, writing `frontend/dist/src/host/bridge.generated.js`. `transport.js` exports `transport` with `available(): boolean`, `call(name, ...args): Promise<any>|null`, `on(event, handler): void`.

- [ ] **Step 1: Write `transport.js`**

Create `frontend/dist/src/host/transport.js`:

```js
// The ONLY module in the frontend that names a host.
//
// Replacing Wails means replacing this file and nothing else on the JS side.
// Resolved lazily so e2e tests can install a stub at any time, and so the app
// degrades rather than throws when running in a plain browser.
const app = () => globalThis.go?.main?.App ?? null

function missing(name) {
  console.warn(`transport: host binding unavailable for ${name} (not running under a host?)`)
  return null
}

export const transport = {
  available: () => app() !== null,

  call(name, ...args) {
    const target = app()
    if (!target || typeof target[name] !== 'function') return missing(name)
    return target[name](...args)
  },

  on(event, handler) {
    globalThis.runtime?.EventsOn?.(event, handler)
  },
}
```

- [ ] **Step 2: Write `main.go` for the tool**

Create `tools/genbridge/main.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// events are the names Go emits to the frontend. Kept here rather than parsed,
// because they are emitted through the native port by string and a parser would
// be guessing at intent.
var events = []string{"file:open", "files:dropped"}

const outPath = "frontend/dist/src/host/bridge.generated.js"

func main() {
	src, err := os.ReadFile("app.go")
	if err != nil {
		fail("read app.go: %v", err)
	}
	methods, err := ParseApp(string(src))
	if err != nil {
		fail("%v", err)
	}
	if len(methods) == 0 {
		fail("no exported App methods found; the parser is broken, not the code")
	}

	// A method bound to the frontend without its panic guard reaches nobody when
	// it fails. Refuse to generate rather than emit a binding for it.
	var unguarded []string
	for _, m := range methods {
		if !m.Guarded {
			unguarded = append(unguarded, m.Name)
		}
	}
	if len(unguarded) > 0 {
		fail("these bound methods lack `defer a.reportPanic(\"Name\")` as their first statement:\n  %v", unguarded)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fail("%v", err)
	}
	if err := os.WriteFile(outPath, []byte(EmitJS(methods, events)), 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Printf("wrote %s (%d methods, %d events)\n", outPath, len(methods), len(events))
}

func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "genbridge: "+format+"\n", args...)
	os.Exit(1)
}
```

- [ ] **Step 3: Generate the bridge and check the output**

Run: `go run ./tools/genbridge`
Expected: `wrote frontend/dist/src/host/bridge.generated.js (18 methods, 2 events)`

Then confirm it parses as JavaScript:

Run: `node --check frontend/dist/src/host/bridge.generated.js`
Expected: no output.

- [ ] **Step 4: Verify the generator refuses an unguarded method**

Temporarily delete the `defer a.reportPanic("SetDirty")` line from `app.go`, then:

Run: `go run ./tools/genbridge`
Expected: exit 1, message naming `SetDirty`.

Restore the line and re-run to confirm it succeeds again.

- [ ] **Step 5: Commit**

```bash
git add tools/genbridge/main.go frontend/dist/src/host/
git commit -m "feat: generate the tracked bridge, and refuse to bind an unguarded method"
```

---

### Task 5: Route the frontend through the generated bridge

`bridge.js` keeps its public shape so no consumer changes in this task. Its body becomes a re-export.

**Files:**
- Modify: `frontend/dist/src/bridge.js`
- Test: `e2e/e2e_test.go` (existing tests must pass unchanged)

**Interfaces:**
- Consumes: `bridge` from `frontend/dist/src/host/bridge.generated.js`.
- Produces: `frontend/dist/src/bridge.js` continues to export `bridge` with the same method names it exports today.

- [ ] **Step 1: Replace the body of `bridge.js`**

Replace the entire contents of `frontend/dist/src/bridge.js` with:

```js
// The bridge is generated from app.go by tools/genbridge. This module exists so
// the rest of the frontend keeps a stable import path, and so the generated file
// can be regenerated without touching consumers.
//
// Regenerate with: go run ./tools/genbridge
export { bridge } from './host/bridge.generated.js'
```

- [ ] **Step 2: Check syntax**

Run: `node --check frontend/dist/src/bridge.js`
Expected: no output.

- [ ] **Step 3: Run the full e2e suite**

Run: `go test ./e2e -count=1`
Expected: ok. These tests are unchanged; a failure here means the generated bridge does not match the hand-written one it replaced. Compare the two method lists before changing any test.

- [ ] **Step 4: Commit**

```bash
git add frontend/dist/src/bridge.js
git commit -m "refactor: the bridge is generated, and bridge.js re-exports it"
```

---

### Task 6: Move the event channel out of `app.js`

`app.js` reaches `globalThis.runtime.EventsOn` directly at two sites — a raw host global outside the module that owns host coupling.

**Files:**
- Modify: `frontend/dist/src/app.js` (the two `globalThis.runtime?.EventsOn?.` call sites)

**Interfaces:**
- Consumes: `bridge.onFileOpen(handler)` and `bridge.onFilesDropped(handler)` from the generated bridge.
- Produces: no new interface.

- [ ] **Step 1: Replace the `files:dropped` subscription**

In `frontend/dist/src/app.js`, replace:

```js
  globalThis.runtime?.EventsOn?.('files:dropped', (paths) => handleDroppedFiles(paths))
```

with:

```js
  bridge.onFilesDropped((paths) => handleDroppedFiles(paths))
```

- [ ] **Step 2: Replace the `file:open` subscription**

In the same file, replace the `globalThis.runtime?.EventsOn?.('file:open', ...)` call with `bridge.onFileOpen(...)`, keeping the existing handler body exactly as it is.

- [ ] **Step 3: Confirm no host globals remain in the frontend outside transport.js**

Run:

```bash
grep -rn "globalThis.go\|globalThis.runtime\|window.go" frontend/dist/src --include="*.js" | grep -v "host/transport.js"
```

Expected: no output.

- [ ] **Step 4: Check syntax and run the full e2e suite**

Run: `node --check frontend/dist/src/app.js && go test ./e2e -count=1`
Expected: no output from `node`, then `ok`.

- [ ] **Step 5: Commit**

```bash
git add frontend/dist/src/app.js
git commit -m "refactor: the frontend subscribes through the bridge, not a host global"
```

---

### Task 7: Introduce `hostPort` and shrink `main.go`

**Files:**
- Create: `host.go`
- Create: `host_wails.go`
- Modify: `main.go`
- Modify: `app.go` (move `wailsNative` into `host_wails.go`)

**Interfaces:**
- Consumes: `nativePort` from Task 1.
- Produces:

```go
type hostPort interface {
	Native() nativePort
	Run(cfg hostConfig) error
}

type hostConfig struct {
	Title       string
	Width       int
	Height      int
	Assets      fs.FS
	OnStartup   func(context.Context)
	OnBeforeClose func(context.Context) bool
	OnFileOpen  func(path string)
	Bind        []interface{}
}
```

- [ ] **Step 1: Write `host.go`**

```go
package main

import (
	"context"
	"io/fs"
)

// hostPort is what the application needs from whatever runs it: a window, a way
// to serve its assets, lifecycle callbacks, and the native operations.
//
// Defined from the application's needs, never from what a particular host
// offers. A boundary drawn around the shape of the current host encodes that
// host, and makes every later replacement expensive for a reason we created
// rather than one that is real.
//
// It composes nativePort rather than replacing it: those eleven operations
// already have a tested shape and a working fake.
type hostPort interface {
	Native() nativePort
	Run(cfg hostConfig) error
}

// hostConfig is the application's description of the window it wants.
type hostConfig struct {
	Title         string
	Width         int
	Height        int
	Assets        fs.FS
	OnStartup     func(context.Context)
	OnBeforeClose func(context.Context) bool
	OnFileOpen    func(path string)
	Bind          []interface{}
}
```

- [ ] **Step 2: Write `host_wails.go`**

Move `type wailsNative struct{}` and all its methods out of `app.go` into this new file, unchanged, and add:

```go
package main

import (
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// wailsHost is the only implementation of hostPort today. Replacing Wails means
// adding a sibling of this file and changing one line of main().
type wailsHost struct{}

func (wailsHost) Native() nativePort { return wailsNative{} }

func (wailsHost) Run(cfg hostConfig) error {
	return wails.Run(&options.App{
		Title:  cfg.Title,
		Width:  cfg.Width,
		Height: cfg.Height,
		AssetServer: &assetserver.Options{
			Assets: cfg.Assets,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		// The bundle advertises CFBundleDocumentTypes, so macOS routes a
		// double-clicked .md file here. At launch this fires before the webview
		// exists, which is why App holds the path until the frontend asks (#53).
		Mac: &mac.Options{
			OnFileOpen: cfg.OnFileOpen,
		},
		LogLevelProduction: logger.ERROR,
		OnStartup:          cfg.OnStartup,
		OnBeforeClose:      cfg.OnBeforeClose,
		Bind:               cfg.Bind,
	})
}
```

- [ ] **Step 3: Rewrite `main.go`**

```go
package main

import "embed"

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	host := wailsHost{}
	app := NewApp(host.Native())

	err := host.Run(hostConfig{
		Title:         "Dr. Markdown",
		Width:         1440,
		Height:        900,
		Assets:        assets,
		OnStartup:     app.startup,
		OnBeforeClose: app.beforeClose,
		OnFileOpen:    app.openFileFromOS,
		Bind:          []interface{}{app},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
```

- [ ] **Step 4: Update `NewApp` to take the native port**

In `app.go`, change `func NewApp() *App` to `func NewApp(native nativePort) *App`, and in its body replace `native: wailsNative{},` with `native: native,`. Leave every other dependency as it is.

- [ ] **Step 5: Build and run the full suite**

Run: `gofmt -w . && go vet ./... && go test ./... -count=1`
Expected: all ok. Any pre-existing test needing a change is a constraint violation — stop and report.

- [ ] **Step 6: Commit**

```bash
git add host.go host_wails.go main.go app.go
git commit -m "refactor: the app asks a hostPort for a window, not Wails"
```

---

### Task 8: Delete the superseded gates, add the boundary gates, record the measurement

**Files:**
- Delete: `bridge_contract_test.go`
- Modify: `crash_test.go` (remove `TestEveryBoundMethodReportsItsPanics` and `TestLifecycleCallbacksReportTheirPanics`)
- Create: `genbridge_test.go`
- Create: `e2e/host_boundary_test.go`
- Modify: `docs/architext/data/nodes.json`, `docs/architext/data/risks.json`

**Interfaces:**
- Consumes: everything above.
- Produces: no new interface.

- [ ] **Step 1: Write the regenerate-and-diff gate**

Create `genbridge_test.go` at the repo root:

```go
package main

import (
	"os"
	"os/exec"
	"testing"
)

// A generated file that is tracked but stale is the defect this replaces: the
// Wails binding is gitignored, and a stale copy on a build machine once turned
// SyncDocuments into a silent no-op, leaving Go with no documents at all.
// Tracking the artefact only helps if something proves it matches its source.
func TestGeneratedBridgeMatchesAppGo(t *testing.T) {
	const path = "frontend/dist/src/host/bridge.generated.js"
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the generated bridge is missing: %v", err)
	}
	t.Cleanup(func() { _ = os.WriteFile(path, before, 0o644) })

	out, err := exec.Command("go", "run", "./tools/genbridge").CombinedOutput()
	if err != nil {
		t.Fatalf("genbridge failed: %v\n%s", err, out)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the committed bridge does not match app.go. Run `go run ./tools/genbridge` and commit the result.")
	}
}
```

- [ ] **Step 2: Run it to verify it passes, then verify it can fail**

Run: `go test . -run TestGeneratedBridgeMatchesAppGo -v`
Expected: PASS.

Now prove the gate can go red: append a space to the last line of `frontend/dist/src/host/bridge.generated.js`, re-run, expect FAIL, then restore with `go run ./tools/genbridge`.

- [ ] **Step 3: Write the boundary gate**

Create `e2e/host_boundary_test.go`:

```go
package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The whole point of the boundary is that a host swap is a leaf change. That is
// only true while the host stays confined; this test is what makes "only two
// files know about Wails" a fact rather than an aspiration.
func TestOnlyTheHostFilesNameWails(t *testing.T) {
	allowed := map[string]bool{"host_wails.go": true, "main.go": true}

	var offenders []string
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.Contains(path, "/tools/") || allowed[filepath.Base(path)] {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), "wailsapp/wails") {
			offenders = append(offenders, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("these files import Wails outside the host boundary:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}
```

- [ ] **Step 4: Delete the superseded tests**

```bash
git rm bridge_contract_test.go
```

In `crash_test.go`, delete `TestEveryBoundMethodReportsItsPanics` and `TestLifecycleCallbacksReportTheirPanics` entirely, along with the now-unused `regexp`, `sort` and `strconv` imports. Keep `TestPanicIsRecordedAndShownAndStillPropagates`, `TestAnOperationThatDoesNotPanicRecordsNothing` and `TestRepeatedPanicsRecordEveryTimeButShowOneDialog` — those assert behaviour, not drift, and generation does not supersede them.

The lifecycle callbacks are no longer covered by a gate. Add this to `crash_test.go` in their place:

```go
// The three Wails lifecycle callbacks are not bound methods, so genbridge never
// sees them and cannot refuse to generate when one loses its guard. They are
// checked here instead, by name, because main.go registers them by name.
func TestLifecycleCallbacksReportTheirPanics(t *testing.T) {
	src, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"startup", "beforeClose", "openFileFromOS"} {
		if !strings.Contains(string(src), `defer a.reportPanic("`+name+`")`) {
			t.Errorf("%s does not report a panic, and a panic there kills the process silently", name)
		}
	}
}
```

- [ ] **Step 5: Run the complete gate**

Run:

```bash
gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Then: `for f in frontend/dist/src/*.js frontend/dist/src/host/*.js; do node --check "$f" || break; done`

Expected: `gofmt` silent, everything ok.

- [ ] **Step 6: Measure the host surface — this is the deliverable**

Run:

```bash
wc -l host_wails.go main.go frontend/dist/src/host/transport.js
```

Record the three numbers and their total in the PR description under the heading "Host-specific surface". This total is the migration cost for options B and C, and it is what the A/B/C reassessment is based on.

- [ ] **Step 7: Update Architext**

In `docs/architext/data/nodes.json`, add a node for the host boundary describing `hostPort`, `wailsHost` and `transport.js`, and note that Wails is confined to two Go files and one JS module. In `docs/architext/data/risks.json`, update `no-recorded-state-for-rca` to record that #61 remains open and is now a one-line policy decision under any replacement rather than an upstream bug.

Run: `architext validate .`
Expected: `Architext validation passed.`

- [ ] **Step 8: Commit and open the PR**

```bash
git add -A
git commit -m "refactor: confine the host to two Go files and one JS module"
export GH_TOKEN=$(gh auth token --user robot-accomplice)
git push -u origin refactor/host-boundary
gh pr create --base develop --title "refactor: confine the host to two Go files and one JS module"
```

---

## Self-Review

**Spec coverage.** Every spec section maps to a task: the `SubscribeFileDrop` split → Task 1; the event channel leaving `app.js` → Task 6; `go/ast` generation → Tasks 2–4; the tracked artefact and its gate → Tasks 4 and 8; `hostPort` composing `nativePort` → Task 7; the three deletions → Task 8; the measurement → Task 8 Step 6; no behaviour change → the constraint restated in Tasks 1, 5, 6 and 7.

**Placeholders.** None. Every code step carries the code.

**Type consistency.** `nativePort.SubscribeFileDrop(ctx, func([]string))` and `EmitFilesDropped(ctx, []string)` are defined in Task 1 and used in Task 7. `Method{Name, Params, Guarded}` and `ParseApp` are defined in Task 2 and used in Tasks 3 and 4. `EmitJS(methods, events)` is defined in Task 3 and used in Task 4. `transport.available/call/on` is defined in Task 4 and used by generated output from Task 3. `hostPort`/`hostConfig` are defined in Task 7 and used in `main.go` in the same task. `NewApp` changes signature in Task 7 Step 4, which is the only caller.

**One deliberate asymmetry, flagged rather than hidden.** Task 8 deletes the bound-method panic gate because `genbridge` refuses to generate an unguarded binding — stronger than a test. It *re-adds* a lifecycle gate, because those three callbacks are not bound methods and the generator never sees them. Deleting both would have left them uncovered.
