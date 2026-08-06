#!/usr/bin/env bash
# Fetches pinned third-party frontend assets into frontend/dist/vendor.
# No Node.js anywhere: we download pre-built, self-contained browser bundles
# over HTTPS. Re-running is idempotent (files are overwritten).
set -euo pipefail

CREPE_VERSION="7.22.0"
CODEMIRROR_VERSION="6.0.2"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENDOR="$ROOT/frontend/dist/vendor"
mkdir -p "$VENDOR/theme"

fetch() {
  echo "fetch $1"
  curl -fsSL "$1" -o "$2"
}

# Milkdown Crepe editor: one self-contained ESM bundle (~2.8 MB).
fetch "https://esm.sh/@milkdown/crepe@${CREPE_VERSION}/es2022/crepe.bundle.mjs" \
  "$VENDOR/crepe.bundle.mjs"

# esm.sh injects `import __Process$ from "/node/process.mjs"` (its Node
# polyfill shim) into the bundle. That root-relative URL 404s under the Wails
# asset server and would abort module loading entirely. The bundle's only use
# is a guarded `typeof __Process$<"u"` check (lezer parse logging), so replace
# the import with an inline undefined stub to keep the bundle self-contained.
if grep -q 'import __Process\$ from "/node/process.mjs";' "$VENDOR/crepe.bundle.mjs"; then
  # Portable in-place edit (GNU/BSD sed safe): -i.bak works on both.
  sed -i.bak 's|import __Process\$ from "/node/process.mjs";|const __Process$=void 0;|' \
    "$VENDOR/crepe.bundle.mjs" && rm "$VENDOR/crepe.bundle.mjs.bak"
  echo "patched out /node/process.mjs import in crepe.bundle.mjs"
fi

# CodeMirror 6 meta-package: basic editing setup (~377 KB). No language packs
# (see Global Constraints re: highlighting).
fetch "https://esm.sh/codemirror@${CODEMIRROR_VERSION}/es2022/codemirror.bundle.mjs" \
  "$VENDOR/codemirror.bundle.mjs"

# Crepe theme CSS: enumerate the published package, pull every theme file.
LIST_URL="https://data.jsdelivr.com/v1/packages/npm/@milkdown/crepe@${CREPE_VERSION}?structure=flat"
BASE_URL="https://cdn.jsdelivr.net/npm/@milkdown/crepe@${CREPE_VERSION}"
curl -fsSL "$LIST_URL" |
  grep -o '"/lib/theme/[^"]*\.css"' |
  tr -d '"' |
  while read -r path; do
    rel="${path#/lib/theme/}"
    mkdir -p "$VENDOR/theme/$(dirname "$rel")"
    fetch "$BASE_URL$path" "$VENDOR/theme/$rel"
  done

# Manifest of theme CSS in load order (common/ sorts first alphabetically).
( cd "$VENDOR/theme" && find . -name '*.css' | sed 's|^\./||' | sort > manifest.txt )

echo "Done:"
du -sh "$VENDOR"
