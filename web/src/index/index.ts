/**
 * Index page entry point.
 *
 * Initializes the file explorer modal for browsing Git repositories.
 */

import { initializeFileExplorer } from './file-explorer'

document.addEventListener('DOMContentLoaded', () => {
  initializeFileExplorer()
})
