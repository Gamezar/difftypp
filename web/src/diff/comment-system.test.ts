import { describe, it, expect, beforeEach } from 'vitest'
import {
  resolveLineInfo,
  buildCommentApiUrl,
  buildCommentHeader,
  initializeCommentSystem,
} from './comment-system'

// ── Pure function tests ──────────────────────────────────────────

describe('resolveLineInfo', () => {
  it('returns line number and side from cell attributes', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '42')
    cell.setAttribute('data-side', 'left')
    expect(resolveLineInfo(cell)).toEqual({ lineNum: 42, side: 'left' })
  })

  it('defaults side to right when data-side is missing', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '10')
    expect(resolveLineInfo(cell)).toEqual({ lineNum: 10, side: 'right' })
  })

  it('falls back to opposite side when line number is 0', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'right')

    const row = document.createElement('tr')
    row.setAttribute('data-left-num', '15')
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toEqual({ lineNum: 15, side: 'left' })
  })

  it('falls back to right when left has no number', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'left')

    const row = document.createElement('tr')
    row.setAttribute('data-right-num', '20')
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toEqual({ lineNum: 20, side: 'right' })
  })

  it('returns null when no row found and line num is 0', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'right')
    // No parent row
    expect(resolveLineInfo(cell, () => null)).toBeNull()
  })

  it('returns null when both sides have no number', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-line-num', '0')
    cell.setAttribute('data-side', 'right')

    const row = document.createElement('tr')
    // No data-left-num attribute
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toBeNull()
  })

  it('returns null when data-line-num is missing', () => {
    const cell = document.createElement('td')
    cell.setAttribute('data-side', 'right')

    const row = document.createElement('tr')
    row.appendChild(cell)

    expect(resolveLineInfo(cell)).toBeNull()
  })
})

describe('buildCommentApiUrl', () => {
  it('builds URL with all parameters', () => {
    const url = buildCommentApiUrl({
      repoPath: '/home/user/repo',
      sourceBranch: 'feature',
      targetBranch: 'main',
      sourceCommit: 'abc123',
      targetCommit: 'def456',
      mode: 'branches',
    })
    expect(url).toBe(
      '/api/review/comment?repo=%2Fhome%2Fuser%2Frepo&source=feature&target=main&source_commit=abc123&target_commit=def456&mode=branches'
    )
  })

  it('encodes special characters', () => {
    const url = buildCommentApiUrl({
      repoPath: '/path with spaces',
      sourceBranch: 'feat/new & improved',
      targetBranch: 'main',
      sourceCommit: '',
      targetCommit: '',
      mode: 'branches',
    })
    expect(url).toContain('%2Fpath%20with%20spaces')
    expect(url).toContain('feat%2Fnew%20%26%20improved')
  })
})

describe('buildCommentHeader', () => {
  it('formats single line comment header', () => {
    expect(buildCommentHeader(42, 42, 'left')).toBe(
      'Comment on line 42 (left)'
    )
  })

  it('formats multi-line comment header', () => {
    expect(buildCommentHeader(10, 15, 'right')).toBe(
      'Comment on lines 10-15 (right)'
    )
  })
})

// ── DOM integration tests ────────────────────────────────────────

describe('initializeCommentSystem', () => {
  const cursorState = { index: -1, side: 'right' }
  const getCursorState = () => cursorState

  it('returns null when diff-table is missing', () => {
    document.body.innerHTML = ''
    expect(initializeCommentSystem(getCursorState)).toBeNull()
  })

  it('returns null when comment-form-container is missing', () => {
    document.body.innerHTML = '<table class="diff-table"></table>'
    expect(initializeCommentSystem(getCursorState)).toBeNull()
  })

  function setupDOM() {
    document.body.innerHTML = `
      <table class="diff-table">
        <tr class="diff-line" data-left-num="1" data-right-num="1" data-line-type="context">
          <td class="diff-line-num" data-line-num="1" data-side="left">1</td>
          <td class="diff-line-num" data-line-num="1" data-side="right">1</td>
          <td>content</td>
        </tr>
        <tr class="diff-line" data-left-num="2" data-right-num="2" data-line-type="context">
          <td class="diff-line-num" data-line-num="2" data-side="left">2</td>
          <td class="diff-line-num" data-line-num="2" data-side="right">2</td>
          <td>content</td>
        </tr>
      </table>
      <div id="comment-form-container" class="hidden">
        <form id="comment-form">
          <input id="cf-file-path" type="hidden" />
          <input id="cf-start-line" type="hidden" />
          <input id="cf-end-line" type="hidden" />
          <input id="cf-side" type="hidden" />
          <textarea id="cf-body"></textarea>
          <span id="comment-form-header"></span>
          <button type="submit">Save</button>
          <button id="cf-cancel" type="button">Cancel</button>
        </form>
      </div>
    `
  }

  it('returns API object when all elements exist', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)
    expect(api).not.toBeNull()
    expect(api!.showCommentForm).toBeInstanceOf(Function)
    expect(api!.hideCommentForm).toBeInstanceOf(Function)
    expect(api!.clearLineSelection).toBeInstanceOf(Function)
    expect(api!.highlightRange).toBeInstanceOf(Function)
    expect(api!.isCommentFormVisible).toBeInstanceOf(Function)
  })

  it('showCommentForm makes form visible', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    expect(api.isCommentFormVisible()).toBe(false)
    api.highlightRange(1, 2, 'right')
    api.showCommentForm(1, 2, 'right', null)
    expect(api.isCommentFormVisible()).toBe(true)
  })

  it('hideCommentForm hides form and clears selection', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    api.highlightRange(1, 1, 'right')
    api.showCommentForm(1, 1, 'right', null)
    api.hideCommentForm()
    expect(api.isCommentFormVisible()).toBe(false)
    expect(
      document.querySelectorAll('.line-selected').length
    ).toBe(0)
  })

  it('highlightRange adds line-selected class to matching rows', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    api.highlightRange(1, 2, 'right')
    const selected = document.querySelectorAll('.line-selected')
    expect(selected.length).toBe(2)
  })

  it('clearLineSelection removes all line-selected classes', () => {
    setupDOM()
    const api = initializeCommentSystem(getCursorState)!
    api.highlightRange(1, 2, 'right')
    expect(document.querySelectorAll('.line-selected').length).toBe(2)
    api.clearLineSelection()
    expect(document.querySelectorAll('.line-selected').length).toBe(0)
  })

  it('cancel button hides form', () => {
    setupDOM()
    initializeCommentSystem(getCursorState)
    const container = document.getElementById('comment-form-container')!
    container.classList.remove('hidden')
    const cancel = document.getElementById('cf-cancel')!
    cancel.click()
    expect(container.classList.contains('hidden')).toBe(true)
  })
})
