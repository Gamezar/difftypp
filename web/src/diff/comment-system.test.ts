import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  resolveLineInfo,
  buildCommentApiUrl,
  buildCommentHeader,
  initializeCommentSystem,
} from './comment-system'
import { saveCursorToHash } from './cursor-persistence'

vi.mock('./cursor-persistence', () => ({
  saveCursorToHash: vi.fn(),
}))

// ── Pure function tests ──────────────────────────────────────────

describe('resolveLineInfo', () => {
  it('returns line number and side from cell attributes', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '42')
    cell.setAttribute('data-side', 'left')
    expect(resolveLineInfo(cell)).toEqual({ lineNum: 42, side: 'left' })
  })

  it('defaults side to right when data-side is missing', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '10')
    expect(resolveLineInfo(cell)).toEqual({ lineNum: 10, side: 'right' })
  })

  it('falls back to opposite side when line number is 0', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'right')

    const row = document.createElement('tr')
    row.setAttribute('data-left-num', '15')
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toEqual({ lineNum: 15, side: 'left' })
  })

  it('falls back to right when left has no number', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'left')

    const row = document.createElement('tr')
    row.setAttribute('data-right-num', '20')
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toEqual({ lineNum: 20, side: 'right' })
  })

  it('returns null when no row found and line num is 0', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'right')
    // No parent row
    expect(resolveLineInfo(cell, () => null)).toBeNull()
  })

  it('returns null when both sides have no number', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'right')

    const row = document.createElement('tr')
    // No data-left-num attribute
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toBeNull()
  })

  it('returns null when data-line-num is missing', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-side', 'right')

    const row = document.createElement('tr')
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toBeNull()
  })
})

describe('buildCommentApiUrl', () => {
  it('builds URL with all parameters', () => {
    const url = buildCommentApiUrl({
      repoPath: '/home/user/repo',
      sourceBranch: 'feature',
      targetBranch: 'main',
      sourceCommit: 'abc123',
      targetCommit: 'def456',
      mode: 'branches',
    })
    expect(url).toBe(
      '/api/review/comment?repo=%2Fhome%2Fuser%2Frepo&source=feature&target=main&source_commit=abc123&target_commit=def456&mode=branches'
    )
  })

  it('encodes special characters', () => {
    const url = buildCommentApiUrl({
      repoPath: '/path with spaces',
      sourceBranch: 'feat/new & improved',
      targetBranch: 'main',
      sourceCommit: '',
      targetCommit: '',
      mode: 'branches',
    })
    expect(url).toContain('%2Fpath%20with%20spaces')
    expect(url).toContain('feat%2Fnew%20%26%20improved')
  })
})

describe('buildCommentHeader', () => {
  it('formats single line comment header', () => {
    expect(buildCommentHeader(42, 42, 'left')).toBe(
      'Comment on line 42 (left)'
    )
  })

  it('formats multi-line comment header', () => {
    expect(buildCommentHeader(10, 15, 'right')).toBe(
      'Comment on lines 10-15 (right)'
    )
  })
})

// ── DOM integration tests ────────────────────────────────────────

