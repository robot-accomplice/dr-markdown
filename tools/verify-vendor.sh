#!/usr/bin/env bash
# Verifies the committed vendored bundles against their recorded digests.
#
# tools/vendor.sh fetches roughly 6 MB of third-party JavaScript over the
# network and, until now, recorded no integrity information at all: NOTICE.md
# carried package names and versions but no hashes, and the Crepe bundle is
# patched in place after download, so the committed artifact matches no upstream
# artifact and its provenance could not be re-established later.
#
# This makes a substituted, corrupted or accidentally-half-refreshed bundle
# visible. It is deliberately a verifier, not a fetcher — it never touches the
# network, so it is safe to run in CI and it checks the bytes that are actually
# committed rather than whatever a fresh download happens to return today.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENDOR="$ROOT/frontend/dist/vendor"
DIGESTS="$ROOT/tools/vendor-digests.txt"

if [ ! -f "$DIGESTS" ]; then
  echo "error: $DIGESTS is missing; run tools/vendor.sh to regenerate it" >&2
  exit 1
fi

# Refuse an empty digest file rather than reporting success over zero checks.
if ! grep -qE '[0-9a-f]{64}' "$DIGESTS"; then
  echo "error: $DIGESTS records no digests; refusing to report a pass over nothing" >&2
  exit 1
fi

cd "$VENDOR"
if command -v shasum >/dev/null 2>&1; then
  shasum -a 256 -c "$DIGESTS"
else
  sha256sum -c "$DIGESTS"
fi

# A digest file that covers only some of the tree would pass the check above
# while leaving the rest unverified, so compare the counts too.
recorded=$(grep -cE '^[0-9a-f]{64}' "$DIGESTS")
present=$(find . -type f ! -name manifest.txt ! -name NOTICE.md | wc -l | tr -d ' ')
if [ "$recorded" -ne "$present" ]; then
  echo "error: $recorded digests recorded but $present vendored files present." >&2
  echo "       A file was added or removed without refreshing the digests." >&2
  exit 1
fi

echo "vendored bundles verified: $recorded files"
