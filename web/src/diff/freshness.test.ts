import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  buildReloadUrl,
  fetchFingerprint,
  messageForMode,
  readSeedFingerprint,
  showStaleBanner,
} from './freshness'

describe('messageForMode', () => {
  it('describes a branch advance', () => {
    expect(messageForMode('branches')).toMatch(/new commits/)
  })

  it('describes staged changes', () => {
    expect(messageForMode('staged')).toMatch(/staged changes/)
  })

  it('describes working tree changes', () => {
    expect(messageForMode('unstaged')).toMatch(/working tree/)
  })

  it('falls back to the branch message for unknown modes', () => {
    expect(messageForMode('something-else')).toMatch(/new commits/)
  })
})

describe('buildReloadUrl', () => {
  it('drops the pinned commits so branch mode re-resolves', () => {
    const url = buildReloadUrl(
      '/diff',
      '?repo=%2Ftmp%2Fr&source=feature&target=main&source_commit=abc&target_commit=def&mode=branches&file=a.go'
    )
    const params = new URLSearchParams(url.split('?')[1])
    expect(url.startsWith('/diff?')).toBe(true)
    expect(params.get('source_commit')).toBeNull()
    expect(params.get('target_commit')).toBeNull()
    // Everything else is preserved.
    expect(params.get('repo')).toBe('/tmp/r')
    expect(params.get('source')).toBe('feature')
    expect(params.get('target')).toBe('main')
    expect(params.get('mode')).toBe('branches')
    expect(params.get('file')).toBe('a.go')
  })
})

describe('showStaleBanner', () => {
  afterEach(() => {
    document.getElementById('stale-diff-banner')?.remove()
  })

  it('renders a banner with reload and dismiss controls', () => {
    showStaleBanner('branches', () => {})
    const banner = document.getElementById('stale-diff-banner')
    expect(banner).not.toBeNull()
    expect(banner!.getAttribute('role')).toBe('status')
    expect(
      banner!.querySelector('[data-testid="stale-diff-reload"]')
    ).not.toBeNull()
    expect(
      banner!.querySelector('[data-testid="stale-diff-dismiss"]')
    ).not.toBeNull()
  })

  it('invokes the reload callback when reload is clicked', () => {
    const onReload = vi.fn()
    showStaleBanner('staged', onReload)
    const reload = document.querySelector<HTMLButtonElement>(
      '[data-testid="stale-diff-reload"]'
    )
    reload!.click()
    expect(onReload).toHaveBeenCalledOnce()
  })

  it('removes the banner when dismissed', () => {
    showStaleBanner('unstaged', () => {})
    const dismiss = document.querySelector<HTMLButtonElement>(
      '[data-testid="stale-diff-dismiss"]'
    )
    dismiss!.click()
    expect(document.getElementById('stale-diff-banner')).toBeNull()
  })

  it('is idempotent — a second call does not stack banners', () => {
    showStaleBanner('branches', () => {})
    showStaleBanner('branches', () => {})
    expect(document.querySelectorAll('#stale-diff-banner')).toHaveLength(1)
  })
})

describe('readSeedFingerprint', () => {
  afterEach(() => {
    document.getElementById('diff-fingerprint')?.remove()
  })

  it('returns null when the seed element is absent', () => {
    expect(readSeedFingerprint()).toBeNull()
  })

  it('returns null when the seed is empty', () => {
    document.body.innerHTML =
      '<div id="diff-fingerprint" data-fingerprint=""></div>'
    expect(readSeedFingerprint()).toBeNull()
  })

  it('returns the embedded render-time fingerprint', () => {
    document.body.innerHTML =
      '<div id="diff-fingerprint" data-fingerprint="seed-abc"></div>'
    expect(readSeedFingerprint()).toBe('seed-abc')
  })
})

describe('fetchFingerprint', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns the fingerprint from a successful response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ mode: 'staged', fingerprint: 'abc123' }),
      })
    )
    expect(await fetchFingerprint('?repo=/tmp/r&mode=staged')).toBe('abc123')
  })

  it('returns null on a non-ok response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, json: () => Promise.resolve({}) })
    )
    expect(await fetchFingerprint('?repo=/tmp/r')).toBeNull()
  })

  it('returns null when the request throws', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network')))
    expect(await fetchFingerprint('?repo=/tmp/r')).toBeNull()
  })

  it('returns null when the payload lacks a string fingerprint', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ mode: 'staged' }),
      })
    )
    expect(await fetchFingerprint('?repo=/tmp/r')).toBeNull()
  })
})
