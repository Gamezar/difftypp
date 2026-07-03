import { describe, it, expect } from 'vitest'
import { computeFetchRange, buildContextRow } from './context-expansion'

describe('computeFetchRange', () => {
  it('reveals one chunk from the top of a large bounded gap', () => {
    expect(
      computeFetchRange({ rightStart: 5, rightEnd: 120, leftStart: 5 }, 20)
    ).toEqual({ start: 5, end: 24 })
  })

  it('clamps the chunk to the end of a small bounded gap', () => {
    expect(
      computeFetchRange({ rightStart: 5, rightEnd: 10, leftStart: 5 }, 20)
    ).toEqual({ start: 5, end: 10 })
  })

  it('takes a full chunk for the bottom gap (null rightEnd)', () => {
    expect(
      computeFetchRange({ rightStart: 42, rightEnd: null, leftStart: 40 }, 20)
    ).toEqual({ start: 42, end: 61 })
  })

  it('returns null when nothing is hidden', () => {
    expect(
      computeFetchRange({ rightStart: 11, rightEnd: 10, leftStart: 11 }, 20)
    ).toBeNull()
  })

  it('returns null for an invalid start', () => {
    expect(
      computeFetchRange({ rightStart: 0, rightEnd: 10, leftStart: 0 }, 20)
    ).toBeNull()
  })
})

describe('buildContextRow', () => {
  it('builds a three-column unified row', () => {
    const row = buildContextRow(12, 10, 'const x = 1', 'g0', false)
    expect(row.classList.contains('diff-line')).toBe(true)
    expect(row.getAttribute('data-line-type')).toBe('context')
    expect(row.getAttribute('data-left-num')).toBe('10')
    expect(row.getAttribute('data-right-num')).toBe('12')

    const cells = row.querySelectorAll('td')
    expect(cells).toHaveLength(3)
    // num(left), num(right), content
    expect(cells[0].getAttribute('data-side')).toBe('left')
    expect(cells[1].getAttribute('data-side')).toBe('right')
    expect(cells[2].classList.contains('diff-line-content')).toBe(true)
    expect(cells[2].textContent).toBe('const x = 1')
  })

  it('builds a four-column split row with content on both sides', () => {
    const row = buildContextRow(12, 10, 'const x = 1', 'g0', true)
    const cells = row.querySelectorAll('td')
    expect(cells).toHaveLength(4)
    // Column order: left num, left content, right num, right content.
    expect(cells[0].getAttribute('data-side')).toBe('left')
    expect(cells[0].getAttribute('data-line-num')).toBe('10')
    expect(cells[1].classList.contains('diff-line-content')).toBe(true)
    expect(cells[1].textContent).toBe('const x = 1')
    expect(cells[2].getAttribute('data-side')).toBe('right')
    expect(cells[2].getAttribute('data-line-num')).toBe('12')
    expect(cells[2].classList.contains('diff-split-divider')).toBe(true)
    expect(cells[3].classList.contains('diff-line-content')).toBe(true)
    expect(cells[3].textContent).toBe('const x = 1')
  })
})
