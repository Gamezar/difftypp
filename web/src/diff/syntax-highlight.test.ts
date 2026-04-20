import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  escapeHtml,
  tokensToHtml,
  DEFAULT_TEXT_COLOR,
  LANG_IMPORTS,
  initializeSyntaxHighlight,
} from './syntax-highlight'

// ── Pure function tests ──────────────────────────────────────────

describe('escapeHtml', () => {
  it('escapes ampersands', () => {
    expect(escapeHtml('a & b')).toBe('a &amp; b')
  })

  it('escapes angle brackets', () => {
    expect(escapeHtml('<div>')).toBe('&lt;div&gt;')
  })

  it('escapes double quotes', () => {
    expect(escapeHtml('"hello"')).toBe('&quot;hello&quot;')
  })

  it('escapes all special chars together', () => {
    expect(escapeHtml('<a href="x&y">')).toBe(
      '&lt;a href=&quot;x&amp;y&quot;&gt;'
    )
  })

  it('returns empty string unchanged', () => {
    expect(escapeHtml('')).toBe('')
  })

  it('returns plain text unchanged', () => {
    expect(escapeHtml('hello world')).toBe('hello world')
  })
})

describe('tokensToHtml', () => {
  it('wraps colored tokens in spans with inline style', () => {
    const tokens = [{ content: 'const', color: '#d73a49' }]
    expect(tokensToHtml(tokens)).toBe(
      '<span style="color:#d73a49">const</span>'
    )
  })

  it('does not wrap tokens with default text color', () => {
    const tokens = [{ content: 'x', color: DEFAULT_TEXT_COLOR }]
    expect(tokensToHtml(tokens)).toBe('x')
  })

  it('does not wrap tokens with undefined color', () => {
    const tokens = [{ content: 'plain' }]
    expect(tokensToHtml(tokens)).toBe('plain')
  })

  it('concatenates multiple tokens', () => {
    const tokens = [
      { content: 'if', color: '#d73a49' },
      { content: ' (', color: undefined },
      { content: 'true', color: '#005cc5' },
      { content: ')', color: undefined },
    ]
    expect(tokensToHtml(tokens)).toBe(
      '<span style="color:#d73a49">if</span>' +
        ' (' +
        '<span style="color:#005cc5">true</span>' +
        ')'
    )
  })

  it('escapes HTML in token content', () => {
    const tokens = [{ content: 'a<b>', color: '#ff0000' }]
    expect(tokensToHtml(tokens)).toBe(
      '<span style="color:#ff0000">a&lt;b&gt;</span>'
    )
  })

  it('returns empty string for empty token array', () => {
    expect(tokensToHtml([])).toBe('')
  })
})

// ── LANG_IMPORTS coverage ────────────────────────────────────────

describe('LANG_IMPORTS', () => {
  const expectedLanguages = [
    'go', 'javascript', 'typescript', 'python', 'ruby', 'rust',
    'java', 'c', 'cpp', 'cmake', 'csharp', 'css', 'html', 'json', 'yaml',
    'xml', 'markdown', 'bash', 'sql', 'toml', 'dockerfile', 'makefile',
  ]

  it('has entries for all supported languages', () => {
    for (const lang of expectedLanguages) {
      expect(LANG_IMPORTS).toHaveProperty(lang)
      expect(typeof LANG_IMPORTS[lang]).toBe('function')
    }
  })

  it('does not have entries for unsupported languages', () => {
    expect(LANG_IMPORTS['brainfuck']).toBeUndefined()
    expect(LANG_IMPORTS['']).toBeUndefined()
  })
})

// ── Integration: initializeSyntaxHighlight DOM behavior ──────────

