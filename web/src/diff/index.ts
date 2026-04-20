/**
 * Diff page entry point.
 *
 * Initializes all diff page subsystems:
 * - Comment system (inline code review)
 * - Cursor navigation (vim-style keyboard nav)
 * - Status filter (file list filtering)
 * - Sidebar navigation (file list sidebar with back button)
 */

import { initializeCommentSystem } from './comment-system'
import { CursorState, initializeCursorNavigation } from './cursor-navigation'
import { initializeSidebar } from './sidebar'
import { initializeStatusFilter } from './status-filter'
import { initializeSyntaxHighlight } from './syntax-highlight'

document.addEventListener('DOMContentLoaded', () => {
  // Cursor state is created by initializeCursorNavigation (phase 2).
  // The comment system (phase 1) receives a lazy getter so it can read
  // the live cursor position at event-handling time without polling.
  let cursorState: CursorState | null = null
  const getCursorState = () => cursorState ?? { index: -1, side: 'right' }

  // Phase 1: comment system (provides API for cursor navigation)
  const commentApi = initializeCommentSystem(getCursorState)

  // Phase 2: cursor navigation (owns the authoritative cursor state)
  cursorState = initializeCursorNavigation(commentApi)

  initializeStatusFilter()
  initializeSidebar()
  initializeSyntaxHighlight()
})
