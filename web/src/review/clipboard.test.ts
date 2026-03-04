import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { initializeClipboard } from './clipboard'

/**
 * DOM fixture used across clipboard tests.
 * Mirrors the review_submitted.html structure.
 */
const DOM_FIXTURE = `
  <button id="copy-btn">Copy to Clipboard</button>
  <pre id="markdown-content">## Review\n- Comment 1\n- Comment 2</pre>
  <div id="copy-success" class="hidden">Copied to clipboard!</div>
`

/**
 * Flush all pending microtasks (resolved promises) without
 * advancing fake timers, so we can inspect intermediate state.
 */
function flushMicrotasks(): Promise<void> {
  return new Promise((resolve) => {
    queueMicrotask(resolve)
  })
}

describe('initializeClipboard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    document.body.innerHTML = DOM_FIXTURE

    Object.assign(navigator, {
      clipboard: {
        writeText: vi.fn().mockResolvedValue(undefined),
      },
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('calls navigator.clipboard.writeText with markdown text on click', async () => {
    initializeClipboard()

    const copyBtn = document.getElementById('copy-btn')!
    const markdownEl = document.getElementById('markdown-content')!

    copyBtn.click()
    await flushMicrotasks()

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      markdownEl.textContent
    )
  })

  it('shows success message after successful copy', async () => {
    initializeClipboard()

    const copyBtn = document.getElementById('copy-btn')!
    const copySuccess = document.getElementById('copy-success')!

    copyBtn.click()
    await flushMicrotasks()

    expect(copySuccess.classList.contains('hidden')).toBe(false)
  })

  it('changes button text to "Copied!" after successful copy', async () => {
    initializeClipboard()

    const copyBtn = document.getElementById('copy-btn')!

    copyBtn.click()
    await flushMicrotasks()

    expect(copyBtn.textContent).toBe('Copied!')
  })

  it('restores button text and hides success after 2000ms', async () => {
    initializeClipboard()

    const copyBtn = document.getElementById('copy-btn')!
    const copySuccess = document.getElementById('copy-success')!

    copyBtn.click()
    await flushMicrotasks()

    // Success state confirmed
    expect(copyBtn.textContent).toBe('Copied!')
    expect(copySuccess.classList.contains('hidden')).toBe(false)

    // Advance past the 2000ms timeout
    vi.advanceTimersByTime(2000)

    expect(copyBtn.textContent).toBe('Copy to Clipboard')
    expect(copySuccess.classList.contains('hidden')).toBe(true)
  })

  it('falls back to text selection when clipboard API fails', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    Object.assign(navigator, { clipboard: { writeText } })

    initializeClipboard()

    const copyBtn = document.getElementById('copy-btn')!
    const markdownEl = document.getElementById('markdown-content')!

    copyBtn.click()
    await flushMicrotasks()

    const selection = window.getSelection()!
    expect(selection.rangeCount).toBeGreaterThan(0)

    const range = selection.getRangeAt(0)
    expect(range.startContainer).toBe(markdownEl)
    expect(range.endContainer).toBe(markdownEl)
  })

  it('does nothing when copy button is missing', () => {
    document.body.innerHTML = `
      <pre id="markdown-content">text</pre>
      <div id="copy-success" class="hidden"></div>
    `
    expect(() => initializeClipboard()).not.toThrow()
  })

  it('does nothing when markdown element is missing', () => {
    document.body.innerHTML = `
      <button id="copy-btn">Copy to Clipboard</button>
      <div id="copy-success" class="hidden"></div>
    `
    expect(() => initializeClipboard()).not.toThrow()
  })
})
