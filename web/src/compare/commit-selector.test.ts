import { describe, it, expect, beforeEach } from 'vitest'
import {
  computeRowState,
  computeNextSelection,
  buildBadgeHtml,
  buildCommitRow,
  shortHash,
  BASE_CLASSES,
  initializeCommitSelector,
} from './commit-selector'
import type { CommitSelection } from './commit-selector'

// ── Pure function tests ──────────────────────────────────────────

describe('computeRowState', () => {
  it('returns target when hash matches target', () => {
    const selection: CommitSelection = { target: 'abc123', source: 'def456' }
    expect(computeRowState('abc123', selection)).toBe('target')
  })

  it('returns source when hash matches source', () => {
    const selection: CommitSelection = { target: 'abc123', source: 'def456' }
    expect(computeRowState('def456', selection)).toBe('source')
  })

  it('returns none when hash matches neither', () => {
    const selection: CommitSelection = { target: 'abc123', source: 'def456' }
    expect(computeRowState('ghi789', selection)).toBe('none')
  })

  it('returns none when both target and source are empty', () => {
    const selection: CommitSelection = { target: '', source: '' }
    expect(computeRowState('abc123', selection)).toBe('none')
  })

  it('prioritizes target when target and source are the same hash', () => {
    const selection: CommitSelection = { target: 'abc123', source: 'abc123' }
    expect(computeRowState('abc123', selection)).toBe('target')
  })
})

describe('computeNextSelection', () => {
  it('deselects target when clicking current target', () => {
    const selection: CommitSelection = { target: 'abc123', source: 'def456' }
    expect(computeNextSelection('abc123', selection)).toEqual({
      target: '',
      source: 'def456',
    })
  })

  it('deselects source when clicking current source', () => {
    const selection: CommitSelection = { target: 'abc123', source: 'def456' }
    expect(computeNextSelection('def456', selection)).toEqual({
      target: 'abc123',
      source: '',
    })
  })

  it('sets target when target is empty', () => {
    const selection: CommitSelection = { target: '', source: '' }
    expect(computeNextSelection('abc123', selection)).toEqual({
      target: 'abc123',
      source: '',
    })
  })

  it('sets source when target is set but source is empty', () => {
    const selection: CommitSelection = { target: 'abc123', source: '' }
    expect(computeNextSelection('def456', selection)).toEqual({
      target: 'abc123',
      source: 'def456',
    })
  })

  it('returns unchanged selection when both are already set', () => {
    const selection: CommitSelection = { target: 'abc123', source: 'def456' }
    expect(computeNextSelection('ghi789', selection)).toEqual({
      target: 'abc123',
      source: 'def456',
    })
  })
})

describe('buildBadgeHtml', () => {
  it('creates a target badge span', () => {
    const el = buildBadgeHtml('target')
    expect(el.tagName).toBe('SPAN')
    expect(el.textContent).toBe('Target')
    expect(el.className).toContain('commit-badge')
    expect(el.className).toContain('bg-blue-100')
    expect(el.className).toContain('text-blue-700')
  })

  it('creates a source badge span', () => {
    const el = buildBadgeHtml('source')
    expect(el.tagName).toBe('SPAN')
    expect(el.textContent).toBe('Source')
    expect(el.className).toContain('commit-badge')
    expect(el.className).toContain('bg-green-100')
    expect(el.className).toContain('text-green-700')
  })
})

// ── DOM integration tests ────────────────────────────────────────

