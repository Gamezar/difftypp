/**
 * Vim-style cursor navigation for the diff view.
 *
 * Implements keyboard-driven code review with:
 * - j/k line navigation with cursor highlighting
 * - J/K visual selection for multi-line comments
 * - Comment operations (create, resolve, delete)
 * - File navigation (h/l prev/next)
 * - Review shortcuts (a/r/s approve/reject/skip)
 * - gg/G jump to first/last line
 * - ?  shortcuts overlay
 */

import { CommentSystemApi } from './comment-system'
import {
  restoreCursorFromHash,
  saveCursorToHash,
} from './cursor-persistence'
import { showLoadingIndicator, afterOverlayPaint } from '../shared/loading'

// ── Pure helper functions (exported for testing) ─────────────────

/**
 * Get the effective side for a diff line based on its type.
 * Additions are always right, deletions always left,
 * context lines use the current cursor side.
 */
export function getEffectiveSide(
  lineType: string | null,
  cursorSide: string
): string {
  if (lineType === 'addition') return 'right'
  if (lineType === 'deletion') return 'left'
  return cursorSide
}

/**
 * Get line number from a row for the given side.
 */
export function getLineNum(
  row: HTMLElement,
  side: string
): number {
  const attr = side === 'left' ? 'data-left-num' : 'data-right-num'
  return parseInt(row.getAttribute(attr) || '', 10) || 0
}

/**
 * Compute footer hint text based on current cursor state.
 */
export function computeFooterHints(opts: {
  deleteConfirming: boolean
  commentFormVisible: boolean
  cursorActive: boolean
  hasSelection: boolean
  hasCommentOnLine: boolean
}): string {
  if (opts.deleteConfirming) {
    return '[y] Confirm delete [Esc] Cancel'
  }
  if (opts.commentFormVisible) {
    return '[Ctrl+Enter] Save comment [Esc] Cancel'
  }
  if (opts.cursorActive) {
    const parts = ['[j/k] Move', '[c] Comment']
    if (opts.hasSelection) parts[0] = '[J/K] Extend'
    if (opts.hasCommentOnLine) parts.push('[x] Resolve', '[d] Delete')
    parts.push('[a/r/s] Review', '[Shift+Q] Back', '[?] Help')
    return parts.join('  ')
  }
  return '[j/k] Navigate [?] Shortcuts [a] Approve [r] Reject [s] Skip [h/l] Prev/Next file [Shift+Q] Back'
}

/**
 * Determine if a row has comment rows following it in the DOM.
 */
export function hasCommentOnLine(row: HTMLElement): boolean {
  const next = row.nextElementSibling as HTMLElement | null
  return next !== null && next.classList.contains('diff-comment-row')
}

/**
 * Get all consecutive comment rows following a diff line.
 */
export function getCommentRows(row: HTMLElement): HTMLElement[] {
  const comments: HTMLElement[] = []
  let next = row.nextElementSibling as HTMLElement | null
  while (next && next.classList.contains('diff-comment-row')) {
    comments.push(next)
    next = next.nextElementSibling as HTMLElement | null
  }
  return comments
}

/**
 * Get the first actionable comment row.
 * Prefers first open comment; falls back to first resolved.
 */
export function getFirstComment(row: HTMLElement): HTMLElement | null {
  const comments = getCommentRows(row)
  if (comments.length === 0) return null
  for (const c of comments) {
    if (!c.classList.contains('diff-comment-resolved')) return c
  }
  return comments[0]
}

/**
 * Find the next diff-line index that has comments, with wrapping.
 */
export function findNextCommentIndex(
  lines: HTMLElement[],
  fromIdx: number,
  direction: 1 | -1
): number {
  const len = lines.length
  if (len === 0) return -1

  const start = direction > 0 ? fromIdx + 1 : fromIdx - 1
  for (
    let i = start;
    direction > 0 ? i < len : i >= 0;
    i += direction
  ) {
    if (hasCommentOnLine(lines[i])) return i
  }

  // Wrap around
  const wrapStart = direction > 0 ? 0 : len - 1
  const wrapEnd = fromIdx
  for (
    let j = wrapStart;
    direction > 0 ? j <= wrapEnd : j >= wrapEnd;
    j += direction
  ) {
    if (hasCommentOnLine(lines[j])) return j
  }

  return -1
}

/**
 * Clamp a value between min and max (inclusive).
 */
export function clamp(val: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, val))
}

