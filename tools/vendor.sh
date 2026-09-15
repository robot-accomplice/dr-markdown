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

# markdown-it: markdown -> HTML for the print surface and the split preview.
#
# Those two surfaces used to share a 43-line hand-written renderer that matched
# headings and fences with regular expressions and handled almost nothing else.
# Measured across sixteen ordinary constructs, SIXTEEN were wrong: a GFM table
# came out as three paragraphs of pipe characters, an ordered list lost its
# numbers and became bullets, nested lists flattened, task lists showed literal
# brackets, inline emphasis inside a heading or a quote stayed as asterisks, a
# wrapped paragraph split into two, and footnotes, reference links, setext
# headings, indented code and two of the three thematic-break spellings were
# emitted verbatim.
#
# That renderer also fed PRINT and PDF EXPORT, which is the artifact that leaves
# the application and cannot be corrected afterwards.
MARKDOWN_IT_VERSION="14.1.0"
fetch "https://esm.sh/markdown-it@${MARKDOWN_IT_VERSION}/es2022/markdown-it.bundle.mjs" \
  "$VENDOR/markdown-it.bundle.mjs"

# Two plugins, because this application's dialect is CommonMark PLUS GFM and
# markdown-it's core is CommonMark alone. Without them the renderer disagrees
# with the editor on exactly two constructs, and both are in the dialect:
#
#   task list   core emits <li>[ ] todo</li>; the editor draws a checkbox
#   footnote    core reads [^1] as a shortcut reference LINK, which is correct
#               CommonMark and wrong here — the editor renders a footnote, and
#               the fidelity work already treats footnote definitions as a
#               construct this app preserves
fetch "https://esm.sh/markdown-it-footnote@4.0.0/es2022/markdown-it-footnote.bundle.mjs" \
  "$VENDOR/markdown-it-footnote.bundle.mjs"
fetch "https://esm.sh/markdown-it-task-lists@2.1.1/es2022/markdown-it-task-lists.bundle.mjs" \
  "$VENDOR/markdown-it-task-lists.bundle.mjs"

# Milkdown Crepe editor: one self-contained ESM bundle (~2.8 MB).
fetch "https://esm.sh/@milkdown/crepe@${CREPE_VERSION}/es2022/crepe.bundle.mjs" \
  "$VENDOR/crepe.bundle.mjs"

# esm.sh injects `import __Process$ from "/node/process.mjs"` (its Node
# polyfill shim) into the bundle. That root-relative URL 404s under the
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
  # app's asset scheme and aborts module loading, so the editor never mounts at
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

# Export CodeMirror's `indentUnit` facet, so how far Tab indents is decided in
# app code instead of here.
#
# Reported from real use: Tab inserted four spaces in raw mode and two in the
# formatted view's code blocks — the same keystroke on the same document giving
# different text. Raw mode used the app's own constant; CodeMirror used the
# facet's default of two spaces, and nothing connected them.
#
# The bundle exports only Crepe's own surface, and the feature accepts
# `extensions`, so exporting the facet is enough: frontend/dist/src/indent.js
# holds the width and editor.js configures it. Rewriting the default value in
# place would work too, and is worse — a product decision would then live in a
# shell script that rewrites a third-party artifact.
#
# The minified name is DISCOVERED from the facet's own definition rather than
# written down. `Of` today is a name the minifier chose and will choose
# differently on the next refresh; the definition is the stable thing.
indent_facet=$(grep -o '[A-Za-z_$][A-Za-z0-9_$]*=[A-Za-z_$][A-Za-z0-9_$]*\.define({combine:t=>{if(!t\.length)return"  "' \
    "$VENDOR/crepe.bundle.mjs" | head -1 | cut -d= -f1)
if [ -z "$indent_facet" ]; then
    echo "error: could not find CodeMirror's indentUnit facet in crepe.bundle.mjs." >&2
    echo "       It is identified by its own definition — a facet whose combine" >&2
    echo "       returns two spaces when unset. Without the export, editor.js" >&2
    echo "       cannot set the indent width and Tab would silently go back to" >&2
    echo "       indenting by two in the formatted view and four in raw." >&2
    exit 1
fi
if ! grep -q "export{Cq as Crepe" "$VENDOR/crepe.bundle.mjs"; then
    echo "error: crepe.bundle.mjs no longer ends with the expected export list." >&2
    exit 1
