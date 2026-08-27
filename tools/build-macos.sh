#!/usr/bin/env bash
# build-macos.sh - production macOS packaging for Dr Markdown.
#
# Replaces the framework build command this project no longer depends on. That
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
#   build/bin/Dr Markdown.app    the app bundle
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

# Withdraw a bundle from LaunchServices, so a build artifact does not appear in
# Launchpad and Spotlight beside the installed application — same icon, same
# name, indistinguishable. Safe to call on a path that is about to be deleted;
# it must be called BEFORE the delete, because a registration outlives its files.
unregister_app() {
    local lsregister="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
    [ -x "$lsregister" ] || return 0
    "$lsregister" -u "$1" >/dev/null 2>&1 || true
}
if [ -z "$VERSION" ]; then
    echo "error: VERSION is empty" >&2
    exit 1
fi

# The bundle is named as the application is named. macOS shows an .app's
# FILENAME in Finder, Launchpad and Spotlight — CFBundleName and
# CFBundleDisplayName have said so all along, and the launcher still read
# "dr-markdown" off the file. The space is why every path here is quoted.
#
# NO PERIOD, deliberately. macOS strips a ".app" suffix for display only when
# the basename has no other dot in it; with one, it cannot tell which dot begins
# the extension and shows the whole filename. Measured:
#
#   Dr. Markdown.app -> "Dr. Markdown.app"   extension shown
#   Dr Markdown.app  -> "Dr Markdown"        stripped
#   Dr.Markdown.app  -> "Dr.Markdown.app"    extension shown
#   DrMarkdown.app   -> "DrMarkdown"         stripped
APP="build/bin/Dr Markdown.app"
DMG="build/dr-markdown.dmg"
STAGE="build/dmg"
# The executable's name is what AppKit puts in the application menu — the first
# menu in the bar, beside the Apple logo. It takes the PROCESS name there and
# ignores CFBundleName, which is why that menu read "dr-markdown" while every
# plist key already said "Dr. Markdown". It must match CFBundleExecutable.
BIN="$APP/Contents/MacOS/Dr Markdown"

echo "==> cleaning"
# Clean the whole output directory, not just this build's bundle.
#
# Removing only "$APP" leaves any PREVIOUSLY named bundle sitting beside it.
# When the app was renamed from dr-markdown.app to Dr. Markdown.app in 1.6.2,
# the old one stayed in build/bin — where macOS indexed it, so the launcher
# offered a stale 1.6.1 alongside the current build. A build directory should
# contain what this build produced and nothing else.
rm -rf "$(dirname "$APP")"
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
# Signing. Developer ID when an identity is supplied, ad-hoc otherwise.
#
# Set DRMD_SIGN_IDENTITY to a Developer ID Application identity to produce a
# build a stranger can open. Without it the build is ad-hoc signed exactly as
# before, which is what CI does and what a contributor without a certificate
# gets — the difference must not be a build failure.
#
# It NEVER falls back silently. If an identity is named and signing fails, the
# build stops: a request to produce a distributable artifact that quietly
# produces an undistributable one is the failure this project keeps finding.
if [ -n "${DRMD_SIGN_IDENTITY:-}" ]; then
    echo "==> signing with $DRMD_SIGN_IDENTITY"
    # --options runtime is the hardened runtime, which notarization requires.
    # --timestamp is a secure timestamp, and it is not merely required: it
    # proves the signature was made while the certificate was valid, so the
    # application keeps opening after that certificate expires.
    #
    # No entitlements file, deliberately. Measured against this bundle: it links
    # only system frameworks, contains exactly one executable with no nested
    # code, and its JavaScript runs in WebKit's own process rather than in this
    # one. None of allow-jit, allow-unsigned-executable-memory or a
    # library-validation exception applies, and adding an entitlement that is
    # not needed widens what the process is permitted to do.
    codesign --force --options runtime --timestamp \
             --sign "$DRMD_SIGN_IDENTITY" "$APP"
else
    echo "==> ad-hoc signing"
    codesign --force --deep --sign - "$APP"
fi

echo "==> verifying the bundle"
lipo -archs "$BIN"
/usr/libexec/PlistBuddy -c "Print :CFBundleShortVersionString" "$APP/Contents/Info.plist"
codesign --verify --deep --strict "$APP" && echo "signature ok"

# The app is notarized and stapled BEFORE it is packaged, so the copy inside
# the DMG carries its own ticket. Skipped entirely without credentials.
if [ -n "${DRMD_NOTARY_PROFILE:-}" ] && [ -n "${DRMD_SIGN_IDENTITY:-}" ]; then
    # Notarize and staple the APP first, before it is packaged.
    #
    # Stapling only the DMG leaves the application inside it without a ticket of
    # its own. Measured: after a notarized DMG was stapled, `stapler validate` on
    # the app inside reported "does not have a ticket stapled to it". Once a user
    # drags that app to /Applications it carries no ticket, so Gatekeeper has to
    # reach Apple on first launch — which is the exact situation stapling exists
    # to avoid, and it fails on a machine with no network.
    #
    # The app is submitted as a zip because notarytool does not accept a bare
    # bundle. The ticket is then stapled to the bundle itself, and the DMG is
    # built from the stapled copy.
    echo "==> notarizing the app"
    APPZIP="build/app-for-notarization.zip"
    rm -f "$APPZIP"
    ditto -c -k --keepParent "$APP" "$APPZIP"
    xcrun notarytool submit "$APPZIP" --keychain-profile "$DRMD_NOTARY_PROFILE" --wait
    rm -f "$APPZIP"
    echo "==> stapling the app"
    xcrun stapler staple "$APP"
    xcrun stapler validate "$APP"
