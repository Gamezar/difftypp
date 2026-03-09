import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  BACK_STACK_KEY,
  FILE_POSITIONS_KEY,
  MAX_BACK_STACK,
  getBackStack,
  saveBackStack,
  pushFileToStack,
  getFilePositions,
  getFilePosition,
  updateBackButton,
  fileUrl,
  getFirstVisibleLineNumber,
  initializeSidebar,
} from './sidebar'

// ── Pure function tests ──────────────────────────────────────────

describe('getBackStack', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('returns empty array when no stored data', () => {
    expect(getBackStack()).toEqual([])
  })

  it('returns stored stack', () => {
    sessionStorage.setItem(BACK_STACK_KEY, JSON.stringify(['a.ts', 'b.ts']))
    expect(getBackStack()).toEqual(['a.ts', 'b.ts'])
  })

  it('returns empty array on invalid JSON', () => {
    sessionStorage.setItem(BACK_STACK_KEY, 'not-json')
    expect(getBackStack()).toEqual([])
  })
})

describe('saveBackStack', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('saves stack to sessionStorage', () => {
    saveBackStack(['file1.ts', 'file2.ts'])
    expect(JSON.parse(sessionStorage.getItem(BACK_STACK_KEY)!)).toEqual([
      'file1.ts',
      'file2.ts',
    ])
  })

  it('trims stack to MAX_BACK_STACK when oversized', () => {
    const oversized = Array.from({ length: MAX_BACK_STACK + 5 }, (_, i) => `file${i}.ts`)
    saveBackStack(oversized)
    const stored = JSON.parse(sessionStorage.getItem(BACK_STACK_KEY)!)
    expect(stored).toHaveLength(MAX_BACK_STACK)
    // Should keep the last MAX_BACK_STACK entries (most recent)
    expect(stored[0]).toBe('file5.ts')
    expect(stored[stored.length - 1]).toBe(`file${MAX_BACK_STACK + 4}.ts`)
  })

  it('saves empty stack', () => {
    saveBackStack([])
    expect(JSON.parse(sessionStorage.getItem(BACK_STACK_KEY)!)).toEqual([])
  })
})

describe('pushFileToStack', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('pushes file onto empty stack', () => {
    pushFileToStack('hello.ts')
    expect(getBackStack()).toEqual(['hello.ts'])
  })

  it('pushes file onto existing stack', () => {
    saveBackStack(['a.ts'])
    pushFileToStack('b.ts')
    expect(getBackStack()).toEqual(['a.ts', 'b.ts'])
  })

  it('does nothing when file is null', () => {
    pushFileToStack(null)
    expect(getBackStack()).toEqual([])
  })
})

// ── File positions ───────────────────────────────────────────────

describe('getFilePositions', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('returns empty object when no stored data', () => {
    expect(getFilePositions()).toEqual({})
  })

  it('returns stored positions', () => {
    sessionStorage.setItem(FILE_POSITIONS_KEY, JSON.stringify({ 'a.ts': 42 }))
    expect(getFilePositions()).toEqual({ 'a.ts': 42 })
  })

  it('returns empty object on invalid JSON', () => {
    sessionStorage.setItem(FILE_POSITIONS_KEY, 'broken')
    expect(getFilePositions()).toEqual({})
  })
})

describe('getFilePosition', () => {
  beforeEach(() => {
    sessionStorage.clear()
  })

  it('returns 0 for unknown file', () => {
    expect(getFilePosition('unknown.ts')).toBe(0)
  })

  it('returns saved line number', () => {
    sessionStorage.setItem(FILE_POSITIONS_KEY, JSON.stringify({ 'main.go': 55 }))
    expect(getFilePosition('main.go')).toBe(55)
  })
})

// ── updateBackButton ─────────────────────────────────────────────

describe('updateBackButton', () => {
  beforeEach(() => {
    sessionStorage.clear()
    document.body.innerHTML = ''
  })

  it('hides container when stack is empty', () => {
    const btn = document.createElement('a')
    const container = document.createElement('div')
    updateBackButton(btn, container)
    expect(container.classList.contains('hidden')).toBe(true)
  })

  it('shows container and sets label when stack has items', () => {
    saveBackStack(['src/components/Widget.tsx'])
    const btn = document.createElement('a')
    const container = document.createElement('div')
    container.classList.add('hidden')
    const label = document.createElement('span')
    label.id = 'back-label'
    document.body.appendChild(label)

    updateBackButton(btn, container)

    expect(container.classList.contains('hidden')).toBe(false)
    expect(label.textContent).toBe('Widget.tsx')
    expect(btn.title).toBe('Back to src/components/Widget.tsx (Backspace)')
  })

  it('uses full name when no path separator', () => {
    saveBackStack(['README.md'])
    const btn = document.createElement('a')
    const container = document.createElement('div')
    const label = document.createElement('span')
    label.id = 'back-label'
    document.body.appendChild(label)

    updateBackButton(btn, container)
    expect(label.textContent).toBe('README.md')
  })
})

// ── fileUrl ──────────────────────────────────────────────────────

