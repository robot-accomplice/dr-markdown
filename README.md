# Dr. Markdown

A native WYSIWYG markdown editor — Go + Wails, no Node.js anywhere.

## Development

```sh
go test ./...
wails dev
```

## Release packaging (macOS)

```sh
tools/build-macos.sh                # darwin/arm64 build + DMG
tools/build-macos.sh --universal    # universal binary (needs both-arch CGO toolchains)
tools/build-macos.sh --install      # also copies the .app to /Applications
```

Outputs:

- `build/bin/dr-markdown.app` — the app bundle
- `build/dr-markdown.dmg` — distributable disk image

The app icon is generated from `tools/genicon` (pure Go stdlib) into
`build/appicon.png`; Wails encodes it to `iconfile.icns` during packaging.
Bundle metadata lives in `build/darwin/Info.plist`.

Builds are self-signed ad-hoc. On other machines Gatekeeper will flag the
app; right-click → Open, or remove the quarantine attribute. Real
distribution (and anything App Store) requires a Developer ID certificate,
signing, and notarization — not wired up yet.
