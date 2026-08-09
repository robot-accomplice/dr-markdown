# Go Session Extraction Implementation Plan (Phase 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the open-document session — the tabs, their dirty state, and the on-disk baseline — its own type with its own invariants, so the rules that once destroyed documents are stated in one place and tested directly.

**Architecture:** A new `internal/session` package owning the state `App` currently holds behind a mutex. `App` keeps the Wails binding surface, the ports and the event log, and delegates state questions to `*session.Session`. No new frameworks; the existing ports are untouched.

**Tech Stack:** Go 1.26.5, Wails v2.13.0. Standard library only.

## Global Constraints

- **Every commit runs the full gate:** `gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1`. CI runs gofmt first, so a gate that omits it is a subset of the real one — that is how a green local run reached a red CI in Phase 2.
- **Behaviour-preserving.** This phase changes structure, not behaviour.
- **The judging criterion:** every existing test stays green **and unchanged**, including `app_m5_test.go`, `app_staleness_test.go` and the whole `e2e` package. A task needing one edited to pass has changed behaviour and is wrong — stop and report.
- **The JSON wire format must not change.** The frontend sends and receives these structs through Wails; the `json:` tags are the contract.
- **Carry the comments across verbatim.** The comments on this state record *why* documents were destroyed once. They are the most valuable thing being moved.
- Attribution rule: no Claude/Anthropic co-author trailers or footers in commits or PRs.

## Scope: narrower than the design proposed, and why

The design (`docs/decisions/2026-08-09-domain-ownership-and-boundaries.md`, section 3) said:

> `OpenDocument`, `SaveDocument`, `ResolveUnsavedChanges` and asset import become use-case types with the existing injected ports. `App` becomes a thin adapter.

**This plan does the state half and deliberately does not do the use-case-types half.** The reason is evidence, not effort:

- `newAppWithDependencies` already injects every port as an interface. `app_staleness_test.go` tests the save path with **zero infrastructure** — no filesystem, no Wails. The property that use-case types exist to produce is **already true here**.
- So converting `SaveDocument` from a method into a `SaveDocument` type would move code between files and rename its receiver. It would not make one new thing testable, or one existing thing safer.
- The design itself ranked this phase last on exactly this basis: it "addresses comprehension rather than correctness."

What is *not* already true is that the session state has an owner. `docs`, `unsyncedDirty`, `currentPath`, `currentText` and `onDisk` sit on `App` behind a shared mutex, and their rules are recorded in comments that read like scar tissue:

> `currentPath` is the last path opened or saved. It names the window title and the Save-As default only; it is **NEVER a write target**, because inferring the target from ambient state is what destroyed documents.

That invariant is enforced today by every reader remembering to honour it. **That is the part worth extracting**, and it is where the historical data-loss bugs actually lived.

If the use-case-type split is wanted for its own sake, it should be its own decision with its own justification, rather than carried along by this one.

## What was measured

| cluster | lines | state it touches | ports it needs |
| --- | --- | --- | --- |
| `SyncDocuments`, `SetDirty`, `UpdateContent`, `dirtyDocuments`, `activeDocument`, `rememberOnDisk` | **56** | `docs`, `mu`, `unsyncedDirty`, `onDisk` | **none** |
| `confirmNoExternalChange` | 21 | `mu` + the baseline | `documents`, `native`, `events` |
| open/save/close flow | 161 | all of the above | all ports |

The 56-line cluster touches **no ports at all**. It is a value object with invariants wearing a mutex, and it is what moves.

`confirmNoExternalChange` stays on `App`: it needs three ports and asks the *user* a question, so it is a use case, not state. It will read its baseline through the new type.

## File Structure

**Create:**

| file | responsibility |
| --- | --- |
| `internal/session/session.go` | `Document` and `Session`: the open tabs, their dirty state, and the per-path on-disk baseline, with the invariants stated once. |
| `internal/session/session_test.go` | Direct tests for those invariants, with no `App` and no ports. |

**Modify:** `app.go` (delegate to the session, drop the moved fields), `docs/architext/data/nodes.json`, `docs/architext/data/decisions.json`.

