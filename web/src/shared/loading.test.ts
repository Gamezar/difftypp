import { describe, it, expect, beforeEach } from 'vitest'
import { showLoadingIndicator, hideLoadingIndicator } from './loading'

describe('showLoadingIndicator', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="loading-overlay" class="hidden"></div>'
  })

  it('removes hidden class from overlay', () => {
    const overlay = document.getElementById('loading-overlay')!
    expect(overlay.classList.contains('hidden')).toBe(true)
    showLoadingIndicator()
    expect(overlay.classList.contains('hidden')).toBe(false)
  })

  it('does nothing when overlay element is missing', () => {
    document.body.innerHTML = ''
    expect(() => showLoadingIndicator()).not.toThrow()
  })
})

describe('hideLoadingIndicator', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="loading-overlay"></div>'
  })

  it('adds hidden class to overlay', () => {
    const overlay = document.getElementById('loading-overlay')!
    expect(overlay.classList.contains('hidden')).toBe(false)
    hideLoadingIndicator()
    expect(overlay.classList.contains('hidden')).toBe(true)
  })
})