fi

echo "==> staging DMG in $STAGE"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -sf /Applications "$STAGE/Applications"

echo "==> creating $DMG"
hdiutil create -volname "Dr Markdown" -srcfolder "$STAGE" -ov -format UDZO "$DMG" >/dev/null
# Withdraw the staged copy BEFORE deleting it. macOS registers an .app the
# moment it appears, and that registration OUTLIVES the files — deleting the
# staging directory leaves a launcher entry pointing at a path that no longer
# exists, which is how a second "Dr Markdown" kept coming back after every
# build with nothing on disk to explain it.
unregister_app "$STAGE/$(basename "$APP")"
rm -rf "$STAGE"

if [ "$INSTALL" -eq 1 ]; then
    echo "==> installing to /Applications"
    # Replace by IDENTITY, not by path.
    #
    # This removed only one hard-coded path, so a bundle carrying the same
    # identifier under a different name survived the install. That is not
    # hypothetical: a hand-renamed "Dr. Markdown.app" at 0.5.1 sat beside a
    # freshly installed 0.6.0 with the same CFBundleIdentifier, and because
    # LaunchServices resolves documents by identifier rather than by path,
    # double-clicking a .md file could open EITHER — showing a version behind
    # with no way to tell from the window.
    #
    # The identifier is read from the bundle being installed, and every
    # candidate is checked against it before removal, so this can only ever
    # remove another copy of this application.
    BUNDLE_ID="$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$APP/Contents/Info.plist")"
    if [ -z "$BUNDLE_ID" ]; then
        echo "error: the bundle being installed has no CFBundleIdentifier, so an" >&2
        echo "       existing install cannot be identified. Refusing to install." >&2
        exit 1
    fi
    for existing in /Applications/*.app "$HOME"/Applications/*.app; do
        [ -d "$existing" ] || continue
        id="$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' \
              "$existing/Contents/Info.plist" 2>/dev/null || true)"
        [ "$id" = "$BUNDLE_ID" ] || continue
        version="$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' \
                   "$existing/Contents/Info.plist" 2>/dev/null || echo '?')"
        echo "    replacing $existing ($version)"
        rm -rf "$existing"
    done
    cp -R "$APP" /Applications/
    echo "    installed /Applications/$(basename "$APP") ($VERSION)"
fi

# Keep build artifacts out of the launcher.
#
# macOS registers any .app it finds, so a bundle built inside the repository
# shows up in Launchpad and Spotlight beside the installed one — same icon, same
# name, indistinguishable. That is not a stale-file problem the clean step can
# solve: it happens the moment a fresh build exists, and it was reported three
# times before this line existed.
#
# The installed copy is unaffected; only the one under build/ is withdrawn, and
# only if it was not just installed from.
if [ "$INSTALL" -eq 0 ]; then
    unregister_app "$APP"
fi

# Notarization. Only when a credential profile is named AND the app was signed
# with a real identity — notarizing an ad-hoc build is rejected by Apple, and
# attempting it would turn a working developer build into a failed one.
#
# Create the profile once:
#   xcrun notarytool store-credentials drmd-notary --key <p8> \
#         --key-id <id> --issuer <issuer-uuid>
#
# The DMG is signed before submission and stapled after. Stapling is what lets
# the application open on a machine with no network: without the ticket
# attached, Gatekeeper has to reach Apple on first launch.
if [ -n "${DRMD_NOTARY_PROFILE:-}" ]; then
    if [ -z "${DRMD_SIGN_IDENTITY:-}" ]; then
        echo "error: DRMD_NOTARY_PROFILE is set but DRMD_SIGN_IDENTITY is not." >&2
        echo "       Apple rejects an ad-hoc signed submission, so this would fail" >&2
        echo "       after the upload rather than before it. Set both or neither." >&2
        exit 1
    fi
    echo "==> signing the DMG"
    codesign --force --sign "$DRMD_SIGN_IDENTITY" "$DMG"
    echo "==> notarizing (this waits on Apple, typically 2-5 minutes)"
    xcrun notarytool submit "$DMG" --keychain-profile "$DRMD_NOTARY_PROFILE" --wait
    echo "==> stapling the ticket"
    xcrun stapler staple "$DMG"
    echo "==> verifying as Gatekeeper will see it"
    xcrun stapler validate "$DMG"
    spctl -a -vvv -t install "$DMG" || {
        echo "error: the notarized DMG did not pass Gatekeeper assessment." >&2
        exit 1
    }
fi

echo ""
echo "Done."
echo "  app: $APP"
echo "  dmg: $DMG"
