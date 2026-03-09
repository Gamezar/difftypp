/**
 * Past reviews module.
 *
 * Handles deletion of individual and all past review entries via the
 * DELETE /api/review/past and DELETE /api/reviews/past endpoints.
 * After a successful deletion the page is reloaded to reflect changes.
 * Errors are shown to the user via a temporary toast notification.
 */

function buildQueryString(params: Record<string, string>): string {
  return Object.entries(params)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&')
}

/** Show a temporary error toast that auto-dismisses after 4 seconds. */
function showError(message: string): void {
  const toast = document.createElement('div')
  toast.className = 'past-review-error-toast'
  toast.textContent = message
  toast.setAttribute('role', 'alert')
  // Inline styles so no extra CSS is needed
  Object.assign(toast.style, {
    position: 'fixed',
    bottom: '1rem',
    right: '1rem',
    background: '#dc2626',
    color: '#fff',
    padding: '0.75rem 1rem',
    borderRadius: '0.5rem',
    fontSize: '0.875rem',
    zIndex: '9999',
    boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
  })
  document.body.appendChild(toast)
  setTimeout(() => toast.remove(), 4000)
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
        showError('Failed to delete past review. Please try again.')
      }
    })
    .catch(() => showError('Network error — could not delete past review.'))
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
        showError('Failed to delete past reviews. Please try again.')
      }
    })
    .catch(() => showError('Network error — could not delete past reviews.'))
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
