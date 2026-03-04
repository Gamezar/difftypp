/**
 * Review page entry point.
 *
 * Initializes clipboard copy functionality for the
 * review submitted confirmation page.
 */

import { initializeClipboard } from './clipboard'

document.addEventListener('DOMContentLoaded', () => {
  initializeClipboard()
})
