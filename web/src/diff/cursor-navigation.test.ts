import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  getEffectiveSide,
  getLineNum,
  computeFooterHints,
  hasCommentOnLine,
  getCommentRows,
  getFirstComment,
  findNextCommentIndex,
  clamp,
  createCursorState,
  initializeCursorNavigation,
} from './cursor-navigation'

// ── Pure function tests ──────────────────────────────────────────

describe('getEffectiveSide', () => {
  it('returns right for additions', () => {
    expect(getEffectiveSide('addition', 'left')).toBe('right')
  })

  it('returns left for deletions', () => {
    expect(getEffectiveSide('deletion', 'right')).toBe('left')
  })

  it('returns cursorSide for context lines', () => {
    expect(getEffectiveSide('context', 'left')).toBe('left')
    expect(getEffectiveSide('context', 'right')).toBe('right')
  })

  it('returns cursorSide for null type', () => {
    expect(getEffectiveSide(null, 'right')).toBe('right')
  })
})

describe('getLineNum', () => {
  it('reads left line number from data attribute', () => {
    const row = document.createElement('tr')
    row.setAttribute('data-left-num', '42')
    expect(getLineNum(row, 'left')).toBe(42)
  })

  it('reads right line number from data attribute', () => {
    const row = document.createElement('tr')
    row.setAttribute('data-right-num', '15')
    expect(getLineNum(row, 'right')).toBe(15)
  })

  it('returns 0 when attribute is missing', () => {
    const row = document.createElement('tr')
    expect(getLineNum(row, 'left')).toBe(0)
  })

  it('returns 0 for non-numeric attribute', () => {
    const row = document.createElement('tr')
    row.setAttribute('data-left-num', 'abc')
    expect(getLineNum(row, 'left')).toBe(0)
  })
})

describe('computeFooterHints', () => {
  it('shows delete confirmation hints', () => {
    expect(
      computeFooterHints({
        deleteConfirming: true,
        commentFormVisible: false,
        cursorActive: true,
        hasSelection: false,
        hasCommentOnLine: true,
      })
    ).toBe('[y] Confirm delete [Esc] Cancel')
  })

  it('shows comment form hints when form is visible', () => {
    expect(
      computeFooterHints({
        deleteConfirming: false,
        commentFormVisible: true,
        cursorActive: true,
        hasSelection: false,
        hasCommentOnLine: false,
      })
    ).toBe('[Ctrl+Enter] Save comment [Esc] Cancel')
  })

  it('shows cursor-active hints with comment operations', () => {
    const hints = computeFooterHints({
      deleteConfirming: false,
      commentFormVisible: false,
      cursorActive: true,
      hasSelection: false,
      hasCommentOnLine: true,
    })
    expect(hints).toContain('[j/k] Move')
    expect(hints).toContain('[c] Comment')
    expect(hints).toContain('[x] Resolve')
    expect(hints).toContain('[d] Delete')
    expect(hints).toContain('[a/r/s] Review')
  })

  it('shows extend hint when selection is active', () => {
    const hints = computeFooterHints({
      deleteConfirming: false,
      commentFormVisible: false,
      cursorActive: true,
      hasSelection: true,
      hasCommentOnLine: false,
    })
    expect(hints).toContain('[J/K] Extend')
    expect(hints).not.toContain('[j/k] Move')
  })

  it('shows default hints when cursor is inactive', () => {
    const hints = computeFooterHints({
      deleteConfirming: false,
      commentFormVisible: false,
      cursorActive: false,
      hasSelection: false,
      hasCommentOnLine: false,
    })
    expect(hints).toContain('[j/k] Navigate')
    expect(hints).toContain('[h/l] Prev/Next file')
  })
})

describe('clamp', () => {
  it('returns value when within range', () => {
    expect(clamp(5, 0, 10)).toBe(5)
  })

  it('clamps to min', () => {
    expect(clamp(-1, 0, 10)).toBe(0)
  })

  it('clamps to max', () => {
    expect(clamp(15, 0, 10)).toBe(10)
  })

  it('handles edge case where min equals max', () => {
    expect(clamp(5, 3, 3)).toBe(3)
  })
})