---

### Task 1: The session type, with the tab invariants

**Files:**
- Create: `internal/session/session.go`, `internal/session/session_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Document struct { Path, Content string; Dirty, Active bool }` with the existing json tags `path`, `content`, `dirty`, `active`.
  - `type Session struct { ... }` — zero value ready to use.
  - `(*Session) Sync(docs []Document)`
  - `(*Session) Active() Document`
  - `(*Session) Dirty() []Document`

- [ ] **Step 1: Write the failing test**

Create `internal/session/session_test.go`:

```go
package session

import "testing"

// The session owns the rules that once destroyed documents. They were enforced
// by every reader remembering to honour them; here they are stated once and
// tested directly, with no App and no ports.

func TestActiveReturnsTheFocusedTab(t *testing.T) {
	var s Session
	s.Sync([]Document{
		{Path: "/a.md", Content: "A"},
		{Path: "/b.md", Content: "B", Active: true},
	})
	if got := s.Active(); got.Path != "/b.md" {
		t.Errorf("Active() = %q, want /b.md", got.Path)
	}
}

// A frontend that has not marked any tab active must still yield a usable
// answer rather than a zero value that names no file.
func TestActiveFallsBackToTheFirstTab(t *testing.T) {
	var s Session
	s.Sync([]Document{{Path: "/a.md"}, {Path: "/b.md"}})
	if got := s.Active(); got.Path != "/a.md" {
		t.Errorf("Active() = %q, want /a.md", got.Path)
	}
}

// With no tabs at all the answer must be a zero Document — never a guess.
func TestActiveWithNoTabsNamesNoFile(t *testing.T) {
	var s Session
	if got := s.Active(); got.Path != "" {
		t.Errorf("Active() on an empty session named %q; it must name no file", got.Path)
	}
}

func TestDirtyReturnsOnlyUnsavedTabs(t *testing.T) {
	var s Session
	s.Sync([]Document{
		{Path: "/a.md", Dirty: true},
		{Path: "/b.md"},
		{Path: "/c.md", Dirty: true},
	})
	got := s.Dirty()
	if len(got) != 2 || got[0].Path != "/a.md" || got[1].Path != "/c.md" {
		t.Errorf("Dirty() = %+v, want /a.md and /c.md", got)
	}
}

// Sync must copy. Handing the caller's slice back would let the frontend's next
// push mutate what the close guard is about to read.
func TestSyncCopiesTheCallersSlice(t *testing.T) {
	var s Session
	docs := []Document{{Path: "/a.md", Dirty: true}}
	s.Sync(docs)
	docs[0].Dirty = false
	if len(s.Dirty()) != 1 {
		t.Error("mutating the caller's slice after Sync changed the session's view")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/session -count=1
```

Expected: FAIL to build — `undefined: Session`, `undefined: Document`.

- [ ] **Step 3: Write the minimal implementation**

Create `internal/session/session.go`:

```go
// Package session owns the open-document session: which tabs exist, which has
// focus, which have unsaved changes, and what this app last read from or wrote
// to each file on disk.
//
// It exists because that state used to live on the Wails binding surface as
// loose fields behind a shared mutex, with its rules recorded only in comments
// that every reader had to remember to honour. Those rules are not stylistic —
// getting one wrong destroyed a document the user had not touched.
package session

import "sync"

// Document is one editor tab as the frontend reports it. The json tags are the
// wire contract with the webview and must not change.
type Document struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Dirty   bool   `json:"dirty"`
	Active  bool   `json:"active"`
}

// Session is safe for concurrent use. Its zero value is ready.
type Session struct {
	mu   sync.Mutex
	docs []Document
}

// Sync replaces the known tabs with what the frontend reported.
//
// The slice is copied: keeping the caller's backing array would let the next
// push from the frontend mutate what the close guard is about to read.
func (s *Session) Sync(docs []Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = append(s.docs[:0], docs...)
}

// Active returns the focused tab, or the first, or a zero value.
//
// The zero value names no file, deliberately. Go must never infer a write
// target from ambient state — that is what destroyed documents.
func (s *Session) Active() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.docs {
		if d.Active {
			return d
		}
	}
	if len(s.docs) > 0 {
		return s.docs[0]
	}
	return Document{}
}

// Dirty returns a copy of every tab with unsaved changes.
func (s *Session) Dirty() []Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Document
	for _, d := range s.docs {
		if d.Dirty {
			out = append(out, d)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/session -count=1 -v
```

