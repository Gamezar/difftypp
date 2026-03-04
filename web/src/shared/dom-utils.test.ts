import { describe, it, expect, beforeEach } from 'vitest'
import { escapeHtml, $, $$ } from './dom-utils'

describe('escapeHtml', () => {
  it('escapes angle brackets', () => {
    expect(escapeHtml('<script>alert("xss")</script>')).toBe(
      '&lt;script&gt;alert("xss")&lt;/script&gt;'
    )
  })

  it('escapes ampersands', () => {
    expect(escapeHtml('foo & bar')).toBe('foo &amp; bar')
  })

  it('escapes double quotes', () => {
    expect(escapeHtml('a "quoted" value')).toBe('a "quoted" value')
  })

  it('returns empty string for empty input', () => {
    expect(escapeHtml('')).toBe('')
  })

  it('preserves plain text', () => {
    expect(escapeHtml('hello world')).toBe('hello world')
  })
})

describe('$', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="test-el">hello</div>'
  })

  it('returns element by id', () => {
    const el = $<HTMLDivElement>('test-el')
    expect(el).not.toBeNull()
    expect(el!.textContent).toBe('hello')
  })

  it('returns null for nonexistent id', () => {
    expect($('nonexistent')).toBeNull()
  })
})

describe('$$', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <ul>
        <li class="item">a</li>
        <li class="item">b</li>
        <li class="item">c</li>
      </ul>
    `
  })

  it('returns array of matching elements', () => {
    const items = $$<HTMLLIElement>('.item')
    expect(items).toHaveLength(3)
    expect(items[0].textContent).toBe('a')
  })

  it('returns empty array when nothing matches', () => {
    expect($$('.nonexistent')).toHaveLength(0)
  })

  it('scopes to a root element', () => {
    document.body.innerHTML = `
      <div id="scope"><span class="x">1</span></div>
      <span class="x">2</span>
    `
    const root = document.getElementById('scope')!
    const items = $$<HTMLSpanElement>('.x', root)
    expect(items).toHaveLength(1)
    expect(items[0].textContent).toBe('1')
  })
})
