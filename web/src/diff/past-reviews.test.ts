import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { showError } from './past-reviews'

describe('showError', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    // Clean up any remaining toasts
    document.querySelectorAll('.past-review-error-toast').forEach(el => el.remove())
    vi.useRealTimers()
  })

  it('creates a toast element in the DOM', () => {
    showError('Something went wrong')

    const toast = document.querySelector('.past-review-error-toast')
    expect(toast).not.toBeNull()
    expect(toast!.textContent).toBe('Something went wrong')
  })

  it('sets role="alert" for accessibility', () => {
    showError('Error message')

    const toast = document.querySelector('.past-review-error-toast')
    expect(toast!.getAttribute('role')).toBe('alert')
  })

  it('auto-dismisses after 4 seconds', () => {
    showError('Temporary error')

    expect(document.querySelector('.past-review-error-toast')).not.toBeNull()

    vi.advanceTimersByTime(4000)

    expect(document.querySelector('.past-review-error-toast')).toBeNull()
  })

  it('replaces the previous toast instead of stacking', () => {
    showError('First error')
    showError('Second error')

    const toasts = document.querySelectorAll('.past-review-error-toast')
    expect(toasts).toHaveLength(1)
    expect(toasts[0].textContent).toBe('Second error')
  })

  it('can be dismissed by clicking', () => {
    showError('Click to dismiss')

    const toast = document.querySelector('.past-review-error-toast') as HTMLElement
    expect(toast).not.toBeNull()

    toast.click()

    expect(document.querySelector('.past-review-error-toast')).toBeNull()
  })

  it('does not throw when clicking after auto-dismiss', () => {
    showError('Already gone')

    const toast = document.querySelector('.past-review-error-toast') as HTMLElement
    vi.advanceTimersByTime(4000)

    // Toast is already removed; clicking should not throw
    expect(() => toast.click()).not.toThrow()
  })
})