fi
sed -i.bak "s|export{Cq as Crepe|export{$indent_facet as indentUnit,Cq as Crepe|" \
    "$VENDOR/crepe.bundle.mjs" && rm "$VENDOR/crepe.bundle.mjs.bak"
echo "exported CodeMirror's indentUnit facet (minified as $indent_facet)"

# Normalize the language the block picker writes into a fence.
#
# The node view's setLanguage puts the picked value straight into the node's
# `language` attribute, and the picker supplies CodeMirror's DISPLAY name — so
# choosing Python from the block's own picker writes ```Python, where every
# other route in this application writes ```python (GitHub #78).
#
# Markdown treats info strings case-insensitively, so nothing renders
# differently. It matters here because this project holds a byte-identical
# round-trip corpus and a 49-construct fidelity survey, and a file that gains a
# capitalised fence purely because of which control the user reached for is an
# inconsistency the corpus would have to encode.
#
# Patched at the PICKER, deliberately, and not on serialize. Normalizing on
# serialize would rewrite fences the USER authored capitalised, which the
# fidelity survey would correctly fail — a document must come back as it went
# in. Only a language the picker just chose is normalized.
#
# What "normalized" means stays in app code: the hook calls
# globalThis.drmd.normalizeLanguage, which is highlighter.js's own function, and
# falls back to the raw value if it is absent so the bundle degrades rather than
# breaks.
setlang_anchor='setLanguage=s=>{var l;this.view.dispatch(this.view.state.tr.setNodeAttribute((l=this.getPos())!=null?l:0,"language",s))}'
if ! grep -qF "$setlang_anchor" "$VENDOR/crepe.bundle.mjs"; then
    echo "error: the code block node view's setLanguage is not where it was." >&2
    echo "       Without this patch the block's language picker writes CodeMirror's" >&2
    echo "       display name into the fence — \`\`\`Python rather than \`\`\`python —" >&2
    echo "       disagreeing with every other route in the app (#78)." >&2
    exit 1
fi
setlang_patched='setLanguage=s=>{var l;this.view.dispatch(this.view.state.tr.setNodeAttribute((l=this.getPos())!=null?l:0,"language",(globalThis.drmd&&globalThis.drmd.normalizeLanguage?globalThis.drmd.normalizeLanguage(s):s)))}'
python3 - "$VENDOR/crepe.bundle.mjs" "$setlang_anchor" "$setlang_patched" <<'PYEOF'
import sys, io
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
s = io.open(path, encoding='utf-8').read()
io.open(path, 'w', encoding='utf-8').write(s.replace(old, new, 1))
PYEOF
echo "routed the block language picker through the app's normalizer"

# Measure the image block in LAYOUT pixels, not in zoomed ones.
#
# The image node view sizes an image on load by measuring its block with
# getBoundingClientRect().width and writing an explicit pixel height back onto
# the <img>. Under the document zoom this application applies — CSS `zoom` on
# #editor-host, chosen because it participates in layout and so stays crisp —
# getBoundingClientRect returns the ZOOMED width while the style it computes is
# applied inside that same zoomed context. The zoom is therefore counted twice.
#
# Measured at 90% on a 1600x420 image, after a mode round trip rebuilt the
# editor:
#
#   correct    container 598 -> height 157 -> width 597.78 -> renders 538
#   observed   container 538 -> height 141 -> width 537.95 -> renders 484
#
# 538 = 598 x 0.9, and 484 = 538 x 0.9. The image came back 54px short of the
# pane it should fill and stayed short (GitHub #131).
#
# clientWidth is the layout width and is unaffected by zoom, which is the
# coordinate space the computed style is applied in. It is also an integer,
# where the rect is fractional; that costs sub-pixel precision on a value that
# is immediately rounded to two decimals anyway.
zoom_anchor='let E=R.getBoundingClientRect().width;if(!E)return;'
if ! grep -qF "$zoom_anchor" "$VENDOR/crepe.bundle.mjs"; then
    echo "error: the image block no longer measures its container the same way." >&2
    echo "       Without this patch an image is sized from a zoomed measurement" >&2
    echo "       and the document zoom is applied twice, so an image comes back" >&2
    echo "       short of its pane after a mode change (#131)." >&2
    exit 1
fi
python3 - "$VENDOR/crepe.bundle.mjs" "$zoom_anchor" 'let E=R.clientWidth;if(!E)return;' <<'PYEOF'
import sys, io
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
s = io.open(path, encoding='utf-8').read()
io.open(path, 'w', encoding='utf-8').write(s.replace(old, new, 1))
PYEOF
echo "sized the image block from layout pixels, so document zoom is not counted twice"

