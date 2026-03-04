/**
 * Clipboard copy for markdown review content.
 *
 * Handles copying the review markdown text to the system clipboard
 * with visual feedback, falling back to text selection when the
 * Clipboard API is unavailable or denied.
 */

const COPY_BUTTON_ID = 'copy-btn'
const MARKDOWN_CONTENT_ID = 'markdown-content'
const COPY_SUCCESS_ID = 'copy-success'
const RESET_DELAY_MS = 2000
const DEFAULT_BUTTON_TEXT = 'Copy to Clipboard'
const COPIED_BUTTON_TEXT = 'Copied!'

/**
 * initializeClipboard wires up the copy button to copy markdown content.
 * No-ops gracefully when required DOM elements are missing.
 */
export function initializeClipboard(): void {
  const copyBtn = document.getElementById(COPY_BUTTON_ID)
  const markdownEl = document.getElementById(MARKDOWN_CONTENT_ID)

  if (!copyBtn || !markdownEl) return

  const copySuccess = document.getElementById(COPY_SUCCESS_ID)

  copyBtn.addEventListener('click', () => {
    const text = markdownEl.textContent ?? ''

    navigator.clipboard.writeText(text).then(
      () => showCopySuccess(copyBtn, copySuccess),
      () => selectTextFallback(markdownEl)
    )
  })
}

/**
 * showCopySuccess displays success feedback and resets after a delay.
 * Removes the 'hidden' class from the success element and changes
 * button text, then restores both after RESET_DELAY_MS.
 */
function showCopySuccess(
  copyBtn: HTMLElement,
  copySuccess: HTMLElement | null
): void {
  copySuccess?.classList.remove('hidden')
  copyBtn.textContent = COPIED_BUTTON_TEXT

  setTimeout(() => {
    copySuccess?.classList.add('hidden')
    copyBtn.textContent = DEFAULT_BUTTON_TEXT
  }, RESET_DELAY_MS)
}

/**
 * selectTextFallback selects the markdown content text in the DOM
 * so the user can manually copy it with Ctrl+C / Cmd+C.
 */
function selectTextFallback(markdownEl: HTMLElement): void {
  const range = document.createRange()
  range.selectNodeContents(markdownEl)

  const selection = window.getSelection()
  if (!selection) return

  selection.removeAllRanges()
  selection.addRange(range)
}
