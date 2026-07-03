/**
 * Compare page entry point.
 *
 * Initializes the commit selector for target/source picking and keeps the
 * recent-commits list fresh as new commits land.
 */

import { initializeCommitListRefresh } from './commit-refresh'
import { initializeCommitSelector } from './commit-selector'

document.addEventListener('DOMContentLoaded', () => {
  const controller = initializeCommitSelector()
  if (controller) initializeCommitListRefresh(controller)
})