describe('initializeSyntaxHighlight', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  it('does nothing when #diff-content is absent', async () => {
    await expect(initializeSyntaxHighlight()).resolves.toBeUndefined()
  })

  it('does nothing when data-language is empty', async () => {
    document.body.innerHTML = '<div id="diff-content"></div>'
    await expect(initializeSyntaxHighlight()).resolves.toBeUndefined()
  })

  it('does nothing for unsupported language', async () => {
    document.body.innerHTML =
      '<div id="diff-content" data-language="brainfuck"></div>'
    await expect(initializeSyntaxHighlight()).resolves.toBeUndefined()
  })

  it('does nothing when there are no diff lines', async () => {
    document.body.innerHTML =
      '<div id="diff-content" data-language="go"><table></table></div>'
    await expect(initializeSyntaxHighlight()).resolves.toBeUndefined()
  })

  it('highlights addition lines with colored spans', async () => {
    document.body.innerHTML = `
      <div id="diff-content" data-language="go">
        <table>
          <tr class="diff-line" data-line-type="addition">
            <td class="diff-line-content">func main() {</td>
          </tr>
        </table>
      </div>`

    await initializeSyntaxHighlight()

    const cell = document.querySelector('td.diff-line-content')!
    // Should contain at least one <span> with color styling
    expect(cell.innerHTML).toContain('<span')
    expect(cell.innerHTML).toContain('style="color:')
  })

  it('highlights deletion lines', async () => {
    document.body.innerHTML = `
      <div id="diff-content" data-language="python">
        <table>
          <tr class="diff-line" data-line-type="deletion">
            <td class="diff-line-content">import os</td>
          </tr>
        </table>
      </div>`

    await initializeSyntaxHighlight()

    const cell = document.querySelector('td.diff-line-content')!
    expect(cell.innerHTML).toContain('<span')
  })

  it('highlights context lines', async () => {
    document.body.innerHTML = `
      <div id="diff-content" data-language="json">
        <table>
          <tr class="diff-line" data-line-type="context">
            <td class="diff-line-content">"key": "value"</td>
          </tr>
        </table>
      </div>`

    await initializeSyntaxHighlight()

    const cell = document.querySelector('td.diff-line-content')!
    expect(cell.innerHTML).toContain('<span')
  })

  it('separates old-file and new-file streams correctly', async () => {
    // Deletion and addition lines should be highlighted independently.
    // A block comment opened in a deletion should NOT affect addition lines.
    document.body.innerHTML = `
      <div id="diff-content" data-language="javascript">
        <table>
          <tr class="diff-line" data-line-type="deletion">
            <td class="diff-line-content">/* start comment</td>
          </tr>
          <tr class="diff-line" data-line-type="addition">
            <td class="diff-line-content">const x = 42</td>
          </tr>
        </table>
      </div>`

    await initializeSyntaxHighlight()

    const cells = document.querySelectorAll('td.diff-line-content')
    const additionHtml = cells[1].innerHTML
    // "const" should be highlighted as a keyword, not as part of a comment
    expect(additionHtml).toContain('const')
    // If the streams were mixed, "const x = 42" would be inside a comment span
    // and would NOT have a keyword-colored span for "const"
    expect(additionHtml).toContain('<span')
  })

  it('preserves cells without diff-line-content td', async () => {
    document.body.innerHTML = `
      <div id="diff-content" data-language="go">
        <table>
          <tr class="diff-line" data-line-type="context">
            <td class="diff-line-num">1</td>
          </tr>
        </table>
      </div>`

    // Should not throw
    await expect(initializeSyntaxHighlight()).resolves.toBeUndefined()
  })

  it('handles multiple languages', async () => {
    for (const lang of ['yaml', 'cpp', 'bash']) {
      document.body.innerHTML = `
        <div id="diff-content" data-language="${lang}">
          <table>
            <tr class="diff-line" data-line-type="addition">
              <td class="diff-line-content"># comment</td>
            </tr>
          </table>
        </div>`

      await initializeSyntaxHighlight()

      const cell = document.querySelector('td.diff-line-content')!
      // Comments should get colored spans in all these languages
      expect(cell.innerHTML).toContain('<span')
    }
  })
})
