// RawEditor wraps the vendored CodeMirror bundle (basic editing; markdown
// syntax highlighting is deferred — see plan Global Constraints).
// The bundle exports no EditorState, so the view builds its state from
// doc + extensions directly.
import { EditorView, basicSetup } from '../vendor/codemirror.bundle.mjs'

export class RawEditor {
  #view = null

  open(host, markdown) {
    host.replaceChildren()
    this.#view = new EditorView({
      parent: host,
      doc: markdown,
      extensions: [basicSetup, EditorView.lineWrapping],
    })
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