# Measure the resize DRAG in layout pixels too.
#
# The same zoom, the same mistake, one handler over: the image block's resize
# handle measures the drag in VIEWPORT pixels — pointer clientY minus the
# image's getBoundingClientRect().top, both zoom-scaled — and writes the result
# as a style height applied INSIDE the zoomed context, which scales it again.
# Measured on the real host at 120% with DRMD_PROBE_IMG=1: a 5px drag set the
# height from 149.96px to 184.93px, and 184.93 = 149.96 x 1.2 + 5. The element
# box stays pane-wide (max-width), so object-fit: cover scales the image up to
# the too-tall box and crops the sides — which is how a banner loses its mark
# after an accidental 4px-handle drag. The handle is invisible until hover and
# parked on the image's bottom edge, so the drag needs no intent.
#
# The divisor reads the live zoom from the image itself — rect height over
# offsetHeight — so it is correct in both engine conventions rather than
# matched to one. Gate: e2e/image_resize_zoom_test.go, verified failing before
# this patch (an 8px drag at 120% moved the rendered height by 47.2px).
drag_anchor='let R=Z.getBoundingClientRect().top,E=T.clientY-R;'
if ! grep -qF "$drag_anchor" "$VENDOR/crepe.bundle.mjs"; then
    echo "error: the image resize handle no longer measures the drag the same way." >&2
    echo "       Without this patch the drag delta is measured in zoomed pixels" >&2
    echo "       and applied inside the zoomed context, so a resize under document" >&2
    echo "       zoom overshoots by the zoom factor and object-fit crops the sides." >&2
    exit 1
fi
python3 - "$VENDOR/crepe.bundle.mjs" "$drag_anchor" 'let R=Z.getBoundingClientRect().top,E=(T.clientY-R)/(Z.offsetHeight?Z.getBoundingClientRect().height/Z.offsetHeight:1);' <<'PYEOF'
import sys, io
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
s = io.open(path, encoding='utf-8').read()
io.open(path, 'w', encoding='utf-8').write(s.replace(old, new, 1))
PYEOF
echo "measured the image resize drag in layout pixels, so zoom is not counted twice there either"

# Keep the image block's PROPORTIONS instead of a pixel height.
#
# The load handler sizes the image once: measure the block, compute the height
# that preserves the natural aspect at that width, write it as an explicit
# pixel style.height. The width then keeps tracking the container
# (max-width: 100%) while the height tracks NOTHING, so any post-load width
# change — and this app's document zoom is CSS `zoom`, which participates in
# layout, so every zoom step re-lays out a percentage-width pane — breaks the
# box's aspect, and object-fit: cover converts the mismatch into cropping.
# Measured on the real host with DRMD_PROBE_IMG=1: in split mode at 130% the
# block went 506px -> 368px while style.height stayed at the 506-wide value,
# box aspect 2.77 against a natural 3.81, and the banner's mark was cut in
# half. Mode switches only SEEMED to heal it: they re-render, and the load
# handler runs again against the new width — until the timing loses.
#
# aspect-ratio makes the box follow the width declaratively, at any width, so
# there is no stale pixel height to go wrong. dataset.height is still written:
# the resize drag's pointerup reads it for the ratio attribute, and the drag
# still writes its own explicit height (the handle is display:none in the app;
# e2e/image_resize_zoom_test.go force-shows it to gate the arithmetic).
size_anchor='Z.dataset.origin=z.toFixed(2),Z.dataset.height=I,Z.style.height=`${I}px`'
if ! grep -qF "$size_anchor" "$VENDOR/crepe.bundle.mjs"; then
    echo "error: the image block no longer writes its load-time size the same way." >&2
    echo "       Without this patch the explicit pixel height goes stale on any" >&2
    echo "       post-load width change (document zoom re-lays out a split pane)," >&2
    echo "       and object-fit: cover crops the image's sides." >&2
    exit 1
fi
python3 - "$VENDOR/crepe.bundle.mjs" "$size_anchor" 'Z.dataset.origin=z.toFixed(2),Z.dataset.height=I,Z.style.aspectRatio=W+"/"+q,Z.style.height=""' <<'PYEOF'
import sys, io
path, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
s = io.open(path, encoding='utf-8').read()
io.open(path, 'w', encoding='utf-8').write(s.replace(old, new, 1))
PYEOF
echo "kept the image block's aspect ratio instead of a pixel height, so width changes cannot crop it"

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