// ── Cursor state ─────────────────────────────────────────────────

export interface CursorState {
  index: number
  side: string
  selectionAnchor: number
  prevIndex: number
  deleteConfirming: boolean
  deleteTarget: HTMLElement | null
  pendingG: boolean
  pendingGTimer: ReturnType<typeof setTimeout> | null
}

export function createCursorState(): CursorState {
  return {
    index: -1,
    side: 'right',
    selectionAnchor: -1,
    prevIndex: -1,
    deleteConfirming: false,
    deleteTarget: null,
    pendingG: false,
    pendingGTimer: null,
  }
}

// ── DOM operations ───────────────────────────────────────────────

function applyCursor(
  state: CursorState,
  lines: HTMLElement[]
): void {
  // Clear previous cursor (O(1) instead of O(n))
  if (state.prevIndex >= 0 && state.prevIndex < lines.length) {
    const prevRow = lines[state.prevIndex]
    prevRow.classList.remove('diff-line-cursor')
    prevRow.querySelectorAll('.diff-line-num').forEach((cell) => {
      cell.classList.remove('cursor-side-active')
    })
  }
  // Also clear current row in case side changed
  if (
    state.index >= 0 &&
    state.index < lines.length &&
    state.index !== state.prevIndex
  ) {
    const curRow = lines[state.index]
    curRow.classList.remove('diff-line-cursor')
    curRow.querySelectorAll('.diff-line-num').forEach((cell) => {
      cell.classList.remove('cursor-side-active')
    })
  }
  state.prevIndex = state.index
  if (state.index < 0 || state.index >= lines.length) return
  const row = lines[state.index]
  row.classList.add('diff-line-cursor')
  const effSide = getEffectiveSide(
    row.getAttribute('data-line-type'),
    state.side
  )
  const sideCell = row.querySelector(
    `.diff-line-num[data-side="${effSide}"]`
  )
  if (sideCell) sideCell.classList.add('cursor-side-active')
}

function applySelection(
  state: CursorState,
  lines: HTMLElement[],
  commentApi: CommentSystemApi | null
): void {
  if (commentApi) commentApi.clearLineSelection()
  if (state.selectionAnchor < 0 || state.index < 0) return
  const lo = Math.min(state.selectionAnchor, state.index)
  const hi = Math.max(state.selectionAnchor, state.index)
  for (let i = lo; i <= hi; i++) {
    lines[i].classList.add('line-selected')
  }
}

function updateFooterHints(text: string): void {
  const el = document.getElementById('keyboard-hints')
  if (el) el.textContent = text
}

function toggleShortcutsOverlay(): void {
  const overlay = document.getElementById('shortcuts-overlay')
  if (overlay) overlay.classList.toggle('hidden')
}

// ── Main initialization ──────────────────────────────────────────

/**
 * Initialize cursor navigation on the diff page.
 *
 * @param commentApi - Comment system API for comment operations
 * @param options - Optional config; pass { signal } from an AbortController to
 *                  allow tearing down document-level listeners (useful in tests).
 */
