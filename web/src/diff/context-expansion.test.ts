import { describe, it, expect } from 'vitest'
import { computeFetchRange } from './context-expansion'

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