describe('initializeCommentSystem', () => {
  const cursorState = { index: -1, side: 'right' }
  const getCursorState = () => cursorState

  it('returns null when diff-table is missing', () => {
    document.body.innerHTML = ''
    expect(initializeCommentSystem(getCursorState)).toBeNull()
  })

  it('returns null when comment-form-container is missing', () => {
    document.body.innerHTML = '<table class="diff-table"></table>'
    expect(initializeCommentSystem(getCursorState)).toBeNull()
  })

  function setupDOM() {
    document.body.innerHTML = `
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-line-num="1" data-side="left">1</td>
          <td class="diff-line-num" data-line-num="1" data-side="right">1</td>
          <td>content</td>
        </tr>
        <tr class="diff-line" data-left-num="2" data-right-num="2" data-line-type="context">
          <td class="diff-line-num" data-line-num="2" data-side="left">2</td>
          <td class="diff-line-num" data-line-num="2" data-side="right">2</td>
          <td>content</td>
        </tr>
      </table>
      <div id="comment-form-container" class="hidden">
        <form id="comment-form">
          <input id="cf-file-path" type="hidden" />
          <input id="cf-start-line" type="hidden" />
          <input id="cf-end-line" type="hidden" />
          <input id="cf-side" type="hidden" />
          <textarea id="cf-body"></textarea>
          <span id="comment-form-header"></span>
          <button type="submit">Save</button>
          <button id="cf-cancel" type="button">Cancel</button>
        </form>
      </div>
    `
  }

  it('returns API object when all elements exist', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)
    expect(api).not.toBeNull()
    expect(api!.showCommentForm).toBeInstanceOf(Function)
    expect(api!.hideCommentForm).toBeInstanceOf(Function)
    expect(api!.clearLineSelection).toBeInstanceOf(Function)
    expect(api!.highlightRange).toBeInstanceOf(Function)
    expect(api!.isCommentFormVisible).toBeInstanceOf(Function)
  })

  it('showCommentForm makes form visible', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    expect(api.isCommentFormVisible()).toBe(false)
    api.highlightRange(1, 2, 'right')
    api.showCommentForm(1, 2, 'right', null)
    expect(api.isCommentFormVisible()).toBe(true)
  })

  it('hideCommentForm hides form and clears selection', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    api.highlightRange(1, 1, 'right')
    api.showCommentForm(1, 1, 'right', null)
    api.hideCommentForm()
    expect(api.isCommentFormVisible()).toBe(false)
    expect(
      document.querySelectorAll('.line-selected').length
    ).toBe(0)
  })

  it('highlightRange adds line-selected class to matching rows', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    api.highlightRange(1, 2, 'right')
    const selected = document.querySelectorAll('.line-selected')
    expect(selected.length).toBe(2)
  })

  it('clearLineSelection removes all line-selected classes', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    api.highlightRange(1, 2, 'right')
    expect(document.querySelectorAll('.line-selected').length).toBe(2)
    api.clearLineSelection()
    expect(document.querySelectorAll('.line-selected').length).toBe(0)
  })

  it('cancel button hides form', () => {
    setupDOM()
    initializeCommentSystem(getCursorState)
    const container = document.getElementById('comment-form-container')!
    container.classList.remove('hidden')
    const cancel = document.getElementById('cf-cancel')!
    cancel.click()
    expect(container.classList.contains('hidden')).toBe(true)
  })
})

// ── Integration tests: form submission ───────────────────────────