export function initializeCursorNavigation(
  commentApi: CommentSystemApi | null,
  options?: { signal?: AbortSignal }
): CursorState {
  const state = createCursorState()
  const listenerOpts = options?.signal ? { signal: options.signal } : undefined

  // Wire up shortcuts overlay close button
  const overlayClose = document.getElementById('shortcuts-close')
  if (overlayClose) {
    overlayClose.addEventListener('click', () => {
      document.getElementById('shortcuts-overlay')?.classList.add('hidden')
    }, listenerOpts)
  }

  const diffTable = document.querySelector('.diff-table') as HTMLElement | null
  let diffLines: HTMLElement[] = diffTable
    ? Array.from(diffTable.querySelectorAll('.diff-line'))
    : []

  // Context expansion inserts new .diff-line rows after init. Rebuild the cached
  // list (re-anchoring the cursor to its current row) when that happens.
  function rebuildDiffLines(): void {
    if (!diffTable) return
    const currentRow =
      state.index >= 0 && state.index < diffLines.length
        ? diffLines[state.index]
        : null
    diffLines = Array.from(diffTable.querySelectorAll('.diff-line'))
    state.index = currentRow ? diffLines.indexOf(currentRow) : state.index
    state.prevIndex = state.index
    if (state.selectionAnchor >= 0) {
      state.selectionAnchor = -1
      commentApi?.clearLineSelection()
    }
  }
  document.addEventListener('diff:rows-changed', rebuildDiffLines, listenerOpts)

  // Restore cursor from hash if available
  if (diffLines.length > 0) {
    const saved = restoreCursorFromHash()
    if (saved && saved.index < diffLines.length) {
      state.index = saved.index
      state.side = saved.side
      applyCursor(state, diffLines)
      diffLines[state.index].scrollIntoView({
        behavior: 'auto',
        block: 'center',
      })
    }
  }

  function updateFooterForState(): void {
    updateFooterHints(
      computeFooterHints({
        deleteConfirming: state.deleteConfirming,
        commentFormVisible: commentApi?.isCommentFormVisible() ?? false,
        cursorActive: state.index >= 0 && diffLines.length > 0,
        hasSelection: state.selectionAnchor >= 0,
        hasCommentOnLine:
          state.index >= 0 && state.index < diffLines.length
            ? hasCommentOnLine(diffLines[state.index])
            : false,
      })
    )
  }

  function moveCursor(delta: number): void {
    if (diffLines.length === 0) return
    if (state.index < 0) {
      state.index = delta > 0 ? 0 : diffLines.length - 1
    } else {
      state.index = clamp(state.index + delta, 0, diffLines.length - 1)
    }
    state.selectionAnchor = -1
    commentApi?.clearLineSelection()
    applyCursor(state, diffLines)
    updateFooterForState()
    diffLines[state.index].scrollIntoView({
      behavior: 'smooth',
      block: 'center',
    })
  }

  function extendSelection(delta: number): void {
    if (diffLines.length === 0 || state.index < 0) return
    if (state.selectionAnchor < 0) state.selectionAnchor = state.index
    state.index = clamp(state.index + delta, 0, diffLines.length - 1)
    applyCursor(state, diffLines)
    applySelection(state, diffLines, commentApi)
    updateFooterForState()
    diffLines[state.index].scrollIntoView({
      behavior: 'smooth',
      block: 'center',
    })
  }

  function openCommentAtCursor(): void {
    if (state.index < 0 || !commentApi) return
    const row = diffLines[state.index]
    const effSide = getEffectiveSide(
      row.getAttribute('data-line-type'),
      state.side
    )

    let startIdx = state.index
    let endIdx = state.index
    if (state.selectionAnchor >= 0) {
      startIdx = Math.min(state.selectionAnchor, state.index)
      endIdx = Math.max(state.selectionAnchor, state.index)
    }

    const startRow = diffLines[startIdx]
    const endRow = diffLines[endIdx]
    const startNum = getLineNum(
      startRow,
      getEffectiveSide(startRow.getAttribute('data-line-type'), state.side)
    )
    const endNum = getLineNum(
      endRow,
      getEffectiveSide(endRow.getAttribute('data-line-type'), state.side)
    )
    if (startNum <= 0 || endNum <= 0) return

    commentApi.clearLineSelection()
    commentApi.highlightRange(startNum, endNum, effSide)
    commentApi.showCommentForm(
      Math.min(startNum, endNum),
      Math.max(startNum, endNum),
      effSide,
      null
    )
    updateFooterForState()
  }

  function resolveCommentAtCursor(): void {
    if (state.index < 0) return
    const comment = getFirstComment(diffLines[state.index])
    if (!comment) return
    const resolveBtn = comment.querySelector(
      '[data-testid^="btn-comment-resolve"], [data-testid^="btn-comment-reopen"]'
    ) as HTMLElement | null
    if (!resolveBtn) return
    const form = resolveBtn.closest('form') as HTMLFormElement | null
    if (!form) return
    const action = form.getAttribute('action')!
    saveCursorToHash(state.index, state.side)
    showLoadingIndicator()
    fetch(action, { method: 'POST' })
      .then((resp) => {
        if (resp.ok) {
          window.location.reload()
        } else {
          document
            .getElementById('loading-overlay')
            ?.classList.add('hidden')
          alert(
            'Failed to resolve/reopen comment (status ' +
              resp.status +
              ').'
          )
        }
      })
      .catch(() => {
        document
          .getElementById('loading-overlay')
          ?.classList.add('hidden')
        alert('Network error — comment was not updated.')
      })
  }

  function startDeleteConfirm(): void {
    if (state.index < 0) return
    const comment = getFirstComment(diffLines[state.index])
    if (!comment) return
    state.deleteConfirming = true
    state.deleteTarget = comment
    comment.classList.add('comment-delete-target')
    diffLines[state.index].classList.add('comment-delete-target')
    updateFooterForState()
  }

  function cancelDeleteConfirm(): void {
    if (state.deleteTarget) {
      state.deleteTarget.classList.remove('comment-delete-target')
    }
    if (state.index >= 0 && state.index < diffLines.length) {
      diffLines[state.index].classList.remove('comment-delete-target')
    }
    state.deleteConfirming = false
    state.deleteTarget = null
    updateFooterForState()
  }

  function confirmDelete(): void {
    if (!state.deleteTarget) return
    const deleteBtn = state.deleteTarget.querySelector(
      '[data-testid^="btn-comment-delete"]'
    ) as HTMLElement | null
    if (!deleteBtn) return
    state.deleteTarget.classList.remove('comment-delete-target')
    if (state.index >= 0 && state.index < diffLines.length) {
      diffLines[state.index].classList.remove('comment-delete-target')
    }
    saveCursorToHash(state.index, state.side)
    const form = deleteBtn.closest('.delete-comment-form') as HTMLFormElement
    if (form) form.requestSubmit()
    state.deleteConfirming = false
    state.deleteTarget = null
  }

  function clearPendingG(): void {
    state.pendingG = false
    if (state.pendingGTimer) {
      clearTimeout(state.pendingGTimer)
      state.pendingGTimer = null
    }
  }

  function handleG(): boolean {
    if (state.pendingG) {
      clearPendingG()
      if (diffLines.length > 0) {
        state.index = 0
        state.selectionAnchor = -1
        commentApi?.clearLineSelection()
        applyCursor(state, diffLines)
        updateFooterForState()
        diffLines[0].scrollIntoView({ behavior: 'smooth', block: 'start' })
      }
      return true
    } else {
      state.pendingG = true
      state.pendingGTimer = setTimeout(() => {
        state.pendingG = false
      }, 500)
      return true
    }
  }

  // ── Keyboard sub-handlers ──
  // Each returns true if the event was handled.

  function handleEscapeKey(event: KeyboardEvent): void {
    event.preventDefault()
    const overlay = document.getElementById('shortcuts-overlay')
    if (overlay && !overlay.classList.contains('hidden')) {
      overlay.classList.add('hidden')
      return
    }
    if (state.deleteConfirming) {
      cancelDeleteConfirm()
      return
    }
    if (commentApi?.isCommentFormVisible()) {
      commentApi.hideCommentForm()
      updateFooterForState()
      return
    }
    if (state.selectionAnchor >= 0) {
      state.selectionAnchor = -1
      commentApi?.clearLineSelection()
      applyCursor(state, diffLines)
      updateFooterForState()
      return
    }
    if (state.index >= 0) {
      state.index = -1
      applyCursor(state, diffLines)
      updateFooterForState()
    }
  }

  function handleShortcutsKey(event: KeyboardEvent): boolean {
    if (event.key !== '?') return false
    event.preventDefault()
    clearPendingG()
    toggleShortcutsOverlay()
    return true
  }

  function handleDeleteConfirmKeys(event: KeyboardEvent): boolean {
    if (!state.deleteConfirming) return false
    if (event.key === 'y') {
      event.preventDefault()
      confirmDelete()
    } else {
      cancelDeleteConfirm()
    }
    return true
  }

  function handleDiffNavigationKeys(event: KeyboardEvent): boolean {
    if (diffLines.length === 0) return false

    // g/G jump handling
    if (event.key === 'g' && !event.shiftKey) {
      if (handleG()) {
        event.preventDefault()
        return true
      }
    } else {
      clearPendingG()
    }

    if (event.key === 'G' && event.shiftKey) {
      event.preventDefault()
      state.index = diffLines.length - 1
      state.selectionAnchor = -1
      commentApi?.clearLineSelection()
      applyCursor(state, diffLines)
      updateFooterForState()
      diffLines[state.index].scrollIntoView({
        behavior: 'smooth',
        block: 'end',
      })
      return true
    }

    // j/k — move cursor
    if (
      (event.key === 'j' || event.key === 'ArrowDown') &&
      !event.shiftKey
    ) {
      event.preventDefault()
      moveCursor(1)
      return true
    }
    if (
      (event.key === 'k' || event.key === 'ArrowUp') &&
      !event.shiftKey
    ) {
      event.preventDefault()
      moveCursor(-1)
      return true
    }

    // Shift+j/k — extend selection
    if (
      event.key === 'J' ||
      (event.key === 'ArrowDown' && event.shiftKey)
    ) {
      event.preventDefault()
      extendSelection(1)
      return true
    }
    if (
      event.key === 'K' ||
      (event.key === 'ArrowUp' && event.shiftKey)
    ) {
      event.preventDefault()
      extendSelection(-1)
      return true
    }

    // Tab — toggle side on context lines
    if (event.key === 'Tab' && state.index >= 0) {
      event.preventDefault()
      const curRow = diffLines[state.index]
      if (curRow.getAttribute('data-line-type') === 'context') {
        state.side = state.side === 'right' ? 'left' : 'right'
        applyCursor(state, diffLines)
        updateFooterForState()
      }
      return true
    }

    return false
  }

  function handleDiffCommentKeys(event: KeyboardEvent): boolean {
    if (diffLines.length === 0) return false

    // c — open comment
    if (event.key === 'c' && !event.ctrlKey && !event.metaKey) {
      event.preventDefault()
      openCommentAtCursor()
      return true
    }

    // x — resolve/reopen
    if (event.key === 'x') {
      if (
        state.index >= 0 &&
        hasCommentOnLine(diffLines[state.index])
      ) {
        event.preventDefault()
        resolveCommentAtCursor()
        return true
      }
    }

    // d — delete (with confirmation)
    if (event.key === 'd' && !event.ctrlKey && !event.metaKey) {
      if (
        state.index >= 0 &&
        hasCommentOnLine(diffLines[state.index])
      ) {
        event.preventDefault()
        startDeleteConfirm()
        return true
      }
    }

    // ] — next comment
    if (event.key === ']') {
      event.preventDefault()
      const fromIdx = state.index >= 0 ? state.index : -1
      const nextIdx = findNextCommentIndex(diffLines, fromIdx, 1)
      if (nextIdx >= 0) {
        state.index = nextIdx
        state.selectionAnchor = -1
        commentApi?.clearLineSelection()
        applyCursor(state, diffLines)
        updateFooterForState()
        diffLines[state.index].scrollIntoView({
          behavior: 'smooth',
          block: 'center',
        })
      }
      return true
    }
    // [ — prev comment
    if (event.key === '[') {
      event.preventDefault()
      const fromIdxP = state.index >= 0 ? state.index : diffLines.length
      const prevIdx = findNextCommentIndex(diffLines, fromIdxP, -1)
      if (prevIdx >= 0) {
        state.index = prevIdx
        state.selectionAnchor = -1
        commentApi?.clearLineSelection()
        applyCursor(state, diffLines)
        updateFooterForState()
        diffLines[state.index].scrollIntoView({
          behavior: 'smooth',
          block: 'center',
        })
      }
      return true
    }

    return false
  }

  function handleFileNavKeys(event: KeyboardEvent): boolean {
    // Shift+Q - back to compare selection
    if (event.key === 'Q' && event.shiftKey) {
      const backToCompareLink = document.getElementById(
        'back-to-compare-link'
      ) as HTMLElement | null
      if (backToCompareLink) {
        event.preventDefault()
        showLoadingIndicator()
        afterOverlayPaint(() => {
          backToCompareLink.click()
        })
        return true
      }
    }

    // h/l and ArrowLeft/ArrowRight — prev/next file
    if (event.key === 'h' || event.key === 'ArrowLeft') {
      if (document.getElementById('prev-file-link')) {
        event.preventDefault()
        showLoadingIndicator()
        afterOverlayPaint(() => {
          document.getElementById('prev-file-link')?.click()
        })
        return true
      }
    }
    if (event.key === 'l' || event.key === 'ArrowRight') {
      if (document.getElementById('next-file-link')) {
        event.preventDefault()
        showLoadingIndicator()
        afterOverlayPaint(() => {
          document.getElementById('next-file-link')?.click()
        })
        return true
      }
    }

    // S (shift+s) — submit review
    if (event.key === 'S' && event.shiftKey) {
      const submitBtn = document.querySelector(
        '[data-testid="btn-submit-review"]'
      ) as HTMLElement | null
      if (submitBtn) {
        event.preventDefault()
        const form = submitBtn.closest('form') as HTMLFormElement
        form.submit()
        return true
      }
    }

    return false
  }

  function handleReviewKeys(event: KeyboardEvent): boolean {
    const approveForm = document.querySelector(
      'form[action*="status=approved"]'
    ) as HTMLFormElement | null
    if (!approveForm) return false

    if (event.key === 'a' && !event.ctrlKey && !event.metaKey) {
      event.preventDefault()
      showLoadingIndicator()
      afterOverlayPaint(() => {
        ;(
          document.querySelector(
            'form[action*="status=approved"]'
          ) as HTMLFormElement
        ).submit()
      })
      return true
    }
    if (event.key === 'r' && !event.ctrlKey && !event.metaKey) {
      event.preventDefault()
      showLoadingIndicator()
      afterOverlayPaint(() => {
        ;(
          document.querySelector(
            'form[action*="status=rejected"]'
          ) as HTMLFormElement
        ).submit()
      })
      return true
    }
    if (
      event.key === 's' &&
      !event.ctrlKey &&
      !event.metaKey &&
      !event.shiftKey
    ) {
      event.preventDefault()
      showLoadingIndicator()
      afterOverlayPaint(() => {
        ;(
          document.querySelector(
            'form[action*="status=skipped"]'
          ) as HTMLFormElement
        ).submit()
      })
      return true
    }

    return false
  }

  function handleFileListKeys(event: KeyboardEvent): boolean {
    const filesList = document.getElementById('files-list')
    if (!filesList) return false

    const files = Array.from(
      filesList.querySelectorAll('li:not(.hidden)')
    ) as HTMLElement[]
    if (files.length === 0) return false

    if (
      event.key === 'j' ||
      event.key === 'ArrowDown' ||
      event.key === 'k' ||
      event.key === 'ArrowUp'
    ) {
      event.preventDefault()
      let currentIndex = -1
      for (let i = 0; i < files.length; i++) {
        if (files[i].classList.contains('bg-gray-100')) {
          currentIndex = i
          files[i].classList.remove('bg-gray-100')
          break
        }
      }
      let newIndex = currentIndex
      if (event.key === 'j' || event.key === 'ArrowDown') {
        newIndex = (currentIndex + 1) % files.length
      } else {
        newIndex = (currentIndex - 1 + files.length) % files.length
      }
      files[newIndex].classList.add('bg-gray-100')
      files[newIndex].scrollIntoView({
        behavior: 'smooth',
        block: 'nearest',
      })
      return true
    }

    if (event.key === 'Enter') {
      for (const file of files) {
        if (file.classList.contains('bg-gray-100')) {
          const viewLink = file.querySelector('a') as HTMLAnchorElement | null
          if (viewLink) {
            showLoadingIndicator()
            afterOverlayPaint(() => {
              viewLink.click()
            })
          }
          break
        }
      }
      return true
    }

    return false
  }

  // ── Main keydown dispatcher ──

  document.addEventListener('keydown', (event) => {
    const target = event.target as HTMLElement
    const inTextInput =
      target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.tagName === 'SELECT'

    // Escape always works, even in text inputs
    if (event.key === 'Escape') {
      handleEscapeKey(event)
      return
    }

    // Skip all other keys when typing in inputs
    if (inTextInput) return

    if (handleShortcutsKey(event)) return
    if (handleDeleteConfirmKeys(event)) return
    if (handleDiffNavigationKeys(event)) return
    if (handleDiffCommentKeys(event)) return
    if (handleFileNavKeys(event)) return
    if (handleReviewKeys(event)) return
    handleFileListKeys(event)
  }, listenerOpts)

  // Review form loading indicator
  document.querySelectorAll('.review-form').forEach((form) => {
    form.addEventListener('submit', function (this: HTMLFormElement, event) {
      event.preventDefault()
      showLoadingIndicator()
      const self = this
      afterOverlayPaint(() => {
        self.submit()
      })
    })
  })

  // Prev/Next file buttons (mouse click)
  const prevFileBtn = document.getElementById('prev-file')
  const nextFileBtn = document.getElementById('next-file')

  if (prevFileBtn && document.getElementById('prev-file-link')) {
    prevFileBtn.addEventListener('click', () => {
      showLoadingIndicator()
      afterOverlayPaint(() => {
        document.getElementById('prev-file-link')?.click()
      })
    })
  }

  if (nextFileBtn && document.getElementById('next-file-link')) {
    nextFileBtn.addEventListener('click', () => {
      showLoadingIndicator()
      afterOverlayPaint(() => {
        document.getElementById('next-file-link')?.click()
      })
    })
  }

  // Initial footer update
  updateFooterForState()

  return state
}
