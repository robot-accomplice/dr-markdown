#!/usr/bin/env bash
# tools/smoke-macos.sh - pre-publish smoke test for the packaged DMGs.
#
# The Go tests prove the source; this proves the ARTIFACT a user downloads.
# For each DMG: mount it, run the binary's own verification modes against the
# packaged build, and check the signature and notarization a user's Mac will
# check. Any failure exits non-zero — a release that cannot smoke-test its
# artifacts does not publish.
#
# Usage:
#   tools/smoke-macos.sh [--ad-hoc-ok] [--expect-version X.Y.Z] <dmg>...
#
#   --ad-hoc-ok         skip the notarization/Gatekeeper checks for a local
#                       unsigned build. The release ceremony never passes this.
#   --expect-version V  assert the bundle's CFBundleShortVersionString equals V
#                       instead of the repo's VERSION file. Exists so retained
#                       DMGs from past releases can exercise this script.
#                       The gates require a binary from v1.6.3 or later: older
#                       binaries (e.g. the retained v1.6.2 DMGs) do not know
#                       -quit, -walk or -gates, silently ignore unknown flags,
#                       and launch the full GUI instead of exiting.
set -euo pipefail

cd "$(dirname "$0")/.."

AD_HOC_OK=0
EXPECT_VERSION="$(tr -d '[:space:]' < VERSION)"
DMGS=()
while [ $# -gt 0 ]; do
    case "$1" in
        --ad-hoc-ok) AD_HOC_OK=1 ;;
        --expect-version) shift; EXPECT_VERSION="${1:?--expect-version needs a value}" ;;
        -*) echo "unknown flag: $1" >&2; exit 2 ;;
        *) DMGS+=("$1") ;;
    esac
    shift
done
if [ ${#DMGS[@]} -eq 0 ]; then
    echo "usage: tools/smoke-macos.sh [--ad-hoc-ok] [--expect-version X.Y.Z] <dmg>..." >&2
    exit 2
fi

LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

unregister_app() {
    [ -x "$LSREGISTER" ] || return 0
    "$LSREGISTER" -u "$1" >/dev/null 2>&1 || true
}

# Runs in a subshell so the EXIT trap cleans up one artifact's mount and temp
# dir even when a gate fails halfway through.
smoke_one() (
    set -e
    dmg="$1"
    tmp="$(mktemp -d)"
    mnt="$tmp/mnt"
    mkdir -p "$mnt"
    app=""
    mounted_app=""

    cleanup() {
        # Mounting the DMG registers the bundle with LaunchServices and
        # re-points the bundle ID at the /Volumes copy (measured 2026-09-01),
        # so this unregistration is load-bearing, not tidiness.
        [ -n "$app" ] && unregister_app "$app"
        [ -n "$mounted_app" ] && unregister_app "$mounted_app"
        hdiutil detach "$mnt" -quiet >/dev/null 2>&1 || true
        rm -rf "$tmp"
    }
    trap cleanup EXIT

    echo "==> $dmg"
    echo "==> mounting"
    hdiutil attach -nobrowse -readonly -mountpoint "$mnt" "$dmg" >/dev/null

    # The bundle was renamed between releases — "Dr. Markdown.app" shipped at
    # 1.6.2, "Dr Markdown.app" ships now — and retained DMGs must still smoke,
    # so the bundle and executable names come from the DMG's own Info.plist
    # rather than from the current names hard-coded here.
    for candidate in "$mnt"/*.app; do
        [ -d "$candidate" ] || continue
        mounted_app="$candidate"
        break
    done
    if [ -z "$mounted_app" ]; then
        echo "error: no .app bundle found in $dmg" >&2
        exit 1
    fi

    echo "==> copying the app out"
    cp -R "$mounted_app" "$tmp/"
    app="$tmp/$(basename "$mounted_app")"
    bin="$app/Contents/MacOS/$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$app/Contents/Info.plist")"

    echo "==> version"
    got="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$app/Contents/Info.plist")"
    if [ "$got" != "$EXPECT_VERSION" ]; then
        echo "error: bundle version is $got, expected $EXPECT_VERSION" >&2
        exit 1
    fi
    echo "    $got"

    echo "==> gates against the packaged binary"
    for gate in -menu -close -quit -nav -walk -gates; do
        echo "    $gate"
        "$bin" "$gate"
    done

    echo "==> signature"
    # Not "codesign ... && echo ok": the left side of && is exempt from
    # set -e, so a failed verify would silently pass. Keep them separate.
    codesign --verify --deep --strict "$app"
    echo "    signature ok"

    if [ "$AD_HOC_OK" -eq 0 ]; then
        echo "==> notarization and Gatekeeper"
        xcrun stapler validate "$app"
        spctl -a -vvv -t exec "$app"
        spctl -a -vvv -t open --context context:primary-signature "$dmg"
    fi
)

for dmg in "${DMGS[@]}"; do
    smoke_one "$dmg"
done
echo ""
echo "smoke ok: ${#DMGS[@]} artifact(s)"
