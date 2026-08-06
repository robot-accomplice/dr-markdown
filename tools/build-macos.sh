#!/usr/bin/env bash
# build-macos.sh — production macOS packaging for Dr. Markdown.
#
# Usage:
#   tools/build-macos.sh                 build darwin/arm64, stage + create DMG
#   tools/build-macos.sh --universal     build darwin/universal instead
#                                        (requires CGO toolchains for BOTH
#                                        amd64 and arm64, e.g. a clang that
#                                        can cross-compile via -arch)
#   tools/build-macos.sh --install       also copy the .app to /Applications
#
# Outputs:
#   build/bin/dr-markdown.app   the app bundle
#   build/dr-markdown.dmg       distributable disk image
set -euo pipefail

cd "$(dirname "$0")/.."

PLATFORM="darwin/arm64"
INSTALL=0
for arg in "$@"; do
    case "$arg" in
        --universal) PLATFORM="darwin/universal" ;;
        --install)   INSTALL=1 ;;
        *) echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

echo "==> wails build -clean -platform $PLATFORM"
wails build -clean -platform "$PLATFORM"

APP="build/bin/dr-markdown.app"
DMG="build/dr-markdown.dmg"
STAGE="build/dmg"

if [ ! -d "$APP" ]; then
    echo "error: expected app bundle not found at $APP" >&2
    exit 1
fi

echo "==> staging DMG in $STAGE"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -sf /Applications "$STAGE/Applications"

echo "==> creating $DMG"
hdiutil create -volname "Dr. Markdown" -srcfolder "$STAGE" -ov -format UDZO "$DMG"
rm -rf "$STAGE"

if [ "$INSTALL" -eq 1 ]; then
    echo "==> installing $APP to /Applications (may prompt for permission)"
    cp -R "$APP" /Applications/
    echo "==> installed /Applications/dr-markdown.app"
fi

echo ""
echo "Done."
echo "  app: $APP"
echo "  dmg: $DMG"
