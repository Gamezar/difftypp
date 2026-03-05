import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
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

// ── Integration tests with commentApi ────────────────────────────

describe('initializeCursorNavigation with commentApi', () => {
  let ac: AbortController
  let commentApi: {
    showCommentForm: ReturnType<typeof vi.fn>
    hideCommentForm: ReturnType<typeof vi.fn>
    clearLineSelection: ReturnType<typeof vi.fn>
    highlightRange: ReturnType<typeof vi.fn>
    isCommentFormVisible: ReturnType<typeof vi.fn>
  }

  beforeEach(() => {
    ac = new AbortController()
    commentApi = {
      showCommentForm: vi.fn(),
      hideCommentForm: vi.fn(),
      clearLineSelection: vi.fn(),
      highlightRange: vi.fn(),
      isCommentFormVisible: vi.fn().mockReturnValue(false),
    }
  })

  afterEach(() => {
    ac.abort()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  function setupDiffWithComments() {
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
        <tr class="diff-comment-row" id="comment-1">
          <td colspan="3">
            <form action="/api/comment/resolve/1">
              <button data-testid="btn-comment-resolve-1">Resolve</button>
            </form>
            <form class="delete-comment-form" action="/api/comment/delete/1">
              <button data-testid="btn-comment-delete-1" type="submit">Delete</button>
            </form>
          </td>
        </tr>
        <tr class="diff-line" data-left-num="2" data-right-num="2" data-line-type="addition">
          <td class="diff-line-num" data-side="left" data-line-num="0"></td>
          <td class="diff-line-num" data-side="right" data-line-num="2">2</td>
          <td>+ added line</td>
        </tr>
        <tr class="diff-line" data-left-num="3" data-right-num="3" data-line-type="context">
          <td class="diff-line-num" data-side="left" data-line-num="3">3</td>
          <td class="diff-line-num" data-side="right" data-line-num="3">3</td>
          <td>context line 2</td>
        </tr>
      </table>
    `
  }

  function press(key: string, opts: Partial<KeyboardEventInit> = {}) {
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key, bubbles: true, ...opts })
    )
  }

  // ── handleDiffCommentKeys ──

  describe('c key (open comment)', () => {
    it('calls showCommentForm via commentApi', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j') // move to line 0
      expect(state.index).toBe(0)
      press('c')
      expect(commentApi.showCommentForm).toHaveBeenCalled()
    })

    it('does not call showCommentForm when cursor is inactive', () => {
      setupDiffWithComments()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('c')
      // Cursor index is -1, so showCommentForm should not be called
      expect(commentApi.showCommentForm).not.toHaveBeenCalled()
    })

    it('passes line range with selection to showCommentForm', () => {
      setupDiffWithComments()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j') // line 0
      press('J', { shiftKey: true }) // extend to line 1
      press('c')
      expect(commentApi.highlightRange).toHaveBeenCalled()
      expect(commentApi.showCommentForm).toHaveBeenCalled()
      const args = commentApi.showCommentForm.mock.calls[0]
      // startLine <= endLine
      expect(args[0]).toBeLessThanOrEqual(args[1])
    })
  })

  describe('x key (resolve/reopen comment)', () => {
    it('sends fetch POST to resolve comment action', async () => {
      setupDiffWithComments()
      const fetchSpy = vi.fn().mockResolvedValue({ ok: true })
      vi.stubGlobal('fetch', fetchSpy)
      // Mock window.location.reload
      const reloadSpy = vi.fn()
      Object.defineProperty(window, 'location', {
        value: { ...window.location, reload: reloadSpy, hash: '' },
        writable: true,
        configurable: true,
      })

      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j') // move to line 0 (which has a comment)
      press('x')

      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/comment/resolve/1',
        { method: 'POST' }
      )
      await vi.waitFor(() => {
        expect(reloadSpy).toHaveBeenCalled()
      })
    })

    it('shows alert on resolve failure', async () => {
      setupDiffWithComments()
      const fetchSpy = vi
        .fn()
        .mockResolvedValue({ ok: false, status: 500 })
      vi.stubGlobal('fetch', fetchSpy)
      const alertSpy = vi.fn()
      vi.stubGlobal('alert', alertSpy)

      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j')
      press('x')

      await vi.waitFor(() => {
        expect(alertSpy).toHaveBeenCalledWith(
          expect.stringContaining('500')
        )
      })
    })

    it('shows alert on network error', async () => {
      setupDiffWithComments()
      const fetchSpy = vi
        .fn()
        .mockRejectedValue(new TypeError('Network error'))
      vi.stubGlobal('fetch', fetchSpy)
      const alertSpy = vi.fn()
      vi.stubGlobal('alert', alertSpy)

      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j')
      press('x')

      await vi.waitFor(() => {
        expect(alertSpy).toHaveBeenCalledWith(
          expect.stringContaining('Network error')
        )
      })
    })

    it('ignores x key when line has no comment', () => {
      setupDiffWithComments()
      const fetchSpy = vi.fn()
      vi.stubGlobal('fetch', fetchSpy)

      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j') // line 0 has comment
      press('j') // line 1 has no comment
      press('x')
      expect(fetchSpy).not.toHaveBeenCalled()
    })
  })

  // ── handleDeleteConfirmKeys ──

  describe('d/y keys (delete comment with confirmation)', () => {
    it('d key starts delete confirmation on line with comment', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j') // line 0 has comment
      press('d')
      expect(state.deleteConfirming).toBe(true)
      const hints = document.getElementById('keyboard-hints')!
      expect(hints.textContent).toContain('Confirm delete')
    })

    it('y confirms deletion and submits delete form', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j')
      press('d')
      expect(state.deleteConfirming).toBe(true)

      // Mock requestSubmit
      const deleteForm = document.querySelector(
        '.delete-comment-form'
      ) as HTMLFormElement
      const requestSubmitSpy = vi.fn()
      deleteForm.requestSubmit = requestSubmitSpy

      press('y')
      expect(requestSubmitSpy).toHaveBeenCalled()
      expect(state.deleteConfirming).toBe(false)
    })

    it('Escape cancels delete confirmation', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j')
      press('d')
      expect(state.deleteConfirming).toBe(true)
      press('Escape')
      expect(state.deleteConfirming).toBe(false)
    })

    it('any other key cancels delete confirmation', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j')
      press('d')
      expect(state.deleteConfirming).toBe(true)
      press('n') // any key other than 'y'
      expect(state.deleteConfirming).toBe(false)
    })

    it('d key ignored on line without comment', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j') // line 0 (has comment)
      press('j') // line 1 (no comment)
      press('d')
      expect(state.deleteConfirming).toBe(false)
    })

    it('adds visual indicator during delete confirmation', () => {
      setupDiffWithComments()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j')
      press('d')
      const commentRow = document.getElementById('comment-1')!
      expect(commentRow.classList.contains('comment-delete-target')).toBe(
        true
      )
    })

    it('removes visual indicator after cancel', () => {
      setupDiffWithComments()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j')
      press('d')
      press('Escape')
      const commentRow = document.getElementById('comment-1')!
      expect(commentRow.classList.contains('comment-delete-target')).toBe(
        false
      )
    })
  })

  // ── ] and [ comment navigation ──

  describe('] and [ keys (comment navigation)', () => {
    it('] jumps to next line with comment', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      // Start at no position
      press(']')
      expect(state.index).toBe(0) // line 0 has a comment
    })

    it('[ jumps to previous line with comment', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('G', { shiftKey: true }) // jump to last line
      press('[')
      expect(state.index).toBe(0) // line 0 has comment
    })
  })

  // ── Escape with commentApi ──

  describe('Escape with comment form visible', () => {
    it('hides comment form when visible', () => {
      setupDiffWithComments()
      commentApi.isCommentFormVisible.mockReturnValue(true)
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j') // activate cursor
      press('Escape')
      expect(commentApi.hideCommentForm).toHaveBeenCalled()
    })
  })

  // ── Tab side toggle ──

  describe('Tab key (side toggle)', () => {
    it('toggles side on context line', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j') // line 0 is context type
      expect(state.side).toBe('right')
      press('Tab')
      expect(state.side).toBe('left')
      press('Tab')
      expect(state.side).toBe('right')
    })

    it('does not toggle side on addition line', () => {
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('j') // line 0 (context)
      press('j') // line 1 (addition)
      const sideBefore = state.side
      press('Tab')
      expect(state.side).toBe(sideBefore)
    })
  })

  // ── gg (double-g) jump to first line ──

  describe('gg (jump to first line)', () => {
    it('jumps to first line on double g within 500ms', () => {
      vi.useFakeTimers()
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('G', { shiftKey: true }) // jump to last
      expect(state.index).toBe(2)

      press('g') // first g
      vi.advanceTimersByTime(100)
      press('g') // second g within 500ms
      expect(state.index).toBe(0)
      vi.useRealTimers()
    })

    it('does not jump on single g after timeout', () => {
      vi.useFakeTimers()
      setupDiffWithComments()
      const state = initializeCursorNavigation(commentApi, {
        signal: ac.signal,
      })
      press('G', { shiftKey: true }) // jump to last
      expect(state.index).toBe(2)

      press('g') // first g
      vi.advanceTimersByTime(600) // exceed 500ms timeout
      press('g') // this is a new first g, not a double-g
      vi.advanceTimersByTime(600) // exceed again
      expect(state.index).toBe(2) // should not have jumped
      vi.useRealTimers()
    })
  })

  // ── handleFileNavKeys ──

  describe('h/l keys (file navigation)', () => {
    function setupWithFileLinks() {
      setupDiffWithComments()
      // Add file navigation links
      const prevLink = document.createElement('a')
      prevLink.id = 'prev-file-link'
      prevLink.href = '/diff?file=prev.go'
      prevLink.addEventListener('click', (e) => e.preventDefault())
      document.body.appendChild(prevLink)

      const nextLink = document.createElement('a')
      nextLink.id = 'next-file-link'
      nextLink.href = '/diff?file=next.go'
      nextLink.addEventListener('click', (e) => e.preventDefault())
      document.body.appendChild(nextLink)
    }

    it('h key triggers prev file link click', () => {
      setupWithFileLinks()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      const prevLink = document.getElementById('prev-file-link')!
      const clickSpy = vi.fn((e: Event) => e.preventDefault())
      prevLink.addEventListener('click', clickSpy)

      press('h')
      // afterOverlayPaint uses double-rAF; in jsdom, rAF is synchronous-ish
      // The loading overlay should be shown
      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(false)
    })

    it('l key triggers next file link click', () => {
      setupWithFileLinks()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      const nextLink = document.getElementById('next-file-link')!
      const clickSpy = vi.fn((e: Event) => e.preventDefault())
      nextLink.addEventListener('click', clickSpy)

      press('l')
      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(false)
    })

    it('h key does nothing without prev-file-link', () => {
      setupDiffWithComments()
      // No prev-file-link in DOM
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      // Should not throw
      press('h')
    })
  })

  // ── handleReviewKeys ──

  describe('a/r/s keys (review actions)', () => {
    function setupWithReviewForms() {
      setupDiffWithComments()
      document.body.innerHTML += `
        <form class="review-form" action="/api/review?status=approved">
          <button type="submit">Approve</button>
        </form>
        <form class="review-form" action="/api/review?status=rejected">
          <button type="submit">Reject</button>
        </form>
        <form class="review-form" action="/api/review?status=skipped">
          <button type="submit">Skip</button>
        </form>
      `
    }

    it('a key shows loading and submits approve form', () => {
      setupWithReviewForms()
      const approveForm = document.querySelector(
        'form[action*="status=approved"]'
      ) as HTMLFormElement
      const submitSpy = vi.fn()
      approveForm.submit = submitSpy

      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('a')

      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(false)
    })

    it('r key shows loading for reject form', () => {
      setupWithReviewForms()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('r')

      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(false)
    })

    it('s key shows loading for skip form', () => {
      setupWithReviewForms()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('s')

      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(false)
    })

    it('review keys ignored without approve form', () => {
      setupDiffWithComments()
      // No review forms in DOM
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('a')
      press('r')
      press('s')
      // Should not throw and loading overlay stays hidden
      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(true)
    })
  })

  // ── S key (submit review) ──

  describe('S key (submit review)', () => {
    it('submits the review form via submit button', () => {
      setupDiffWithComments()
      document.body.innerHTML += `
        <form class="review-form" action="/api/review/submit">
          <button data-testid="btn-submit-review" type="submit">Submit Review</button>
        </form>
      `
      const form = document.querySelector(
        '.review-form'
      ) as HTMLFormElement
      const submitSpy = vi.fn()
      form.submit = submitSpy

      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('S', { shiftKey: true })
      expect(submitSpy).toHaveBeenCalled()
    })
  })

  // ── handleFileListKeys ──

  describe('file list navigation (j/k/Enter in files-list)', () => {
    function setupWithFileList() {
      document.body.innerHTML = `
        <div id="loading-overlay" class="hidden"></div>
        <div id="shortcuts-overlay" class="hidden">
          <button id="shortcuts-close">Close</button>
        </div>
        <span id="keyboard-hints"></span>
        <ul id="files-list">
          <li><a href="/diff?file=a.go">a.go</a></li>
          <li><a href="/diff?file=b.go">b.go</a></li>
          <li class="hidden"><a href="/diff?file=c.go">c.go</a></li>
          <li><a href="/diff?file=d.go">d.go</a></li>
        </ul>
      `
    }

    it('j key highlights first visible file', () => {
      setupWithFileList()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j')
      const items = document.querySelectorAll(
        '#files-list li:not(.hidden)'
      )
      expect(items[0].classList.contains('bg-gray-100')).toBe(true)
    })

    it('k key wraps from no selection to second-to-last', () => {
      setupWithFileList()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('k')
      const items = document.querySelectorAll(
        '#files-list li:not(.hidden)'
      )
      // With currentIndex=-1: (-1 - 1 + 3) % 3 = 1 (second item, 0-indexed)
      expect(items[1].classList.contains('bg-gray-100')).toBe(true)
    })

    it('j cycles through visible files', () => {
      setupWithFileList()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j') // highlight first
      press('j') // highlight second
      const items = document.querySelectorAll(
        '#files-list li:not(.hidden)'
      )
      expect(items[0].classList.contains('bg-gray-100')).toBe(false)
      expect(items[1].classList.contains('bg-gray-100')).toBe(true)
    })

    it('Enter clicks the link in highlighted file', () => {
      setupWithFileList()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j') // highlight first

      const link = document.querySelector(
        '#files-list li:not(.hidden) a'
      ) as HTMLAnchorElement
      const clickSpy = vi.fn((e: Event) => e.preventDefault())
      link.addEventListener('click', clickSpy)

      press('Enter')
      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(false)
    })

    it('Enter does nothing when no file is highlighted', () => {
      setupWithFileList()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      // Don't press j first — no file highlighted
      press('Enter')
      // Should not throw, loading overlay stays hidden
      const overlay = document.getElementById('loading-overlay')!
      expect(overlay.classList.contains('hidden')).toBe(true)
    })

    it('j wraps around to first file after last', () => {
      setupWithFileList()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      const items = document.querySelectorAll(
        '#files-list li:not(.hidden)'
      )
      // Press j (items.length + 1) times to wrap
      for (let i = 0; i <= items.length; i++) {
        press('j')
      }
      // Should wrap back to first
      expect(items[0].classList.contains('bg-gray-100')).toBe(true)
    })
  })

  // ── moveCursor calls clearLineSelection ──

  describe('cursor movement with commentApi', () => {
    it('j key calls clearLineSelection', () => {
      setupDiffWithComments()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j')
      expect(commentApi.clearLineSelection).toHaveBeenCalled()
    })

    it('selection extension calls clearLineSelection then applies new selection', () => {
      setupDiffWithComments()
      initializeCursorNavigation(commentApi, { signal: ac.signal })
      press('j') // activate
      commentApi.clearLineSelection.mockClear()
      press('J', { shiftKey: true }) // extend selection
      expect(commentApi.clearLineSelection).toHaveBeenCalled()
    })
  })
})
