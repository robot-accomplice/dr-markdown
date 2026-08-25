#!/usr/bin/env bash
# build-macos.sh — production macOS packaging for Dr. Markdown.
#
# Replaces `wails build`, which this project no longer depends on. That command
# did five things and only one of them was interesting: it compiled the binary,
# assembled a .app bundle, substituted a version into Info.plist, converted an
# icon, and ad-hoc signed the result. All five are standard macOS tooling, and
# doing them here means the packaging is inspectable rather than a black box.
#
# Usage:
#   tools/build-macos.sh                 build darwin/arm64, stage + create DMG
#   tools/build-macos.sh --universal     build a universal binary (arm64+amd64)
#   tools/build-macos.sh --install       also copy the .app to /Applications
#
# Outputs:
#   build/bin/dr-markdown.app   the app bundle
#   build/dr-markdown.dmg       distributable disk image
set -euo pipefail

cd "$(dirname "$0")/.."

UNIVERSAL=0
INSTALL=0
for arg in "$@"; do
    case "$arg" in
        --universal) UNIVERSAL=1 ;;
        --install)   INSTALL=1 ;;
        *) echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

# VERSION is the single source of build identity. Go embeds this same file, and
# TestAppVersionComesFromTheVersionFile pins that they agree — so the bundle and
# the event trail cannot disagree about which build this is.
VERSION="$(tr -d '[:space:]' < VERSION)"
if [ -z "$VERSION" ]; then
    echo "error: VERSION is empty" >&2
    exit 1
fi

APP="build/bin/dr-markdown.app"
DMG="build/dr-markdown.dmg"
STAGE="build/dmg"
BIN="$APP/Contents/MacOS/dr-markdown"

echo "==> cleaning"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

# cgo means every architecture is a real compile, not a cross-link. clang is
# told the target explicitly because CGO_ENABLED builds ignore GOARCH alone.
build_arch() {
    local goarch="$1" clangarch="$2" out="$3"
    echo "==> compiling darwin/$goarch"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$goarch" \
        CGO_CFLAGS="-arch $clangarch" CGO_LDFLAGS="-arch $clangarch" \
        go build -trimpath -ldflags "-s -w" -o "$out" .
}

if [ "$UNIVERSAL" -eq 1 ]; then
    build_arch arm64 arm64 "build/.bin-arm64"
    build_arch amd64 x86_64 "build/.bin-amd64"
    echo "==> lipo -create"
    lipo -create "build/.bin-arm64" "build/.bin-amd64" -output "$BIN"
    rm -f "build/.bin-arm64" "build/.bin-amd64"
else
    build_arch arm64 arm64 "$BIN"
fi

echo "==> Info.plist (version $VERSION)"
sed "s/__VERSION__/$VERSION/g" build/darwin/Info.plist > "$APP/Contents/Info.plist"
printf 'APPL????' > "$APP/Contents/PkgInfo"

echo "==> icon"
# The directory MUST be named <name>.iconset; iconutil rejects a bare
# ".iconset", which has no name before the extension.
#
# genicon writes every slot itself rather than this script scaling one source
# ten ways, because the icon is DIFFERENT artwork at different sizes: the
# stethoscope illustration from 64px up, and the bold M-arrow at 16 and 32,
# where the illustration was measured to be an unreadable smudge. Scaling one
# source would force that detail to survive 16px, which it cannot.
ICONSET="build/icon.iconset"
rm -rf "$ICONSET"
go run ./tools/genicon -artwork build/icon-artwork.png -iconset "$ICONSET"
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/iconfile.icns"
rm -rf "$ICONSET"

# Ad-hoc signature, matching what shipped before. It is NOT notarization and
# does not remove Gatekeeper's warning on first open; it exists so the bundle
# has a stable identity and so macOS will honour its document-type claims.
echo "==> ad-hoc signing"
codesign --force --deep --sign - "$APP"

echo "==> verifying the bundle"
lipo -archs "$BIN"
/usr/libexec/PlistBuddy -c "Print :CFBundleShortVersionString" "$APP/Contents/Info.plist"
codesign --verify --deep --strict "$APP" && echo "signature ok"

echo "==> staging DMG in $STAGE"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -sf /Applications "$STAGE/Applications"

echo "==> creating $DMG"
hdiutil create -volname "Dr. Markdown" -srcfolder "$STAGE" -ov -format UDZO "$DMG" >/dev/null
rm -rf "$STAGE"

if [ "$INSTALL" -eq 1 ]; then
    echo "==> installing to /Applications"
    rm -rf "/Applications/dr-markdown.app"
    cp -R "$APP" /Applications/
fi

echo ""
echo "Done."
echo "  app: $APP"
echo "  dmg: $DMG"
