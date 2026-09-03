# Separate Go use cases from the app binding surface

Date: 2026-09-03
Status: approved design
Release item: `go-use-cases-out-of-app` (stretch) in v1.7.0
(`docs/architext/data/releases/v1-7-0-desktop-integration-and-packaging.json`)

## Purpose

`app.go` has grown to 760 lines mixing three kinds of code: the bound API the
frontend calls, the use-case logic behind it, and host-lifecycle plumbing. The
dependency inversion is already right — every external capability sits behind
an injected port — so this changes comprehension, not correctness. After it,
`app.go` is the adapter: a reader sees every method the frontend can call and
nothing else.

## Decisions

- **Same-package extraction.** Use-case types live in package main in new
  files. The port interfaces stay where they are and do not change; the host
  wiring in `main.go` does not change; existing tests keep working unchanged.
- **Everything with logic moves.** Not only the four use cases the roadmap
  names (open, save, resolve-unsaved, asset import) but also the external-URL
  scheme validation and the default-handler offer. What remains in `App` is
  pure plumbing: delegation, panic guarding, and host-lifecycle state.

## Structure

Four use-case types, each in its own file, constructed once inside
`newAppWithDependencies` from the same `appDependencies`:

- **`documentUseCase`** (`usecase_documents.go`) — open (dialog, recent, OS
  path), save (direct, save-as), the external-change overwrite guard, and
  resolve-unsaved (`promptUnsaved`, `saveCurrent`, dirty-document iteration).
  Owns the `currentPath`/`currentText` state and its mutex — that state is
  only ever mutated by open and save — and title rendering, since the title
  is a function of exactly that state plus the session.
- **`imageUseCase`** (`usecase_images.go`) — picker import, dropped-file
  import, asset load, reveal, and the unsaved-document rejection.
- **`linkUseCase`** (`usecase_links.go`) — the safe-scheme allowlist and URL
  cleaning behind `OpenExternalURL`.
- **`defaultHandlerUseCase`** (`usecase_defaulthandler.go`) — the click-time
  re-guard behind `SetAsDefaultMarkdownHandler` (the pure menu-state decision
  stays in `defaulthandler.go`, which this use case consumes).

Use-case methods take `context.Context` as their first parameter, matching the
ports. Each App bound method becomes a `reportPanic` defer plus one delegation.

## What stays in `app.go`

- The bound method bodies — one panic-guard defer and one delegation each.
- The port interface definitions and their adapters (`documentAdapter`,
  `fontAdapter`, `imageAssetAdapter`), `appDependencies`, and the constructors.
- Host-lifecycle plumbing: `startup`, `openFileFromOS`, `FrontendReady` with
  its `pendingOpen` queue, `recordBlockedNavigation`, `beforeClose` (the host
  callback — it delegates to the document use case's unsaved-changes guard).
- `RecordClientEvent` and the already-thin pass-throughs (`ListFontFamilies`,
  `LoadPreferences`, `SavePreferences`, `SyncDocuments`, `SetDirty`,
  `UpdateContent` — these delegate to `session` directly today and are already
  the thinnest possible adapter).
- The version embed and the icon-free dialog PNG.

## Hard rules

- **Zero behavior change.** Bound method signatures, JSON wire format, event
  names, error strings, and dialog copy stay byte-identical. This is a move,
  not a redesign.
- Ports are untouched; `main.go` and `host_*.go` are untouched.
- The full local gate must pass without modifying a single existing test
  assertion: `gofmt -l . && go vet ./... && ./tools/verify-vendor.sh &&
  go test ./... -count=1`. Mechanical test changes (e.g. a test that
  constructs a moved type directly) are allowed and must be flagged.
- No new abstractions beyond the four types — no interfaces between App and
  the use cases; App holds them as concrete struct fields.

## Out of scope

- `internal/usecase` packaging or port relocation (considered, rejected: a
  much larger diff for the same comprehension win).
- Any frontend or binding change.
- Splitting `session` or re-examining its API.
