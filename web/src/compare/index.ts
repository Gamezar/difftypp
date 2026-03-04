/**
 * Compare page entry point.
 *
 * Initializes the commit selector for target/source picking.
 */

import { initializeCommitSelector } from './commit-selector'

document.addEventListener('DOMContentLoaded', () => {
  initializeCommitSelector()
})
