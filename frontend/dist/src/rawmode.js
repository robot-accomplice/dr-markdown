// RawEditor wraps the vendored CodeMirror bundle (basic editing; markdown
// syntax highlighting is deferred — see plan Global Constraints).
// The bundle exports no EditorState, so the view builds its state from
// doc + extensions directly.
import { EditorView, basicSetup } from '../vendor/codemirror.bundle.mjs'

export class RawEditor {
  #view = null

  // onChange(markdown), when given, fires on every document change so the
  // caller can wire raw-mode edits into dirty tracking.
  open(host, markdown, onChange) {
    host.replaceChildren()
    const extensions = [basicSetup, EditorView.lineWrapping]
    if (onChange) {
      extensions.push(
        EditorView.updateListener.of((u) => {
          if (u.docChanged) onChange(u.state.doc.toString())
        })
      )
    }
    this.#view = new EditorView({ parent: host, doc: markdown, extensions })
  }

  getMarkdown() {
    return this.#view.state.doc.toString()
  }

  replaceAll(text) {
    this.#view.dispatch({
      changes: { from: 0, to: this.#view.state.doc.length, insert: text },
    })
  }

  close() {
    this.#view?.destroy()
    this.#view = null
  }
}
