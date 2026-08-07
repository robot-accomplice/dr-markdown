import { highlightMarkdownSource } from './highlighter.js'

export class RawEditor {
  #textarea = null
  #highlight = null
  #gutter = null
  #onChange = null
  #options = {}

  open(host, markdown, onChange, options = {}) {
    host.replaceChildren()
    this.#onChange = onChange
    this.#options = options

    const editor = document.createElement('div')
    editor.className = 'source-editor cm-editor'

    this.#gutter = document.createElement('div')
    this.#gutter.className = 'cm-gutters'

    const stack = document.createElement('div')
    stack.className = 'source-stack'

    this.#highlight = document.createElement('pre')
    this.#highlight.className = 'source-highlight'
    this.#highlight.setAttribute('aria-hidden', 'true')

    this.#textarea = document.createElement('textarea')
    this.#textarea.className = 'source-input cm-content'
    this.#textarea.spellcheck = false
    this.#textarea.value = markdown
    this.#textarea.wrap = options.softWrap === false ? 'off' : 'soft'

    stack.append(this.#highlight, this.#textarea)
    editor.append(this.#gutter, stack)
    host.append(editor)

    this.#textarea.addEventListener('input', () => {
      this.#render()
      this.#onChange?.(this.getMarkdown())
    })
    this.#textarea.addEventListener('scroll', () => this.#syncScroll())
    this.#render()
  }

  getMarkdown() {
    return this.#textarea?.value ?? ''
  }

  replaceAll(text) {
    if (!this.#textarea) return
    this.#textarea.value = text
    this.#render()
    this.#onChange?.(text)
  }

  close() {
    this.#textarea = null
    this.#highlight = null
    this.#gutter = null
    this.#onChange = null
  }

  #render() {
    const markdown = this.getMarkdown()
    this.#highlight.innerHTML = `${highlightMarkdownSource(markdown, this.#options)}\n`
    const lineCount = Math.max(1, markdown.split('\n').length)
    this.#gutter.replaceChildren(...Array.from({ length: lineCount }, (_, index) => {
      const line = document.createElement('div')
      line.className = 'cm-lineNumber'
      line.textContent = String(index + 1)
      return line
    }))
    this.#syncScroll()
  }

  #syncScroll() {
    if (!this.#textarea || !this.#highlight || !this.#gutter) return
    this.#highlight.scrollTop = this.#textarea.scrollTop
    this.#highlight.scrollLeft = this.#textarea.scrollLeft
    this.#gutter.scrollTop = this.#textarea.scrollTop
  }
}
