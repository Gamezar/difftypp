import { describe, it, expect, afterEach, vi } from 'vitest'
import { commitsChanged, fetchRecentCommits } from './commit-refresh'

describe('commitsChanged', () => {
  it('is false when hashes match in order', () => {
    expect(
      commitsChanged(['a', 'b'], [{ hash: 'a', subject: '' }, { hash: 'b', subject: '' }])
    ).toBe(false)
  })

  it('is true when a new commit is prepended', () => {
    expect(
      commitsChanged(['a', 'b'], [
        { hash: 'c', subject: '' },
        { hash: 'a', subject: '' },
        { hash: 'b', subject: '' },
      ])
    ).toBe(true)
  })

  it('is true when the length differs', () => {
    expect(commitsChanged(['a'], [])).toBe(true)
  })

  it('is true when a hash at the same index differs', () => {
    expect(
      commitsChanged(['a', 'b'], [{ hash: 'a', subject: '' }, { hash: 'x', subject: '' }])
    ).toBe(true)
  })

  it('ignores subject changes (only hashes matter)', () => {
    expect(
      commitsChanged(['a'], [{ hash: 'a', subject: 'reworded' }])
    ).toBe(false)
  })
})

describe('fetchRecentCommits', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns the commits array from a successful response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({ commits: [{ hash: 'a', subject: 'first' }] }),
      })
    )
    expect(await fetchRecentCommits('/tmp/r')).toEqual([
      { hash: 'a', subject: 'first' },
    ])
  })

  it('returns null on a non-ok response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, json: () => Promise.resolve({}) })
    )
    expect(await fetchRecentCommits('/tmp/r')).toBeNull()
  })

  it('returns null when the request throws', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network')))
    expect(await fetchRecentCommits('/tmp/r')).toBeNull()
  })

  it('returns null when the payload has no commits array', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) })
    )
    expect(await fetchRecentCommits('/tmp/r')).toBeNull()
  })
})
