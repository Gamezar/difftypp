import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import {
  computeSplitRatio,
  readStoredRatio,
  initializeSplitResizer,
} from './split-view'

describe('computeSplitRatio', () => {
  // wrapper: left=0, width=1000, gutters=100 each → content area 800 (100..900)
  const rect = { left: 0, width: 1000 }

  it('returns 50 when the pointer is at the content midpoint', () => {
    expect(computeSplitRatio(500, rect, 100)).toBe(50)
  })

  it('grows the left side as the pointer moves right', () => {
    // x=700 → (700-100)/800 = 75%
    expect(computeSplitRatio(700, rect, 100)).toBe(75)
  })

  it('accounts for the wrapper offset', () => {
    // left=200 → content spans 300..1100; midpoint 700 → 50%
    expect(computeSplitRatio(700, { left: 200, width: 1000 }, 100)).toBe(50)
  })

  it('clamps to the minimum near the left edge', () => {
    expect(computeSplitRatio(0, rect, 100)).toBe(15)
  })

  it('clamps to the maximum near the right edge', () => {
    expect(computeSplitRatio(1000, rect, 100)).toBe(85)
  })

  it('falls back to 50 for a degenerate content area', () => {
    expect(computeSplitRatio(50, { left: 0, width: 100 }, 100)).toBe(50)
  })
})

describe('split resizer DOM integration', () => {
  let ac: AbortController

  beforeEach(() => {
    ac = new AbortController()
    localStorage.clear()
  })

  afterEach(() => {
    ac.abort()
    localStorage.clear()
  })

  function setup(): HTMLElement {
    document.body.innerHTML = `
      <div class="diff-table-wrapper diff-split-wrapper overflow-x-auto">
        <table class="diff-table diff-table-split" data-view="split">
          <colgroup>
            <col class="diff-col-num">
            <col class="diff-col-content diff-col-left">
            <col class="diff-col-num">
            <col class="diff-col-content diff-col-right">
          </colgroup>
          <tr class="diff-line" data-line-type="context">
            <td class="diff-line-num" data-side="left" data-line-num="1">1</td>
            <td class="diff-line-content">a</td>
            <td class="diff-line-num diff-split-divider" data-side="right" data-line-num="1">1</td>
            <td class="diff-line-content">a</td>
          </tr>
        </table>
        <div class="diff-split-resizer" data-testid="split-resizer"></div>
      </div>
    `
    return document.querySelector('.diff-split-wrapper') as HTMLElement
  }

  it('restores a persisted ratio onto the wrapper and columns', () => {
    localStorage.setItem('diffty:split-ratio', '70')
    const wrapper = setup()
    initializeSplitResizer({ signal: ac.signal })
    expect(wrapper.style.getPropertyValue('--split-ratio')).toBe('70')
    const colLeft = wrapper.querySelector('col.diff-col-left') as HTMLElement
    const colRight = wrapper.querySelector('col.diff-col-right') as HTMLElement
    expect(colLeft.style.width).toBe('70%')
    expect(colRight.style.width).toBe('30%')
  })

  it('is a no-op when the split wrapper is absent', () => {
    document.body.innerHTML = '<div>no split table here</div>'
    expect(() => initializeSplitResizer({ signal: ac.signal })).not.toThrow()
  })

  it('double-click resets the ratio to 50 and persists it', () => {
    const wrapper = setup()
    wrapper.style.setProperty('--split-ratio', '80')
    initializeSplitResizer({ signal: ac.signal })
    const resizer = wrapper.querySelector('.diff-split-resizer') as HTMLElement
    resizer.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }))
    expect(wrapper.style.getPropertyValue('--split-ratio')).toBe('50')
    expect(readStoredRatio()).toBe(50)
  })

  it('ignores an invalid persisted ratio', () => {
    localStorage.setItem('diffty:split-ratio', 'not-a-number')
    expect(readStoredRatio()).toBeNull()
    const wrapper = setup()
    initializeSplitResizer({ signal: ac.signal })
    // Untouched → falls back to the CSS default (empty inline value).
    expect(wrapper.style.getPropertyValue('--split-ratio')).toBe('')
  })
})
