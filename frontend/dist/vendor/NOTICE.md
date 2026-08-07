# Vendored Third-Party Assets

Fetched by `tools/vendor.sh`. Do not hand-edit.

| Asset | Package | Version | License | Source |
|---|---|---|---|---|
| `crepe.bundle.mjs` + `theme/` | `@milkdown/crepe` | 7.22.0 | MIT | https://github.com/Milkdown/milkdown |
| `codemirror.bundle.mjs` | `codemirror` | 6.0.2 | MIT | https://github.com/codemirror/basic-setup |
| `highlight.min.js` | `highlight.js` | 11.11.1 | BSD-3-Clause | https://github.com/highlightjs/highlight.js |
| `mermaid.min.js` | `mermaid` | 11.6.0 | MIT | https://github.com/mermaid-js/mermaid |

Milkdown bundles ProseMirror (MIT, https://prosemirror.net) and CodeMirror.
CodeMirror bundles @codemirror/* and @lezer/* packages (MIT,
https://github.com/codemirror).
Highlight.js provides the common browser language build used for markdown
source overlays and fenced code block highlighting.
Mermaid renders local fenced `mermaid` diagrams and assistant previews.