describe('comment form submission', () => {
  const SEARCH =
    '?repo=/test&source=feat&target=main&source_commit=abc&target_commit=def&mode=branches&file=test.go'

  let reloadSpy: ReturnType<typeof vi.fn>
  let fetchSpy: ReturnType<typeof vi.fn>
  let alertSpy: ReturnType<typeof vi.fn>

  function setupFullDOM(extra = '') {
    document.body.innerHTML = `
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-line-num="1" data-side="left">1</td>
          <td class="diff-line-num" data-line-num="1" data-side="right">1</td>
          <td>content</td>
        </tr>
        <tr class="diff-line" data-left-num="2" data-right-num="2" data-line-type="context">
          <td class="diff-line-num" data-line-num="2" data-side="left">2</td>
          <td class="diff-line-num" data-line-num="2" data-side="right">2</td>
          <td>content</td>
        </tr>
      </table>
      <div id="comment-form-container" class="hidden">
        <form id="comment-form">
          <input id="cf-file-path" type="hidden" />
          <input id="cf-start-line" type="hidden" />
          <input id="cf-end-line" type="hidden" />
          <input id="cf-side" type="hidden" />
          <textarea id="cf-body"></textarea>
          <span id="comment-form-header"></span>
          <button type="submit">Save</button>
          <button id="cf-cancel" type="button">Cancel</button>
        </form>
      </div>
      ${extra}
    `
  }

  beforeEach(() => {
    reloadSpy = vi.fn()
    Object.defineProperty(window, 'location', {
      value: {
        ...window.location,
        reload: reloadSpy,
        hash: '',
        search: SEARCH,
      },
      writable: true,
      configurable: true,
    })
    alertSpy = vi.fn()
    vi.stubGlobal('alert', alertSpy)
    vi.mocked(saveCursorToHash).mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  /** Helper: init system and submit the comment form. */
  function initAndSubmit(
    getCursor = () => ({ index: 0, side: 'right' })
  ) {
    const api = initializeCommentSystem(getCursor)!
    api.highlightRange(1, 1, 'right')
    api.showCommentForm(1, 1, 'right', null)
    const form = document.getElementById('comment-form') as HTMLFormElement
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    return { api, form }
  }

  it('calls fetch with POST and reloads on success', async () => {
    setupFullDOM()
    fetchSpy = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchSpy)

    initAndSubmit()

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledOnce()
    })

    const [url, opts] = fetchSpy.mock.calls[0]
    expect(url).toContain('/api/review/comment')
    expect(opts.method).toBe('POST')

    await vi.waitFor(() => {
      expect(reloadSpy).toHaveBeenCalledOnce()
    })
  })

  it('re-enables button and alerts on HTTP error', async () => {
    setupFullDOM()
    fetchSpy = vi.fn().mockResolvedValue({ ok: false, status: 500 })
    vi.stubGlobal('fetch', fetchSpy)

    initAndSubmit()

    await vi.waitFor(() => {
      expect(alertSpy).toHaveBeenCalledWith(
        'Failed to save comment (status 500). Please try again.'
      )
    })

    const btn = document.querySelector(
      '#comment-form [type="submit"]'
    ) as HTMLButtonElement
    expect(btn.disabled).toBe(false)
    expect(reloadSpy).not.toHaveBeenCalled()
  })

  it('re-enables button and alerts on network error', async () => {
    setupFullDOM()
    fetchSpy = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchSpy)

    initAndSubmit()

    await vi.waitFor(() => {
      expect(alertSpy).toHaveBeenCalledWith(
        'Network error — comment was not saved.'
      )
    })

    const btn = document.querySelector(
      '#comment-form [type="submit"]'
    ) as HTMLButtonElement
    expect(btn.disabled).toBe(false)
    expect(reloadSpy).not.toHaveBeenCalled()
  })

  it('reads getCursorState lazily at submit time, not at init time', async () => {
    setupFullDOM()
    fetchSpy = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchSpy)

    let cursorIndex = 5
    const getCursor = () => ({ index: cursorIndex, side: 'left' })

    const api = initializeCommentSystem(getCursor)!
    // Change cursor state AFTER init but BEFORE submit
    cursorIndex = 42

    api.highlightRange(1, 1, 'right')
    api.showCommentForm(1, 1, 'right', null)

    const form = document.getElementById('comment-form') as HTMLFormElement
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))

    await vi.waitFor(() => {
      expect(saveCursorToHash).toHaveBeenCalledWith(42, 'left')
    })
  })
})

// ── Integration tests: delete comment ────────────────────────────

