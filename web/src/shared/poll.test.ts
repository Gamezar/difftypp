import { describe, it, expect, afterEach, vi } from 'vitest'
import { POLL_INTERVAL_MS, fetchJSON } from './poll'

describe('POLL_INTERVAL_MS', () => {
  it('is a positive interval', () => {
    expect(POLL_INTERVAL_MS).toBeGreaterThan(0)
  })
})

describe('fetchJSON', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns the parsed body on a successful response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ value: 42 }),
      })
    )
    expect(await fetchJSON<{ value: number }>('/x')).toEqual({ value: 42 })
  })

  it('sends an Accept: application/json header', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    })
    vi.stubGlobal('fetch', fetchMock)
    await fetchJSON('/x')
    expect(fetchMock).toHaveBeenCalledWith('/x', {
      headers: { Accept: 'application/json' },
    })
  })

  it('returns null on a non-ok response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, json: () => Promise.resolve({}) })
    )
    expect(await fetchJSON('/x')).toBeNull()
  })

  it('returns null when the request throws', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network')))
    expect(await fetchJSON('/x')).toBeNull()
  })
})
