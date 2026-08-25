#!/usr/bin/env bash
# Fetches pinned third-party frontend assets into frontend/dist/vendor.
# No Node.js anywhere: we download pre-built, self-contained browser bundles
# over HTTPS. Re-running is idempotent (files are overwritten).
set -euo pipefail

CREPE_VERSION="7.22.0"
HIGHLIGHT_VERSION="11.11.1"
MERMAID_VERSION="11.6.0"

# The stylesheets Crepe's own theme @imports but does not ship. Pinned
# separately because @milkdown/kit re-exports them as bundler subpaths
# (@milkdown/kit/prose/view/style/prosemirror.css and friends) rather than
# shipping the files, so they can only be fetched from upstream.
PM_VIEW_VERSION="1.42.3"
PM_GAPCURSOR_VERSION="1.4.1"
PM_TABLES_VERSION="1.8.5"
PM_VIRTUAL_CURSOR_VERSION="0.4.2"

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
# loader.
#
# What is lost, measured in the built app rather than read off basicSetup's
# feature list: line numbers and the fold gutter, bracket auto-closing, and
# autocompletion. Undo is NOT lost — the node view forwards CodeMirror updates
# into ProseMirror transactions, so the document's own history answers Cmd-Z
# inside a code block. Multi-line editing, highlighting and the default keymap
# all work.
#
# Supplying a replacement does NOT work and must not be attempted again, both
# measured rather than assumed:
#
#   - Passing `extensions` through featureConfigs does not displace the broken
#     default. Crepe CONCATENATES user extensions onto its own array, so the
#     undefined entry survives.
#   - Vendoring the `codemirror` meta-package separately and importing it puts a
#     SECOND copy of @codemirror/state on the page, and CodeMirror rejects the
#     result with "Unrecognized extension value in extension set". That bundle
#     was fetched here for years, imported by nothing, and is now deleted.
#
# Nor can the fetch be fixed from here. @milkdown/crepe declares
# `codemirror: ^6.0.1`, so this is an esm.sh resolution bug; ?deps= and ?alias=
# are ignored because /es2022/crepe.bundle.mjs is a prebuilt artifact, and
# jsdelivr's +esm build resolves correctly but emits 46 external imports, which
# no self-contained, CSP-'self' asset server can load.
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

# The vendored CodeMirror ships the One Dark theme, and its colour constants are
# baked into the bundle as hex literals. app.css suppresses that theme's dark
# BACKGROUND so a code block sits on the app's own surface — which left One
# Dark's foreground alone, drawing purple keywords and grey text on white, while
# the same code two panes away was teal. Two highlighters, no colours in common.
#
# So the constants are rewritten to reference this app's syntax tokens. They are
# emitted straight into the stylesheet CodeMirror injects, so a var() resolves
# there exactly as it would anywhere else, and it also means the formatted
# editor follows the theme instead of ignoring it.
#
# Backgrounds go to `transparent` rather than to a token: the block already
# supplies its own surface, and painting another one over it is what made the
# code area read as a different colour from its own card.
patch_colour() {
    local const="$1" replacement="$2" label="$3"
    if ! grep -q "$const" "$VENDOR/crepe.bundle.mjs"; then
        echo "error: crepe.bundle.mjs no longer contains $const ($label)." >&2
        echo "       The One Dark palette moved, so the formatted editor would go" >&2
        echo "       back to drawing a dark theme's colours on a light surface." >&2
        exit 1
    fi
    sed -i.bak "s|$const|$replacement|g" "$VENDOR/crepe.bundle.mjs" && rm "$VENDOR/crepe.bundle.mjs.bak"
}

patch_colour '"#abb2bf"' '"var(--code-ink)"'      "foreground"
patch_colour '"#c678dd"' '"var(--code-keyword)"'  "keyword"
patch_colour '"#98c379"' '"var(--code-string)"'   "string"
patch_colour '"#61afef"' '"var(--code-variable)"' "function name"
patch_colour '"#e06c75"' '"var(--code-variable)"' "variable"
patch_colour '"#d19a66"' '"var(--code-number)"'   "number"
patch_colour '"#56b6c2"' '"var(--code-keyword)"'  "operator"
patch_colour '"#7d8799"' '"var(--code-comment)"'  "comment"
patch_colour '"#282c34"' '"transparent"'          "editor background"
patch_colour '"#21252b"' '"transparent"'          "gutter background"
patch_colour '"#528bff"' '"var(--accent)"'        "cursor"
echo "remapped the vendored One Dark palette to the app's syntax tokens"

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