describe('delete comment submission', () => {
  const SEARCH =
    '?repo=/test&source=feat&target=main&source_commit=abc&target_commit=def&mode=branches&file=test.go'

  let reloadSpy: ReturnType<typeof vi.fn>
  let fetchSpy: ReturnType<typeof vi.fn>
  let alertSpy: ReturnType<typeof vi.fn>

  function setupDOMWithDelete() {
    document.body.innerHTML = `
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-line-num="1" data-side="left">1</td>
          <td class="diff-line-num" data-line-num="1" data-side="right">1</td>
          <td>content</td>
        </tr>
      </table>
      <div id="comment-form-container" class="hidden">
        <form id="comment-form">
          <input id="cf-file-path" type="hidden" />
          <input id="cf-start-line" type="hidden" />
          <input id="cf-end-line" type="hidden" />
          <input id="cf-side" type="hidden" />
          <textarea id="cf-body"></textarea>
          <span id="comment-form-header"></span>
          <button type="submit">Save</button>
          <button id="cf-cancel" type="button">Cancel</button>
        </form>
      </div>
      <form class="delete-comment-form" action="/api/comment/delete/42">
        <button type="submit" data-testid="btn-comment-delete-42">Delete</button>
      </form>
    `
  }

  beforeEach(() => {
    reloadSpy = vi.fn()
    Object.defineProperty(window, 'location', {
      value: {
        ...window.location,
        reload: reloadSpy,
        hash: '',
        search: SEARCH,
      },
      writable: true,
      configurable: true,
    })
    alertSpy = vi.fn()
    vi.stubGlobal('alert', alertSpy)
    vi.mocked(saveCursorToHash).mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  /** Submit the delete form. */
  function initAndDelete(
    getCursor = () => ({ index: 0, side: 'right' })
  ) {
    initializeCommentSystem(getCursor)
    const form = document.querySelector(
      '.delete-comment-form'
    ) as HTMLFormElement
    form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    return form
  }

  it('calls fetch with DELETE and reloads on success', async () => {
    setupDOMWithDelete()
    fetchSpy = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchSpy)

    initAndDelete()

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledOnce()
    })

    const [url, opts] = fetchSpy.mock.calls[0]
    expect(url).toBe('/api/comment/delete/42')
    expect(opts.method).toBe('DELETE')

    await vi.waitFor(() => {
      expect(reloadSpy).toHaveBeenCalledOnce()
    })
  })

  it('re-enables button and alerts on HTTP error', async () => {
    setupDOMWithDelete()
    fetchSpy = vi.fn().mockResolvedValue({ ok: false, status: 403 })
    vi.stubGlobal('fetch', fetchSpy)

    initAndDelete()

    await vi.waitFor(() => {
      expect(alertSpy).toHaveBeenCalledWith(
        'Failed to delete comment (status 403). Please try again.'
      )
    })

    const btn = document.querySelector(
      '.delete-comment-form [type="submit"]'
    ) as HTMLButtonElement
    expect(btn.disabled).toBe(false)
    expect(reloadSpy).not.toHaveBeenCalled()
  })

  it('re-enables button and alerts on network error', async () => {
    setupDOMWithDelete()
    fetchSpy = vi.fn().mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchSpy)

    initAndDelete()

    await vi.waitFor(() => {
      expect(alertSpy).toHaveBeenCalledWith(
        'Network error — comment was not deleted.'
      )
    })

    const btn = document.querySelector(
      '.delete-comment-form [type="submit"]'
    ) as HTMLButtonElement
    expect(btn.disabled).toBe(false)
    expect(reloadSpy).not.toHaveBeenCalled()
  })
})

// ── Integration tests: drag-select flow ──────────────────────────

