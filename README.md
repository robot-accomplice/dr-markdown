![Dr. Markdown, .MD](docs/assets/banner.png)

# Dr. Markdown, .MD

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/robot-accomplice/dr-markdown)](go.mod)
[![Wails v2.13](https://img.shields.io/badge/Wails-v2.13.0-red.svg)](https://wails.io)
[![Platform](https://img.shields.io/badge/platform-macOS-lightgrey.svg)](#building--installing-macos)
[![Node.js](https://img.shields.io/badge/Node.js-not%20required-success.svg)](#architecture)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/robot-accomplice/dr-markdown/issues)

Dr. Markdown is a native WYSIWYG markdown editor. It pairs a Go shell (Wails) with a real editing surface — Milkdown Crepe on ProseMirror, CodeMirror for raw mode — vendored as self-contained ESM bundles. Markdown on disk is the source of truth; what you see is what gets saved.

**Why?** Typora set the bar for distraction-free markdown editing, then went closed and paid. Dr. Markdown is the open-source answer: MIT licensed, native via the OS webview (no Electron), and built with zero Node.js anywhere — not at development time, not at build time, not at runtime. If you have Go, you can build it.

## Features

### Available today

- WYSIWYG editing of GitHub-flavored markdown: tables, task lists, strikethrough, and syntax-highlighted code blocks (via Milkdown Crepe's built-ins)
- Raw and split markdown modes with formatted preview/source switching
- Native syntax highlighting for raw markdown and language-tagged fenced code blocks
- Mermaid Diagram rendering plus a guided Mermaid starter assistant
- Contextual table, code-block language, diagram, and image controls on the document surface
- Image import from the ribbon or by dropping files onto the window, copied into a `<document>.assets/` folder and referenced by relative path so the markdown stays portable
- Image width, alt text, replace, reveal-in-Finder, and delete controls on the selected image; a missing asset renders as a visible broken state rather than blank space
- Persistent settings for document font, code font, ligatures, editor width, default mode, and format-on-save
- Recent markdown documents on the start screen, backed by native preference storage
- Print and PDF export through the native print dialog path
- Native open/save dialogs; files associate with the app on macOS
- Atomic saves for documents *and* preferences — a crashed write never leaves you a truncated file, and an unreadable preference store is quarantined and replaced with defaults rather than stopping the app from starting
- Dirty tracking with an unsaved-changes close guard and an open-over-dirty guard
- A round-trip corpus (markdown → WYSIWYG → markdown) driven by chromedp, comparing the editor's own serialized output against the fixture — verified to fail when the serializer is broken
- macOS packaging: `tools/build-macos.sh` produces an ad-hoc-signed `.app` and a distributable `.dmg` with a custom icon. It is **not** Developer ID signed or notarized, so Gatekeeper will flag it on any machine but the build host (see [Known limitations](#known-limitations))

### On the roadmap

- Comments and review workflow
- Direct PDF/HTML export artifacts beyond the OS print dialog path
- Sharing, sync/Git, and extensions surfaces
- Developer ID signing, notarization, and Windows/Linux packaging

## Screenshots

Screenshots are generated from the checked-in frontend with:

```sh
go run ./tools/screenshots
```

Run that command whenever a UI-facing fix changes layout, chrome, controls, or
visible editor behavior, and commit the refreshed files under
`docs/assets/screenshots/`.

### Start screen

![Dr. Markdown start screen](docs/assets/screenshots/start.png)

### Formatted editor

![Dr. Markdown formatted editor](docs/assets/screenshots/formatted.png)

### Raw editor

![Dr. Markdown raw editor](docs/assets/screenshots/raw.png)

### Split editor

![Dr. Markdown split editor](docs/assets/screenshots/split.png)

### Settings

![Dr. Markdown settings](docs/assets/screenshots/settings.png)

## Getting started

Prerequisites:

- Go 1.26+
- Wails CLI v2.13.0: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0`
- Chrome (for the chromedp-based e2e tests)
- macOS (only for the packaging step)

```sh
go test ./...   # unit + round-trip corpus + e2e
wails dev       # live-reload development build
```

## Building & installing (macOS)

```sh
tools/build-macos.sh                # darwin/arm64 build + DMG
tools/build-macos.sh --universal    # universal binary (needs both-arch CGO toolchains)
tools/build-macos.sh --install      # also copies the .app to /Applications
```

Outputs:

- `build/bin/dr-markdown.app` — the app bundle
- `build/dr-markdown.dmg` — drag the app to Applications and you're done

Builds are self-signed ad-hoc. On machines other than the build host, Gatekeeper will flag the app: right-click → Open (or remove the quarantine attribute). Developer ID signing and notarization are future work.

## Architecture

Three layers, no Node anywhere:

1. **Go shell** — Wails v2 window, native dialogs, atomic file I/O, dirty-state guards.
2. **Vendored ESM bundles** — Milkdown Crepe and CodeMirror, pre-bundled once and committed; the app loads them as-is.
3. **Hand-written app JS** — the glue: mode switching, load/save plumbing, dirty tracking.

Markdown on disk is the source of truth. The WYSIWYG surface round-trips through it, and the chromedp round-trip corpus in `testdata/roundtrip` is the correctness gate — if a fixture doesn't round-trip, the change doesn't land. Full design notes: [docs/superpowers/specs/2026-08-05-dr-markdown-design.md](docs/superpowers/specs/2026-08-05-dr-markdown-design.md).

## Tech stack

| Component | Version |
| --- | --- |
| Go | 1.26.5 |
| Wails | v2.13.0 |
| Milkdown Crepe | 7.22.0 |
| CodeMirror | 6.0.2 |
| Highlight.js | 11.11.1 |
| Mermaid | 11.6.0 |
| chromedp (e2e) | v0.16.0 |

## Known limitations

- Code-block hover/right-click language editing targets rendered fenced blocks; deeper cursor-aware block editing is still future work.
- Direct PDF file generation is not implemented; PDF export uses the OS print dialog's Save as PDF path.
- Images must be inserted into a saved document — an unsaved document has no location to resolve a portable relative asset path against, so the import is refused up front.
- Image sizing is written as an `<img src alt width>` tag because CommonMark has no size syntax; clearing the width restores plain `![alt](path)`.
- Moving a markdown file without its `.assets` folder is detected and shown as a missing asset, but not repaired; asset folders are not garbage collected when an image is deleted from the document.
- Raw/Split marker hiding preserves source caret alignment by hiding marker glyph visibility rather than reflowing source text.
- Native-dialog flows (open/save/dirty guards) have manual checks pending beyond the automated corpus.

## Contributing

PRs welcome. `go test ./...` must stay green, and every fixture in the round-trip corpus must keep round-tripping. Hard constraint: no Node.js toolchain additions — the vendored ESM bundles are refreshed by maintainers via `tools/vendor.sh`, never by contributors at build time.

## License

[MIT](LICENSE)