describe('createCursorState', () => {
  it('initializes with default values', () => {
    const state = createCursorState()
    expect(state.index).toBe(-1)
    expect(state.side).toBe('right')
    expect(state.selectionAnchor).toBe(-1)
    expect(state.prevIndex).toBe(-1)
    expect(state.deleteConfirming).toBe(false)
    expect(state.deleteTarget).toBeNull()
    expect(state.pendingG).toBe(false)
    expect(state.pendingGTimer).toBeNull()
  })
})

// ── DOM-dependent pure function tests ────────────────────────────

describe('hasCommentOnLine', () => {
  it('returns true when next sibling is a comment row', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
        <tr class="diff-comment-row"><td>comment</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    expect(hasCommentOnLine(line as HTMLElement)).toBe(true)
  })

  it('returns false when next sibling is not a comment row', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
        <tr class="diff-line"><td>more code</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    expect(hasCommentOnLine(line as HTMLElement)).toBe(false)
  })

  it('returns false when there is no next sibling', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    expect(hasCommentOnLine(line as HTMLElement)).toBe(false)
  })
})

describe('getCommentRows', () => {
  it('returns all consecutive comment rows', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
        <tr class="diff-comment-row"><td>comment 1</td></tr>
        <tr class="diff-comment-row"><td>comment 2</td></tr>
        <tr class="diff-line"><td>more code</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    expect(getCommentRows(line as HTMLElement)).toHaveLength(2)
  })

  it('returns empty array when no comments follow', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
        <tr class="diff-line"><td>more code</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    expect(getCommentRows(line as HTMLElement)).toHaveLength(0)
  })
})

describe('getFirstComment', () => {
  it('returns first open comment when available', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
        <tr class="diff-comment-row diff-comment-resolved" id="c1"><td>resolved</td></tr>
        <tr class="diff-comment-row" id="c2"><td>open</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    const first = getFirstComment(line as HTMLElement)
    expect(first?.id).toBe('c2')
  })

  it('returns first resolved when all are resolved', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
        <tr class="diff-comment-row diff-comment-resolved" id="c1"><td>resolved 1</td></tr>
        <tr class="diff-comment-row diff-comment-resolved" id="c2"><td>resolved 2</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    const first = getFirstComment(line as HTMLElement)
    expect(first?.id).toBe('c1')
  })

  it('returns null when no comments', () => {
    document.body.innerHTML = `
      <table>
        <tr class="diff-line" id="line1"><td>code</td></tr>
      </table>
    `
    const line = document.getElementById('line1')!
    expect(getFirstComment(line as HTMLElement)).toBeNull()
  })
})

describe('findNextCommentIndex', () => {
  function makeLinesWithComments(commentIndices: number[]): HTMLElement[] {
    const table = document.createElement('table')
    const lines: HTMLElement[] = []
    for (let i = 0; i < 5; i++) {
      const row = document.createElement('tr')
      row.className = 'diff-line'
      table.appendChild(row)
      lines.push(row)
      if (commentIndices.includes(i)) {
        const comment = document.createElement('tr')
        comment.className = 'diff-comment-row'
        table.appendChild(comment)
      }
    }
    document.body.innerHTML = ''
    document.body.appendChild(table)
    return lines
  }

  it('finds next comment forward', () => {
    const lines = makeLinesWithComments([1, 3])
    expect(findNextCommentIndex(lines, 0, 1)).toBe(1)
  })

  it('finds next comment backward', () => {
    const lines = makeLinesWithComments([1, 3])
    expect(findNextCommentIndex(lines, 3, -1)).toBe(1)
  })

  it('wraps forward when no comment after current', () => {
    const lines = makeLinesWithComments([1])
    expect(findNextCommentIndex(lines, 2, 1)).toBe(1)
  })

  it('wraps backward when no comment before current', () => {
    const lines = makeLinesWithComments([3])
    expect(findNextCommentIndex(lines, 2, -1)).toBe(3)
  })

  it('returns -1 when no comments exist', () => {
    const lines = makeLinesWithComments([])
    expect(findNextCommentIndex(lines, 0, 1)).toBe(-1)
  })

  it('returns -1 for empty lines array', () => {
    expect(findNextCommentIndex([], 0, 1)).toBe(-1)
  })
})

// ── Integration tests ────────────────────────────────────────────