# Crepe's theme CSS @imports four stylesheets from OTHER npm packages, by bare
# specifier. A browser resolves a bare specifier in CSS relative to the
# stylesheet's own URL, so each became a request under vendor/theme/common/ for
# a path that does not exist, and 404ed:
#
#   common/prosemirror.css  -> @milkdown/kit/prose/view/style/prosemirror.css
#   common/cursor.css       -> @milkdown/kit/prose/gapcursor/style/gapcursor.css
#                              prosemirror-virtual-cursor/style/virtual-cursor.css
#   common/table.css        -> @milkdown/kit/prose/tables/style/tables.css
#
# So ProseMirror's own base editor stylesheet, the gap cursor, the virtual
# cursor and the table styles never loaded at all. A failed @import is silent —
# the importing sheet still applies, nothing throws, and no test asserts on a
# stylesheet that is missing — which is why this shipped. It was found by the
# host harness logging ASSET MISS while driving the real app.
#
# The files are fetched from upstream rather than from @milkdown/kit, because
# kit re-exports those paths as bundler subpaths and does not ship them: every
# @milkdown/kit/prose/... URL above returns 404 from the registry.
fetch "https://cdn.jsdelivr.net/npm/prosemirror-view@${PM_VIEW_VERSION}/style/prosemirror.css" \
  "$VENDOR/theme/common/pm-view.css"
fetch "https://cdn.jsdelivr.net/npm/prosemirror-gapcursor@${PM_GAPCURSOR_VERSION}/style/gapcursor.css" \
  "$VENDOR/theme/common/pm-gapcursor.css"
fetch "https://cdn.jsdelivr.net/npm/prosemirror-tables@${PM_TABLES_VERSION}/style/tables.css" \
  "$VENDOR/theme/common/pm-tables.css"
fetch "https://cdn.jsdelivr.net/npm/prosemirror-virtual-cursor@${PM_VIRTUAL_CURSOR_VERSION}/style/virtual-cursor.css" \
  "$VENDOR/theme/common/pm-virtual-cursor.css"

# Point the vendored sheets at the local copies. katex is REMOVED rather than
# satisfied: Crepe.Feature.Latex is disabled in editor.js, so fetching a 300KB
# stylesheet for a feature that is off would be worse than the 404.
rewrite_import() {
    local file="$1" spec="$2" local_name="$3"
    if ! grep -q "$spec" "$VENDOR/theme/$file"; then
        echo "error: $file no longer imports $spec." >&2
        echo "       Either the vendored theme changed shape or this rewrite already ran." >&2
        echo "       An unrewritten bare specifier 404s silently and the editor loses that" >&2
        echo "       stylesheet with nothing reporting it." >&2
        exit 1
    fi
    sed -i.bak "s|@import '$spec';|@import './$local_name';|" "$VENDOR/theme/$file" && rm "$VENDOR/theme/$file.bak"
}

rewrite_import common/prosemirror.css "@milkdown/kit/prose/view/style/prosemirror.css"     pm-view.css
rewrite_import common/cursor.css      "@milkdown/kit/prose/gapcursor/style/gapcursor.css"  pm-gapcursor.css
rewrite_import common/cursor.css      "prosemirror-virtual-cursor/style/virtual-cursor.css" pm-virtual-cursor.css
rewrite_import common/table.css       "@milkdown/kit/prose/tables/style/tables.css"        pm-tables.css

if grep -q "katex/dist/katex.min.css" "$VENDOR/theme/common/latex.css"; then
    sed -i.bak "/katex\/dist\/katex.min.css/d" "$VENDOR/theme/common/latex.css" && rm "$VENDOR/theme/common/latex.css.bak"
    echo "dropped the katex import (Latex feature is disabled)"
fi
echo "rewrote the theme's cross-package @imports to local copies"

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