Expected: PASS, five tests.

- [ ] **Step 5: Run the full gate**

```bash
gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. Nothing is wired up yet, so `app.go` is untouched.

- [ ] **Step 6: Commit**

```bash
git add internal/session
git commit -m "feat: internal/session owns the open-document session

The tabs, their dirty state and their focus lived as loose fields on the Wails
binding surface behind a shared mutex, with their rules recorded only in
comments each reader had to remember. Getting one wrong destroyed a document the
user had not touched.

Not wired up yet. Five tests state the invariants directly, including the two
that matter most: an empty session names NO file rather than guessing one, and
Sync copies the caller's slice so a later push cannot mutate what the close
guard is about to read."
```

---

### Task 2: Move dirty tracking and content updates onto the session

**Files:**
- Modify: `internal/session/session.go`, `internal/session/session_test.go`, `app.go`

**Interfaces:**
- Consumes: Task 1's `Session`.
- Produces:
  - `(*Session) SetDirty(dirty bool)`
  - `(*Session) HasUnsyncedDirty() bool`
  - `(*Session) UpdateActiveContent(content string)`

- [ ] **Step 1: Write the failing test**

Append to `internal/session/session_test.go`:

```go
// A frontend that reports dirty before it has ever synced its tabs must force a
// prompt without naming a file. Not knowing WHICH document is dirty has to lead
// to asking the user, never to writing a guess.
func TestDirtyWithNoSyncedTabsForcesAPromptWithoutNamingAFile(t *testing.T) {
	var s Session
	s.SetDirty(true)
	if !s.HasUnsyncedDirty() {
		t.Error("dirty reported before any sync must be remembered")
	}
	if len(s.Dirty()) != 0 {
		t.Error("it must not invent a dirty document")
	}
	if got := s.Active(); got.Path != "" {
		t.Errorf("it must not name a file, got %q", got.Path)
	}
}

// Once tabs exist, the flag belongs to the active tab and the standalone flag
// must clear — otherwise a stale unsynced flag outlives the condition.
func TestDirtyWithSyncedTabsMarksTheActiveTab(t *testing.T) {
	var s Session
	s.Sync([]Document{{Path: "/a.md"}, {Path: "/b.md", Active: true}})
	s.SetDirty(true)
	if s.HasUnsyncedDirty() {
		t.Error("the unsynced flag must not be set once tabs are known")
	}
	dirty := s.Dirty()
	if len(dirty) != 1 || dirty[0].Path != "/b.md" {
		t.Errorf("Dirty() = %+v, want only /b.md", dirty)
	}
}

func TestUpdateActiveContentTargetsOnlyTheActiveTab(t *testing.T) {
	var s Session
	s.Sync([]Document{{Path: "/a.md", Content: "A"}, {Path: "/b.md", Content: "B", Active: true}})
	s.UpdateActiveContent("edited")
	if got := s.Active(); got.Content != "edited" {
		t.Errorf("active content = %q, want edited", got.Content)
	}
	for _, d := range s.Dirty() {
		_ = d
	}
	s.Sync(append([]Document{}, s.snapshotForTest()...))
	for _, d := range s.snapshotForTest() {
		if d.Path == "/a.md" && d.Content != "A" {
			t.Errorf("the inactive tab's content was overwritten: %q", d.Content)
		}
	}
}
```

Add a tiny test-only accessor to `session.go` so the last assertion can read every tab:

```go
// snapshotForTest returns a copy of every tab. Test-only: production code asks
// specific questions (Active, Dirty) rather than reading the whole list.
func (s *Session) snapshotForTest() []Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Document(nil), s.docs...)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/session -count=1
```

Expected: FAIL to build — `SetDirty`, `HasUnsyncedDirty`, `UpdateActiveContent` undefined.

- [ ] **Step 3: Implement**

Add to `internal/session/session.go` — the `unsyncedDirty` field on `Session`, and:

```go
// SetDirty records the frontend's dirty state for the active tab.
//
// With no synced documents the flag is kept on its own rather than invented
// against the last opened path: not knowing which file is dirty must lead to
// asking the user, never to writing a guess.
func (s *Session) SetDirty(dirty bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unsyncedDirty = dirty && len(s.docs) == 0
	for i := range s.docs {
		if s.docs[i].Active {
			s.docs[i].Dirty = dirty
		}
	}
}

