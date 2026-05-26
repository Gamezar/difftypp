/**
 * Syntax highlighting for diff lines using Shiki (VSCode-quality TextMate grammars).
 *
 * Uses shiki's JavaScript engine (no WASM) with only the languages we need.
 * Tokenizes old-file and new-file streams separately as full code blocks,
 * preserving multi-line context (block comments, namespaces, templates, etc.).
 */

import { createHighlighterCore, type HighlighterCore, type LanguageInput } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'

// Lazy-import language grammars — only loaded when needed
export const LANG_IMPORTS: Record<string, () => Promise<unknown>> = {
  go: () => import('shiki/langs/go.mjs'),
  javascript: () => import('shiki/langs/javascript.mjs'),
  typescript: () => import('shiki/langs/typescript.mjs'),
  python: () => import('shiki/langs/python.mjs'),
  ruby: () => import('shiki/langs/ruby.mjs'),
  rust: () => import('shiki/langs/rust.mjs'),
  java: () => import('shiki/langs/java.mjs'),
  c: () => import('shiki/langs/c.mjs'),
  cpp: () => import('shiki/langs/cpp.mjs'),
  cmake: () => import('shiki/langs/cmake.mjs'),
  csharp: () => import('shiki/langs/csharp.mjs'),
  css: () => import('shiki/langs/css.mjs'),
  html: () => import('shiki/langs/html.mjs'),
  json: () => import('shiki/langs/json.mjs'),
  yaml: () => import('shiki/langs/yaml.mjs'),
  xml: () => import('shiki/langs/xml.mjs'),
  markdown: () => import('shiki/langs/markdown.mjs'),
  bash: () => import('shiki/langs/bash.mjs'),
  sql: () => import('shiki/langs/sql.mjs'),
  toml: () => import('shiki/langs/toml.mjs'),
  dockerfile: () => import('shiki/langs/dockerfile.mjs'),
  makefile: () => import('shiki/langs/makefile.mjs'),
}

/**
 * Escape HTML special chars for safe innerHTML insertion.
 */
export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

/**
 * Render a single line's tokens as HTML spans with inline color styles.
 * Tokens with the default text color are rendered without a span wrapper.
 */
// Tied to catppuccin-latte (base color). Update if the theme changes.
export const DEFAULT_TEXT_COLOR = '#4c4f69'

export function tokensToHtml(tokens: Array<{ content: string; color?: string }>): string {
  return tokens
    .map(t => {
      const escaped = escapeHtml(t.content)
      if (t.color && t.color !== DEFAULT_TEXT_COLOR) {
        return `<span style="color:${t.color}">${escaped}</span>`
      }
      return escaped
    })
    .join('')
}

/**
 * Apply syntax highlighting to all diff lines in the current view.
 * Async — lazily loads only the needed language grammar.
 */
export async function initializeSyntaxHighlight(): Promise<void> {
  const diffContent = document.getElementById('diff-content')
  if (!diffContent) return

  const language = diffContent.dataset.language
  if (!language || !LANG_IMPORTS[language]) return

  let highlighter: HighlighterCore
  try {
    const langModule = await LANG_IMPORTS[language]()
    highlighter = await createHighlighterCore({
      themes: [import('shiki/themes/catppuccin-latte.mjs')],
      langs: [langModule as LanguageInput],
      engine: createJavaScriptRegexEngine(),
    })
  } catch (err) {
    console.warn('diffty: failed to initialize syntax highlighter', err)
    return
  }

  const rows = diffContent.querySelectorAll<HTMLTableRowElement>('tr.diff-line')
  if (rows.length === 0) return

  // Collect lines into old-file and new-file streams.
  // Context lines go into the new stream only — they exist in both versions,
  // so assigning to one avoids double-highlighting (same row, last write wins).
  const oldLines: { row: HTMLTableRowElement; text: string }[] = []
  const newLines: { row: HTMLTableRowElement; text: string }[] = []

  rows.forEach(row => {
    const cell = row.querySelector<HTMLTableCellElement>('td.diff-line-content')
    if (!cell) return

    const text = cell.textContent ?? ''
    const lineType = row.dataset.lineType

    if (lineType === 'deletion') {
      oldLines.push({ row, text })
    } else {
      newLines.push({ row, text })
    }
  })

  const highlightStream = (stream: { row: HTMLTableRowElement; text: string }[]) => {
    if (stream.length === 0) return

    const code = stream.map(s => s.text).join('\n')
    const result = highlighter.codeToTokens(code, { lang: language, theme: 'catppuccin-latte' })

    if (result.tokens.length < stream.length) {
      console.warn(
        `diffty: shiki returned ${result.tokens.length} token rows for ${stream.length} lines — trailing lines kept server HTML`
      )
    }

    result.tokens.forEach((lineTokens, i) => {
      if (i >= stream.length) return
      const cell = stream[i].row.querySelector<HTMLTableCellElement>('td.diff-line-content')
      if (!cell) return
      const html = tokensToHtml(lineTokens)
      if (html) cell.innerHTML = html
    })
  }

  highlightStream(oldLines)
  highlightStream(newLines)

  highlighter.dispose()
}
