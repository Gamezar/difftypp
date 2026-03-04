/**
 * Comment system for inline code review.
 *
 * Handles drag-select on diff line numbers to create comment ranges,
 * comment form positioning and submission, and comment deletion via fetch.
 */

import { saveCursorToHash } from './cursor-persistence'

// ── Pure functions ────────────────────────────────────────────────

export interface LineInfo {
  lineNum: number
  side: 'left' | 'right'
}

/**
 * Resolve a valid line number and side from a clicked line-number cell.
 * Falls back to the opposite side if the clicked side has no number.
 */
export function resolveLineInfo(
  cell: HTMLElement,
  getRow: () => HTMLElement | null = () => cell.closest('tr')
): LineInfo | null {
  let lineNum = parseInt(cell.getAttribute('data-line-num') || '', 10) || 0
  let side = (cell.getAttribute('data-side') || 'right') as 'left' | 'right'

  if (lineNum <= 0) {
    const row = getRow()
    if (!row) return null
    const otherAttr = side === 'right' ? 'data-left-num' : 'data-right-num'
    const otherSide: 'left' | 'right' = side === 'right' ? 'left' : 'right'
    lineNum = parseInt(row.getAttribute(otherAttr) || '', 10) || 0
    side = otherSide
  }

  if (lineNum <= 0) return null
  return { lineNum, side }
}

/**
 * Build the API URL for creating a comment.
 */
export function buildCommentApiUrl(params: {
  repoPath: string
  sourceBranch: string
  targetBranch: string
  sourceCommit: string
  targetCommit: string
  mode: string
}): string {
  return (
    '/api/review/comment?repo=' +
    encodeURIComponent(params.repoPath) +
    '&source=' +
    encodeURIComponent(params.sourceBranch) +
    '&target=' +
    encodeURIComponent(params.targetBranch) +
    '&source_commit=' +
    encodeURIComponent(params.sourceCommit) +
    '&target_commit=' +
    encodeURIComponent(params.targetCommit) +
    '&mode=' +
    encodeURIComponent(params.mode)
  )
}

/**
 * Build the comment form header text.
 */
export function buildCommentHeader(
  startLine: number,
  endLine: number,
  side: string
): string {
  if (startLine === endLine) {
    return `Comment on line ${startLine} (${side})`
  }
  return `Comment on lines ${startLine}-${endLine} (${side})`
}

// ── Types for inter-module communication ─────────────────────────

export interface CommentSystemApi {
  showCommentForm: (
    startLine: number,
    endLine: number,
    side: string,
    event: Event | null
  ) => void
  hideCommentForm: () => void
  clearLineSelection: () => void
  highlightRange: (start: number, end: number, side: string) => void
  isCommentFormVisible: () => boolean
}

// ── DOM wiring ───────────────────────────────────────────────────

/**
 * Initialize the comment system on the diff page.
 * Returns an API object that the cursor navigation system can call.
 *
 * @param getCursorState - Getter for the current cursor position. Called lazily
 *   at event-handling time (e.g. when saving a comment), so the cursor navigation
 *   module can be initialized after this function returns.
 */