describe('initializeCursorNavigation', () => {
  let ac: AbortController

  beforeEach(() => {
    ac = new AbortController()
  })

  afterEach(() => {
    ac.abort()
  })

  function setupDiffDOM() {
    document.body.innerHTML = `
      <div id="loading-overlay" class="hidden"></div>
      <div id="shortcuts-overlay" class="hidden">
        <button id="shortcuts-close">Close</button>
      </div>
      <span id="keyboard-hints"></span>
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-side="left" data-line-num="1">1</td>
          <td class="diff-line-num" data-side="right" data-line-num="1">1</td>
          <td>context line</td>
        </tr>
        <tr class="diff-line" data-left-num="2" data-right-num="2" data-line-type="addition">
          <td class="diff-line-num" data-side="left" data-line-num="0"></td>
          <td class="diff-line-num" data-side="right" data-line-num="2">2</td>
          <td>+ added line</td>
        </tr>
        <tr class="diff-line" data-left-num="3" data-right-num="3" data-line-type="deletion">
          <td class="diff-line-num" data-side="left" data-line-num="3">3</td>
          <td class="diff-line-num" data-side="right" data-line-num="0"></td>
          <td>- deleted line</td>
        </tr>
      </table>
    `
  }

  it('initializes cursor state with defaults', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    expect(state.index).toBe(-1)
    expect(state.side).toBe('right')
  })

  it('moves cursor down with j key', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'j', bubbles: true })
    )
    expect(state.index).toBe(0)
    const row = document.querySelectorAll('.diff-line')[0]
    expect(row.classList.contains('diff-line-cursor')).toBe(true)
  })

  it('moves cursor up with k key', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    // Move down twice first
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'j', bubbles: true })
    )
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'j', bubbles: true })
    )
    expect(state.index).toBe(1)
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'k', bubbles: true })
    )
    expect(state.index).toBe(0)
  })

  it('does not move cursor below last line', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    for (let i = 0; i < 10; i++) {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'j', bubbles: true })
      )
    }
    expect(state.index).toBe(2) // 3 lines, 0-indexed
  })

  it('deactivates cursor with Escape', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'j', bubbles: true })
    )
    expect(state.index).toBe(0)
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    )
    expect(state.index).toBe(-1)
  })

  it('extends selection with Shift+J', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    // First activate cursor
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'j', bubbles: true })
    )
    expect(state.index).toBe(0)
    // Extend selection
    document.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'J',
        shiftKey: true,
        bubbles: true,
      })
    )
    expect(state.selectionAnchor).toBe(0)
    expect(state.index).toBe(1)
  })

  it('clears selection with Escape', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'j', bubbles: true })
    )
    document.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'J',
        shiftKey: true,
        bubbles: true,
      })
    )
    expect(state.selectionAnchor).toBe(0)
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    )
    expect(state.selectionAnchor).toBe(-1)
  })

  it('jumps to last line with G', () => {
    setupDiffDOM()
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    document.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'G',
        shiftKey: true,
        bubbles: true,
      })
    )
    expect(state.index).toBe(2)
  })

  it('updates footer hints', () => {
    setupDiffDOM()
    initializeCursorNavigation(null, { signal: ac.signal })
    const hints = document.getElementById('keyboard-hints')!
    expect(hints.textContent).toContain('[j/k] Navigate')
  })

  it('toggles shortcuts overlay with ?', () => {
    setupDiffDOM()
    initializeCursorNavigation(null, { signal: ac.signal })
    const overlay = document.getElementById('shortcuts-overlay')!
    expect(overlay.classList.contains('hidden')).toBe(true)
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: '?', bubbles: true })
    )
    expect(overlay.classList.contains('hidden')).toBe(false)
  })

  it('closes shortcuts overlay with Escape', () => {
    setupDiffDOM()
    initializeCursorNavigation(null, { signal: ac.signal })
    const overlay = document.getElementById('shortcuts-overlay')!
    overlay.classList.remove('hidden')
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    )
    expect(overlay.classList.contains('hidden')).toBe(true)
  })

  it('ignores keys when focus is in a text input', () => {
    setupDiffDOM()
    document.body.innerHTML += '<input id="test-input" type="text" />'
    const state = initializeCursorNavigation(null, { signal: ac.signal })
    const input = document.getElementById('test-input')!
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'j', bubbles: true })
    )
    expect(state.index).toBe(-1) // cursor should not move
  })
})
