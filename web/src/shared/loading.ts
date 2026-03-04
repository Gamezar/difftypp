/**
 * showLoadingIndicator makes the fixed-position loading overlay visible.
 */
export function showLoadingIndicator(): void {
  const overlay = document.getElementById('loading-overlay')
  if (overlay) overlay.classList.remove('hidden')
}

/**
 * hideLoadingIndicator hides the loading overlay.
 */
export function hideLoadingIndicator(): void {
  const overlay = document.getElementById('loading-overlay')
  if (overlay) overlay.classList.add('hidden')
}

/**
 * afterOverlayPaint executes fn after the browser has painted.
 * Uses a double-rAF trick to ensure the loading overlay is visible
 * before triggering navigation or form submission.
 */
export function afterOverlayPaint(fn: () => void): void {
  requestAnimationFrame(() => {
    requestAnimationFrame(fn)
  })
}