export function initializeCommentSystem(
  getCursorState: () => { index: number; side: string }
): CommentSystemApi | null {
  const diffTable = document.querySelector('.diff-table') as HTMLElement | null
  if (!diffTable) return null

  const commentFormContainer = document.getElementById(
    'comment-form-container'
  )
  if (!commentFormContainer) return null

  const cfForm = document.getElementById('comment-form') as HTMLFormElement
  const cfFilePath = document.getElementById(
    'cf-file-path'
  ) as HTMLInputElement
  const cfStartLine = document.getElementById(
    'cf-start-line'
  ) as HTMLInputElement
  const cfEndLine = document.getElementById('cf-end-line') as HTMLInputElement
  const cfSide = document.getElementById('cf-side') as HTMLInputElement
  const cfBody = document.getElementById('cf-body') as HTMLTextAreaElement
  const cfHeader = document.getElementById('comment-form-header')
  const cfCancel = document.getElementById('cf-cancel')

  // Current query params for building the form action URL
  const params = new URLSearchParams(window.location.search)
  const apiParams = {
    repoPath: params.get('repo') || '',
    sourceBranch: params.get('source') || '',
    targetBranch: params.get('target') || '',
    sourceCommit: params.get('source_commit') || '',
    targetCommit: params.get('target_commit') || '',
    mode: params.get('mode') || 'branches',
  }
  const filePath = params.get('file') || ''

  // Drag selection state
  let isDragging = false
  let dragStartLine = 0
  let dragStartSide = ''
  let dragCurrentLine = 0

  function highlightRange(start: number, end: number, side: string): void {
    const lo = Math.min(start, end)
    const hi = Math.max(start, end)
    const attr = side === 'left' ? 'data-left-num' : 'data-right-num'
    const rows = diffTable!.querySelectorAll('.diff-line')
    rows.forEach((row) => {
      const num = parseInt(row.getAttribute(attr) || '', 10) || 0
      if (num >= lo && num <= hi) {
        row.classList.add('line-selected')
      }
    })
  }

  function clearLineSelection(): void {
    diffTable!.querySelectorAll('.line-selected').forEach((el) => {
      el.classList.remove('line-selected')
    })
  }

  function showCommentForm(
    startLine: number,
    endLine: number,
    side: string,
    _event: Event | null
  ): void {
    const action = buildCommentApiUrl(apiParams)
    cfForm.setAttribute('action', action)

    cfFilePath.value = filePath
    cfStartLine.value = String(startLine)
    cfEndLine.value = String(endLine)
    cfSide.value = side
    cfBody.value = ''

    if (cfHeader) {
      cfHeader.textContent = buildCommentHeader(startLine, endLine, side)
    }

    // Position the form near the last selected row
    const selectedRows = diffTable!.querySelectorAll('.line-selected')
    if (selectedRows.length > 0) {
      const lastRow = selectedRows[selectedRows.length - 1]
      const rect = lastRow.getBoundingClientRect()
      commentFormContainer!.style.position = 'absolute'
      commentFormContainer!.style.top = window.scrollY + rect.bottom + 4 + 'px'
      commentFormContainer!.style.left = rect.left + 60 + 'px'
    }

    commentFormContainer!.classList.remove('hidden')
    cfBody.focus()
  }

  function hideCommentForm(): void {
    commentFormContainer!.classList.add('hidden')
    clearLineSelection()
  }

  function isCommentFormVisible(): boolean {
    return !commentFormContainer!.classList.contains('hidden')
  }

  // ── Event handlers ──

  // Click on a line number to start drag
  diffTable.addEventListener('mousedown', (e) => {
    const target = e.target as HTMLElement
    const cell = target.closest(
      '.diff-line-num[data-line-num]'
    ) as HTMLElement | null
    if (!cell) return
    const info = resolveLineInfo(cell)
    if (!info) return

    e.preventDefault()
    isDragging = true
    dragStartLine = info.lineNum
    dragStartSide = info.side
    dragCurrentLine = info.lineNum

    clearLineSelection()
    highlightRange(dragStartLine, dragCurrentLine, dragStartSide)
  })

  // Track drag movement
  diffTable.addEventListener('mousemove', (e) => {
    if (!isDragging) return
    const target = e.target as HTMLElement
    const cell = target.closest(
      '.diff-line-num[data-line-num]'
    ) as HTMLElement | null
    if (!cell) return
    const info = resolveLineInfo(cell)
    if (!info) return

    dragCurrentLine = info.lineNum
    clearLineSelection()
    highlightRange(dragStartLine, dragCurrentLine, dragStartSide)
  })

  // End drag — show comment form
  document.addEventListener('mouseup', () => {
    if (!isDragging) return
    isDragging = false

    const startLine = Math.min(dragStartLine, dragCurrentLine)
    const endLine = Math.max(dragStartLine, dragCurrentLine)

    showCommentForm(startLine, endLine, dragStartSide, null)
  })

  if (cfCancel) {
    cfCancel.addEventListener('click', hideCommentForm)
  }

  // Ctrl+Enter to save comment from textarea
  cfBody.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      cfForm.requestSubmit()
    }
  })

  // Add comment form — use fetch to avoid page jump to top
  cfForm.addEventListener('submit', (e) => {
    e.preventDefault()
    const submitBtn = cfForm.querySelector(
      '[type="submit"]'
    ) as HTMLButtonElement
    if (submitBtn.disabled) return
    submitBtn.disabled = true
    const originalText = submitBtn.innerHTML
    submitBtn.textContent = 'Saving...'
    const action = cfForm.getAttribute('action')!
    const formData = new URLSearchParams(new FormData(cfForm) as never)
    const cursor = getCursorState()
    saveCursorToHash(cursor.index, cursor.side)
    fetch(action, { method: 'POST', body: formData })
      .then((resp) => {
        if (resp.ok) {
          window.location.reload()
        } else {
          submitBtn.disabled = false
          submitBtn.innerHTML = originalText
          alert(
            'Failed to save comment (status ' +
              resp.status +
              '). Please try again.'
          )
        }
      })
      .catch(() => {
        submitBtn.disabled = false
        submitBtn.innerHTML = originalText
        alert('Network error — comment was not saved.')
      })
  })

  // Handle delete forms
  document.querySelectorAll('.delete-comment-form').forEach((form) => {
    form.addEventListener('submit', (e) => {
      e.preventDefault()
      const submitBtn = (form as HTMLElement).querySelector(
        '[type="submit"], button'
      ) as HTMLButtonElement | null
      if (submitBtn && submitBtn.disabled) return
      if (submitBtn) submitBtn.disabled = true
      const action = (form as HTMLFormElement).getAttribute('action')!
      const cursor = getCursorState()
      saveCursorToHash(cursor.index, cursor.side)
      fetch(action, { method: 'DELETE' })
        .then((resp) => {
          if (resp.ok) {
            window.location.reload()
          } else {
            if (submitBtn) submitBtn.disabled = false
            alert(
              'Failed to delete comment (status ' +
                resp.status +
                '). Please try again.'
            )
          }
        })
        .catch(() => {
          if (submitBtn) submitBtn.disabled = false
          alert('Network error — comment was not deleted.')
        })
    })
  })

  return {
    showCommentForm,
    hideCommentForm,
    clearLineSelection,
    highlightRange,
    isCommentFormVisible,
  }
}