describe('initializeCommitSelector (DOM integration)', () => {
  const COMMITS_HTML = `
    <input type="text" id="target" value="">
    <input type="text" id="source" value="">
    <ul id="commits-list">
      <li data-hash="abc123" class="${BASE_CLASSES} hover:bg-gray-50">
        <code>abc123</code>
        <span>First commit</span>
      </li>
      <li data-hash="def456" class="${BASE_CLASSES} hover:bg-gray-50">
        <code>def456</code>
        <span>Second commit</span>
      </li>
      <li data-hash="ghi789" class="${BASE_CLASSES} hover:bg-gray-50">
        <code>ghi789</code>
        <span>Third commit</span>
      </li>
    </ul>
  `

  beforeEach(() => {
    document.body.innerHTML = COMMITS_HTML
  })

  it('does nothing when commits-list is missing', () => {
    document.body.innerHTML = ''
    expect(() => initializeCommitSelector()).not.toThrow()
  })

  it('selects first click as target', () => {
    initializeCommitSelector()
    const row = document.querySelector('li[data-hash="abc123"]')!
    row.dispatchEvent(new Event('click', { bubbles: true }))

    const targetInput = document.getElementById('target') as HTMLInputElement
    expect(targetInput.value).toBe('abc123')
    expect(row.className).toContain('bg-blue-50')
    expect(row.querySelector('.commit-badge')!.textContent).toBe('Target')
  })

  it('selects second click as source', () => {
    initializeCommitSelector()
    const row1 = document.querySelector('li[data-hash="abc123"]')!
    const row2 = document.querySelector('li[data-hash="def456"]')!

    row1.dispatchEvent(new Event('click', { bubbles: true }))
    row2.dispatchEvent(new Event('click', { bubbles: true }))

    const targetInput = document.getElementById('target') as HTMLInputElement
    const sourceInput = document.getElementById('source') as HTMLInputElement
    expect(targetInput.value).toBe('abc123')
    expect(sourceInput.value).toBe('def456')
    expect(row1.className).toContain('bg-blue-50')
    expect(row2.className).toContain('bg-green-50')
  })

  it('deselects target when clicking it again', () => {
    initializeCommitSelector()
    const row = document.querySelector('li[data-hash="abc123"]')!

    row.dispatchEvent(new Event('click', { bubbles: true }))
    expect(row.className).toContain('bg-blue-50')

    row.dispatchEvent(new Event('click', { bubbles: true }))
    expect(row.className).toContain('hover:bg-gray-50')
    expect(row.querySelector('.commit-badge')).toBeNull()
  })

  it('deselects source when clicking it again', () => {
    initializeCommitSelector()
    const row1 = document.querySelector('li[data-hash="abc123"]')!
    const row2 = document.querySelector('li[data-hash="def456"]')!

    row1.dispatchEvent(new Event('click', { bubbles: true }))
    row2.dispatchEvent(new Event('click', { bubbles: true }))
    expect(row2.className).toContain('bg-green-50')

    row2.dispatchEvent(new Event('click', { bubbles: true }))
    expect(row2.className).toContain('hover:bg-gray-50')
    expect(row2.querySelector('.commit-badge')).toBeNull()
  })

  it('ignores third click when both target and source are set', () => {
    initializeCommitSelector()
    const row1 = document.querySelector('li[data-hash="abc123"]')!
    const row2 = document.querySelector('li[data-hash="def456"]')!
    const row3 = document.querySelector('li[data-hash="ghi789"]')!

    row1.dispatchEvent(new Event('click', { bubbles: true }))
    row2.dispatchEvent(new Event('click', { bubbles: true }))
    row3.dispatchEvent(new Event('click', { bubbles: true }))

    expect(row3.className).toContain('hover:bg-gray-50')
    expect(row3.querySelector('.commit-badge')).toBeNull()
  })

  it('updates hidden input values on selection', () => {
    initializeCommitSelector()
    const row1 = document.querySelector('li[data-hash="abc123"]')!
    const row2 = document.querySelector('li[data-hash="def456"]')!

    row1.dispatchEvent(new Event('click', { bubbles: true }))
    row2.dispatchEvent(new Event('click', { bubbles: true }))

    const targetInput = document.getElementById('target') as HTMLInputElement
    const sourceInput = document.getElementById('source') as HTMLInputElement
    expect(targetInput.value).toBe('abc123')
    expect(sourceInput.value).toBe('def456')
  })

  it('removes existing badge before adding new one', () => {
    initializeCommitSelector()
    const row = document.querySelector('li[data-hash="abc123"]')!

    // Select as target
    row.dispatchEvent(new Event('click', { bubbles: true }))
    expect(row.querySelectorAll('.commit-badge')).toHaveLength(1)

    // Deselect and reselect — should still have only one badge
    row.dispatchEvent(new Event('click', { bubbles: true }))
    row.dispatchEvent(new Event('click', { bubbles: true }))
    expect(row.querySelectorAll('.commit-badge')).toHaveLength(1)
  })

  it('applies correct CSS classes for unselected rows', () => {
    initializeCommitSelector()
    const row1 = document.querySelector('li[data-hash="abc123"]')!
    const row2 = document.querySelector('li[data-hash="def456"]')!

    row1.dispatchEvent(new Event('click', { bubbles: true }))

    // Unselected row should have hover style
    expect(row2.className).toContain('hover:bg-gray-50')
    expect(row2.className).not.toContain('bg-blue-50')
    expect(row2.className).not.toContain('bg-green-50')
  })

  it('returns null when there is no commit list', () => {
    document.body.innerHTML = ''
    expect(initializeCommitSelector()).toBeNull()
  })

  it('returns a controller exposing the current hashes', () => {
    const controller = initializeCommitSelector()!
    expect(controller.currentHashes()).toEqual(['abc123', 'def456', 'ghi789'])
  })
})