describe('drag-select flow', () => {
  const SEARCH =
    '?repo=/test&source=feat&target=main&source_commit=abc&target_commit=def&mode=branches&file=test.go'

  function setupDragDOM() {
    Object.defineProperty(window, 'location', {
      value: {
        ...window.location,
        reload: vi.fn(),
        hash: '',
        search: SEARCH,
      },
      writable: true,
      configurable: true,
    })
    document.body.innerHTML = `
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-line-num="1" data-side="left">1</td>
          <td class="diff-line-num" data-line-num="1" data-side="right">1</td>
          <td>content</td>
        </tr>
        <tr class="diff-line" data-left-num="2" data-right-num="2" data-line-type="context">
          <td class="diff-line-num" data-line-num="2" data-side="left">2</td>
          <td class="diff-line-num" data-line-num="2" data-side="right">2</td>
          <td>content</td>
        </tr>
      </table>
      <div id="comment-form-container" class="hidden">
        <form id="comment-form">
          <input id="cf-file-path" type="hidden" />
          <input id="cf-start-line" type="hidden" />
          <input id="cf-end-line" type="hidden" />
          <input id="cf-side" type="hidden" />
          <textarea id="cf-body"></textarea>
          <span id="comment-form-header"></span>
          <button type="submit">Save</button>
          <button id="cf-cancel" type="button">Cancel</button>
        </form>
      </div>
    `
  }

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('mousedown starts drag, mousemove extends, mouseup shows form', () => {
    setupDragDOM()
    const getCursor = () => ({ index: 0, side: 'right' })
    initializeCommentSystem(getCursor)

    const lineNumCells = document.querySelectorAll(
      '.diff-line-num[data-line-num]'
    )
    // line 1 left cell
    const startCell = lineNumCells[0] as HTMLElement
    // line 2 left cell
    const endCell = lineNumCells[2] as HTMLElement

    const diffTable = document.querySelector('.diff-table') as HTMLElement

    // mousedown on first line number
    diffTable.dispatchEvent(
      new MouseEvent('mousedown', { bubbles: true, target: startCell } as MouseEventInit)
    )
    // jsdom doesn't route event.target through dispatchEvent, simulate directly
    startCell.dispatchEvent(
      new MouseEvent('mousedown', { bubbles: true })
    )

    // mousemove to second line number
    endCell.dispatchEvent(
      new MouseEvent('mousemove', { bubbles: true })
    )

    // mouseup on document to end drag
    document.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))

    const container = document.getElementById('comment-form-container')!
    expect(container.classList.contains('hidden')).toBe(false)
  })

  it('highlights selected range during drag', () => {
    setupDragDOM()
    const getCursor = () => ({ index: 0, side: 'right' })
    initializeCommentSystem(getCursor)

    const leftCells = document.querySelectorAll(
      '.diff-line-num[data-side="left"]'
    )
    const startCell = leftCells[0] as HTMLElement

    // mousedown on first line number
    startCell.dispatchEvent(
      new MouseEvent('mousedown', { bubbles: true })
    )

    // After mousedown, the start row should be highlighted
    const selected = document.querySelectorAll('.line-selected')
    expect(selected.length).toBeGreaterThanOrEqual(1)
  })
})

// ── Integration tests: Ctrl+Enter shortcut ───────────────────────

describe('Ctrl+Enter shortcut', () => {
  const SEARCH =
    '?repo=/test&source=feat&target=main&source_commit=abc&target_commit=def&mode=branches&file=test.go'

  beforeEach(() => {
    Object.defineProperty(window, 'location', {
      value: {
        ...window.location,
        reload: vi.fn(),
        hash: '',
        search: SEARCH,
      },
      writable: true,
      configurable: true,
    })
    document.body.innerHTML = `
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-line-num="1" data-side="left">1</td>
          <td class="diff-line-num" data-line-num="1" data-side="right">1</td>
          <td>content</td>
        </tr>
      </table>
      <div id="comment-form-container" class="hidden">
        <form id="comment-form">
          <input id="cf-file-path" type="hidden" />
          <input id="cf-start-line" type="hidden" />
          <input id="cf-end-line" type="hidden" />
          <input id="cf-side" type="hidden" />
          <textarea id="cf-body"></textarea>
          <span id="comment-form-header"></span>
          <button type="submit">Save</button>
          <button id="cf-cancel" type="button">Cancel</button>
        </form>
      </div>
    `
    vi.mocked(saveCursorToHash).mockClear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('Ctrl+Enter on textarea triggers form submit', async () => {
    const fetchSpy = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchSpy)

    const getCursor = () => ({ index: 0, side: 'right' })
    initializeCommentSystem(getCursor)

    const textarea = document.getElementById('cf-body') as HTMLTextAreaElement
    textarea.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'Enter',
        ctrlKey: true,
        bubbles: true,
      })
    )

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledOnce()
    })
  })

  it('Meta+Enter on textarea also triggers form submit', async () => {
    const fetchSpy = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchSpy)

    const getCursor = () => ({ index: 0, side: 'right' })
    initializeCommentSystem(getCursor)

    const textarea = document.getElementById('cf-body') as HTMLTextAreaElement
    textarea.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'Enter',
        metaKey: true,
        bubbles: true,
      })
    )

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledOnce()
    })
  })
})

