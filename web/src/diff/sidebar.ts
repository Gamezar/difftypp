/**
 * Sidebar navigation with back-button stack and per-file scroll position tracking.
 *
 * - Shows all changed files in a sticky sidebar when viewing a single file diff
 * - Back navigation uses a stack of previously visited files (capped at 20)
 * - Per-file scroll positions are saved/restored via sessionStorage
 */

import { showLoadingIndicator } from '../shared/loading'

const BACK_STACK_KEY = 'difftypp-back-stack'
const FILE_POSITIONS_KEY = 'difftypp-file-positions'
const MAX_BACK_STACK = 20
const HEADER_HEIGHT = 80

/* ===== Per-file scroll positions ===== */

function getFilePositions(): Record<string, number> {
  try {
    const raw = sessionStorage.getItem(FILE_POSITIONS_KEY)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

function getFilePosition(file: string): number {
  return getFilePositions()[file] || 0
}

function saveCurrentFilePosition(): void {
  const params = new URLSearchParams(window.location.search)
  const file = params.get('file')
  if (!file) return
  const line = getFirstVisibleLineNumber()
  if (line > 0) {
    const positions = getFilePositions()
    positions[file] = line
    sessionStorage.setItem(FILE_POSITIONS_KEY, JSON.stringify(positions))
  }
}

/* ===== Back stack (file names only) ===== */

function getBackStack(): string[] {
  try {
    const raw = sessionStorage.getItem(BACK_STACK_KEY)
    return raw ? JSON.parse(raw) : []
  } catch {
    return []
  }
}

function saveBackStack(stack: string[]): void {
  if (stack.length > MAX_BACK_STACK) stack = stack.slice(stack.length - MAX_BACK_STACK)
  sessionStorage.setItem(BACK_STACK_KEY, JSON.stringify(stack))
}

function pushFileToStack(file: string | null): void {
  if (!file) return
  const stack = getBackStack()
  stack.push(file)
  saveBackStack(stack)
}

function updateBackButton(
  backBtn: HTMLElement,
  backContainer: HTMLElement
): void {
  const stack = getBackStack()
  if (stack.length > 0) {
    const top = stack[stack.length - 1]
    const shortName = top.split('/').pop() || top
    const label = document.getElementById('back-label')
    if (label) label.textContent = shortName
    backBtn.title = 'Back to ' + top + ' (Backspace)'
    backContainer.classList.remove('hidden')
  } else {
    backContainer.classList.add('hidden')
  }
}

function fileUrl(file: string): string {
  const params = new URLSearchParams(window.location.search)
  params.set('file', file)
  return window.location.pathname + '?' + params.toString()
}

function goBack(): void {
  const stack = getBackStack()
  if (stack.length === 0) return
  saveCurrentFilePosition()
  const file = stack.pop()!
  saveBackStack(stack)
  showLoadingIndicator()
  window.location.href = fileUrl(file)
}

/* ===== Scroll position helpers ===== */

function getFirstVisibleLineNumber(): number {
  const diffContent = document.getElementById('diff-content')
  if (!diffContent) return 0

  const rows = diffContent.querySelectorAll('.diff-line')
  for (let i = 0; i < rows.length; i++) {
    const rect = rows[i].getBoundingClientRect()
    if (rect.bottom > HEADER_HEIGHT && rect.top < window.innerHeight) {
      const rightNum =
        parseInt(rows[i].getAttribute('data-right-num') || '', 10) || 0
      const leftNum =
        parseInt(rows[i].getAttribute('data-left-num') || '', 10) || 0
      return rightNum || leftNum
    }
  }
  return 0
}

function scrollToLine(lineNum: number): void {
  const diffContent = document.getElementById('diff-content')
  if (!diffContent) return

  const rows = diffContent.querySelectorAll('.diff-line')
  for (let i = 0; i < rows.length; i++) {
    const row = rows[i] as HTMLElement
    const rightNum =
      parseInt(row.getAttribute('data-right-num') || '', 10) || 0
    const leftNum =
      parseInt(row.getAttribute('data-left-num') || '', 10) || 0
    if (rightNum === lineNum || leftNum === lineNum) {
      const rect = row.getBoundingClientRect()
      window.scrollTo(0, window.scrollY + rect.top - HEADER_HEIGHT)
      row.style.outline = '2px solid #3b82f6'
      setTimeout(() => {
        row.style.outline = ''
      }, 1500)
      break
    }
  }
}

/* ===== Main initialization ===== */

export function initializeSidebar(): void {
  // Scroll active sidebar item into view
  const activeItem = document.querySelector('.sidebar-file-item.active')
  if (activeItem) {
    activeItem.scrollIntoView({ block: 'center' })
  }

  const params = new URLSearchParams(window.location.search)
  const currentFile = params.get('file')

  // Restore saved scroll position for current file
  if (currentFile) {
    const savedLine = getFilePosition(currentFile)
    if (savedLine > 0) {
      // Hide content to prevent visible scroll jump, scroll instantly, then reveal
      const mainContent = document.querySelector('.diff-main-content') as HTMLElement
      if (mainContent) {
        mainContent.style.visibility = 'hidden'
      }
      scrollToLine(savedLine)
      if (mainContent) {
        // Use rAF to reveal after the browser has painted at the correct scroll position
        requestAnimationFrame(() => {
          mainContent.style.visibility = ''
        })
      }
    }
  }

  // Set up back button
  const backBtn = document.getElementById('back-to-prev-file')
  const backContainer = document.getElementById('sidebar-back-container')
  if (!backBtn || !backContainer) return

  updateBackButton(backBtn, backContainer)

  // Intercept sidebar links: save position, push file to stack, navigate
  const sidebarLinks = document.querySelectorAll('.sidebar-file-link')
  sidebarLinks.forEach((link) => {
    link.addEventListener('click', (e) => {
      e.preventDefault()
      if ((link as HTMLElement).closest('.sidebar-file-item.active')) return
      saveCurrentFilePosition()
      pushFileToStack(currentFile)
      window.location.href = (link as HTMLAnchorElement).href
    })
  })

  // Also push for prev/next file buttons
  const prevBtn = document.getElementById('prev-file')
  const nextBtn = document.getElementById('next-file')
  if (prevBtn)
    prevBtn.addEventListener('click', () => {
      saveCurrentFilePosition()
      pushFileToStack(currentFile)
    })
  if (nextBtn)
    nextBtn.addEventListener('click', () => {
      saveCurrentFilePosition()
      pushFileToStack(currentFile)
    })

  // Backspace key to go back
  document.addEventListener('keydown', (e) => {
    const target = e.target as HTMLElement
    if (
      target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.tagName === 'SELECT'
    )
      return
    if (e.key === 'Backspace') {
      e.preventDefault()
      goBack()
    }
  })

  // Click handler for back button
  backBtn.addEventListener('click', (e) => {
    e.preventDefault()
    goBack()
  })
}