// ── Row construction ─────────────────────────────────────────────

describe('shortHash', () => {
  it('truncates long hashes to 8 chars', () => {
    expect(shortHash('0123456789abcdef')).toBe('01234567')
  })

  it('leaves short hashes untouched', () => {
    expect(shortHash('abc')).toBe('abc')
  })
})

describe('buildCommitRow', () => {
  it('builds an li matching the server markup', () => {
    const li = buildCommitRow({ hash: '0123456789ab', subject: 'Fix bug' })
    expect(li.tagName).toBe('LI')
    expect(li.getAttribute('data-hash')).toBe('0123456789ab')
    expect(li.getAttribute('data-testid')).toBe('commit-row')
    expect(li.querySelector('code')!.textContent).toBe('01234567')
    expect(li.querySelector('span')!.textContent).toBe('Fix bug')
  })
})

// ── Live refresh ─────────────────────────────────────────────────

describe('commit selector refresh', () => {
  const COMMITS_HTML = `
    <input type="text" id="target" value="">
    <input type="text" id="source" value="">
    <ul id="commits-list">
      <li data-hash="abc123" data-testid="commit-row" class="${BASE_CLASSES} hover:bg-gray-50">
        <code>abc123</code><span>First</span>
      </li>
      <li data-hash="def456" data-testid="commit-row" class="${BASE_CLASSES} hover:bg-gray-50">
        <code>def456</code><span>Second</span>
      </li>
    </ul>
  `

  beforeEach(() => {
    document.body.innerHTML = COMMITS_HTML
  })

  it('prepends a new commit and flashes only the new row', () => {
    const controller = initializeCommitSelector()!
    controller.refresh([
      { hash: 'new789', subject: 'Newest' },
      { hash: 'abc123', subject: 'First' },
      { hash: 'def456', subject: 'Second' },
    ])

    expect(controller.currentHashes()).toEqual(['new789', 'abc123', 'def456'])
    const newRow = document.querySelector('li[data-hash="new789"]')!
    const oldRow = document.querySelector('li[data-hash="abc123"]')!
    expect(newRow.className).toContain('commit-row-new')
    expect(oldRow.className).not.toContain('commit-row-new')
  })

  it('preserves the current selection across a refresh', () => {
    const controller = initializeCommitSelector()!
    // Select abc123 as target.
    document
      .querySelector('li[data-hash="abc123"]')!
      .dispatchEvent(new Event('click', { bubbles: true }))

    controller.refresh([
      { hash: 'new789', subject: 'Newest' },
      { hash: 'abc123', subject: 'First' },
      { hash: 'def456', subject: 'Second' },
    ])

    const targetInput = document.getElementById('target') as HTMLInputElement
    expect(targetInput.value).toBe('abc123')
    const reselected = document.querySelector('li[data-hash="abc123"]')!
    expect(reselected.className).toContain('bg-blue-50')
    expect(reselected.querySelector('.commit-badge')!.textContent).toBe('Target')
  })

  it('keeps click selection working on refreshed rows', () => {
    const controller = initializeCommitSelector()!
    controller.refresh([
      { hash: 'new789', subject: 'Newest' },
      { hash: 'abc123', subject: 'First' },
    ])

    document
      .querySelector('li[data-hash="new789"]')!
      .dispatchEvent(new Event('click', { bubbles: true }))

    const targetInput = document.getElementById('target') as HTMLInputElement
    expect(targetInput.value).toBe('new789')
  })
})
