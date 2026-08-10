#!/usr/bin/env bash
# Fetches pinned third-party frontend assets into frontend/dist/vendor.
# No Node.js anywhere: we download pre-built, self-contained browser bundles
# over HTTPS. Re-running is idempotent (files are overwritten).
set -euo pipefail

CREPE_VERSION="7.22.0"
CODEMIRROR_VERSION="6.0.2"
HIGHLIGHT_VERSION="11.11.1"
MERMAID_VERSION="11.6.0"

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
elif grep -q '/node/process.mjs' "$VENDOR/crepe.bundle.mjs"; then
  # The shim is still there but no longer matches the pattern above. Skipping
  # the patch silently is not an option: the unpatched import 404s under the
  # Wails asset server and aborts module loading, so the editor never mounts at
  # all — a total failure produced by a refresh that printed nothing.
  echo "error: crepe.bundle.mjs still references /node/process.mjs but the expected" >&2
  echo "       import statement did not match, so the patch was not applied." >&2
  echo "       esm.sh changed the shim's shape. Update the pattern above before" >&2
  echo "       committing this bundle, or the editor will fail to load." >&2
  exit 1
fi

# esm.sh resolves the bare specifier `codemirror` inside the Crepe bundle to
# CodeMirror *5* — a namespace of defineMode/defineMIME/registerHelper, which
# has no `basicSetup` export because that is a v6 addition. Crepe's code-mirror
# feature builds its extension set as `[keymap, fme.basicSetup, ...]`, so the
# second entry is undefined, and CodeMirror's EditorState.create throws
# "Cannot read properties of undefined (reading 'extension')".
#
# That throw happens inside the IntersectionObserver callback that upgrades a
# code block, and only AFTER the node view has set `initialized = true`. So it
# is never retried: every code block stays on its placeholder <pre>, inside a
# `contenteditable="false"` wrapper, for the life of the document. Code blocks
# were uneditable in Formatted mode from the day highlighting landed until this
# patch (#77), which violates the rule the whole editor exists to serve.
#
# Dropping the undefined entry is the whole fix. Everything Crepe supplies
# separately survives: the default keymap is a different entry, the highlight
# style is a different entry, and languages load through the node view's own
# loader. What is genuinely lost is what basicSetup alone carries — in-block
# undo history, line numbers, autocompletion and bracket matching.
#
# Wiring the codemirror.bundle.mjs fetched below does NOT work and must not be
# attempted again: it puts a second copy of @codemirror/state on the page, and
# CodeMirror rejects the result with "Unrecognized extension value in extension
# set". Measured, not assumed.
if grep -q 'fme\.basicSetup' "$VENDOR/crepe.bundle.mjs"; then
  sed -i.bak 's|fme\.basicSetup|[]|' "$VENDOR/crepe.bundle.mjs" && rm "$VENDOR/crepe.bundle.mjs.bak"
  echo "patched out the undefined basicSetup extension in crepe.bundle.mjs"
else
  # Silence here would ship uneditable code blocks again, and the suite would
  # stay green until someone tried to type. Fail instead: either upstream fixed
  # the resolution (drop this patch) or the expression was minified to a new
  # name (update the pattern).
  echo "error: crepe.bundle.mjs no longer contains 'fme.basicSetup', so the" >&2
  echo "       CodeMirror patch was not applied. Either esm.sh now resolves" >&2
  echo "       codemirror to v6 and this patch is obsolete, or minification" >&2
  echo "       renamed the binding. Check which, before committing this bundle:" >&2
  echo "       an unpatched bundle makes every code block uneditable (#77)." >&2
  exit 1
fi

# CodeMirror 6 meta-package: basic editing setup (~377 KB). No language packs
# (see Global Constraints re: highlighting).
#
# NOTE: nothing imports this today, and the patch above records why it cannot be
# used to repair the Crepe bundle. It is kept pending a decision on removal.
fetch "https://esm.sh/codemirror@${CODEMIRROR_VERSION}/es2022/codemirror.bundle.mjs" \
  "$VENDOR/codemirror.bundle.mjs"

# Highlight.js common browser build: syntax highlighting for markdown source
# overlays and language-tagged fenced code blocks.
fetch "https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@${HIGHLIGHT_VERSION}/build/highlight.min.js" \
  "$VENDOR/highlight.min.js"

# Mermaid browser build for rendered diagram blocks and assistant previews.
fetch "https://cdn.jsdelivr.net/npm/mermaid@${MERMAID_VERSION}/dist/mermaid.min.js" \
  "$VENDOR/mermaid.min.js"

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

# Record what we actually committed. The Crepe bundle is patched in place after
# download, so it matches no upstream artifact and a digest taken here is the
# only durable record of the bytes this repository ships. tools/verify-vendor.sh
# checks them without touching the network, and CI runs it on every push.
DIGESTS="$ROOT/tools/vendor-digests.txt"
( cd "$VENDOR" && find . -type f ! -name manifest.txt ! -name NOTICE.md | sort |
  xargs shasum -a 256 > "$DIGESTS" )
echo "recorded $(grep -c . "$DIGESTS") digests in tools/vendor-digests.txt"

echo "Done:"
du -sh "$VENDOR"
