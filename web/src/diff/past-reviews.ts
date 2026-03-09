/**
 * Past reviews module.
 *
 * Handles deletion of individual and all past review entries via the
 * DELETE /api/review/past and DELETE /api/reviews/past endpoints.
 * After a successful deletion the page is reloaded to reflect changes.
 */

function buildQueryString(params: Record<string, string>): string {
  return Object.entries(params)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&')
}

function deletePastReview(btn: HTMLElement): void {
  const repo = btn.dataset.repo ?? ''
  const source = btn.dataset.source ?? ''
  const target = btn.dataset.target ?? ''
  const sourceCommit = btn.dataset.sourceCommit ?? ''
  const targetCommit = btn.dataset.targetCommit ?? ''
  const mode = btn.dataset.mode ?? ''
  const pastSourceCommit = btn.dataset.pastSourceCommit ?? ''
  const pastTargetCommit = btn.dataset.pastTargetCommit ?? ''

  const qs = buildQueryString({
    repo, source, target,
    source_commit: sourceCommit,
    target_commit: targetCommit,
    mode,
    past_source_commit: pastSourceCommit,
    past_target_commit: pastTargetCommit,
  })

  fetch(`/api/review/past?${qs}`, { method: 'DELETE' })
    .then(res => {
      if (res.ok) {
        window.location.reload()
      } else {
        console.warn('Failed to delete past review', res.status)
      }
    })
    .catch(err => console.warn('Network error deleting past review', err))
}

function deleteAllPastReviews(btn: HTMLElement): void {
  const repo = btn.dataset.repo ?? ''
  const source = btn.dataset.source ?? ''
  const target = btn.dataset.target ?? ''
  const sourceCommit = btn.dataset.sourceCommit ?? ''
  const targetCommit = btn.dataset.targetCommit ?? ''
  const mode = btn.dataset.mode ?? ''

  const qs = buildQueryString({
    repo, source, target,
    source_commit: sourceCommit,
    target_commit: targetCommit,
    mode,
  })

  fetch(`/api/reviews/past?${qs}`, { method: 'DELETE' })
    .then(res => {
      if (res.ok) {
        window.location.reload()
      } else {
        console.warn('Failed to delete all past reviews', res.status)
      }
    })
    .catch(err => console.warn('Network error deleting all past reviews', err))
}

export function initializePastReviews(): void {
  // Wire up individual delete buttons
  document.querySelectorAll<HTMLElement>('.delete-past-review').forEach(btn => {
    btn.addEventListener('click', () => deletePastReview(btn))
  })

  // Wire up "Delete All" button
  const deleteAllBtn = document.getElementById('delete-all-past-reviews')
  if (deleteAllBtn) {
    deleteAllBtn.addEventListener('click', () => deleteAllPastReviews(deleteAllBtn))
  }
}
