/**
 * Syntax highlighting for diff lines using highlight.js.
 *
 * Highlights each line individually so diff backgrounds (green/red)
 * are preserved. Uses the language detected server-side (data-language
 * attribute on the diff table).
 */

declare const hljs: {
  highlight: (code: string, options: { language: string }) => { value: string }
  getLanguage: (name: string) => unknown | undefined
}

export function initializeSyntaxHighlight(): void {
  if (typeof hljs === 'undefined') return

  const table = document.querySelector('.diff-table') as HTMLElement | null
  if (!table) return

  const language = table.dataset.language
  if (!language || !hljs.getLanguage(language)) return

  const cells = table.querySelectorAll('.diff-line-content')
  cells.forEach((cell) => {
    const text = cell.textContent || ''
    if (!text.trim()) return
    try {
      const result = hljs.highlight(text, { language })
      cell.innerHTML = result.value
    } catch {
      // Leave unhighlighted on error
    }
  })
}