// HasUnsyncedDirty reports a dirty frontend that never named its documents.
func (s *Session) HasUnsyncedDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unsyncedDirty
}

// UpdateActiveContent stores the latest markdown for the active tab, pushed
// debounced by the frontend, so the close guard can save without a round trip.
func (s *Session) UpdateActiveContent(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.docs {
		if s.docs[i].Active {
			s.docs[i].Content = content
		}
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/session -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Delegate from `app.go`**

Add a `session *session.Session` field to `App`, initialised in `newAppWithDependencies`. Replace the bodies of `SyncDocuments`, `SetDirty`, `UpdateContent`, `dirtyDocuments` and `activeDocument` with delegations, keeping the `a.updateTitle()` calls where they already are:

```go
func (a *App) SyncDocuments(docs []OpenDocument) {
	a.session.Sync(docs)
	a.updateTitle()
}

func (a *App) SetDirty(dirty bool) {
	a.session.SetDirty(dirty)
	a.updateTitle()
}

func (a *App) UpdateContent(content string) { a.session.UpdateActiveContent(content) }

func (a *App) dirtyDocuments() []OpenDocument { return a.session.Dirty() }

func (a *App) activeDocument() OpenDocument { return a.session.Active() }
```

Then delete `docs` and `unsyncedDirty` from the `App` struct, and add the alias so the Wails binding signature and the JSON wire format are unchanged:

```go
// OpenDocument is one editor tab as the frontend sees it. Aliased rather than
// redefined so the Wails binding signature and the JSON wire format are
// byte-identical to before the move.
type OpenDocument = session.Document
```

Every remaining `a.mu` use that guarded only `a.docs` goes with it; the mutex stays for `currentPath`, `currentText` and `onDisk` until Task 3.

- [ ] **Step 6: Run the full gate**

```bash
gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green, **with no existing test edited**. `TestFrontendReportsEveryTabWithItsOwnPath` and the close-guard tests in `app_m5_test.go` are the sharp checks — they exist because this exact state once wrote one tab's text over another tab's file.

- [ ] **Step 7: Commit**

```bash
git add internal/session app.go
git commit -m "refactor: App delegates tab state to the session

docs and unsyncedDirty leave the binding surface. OpenDocument becomes a type
alias for session.Document so the Wails signature and the JSON wire format are
byte-identical — the json tags are the contract with the webview.

The rule that a dirty frontend with no synced tabs must force a prompt WITHOUT
naming a file is now a test in the package that owns it, rather than a comment
on a method that happens to honour it."
```

---

### Task 3: Move the on-disk baseline onto the session

**Files:**
- Modify: `internal/session/session.go`, `internal/session/session_test.go`, `app.go`

**Interfaces:**
- Consumes: Task 2's `Session`.
- Produces:
  - `(*Session) RememberOnDisk(path, content string)`
  - `(*Session) BaselineFor(path string) (string, bool)`

- [ ] **Step 1: Write the failing test**

```go
// The baseline is what the app last READ FROM or WROTE TO a file. Comparing
// against what was first opened instead would make every second save look like
// an external edit, and a prompt users learn to click through protects nothing.
func TestBaselineTracksTheLastReadOrWrite(t *testing.T) {
	var s Session
	if _, known := s.BaselineFor("/a.md"); known {
		t.Error("a path the app has never touched must have no baseline")
	}
	s.RememberOnDisk("/a.md", "first")
	if got, known := s.BaselineFor("/a.md"); !known || got != "first" {
		t.Errorf("BaselineFor = %q, %v; want first, true", got, known)
	}
	s.RememberOnDisk("/a.md", "second")
	if got, _ := s.BaselineFor("/a.md"); got != "second" {
		t.Errorf("BaselineFor = %q; a later write must replace the baseline", got)
	}
}

// Baselines are per path. One file's save must not silence another's conflict.
func TestBaselinesAreIndependentPerPath(t *testing.T) {
	var s Session
	s.RememberOnDisk("/a.md", "A")
	s.RememberOnDisk("/b.md", "B")
	if got, _ := s.BaselineFor("/a.md"); got != "A" {
		t.Errorf("/a.md baseline = %q, want A", got)
	}
	if _, known := s.BaselineFor("/c.md"); known {
		t.Error("an untouched path must still have no baseline")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test ./internal/session -count=1
```

Expected: FAIL to build — `RememberOnDisk`, `BaselineFor` undefined.

- [ ] **Step 3: Implement**

Add the `onDisk map[string]string` field to `Session` and:

```go
// RememberOnDisk records what this app last read from or wrote to a file. It is
// the baseline the staleness check compares against, and it is NEVER a write
// target.
func (s *Session) RememberOnDisk(path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.onDisk == nil {
		s.onDisk = map[string]string{}
	}
	s.onDisk[path] = content
}

// BaselineFor returns what the app believes is on disk at path, and whether it
// has any belief at all. A path it has never touched reports false, so the
// caller can save without interrupting the user over a comparison it cannot make.
func (s *Session) BaselineFor(path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, known := s.onDisk[path]
	return content, known
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/session -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Delegate from `app.go`**

Delete `rememberOnDisk` and the `onDisk` field. Replace its call sites with `a.session.RememberOnDisk(path, content)`. In `confirmNoExternalChange`, replace the locked map read with:

```go
	expected, known := a.session.BaselineFor(path)
	if !known {
		return nil
	}
```

`confirmNoExternalChange` stays on `App` — it needs three ports and asks the user a question, so it is a use case, not state.

- [ ] **Step 6: Run the full gate**

```bash
gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green. All five tests in `app_staleness_test.go` are the sharp checks — they were written for exactly this control and must pass **unchanged**.

- [ ] **Step 7: Commit**

```bash
git add internal/session app.go
git commit -m "refactor: the on-disk baseline moves to the session

RememberOnDisk and BaselineFor own what the app last read from or wrote to each
file. BaselineFor returns (content, known) so a path the app never touched is
distinguishable from one whose content happens to be empty — the caller must be
able to tell 'no belief' from 'believed empty' before it decides to interrupt
the user.

confirmNoExternalChange stays on App: it needs three ports and asks the user a
question, so it is a use case, not state."
```

---

### Task 4: Retire the App mutex and record the phase

**Files:**
- Modify: `app.go`, `docs/architext/data/nodes.json`, `docs/architext/data/decisions.json`

**Interfaces:**
- Consumes: the session from Tasks 1–3.
- Produces: an `App` whose remaining state is `ctx`, `currentPath`, `currentText`, the ports, the event log and the session.

- [ ] **Step 1: Check what the mutex still guards**

```bash
grep -n "a.mu" app.go
```

Every remaining use should guard only `currentPath` / `currentText`. If any still guards `docs`, `unsyncedDirty` or `onDisk`, a delegation was missed — fix that before continuing.

- [ ] **Step 2: Write the failing test**

Append to `app_staleness_test.go`:

```go
// The session owns the tabs and the on-disk baseline. If App still declares
// them, two copies of that state exist and the invariants are enforced in two
// places — which is the condition this phase removed.
func TestAppDoesNotDuplicateSessionState(t *testing.T) {
	source, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"docs []OpenDocument", "unsyncedDirty bool", "onDisk      map[string]string", "onDisk map[string]string"} {
		if strings.Contains(string(source), field) {
			t.Errorf("App still declares %q; the session owns it now", field)
		}
	}
}
```

Add `"os"` and `"strings"` to that file's imports if absent.

- [ ] **Step 3: Run the test to verify it fails or passes**

```bash
go test . -run TestAppDoesNotDuplicateSessionState -count=1 -v
```

If Tasks 2 and 3 removed the fields, this passes immediately — that is fine here, because its job is to keep them gone. Confirm it can fail by re-adding one field temporarily, running it, and removing it again. **Do not skip that confirmation**: this repo has a documented history of tests that could not fail.

- [ ] **Step 4: Run the full gate**

```bash
gofmt -l . && go vet ./... && ./tools/verify-vendor.sh && go test ./... -count=1
```

Expected: green.

- [ ] **Step 5: Record the phase in Architext**

In `docs/architext/data/nodes.json`, add a node:

```json
{
  "id": "document-session",
  "type": "module",
  "name": "Document session",
  "summary": "Owns the open editor tabs, their dirty state and the per-path record of what the app last read from or wrote to disk.",
  "responsibilities": [
    "Hold the open tabs as the frontend reports them, copied rather than aliased",
    "Answer which tab is active and which have unsaved changes, naming no file when it cannot know",
    "Record a dirty frontend that never synced its documents, without inventing a write target",
    "Hold the per-path on-disk baseline the staleness check compares against"
  ],
  "owner": "Project maintainers",
  "sourcePaths": ["internal/session/session.go"],
  "runtime": "Go",
  "interfaces": ["Sync", "Active", "Dirty", "SetDirty", "HasUnsyncedDirty", "UpdateActiveContent", "RememberOnDisk", "BaselineFor"],
  "dependencies": [],
  "dataHandled": ["markdown-document"],
  "security": [
    "Never infers a write target from ambient state: an empty session names no file, which is the defect that once wrote one tab's content over another tab's file"
  ],
  "observability": ["internal/session tests"],
  "relatedFlows": ["open-save-document"],
  "relatedDecisions": ["domain-ownership-and-boundaries"],
  "knownRisks": [],
  "verification": ["TestActiveWithNoTabsNamesNoFile", "TestDirtyWithNoSyncedTabsForcesAPromptWithoutNamingAFile", "TestBaselineTracksTheLastReadOrWrite"]
}
```

Add `document-session` to the `wails-go-api` node's `dependencies`. Prepend to the decision's `consequences`:

```
"Phase 3 LANDED 2026-08-09: the open-document session moved to internal/session — tabs, dirty state and the on-disk baseline, with their invariants tested directly rather than through the binding surface. Scope was deliberately narrowed from the design: the use-case-type split was NOT done, because newAppWithDependencies already injects every port and the save path was already testable with zero infrastructure, so it would have moved code without making anything newly safe."
```

```bash
architext validate .
```

Expected: `Architext validation passed.`

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: App keeps the bindings, the session keeps the state

The tabs, the unsynced-dirty flag and the on-disk baseline are gone from the
binding surface. What remains on App is ctx, currentPath, currentText, the
ports, the event log and the session.

A test now fails if any of those fields comes back, because two copies of this
state means the invariants are enforced in two places — the condition this phase
removed. Confirmed the test can fail by re-adding a field, not by reading it.

Architext records the new node and the narrowed scope, with its reason."
```

---

## Self-Review

**Spec coverage.** Design section 3's state half → Tasks 1–4. Its use-case-type half → deliberately not done, with the reason recorded in the plan, the commit and the decision record rather than dropped silently.

**Placeholder scan.** No TBDs. Every step carries its code and its exact command. Task 4 Step 3 says explicitly how to confirm a test that passes immediately can still fail, rather than assuming it.

**Type consistency.** `Document` is the session type throughout; `OpenDocument` becomes an alias for it in Task 2 and is used unchanged everywhere else in `app.go`. `BaselineFor` returns `(string, bool)` in Task 3's Produces and is consumed with that shape in `confirmNoExternalChange` in the same task. `Session`'s zero value is stated as ready in Task 1 and every test relies on `var s Session`.

**One gap found and closed during review:** Task 2's `UpdateActiveContent` test needed to read a non-active tab to prove it was left alone, which the public API deliberately does not allow. Rather than widening the API for a test, Step 1 adds a clearly-named `snapshotForTest` helper with a comment saying why production code does not read the whole list.