// ── Integration tests: showCommentForm field values ──────────────

describe('showCommentForm sets form field values', () => {
  const SEARCH =
    '?repo=/test&source=feat&target=main&source_commit=abc&target_commit=def&mode=branches&file=test.go'

  beforeEach(() => {
    Object.defineProperty(window, 'location', {
      value: {
        ...window.location,
        reload: vi.fn(),
        hash: '',
        search: SEARCH,
      },
      writable: true,
      configurable: true,
    })
    document.body.innerHTML = `
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-line-num="1" data-side="left">1</td>
          <td class="diff-line-num" data-line-num="1" data-side="right">1</td>
          <td>content</td>
        </tr>
        <tr class="diff-line" data-left-num="5" data-right-num="5" data-line-type="context">
          <td class="diff-line-num" data-line-num="5" data-side="left">5</td>
          <td class="diff-line-num" data-line-num="5" data-side="right">5</td>
          <td>content</td>
        </tr>
      </table>
      <div id="comment-form-container" class="hidden">
        <form id="comment-form">
          <input id="cf-file-path" type="hidden" />
          <input id="cf-start-line" type="hidden" />
          <input id="cf-end-line" type="hidden" />
          <input id="cf-side" type="hidden" />
          <textarea id="cf-body"></textarea>
          <span id="comment-form-header"></span>
          <button type="submit">Save</button>
          <button id="cf-cancel" type="button">Cancel</button>
        </form>
      </div>
    `
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('populates hidden fields with correct values', () => {
    const getCursor = () => ({ index: 0, side: 'right' })
    const api = initializeCommentSystem(getCursor)!
    api.highlightRange(1, 5, 'left')
    api.showCommentForm(1, 5, 'left', null)

    expect(
      (document.getElementById('cf-file-path') as HTMLInputElement).value
    ).toBe('test.go')
    expect(
      (document.getElementById('cf-start-line') as HTMLInputElement).value
    ).toBe('1')
    expect(
      (document.getElementById('cf-end-line') as HTMLInputElement).value
    ).toBe('5')
    expect(
      (document.getElementById('cf-side') as HTMLInputElement).value
    ).toBe('left')
  })

  it('sets single-line header text', () => {
    const getCursor = () => ({ index: 0, side: 'right' })
    const api = initializeCommentSystem(getCursor)!
    api.highlightRange(1, 1, 'right')
    api.showCommentForm(1, 1, 'right', null)

    expect(
      document.getElementById('comment-form-header')!.textContent
    ).toBe('Comment on line 1 (right)')
  })

  it('sets multi-line header text', () => {
    const getCursor = () => ({ index: 0, side: 'right' })
    const api = initializeCommentSystem(getCursor)!
    api.highlightRange(1, 5, 'left')
    api.showCommentForm(1, 5, 'left', null)

    expect(
      document.getElementById('comment-form-header')!.textContent
    ).toBe('Comment on lines 1-5 (left)')
  })

  it('sets form action to the correct API URL', () => {
    const getCursor = () => ({ index: 0, side: 'right' })
    const api = initializeCommentSystem(getCursor)!
    api.highlightRange(1, 1, 'right')
    api.showCommentForm(1, 1, 'right', null)

    const form = document.getElementById('comment-form')!
    const action = form.getAttribute('action')!
    expect(action).toContain('/api/review/comment')
    expect(action).toContain('repo=%2Ftest')
    expect(action).toContain('source=feat')
    expect(action).toContain('target=main')
  })

  it('clears textarea body on show', () => {
    const getCursor = () => ({ index: 0, side: 'right' })
    const api = initializeCommentSystem(getCursor)!

    // Pre-fill the textarea
    const textarea = document.getElementById('cf-body') as HTMLTextAreaElement
    textarea.value = 'old text'

    api.highlightRange(1, 1, 'right')
    api.showCommentForm(1, 1, 'right', null)

    expect(textarea.value).toBe('')
  })
})