describe('fileUrl', () => {
  beforeEach(() => {
    history.replaceState(null, '', '/diff?repo=/tmp/repo&source=main&target=dev')
  })

  it('sets file param in current URL', () => {
    const url = fileUrl('src/app.ts')
    expect(url).toContain('file=src%2Fapp.ts')
    expect(url).toContain('repo=%2Ftmp%2Frepo')
    expect(url.startsWith('/diff?')).toBe(true)
  })

  it('replaces existing file param', () => {
    history.replaceState(null, '', '/diff?repo=/tmp/repo&file=old.ts')
    const url = fileUrl('new.ts')
    expect(url).toContain('file=new.ts')
    expect(url).not.toContain('file=old.ts')
  })
})

// ── getFirstVisibleLineNumber ────────────────────────────────────

describe('getFirstVisibleLineNumber', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })

  it('returns 0 when no diff-content element exists', () => {
    expect(getFirstVisibleLineNumber()).toBe(0)
  })

  it('returns 0 when diff-content has no rows', () => {
    const div = document.createElement('div')
    div.id = 'diff-content'
    document.body.appendChild(div)
    expect(getFirstVisibleLineNumber()).toBe(0)
  })
})

// ── initializeSidebar DOM integration ────────────────────────────

describe('initializeSidebar', () => {
  beforeEach(() => {
    sessionStorage.clear()
    document.body.innerHTML = ''
    history.replaceState(null, '', '/diff?repo=/tmp/repo&file=src/main.ts')
  })

  it('does not throw when sidebar elements are absent', () => {
    expect(() => initializeSidebar()).not.toThrow()
  })

  it('hides back container when stack is empty', () => {
    document.body.innerHTML = `
      <div id="sidebar-back-container" class="hidden">
        <a id="back-to-prev-file"><span id="back-label">Back</span></a>
      </div>
    `
    initializeSidebar()
    expect(
      document.getElementById('sidebar-back-container')!.classList.contains('hidden')
    ).toBe(true)
  })

  it('shows back container when stack has items', () => {
    saveBackStack(['prev.ts'])
    document.body.innerHTML = `
      <div id="sidebar-back-container" class="hidden">
        <a id="back-to-prev-file"><span id="back-label">Back</span></a>
      </div>
    `
    initializeSidebar()
    expect(
      document.getElementById('sidebar-back-container')!.classList.contains('hidden')
    ).toBe(false)
    expect(document.getElementById('back-label')!.textContent).toBe('prev.ts')
  })

  it('intercepts sidebar link clicks and pushes current file to stack', () => {
    document.body.innerHTML = `
      <div id="sidebar-back-container" class="hidden">
        <a id="back-to-prev-file"><span id="back-label">Back</span></a>
      </div>
      <ul>
        <li class="sidebar-file-item">
          <a class="sidebar-file-link" href="/diff?repo=/tmp/repo&file=other.ts">other.ts</a>
        </li>
      </ul>
    `
    initializeSidebar()

    const link = document.querySelector('.sidebar-file-link') as HTMLAnchorElement
    link.click()

    // Current file should be pushed to the back stack
    expect(getBackStack()).toEqual(['src/main.ts'])
  })

  it('does not push when clicking active item', () => {
    document.body.innerHTML = `
      <div id="sidebar-back-container" class="hidden">
        <a id="back-to-prev-file"><span id="back-label">Back</span></a>
      </div>
      <ul>
        <li class="sidebar-file-item active">
          <a class="sidebar-file-link" href="/diff?repo=/tmp/repo&file=src/main.ts">main.ts</a>
        </li>
      </ul>
    `
    initializeSidebar()

    const link = document.querySelector('.sidebar-file-link') as HTMLAnchorElement
    link.click()

    expect(getBackStack()).toEqual([])
  })

  it('backspace key triggers goBack', () => {
    saveBackStack(['previous.ts'])
    document.body.innerHTML = `
      <div id="sidebar-back-container">
        <a id="back-to-prev-file"><span id="back-label">Back</span></a>
      </div>
    `
    // Prevent actual navigation
    const originalHref = Object.getOwnPropertyDescriptor(window, 'location')
    const locationMock = { ...window.location, set href(_: string) {} }
    Object.defineProperty(window, 'location', {
      value: locationMock,
      writable: true,
    })

    initializeSidebar()

    const event = new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true })
    document.dispatchEvent(event)

    // Stack should be popped
    expect(getBackStack()).toEqual([])

    // Restore
    if (originalHref) {
      Object.defineProperty(window, 'location', originalHref)
    }
  })

  it('backspace is ignored when focus is on input', () => {
    saveBackStack(['previous.ts'])
    document.body.innerHTML = `
      <div id="sidebar-back-container">
        <a id="back-to-prev-file"><span id="back-label">Back</span></a>
      </div>
      <input id="test-input" type="text" />
    `
    initializeSidebar()

    const input = document.getElementById('test-input')!
    const event = new KeyboardEvent('keydown', {
      key: 'Backspace',
      bubbles: true,
    })
    Object.defineProperty(event, 'target', { value: input })
    document.dispatchEvent(event)

    // Stack should NOT be popped
    expect(getBackStack()).toEqual(['previous.ts'])
  })
})
