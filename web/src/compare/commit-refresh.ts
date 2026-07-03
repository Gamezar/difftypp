/**
 * Recent-commits auto-refresh for the compare page (commits mode).
 *
 * The commit-selection list is rendered once when the page loads, so commits
 * created afterwards never appear until a manual reload. This module polls the
 * recent-commits API and, when the list changes, re-renders it in place via the
 * commit selector's controller — preserving the current target/source
 * selection and scroll position.
 */

import { POLL_INTERVAL_MS, fetchJSON } from '../shared/poll'
import type { Commit, CommitSelectorController } from './commit-selector'

/**
 * fetchRecentCommits returns the repository's most recent commits, or null on
 * any transient failure so the caller can keep showing the existing list.
 */
export async function fetchRecentCommits(repo: string): Promise<Commit[] | null> {
  const data = await fetchJSON<{ commits?: Commit[] }>(
    `/api/recent-commits?repo=${encodeURIComponent(repo)}`
  )
  return data && Array.isArray(data.commits) ? data.commits : null
}

/**
 * commitsChanged reports whether the ordered hash list differs from the commits
 * just fetched — a different length, or any hash out of place at the same index.
 */
export function commitsChanged(prev: string[], next: Commit[]): boolean {
  if (prev.length !== next.length) return true
  for (let i = 0; i < next.length; i++) {
    if (prev[i] !== next[i].hash) return true
  }
  return false
}

/**
 * initializeCommitListRefresh starts polling on the compare page when it is in
 * commits mode, refreshing the list through the given controller whenever the
 * commits change. A no-op on other modes or pages without a commit list.
 */
export function initializeCommitListRefresh(
  controller: CommitSelectorController
): void {
  const params = new URLSearchParams(window.location.search)
  const repo = params.get('repo')
  const mode = params.get('mode')
  if (!repo || mode !== 'commits') return

  const poll = async () => {
    const commits = await fetchRecentCommits(repo)
    if (!commits) return
    if (commitsChanged(controller.currentHashes(), commits)) {
      controller.refresh(commits)
    }
  }

  window.setInterval(() => void poll(), POLL_INTERVAL_MS)

  // A user returning to the tab wants the list brought up to date immediately.
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') void poll()
  })
}
