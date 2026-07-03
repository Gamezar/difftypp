/**
 * Diff freshness polling.
 *
 * A diff page pins the commits it resolved (branch mode) or is computed against
 * the HEAD / working-tree snapshot that was current when it loaded
 * (staged/unstaged). If the branch advances or the working tree changes
 * underneath the reviewer, the diff on screen silently goes stale. This module
 * polls a lightweight server fingerprint and, when it changes, surfaces a
 * banner offering to reload — rather than auto-reloading, which would discard
 * scroll position and any half-written comment.
 */

import { POLL_INTERVAL_MS, fetchJSON } from '../shared/poll'

interface DiffStatus {
  mode: string
  fingerprint: string
}

/**
 * fetchFingerprint asks the server for the current fingerprint of the diff
 * described by the page's query string. Returns null on any transient failure
 * so the caller can keep its existing baseline instead of false-alarming.
 */
export async function fetchFingerprint(search: string): Promise<string | null> {
  const data = await fetchJSON<DiffStatus>(`/api/diff-status${search}`)
  return data && typeof data.fingerprint === 'string' ? data.fingerprint : null
}

/**
 * buildReloadUrl produces the URL to reload at the current state of the diff.
 * The pinned commit hashes are dropped so branch mode re-resolves to the live
 * branch tips; staged/unstaged re-derive them from HEAD regardless. The
 * selected file is preserved so the reviewer lands back where they were.
 */
export function buildReloadUrl(pathname: string, search: string): string {
  const params = new URLSearchParams(search)
  params.delete('source_commit')
  params.delete('target_commit')
  return `${pathname}?${params.toString()}`
}

/**
 * messageForMode returns the banner copy describing what changed underneath
 * the reviewer for a given diff mode.
 */
export function messageForMode(mode: string): string {
  switch (mode) {
    case 'staged':
      return 'The staged changes have changed since you opened this diff.'
    case 'unstaged':
      return 'The working tree has changed since you opened this diff.'
    default:
      return 'This branch has new commits since you opened this diff.'
  }
}

/**
 * showStaleBanner renders the reload prompt. It is idempotent: a second call
 * while a banner is already showing is a no-op.
 */
export function showStaleBanner(mode: string, onReload: () => void): void {
  if (document.getElementById('stale-diff-banner')) return

  const banner = document.createElement('div')
  banner.id = 'stale-diff-banner'
  banner.className = 'stale-diff-banner'
  banner.setAttribute('role', 'status')
  banner.dataset.testid = 'stale-diff-banner'

  const text = document.createElement('span')
  text.className = 'stale-diff-banner-text'
  text.textContent = messageForMode(mode)

  const reload = document.createElement('button')
  reload.type = 'button'
  reload.className = 'stale-diff-banner-reload'
  reload.dataset.testid = 'stale-diff-reload'
  reload.textContent = 'Reload diff'
  reload.addEventListener('click', onReload)

  const dismiss = document.createElement('button')
  dismiss.type = 'button'
  dismiss.className = 'stale-diff-banner-dismiss'
  dismiss.dataset.testid = 'stale-diff-dismiss'
  dismiss.setAttribute('aria-label', 'Dismiss')
  dismiss.textContent = '×'
  dismiss.addEventListener('click', () => banner.remove())

  banner.append(text, reload, dismiss)
  document.body.appendChild(banner)
}

/**
 * readSeedFingerprint returns the render-time fingerprint the server embedded in
 * the page, or null when it's absent/empty. Seeding the baseline from this
 * (rather than from the first poll) closes the window between the server
 * rendering the diff and the first poll landing, during which a change would
 * otherwise be absorbed into the baseline and never surfaced.
 */
export function readSeedFingerprint(
  doc: Document = document
): string | null {
  const el = doc.getElementById('diff-fingerprint')
  const fp = el?.getAttribute('data-fingerprint')
  return fp && fp.length > 0 ? fp : null
}

/**
 * initializeFreshnessPolling wires up the polling loop on a diff page. It
 * establishes a baseline fingerprint, then polls until the fingerprint changes
 * (showing the banner and stopping) or the page is unloaded. Commits mode is
 * skipped: two pinned hashes can never drift apart.
 */
export function initializeFreshnessPolling(): void {
  const search = window.location.search
  const params = new URLSearchParams(search)
  const repo = params.get('repo')
  const mode = params.get('mode') ?? 'branches'

  if (!repo || mode === 'commits') return

  // Prefer the server-provided render-time baseline; fall back to seeding from
  // the first poll when it isn't present.
  let baseline: string | null = readSeedFingerprint()
  let stopped = false
  let timer: number | undefined

  const stop = () => {
    stopped = true
    if (timer !== undefined) window.clearInterval(timer)
  }

  const check = async () => {
    if (stopped) return
    const fp = await fetchFingerprint(search)
    if (fp === null) return // transient error — keep the existing baseline
    if (baseline === null) {
      baseline = fp
      return
    }
    if (fp !== baseline) {
      stop()
      showStaleBanner(mode, () => {
        window.location.href = buildReloadUrl(window.location.pathname, search)
      })
    }
  }

  // Establish the baseline immediately, then poll on an interval.
  void check()
  timer = window.setInterval(() => void check(), POLL_INTERVAL_MS)

  // A reviewer returning to the tab wants an immediate freshness check.
  document.addEventListener('visibilitychange', () => {
    if (!stopped && document.visibilityState === 'visible') void check()
  })
}
