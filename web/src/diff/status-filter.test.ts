import { describe, it, expect, beforeEach } from 'vitest'
import {
  computeVisibility,
  formatFilesCount,
  buildNoFilesMessage,
  initializeStatusFilter,
} from './status-filter'

describe('computeVisibility', () => {
  const statuses = ['approved', 'rejected', 'pending', 'approved', 'skipped']

  it('shows all items when filter is "all"', () => {
    expect(computeVisibility(statuses, 'all')).toEqual([
      true,
      true,
      true,
      true,
      true,
    ])
  })

  it('filters to only approved items', () => {
    expect(computeVisibility(statuses, 'approved')).toEqual([
      true,
      false,
      false,
      true,
      false,
    ])
  })

  it('filters to only rejected items', () => {
    expect(computeVisibility(statuses, 'rejected')).toEqual([
      false,
      true,
      false,
      false,
      false,
    ])
  })

  it('returns all false when no items match', () => {
    expect(computeVisibility(statuses, 'nonexistent')).toEqual([
      false,
      false,
      false,
      false,
      false,
    ])
  })

  it('handles empty array', () => {
    expect(computeVisibility([], 'all')).toEqual([])
  })
})

describe('formatFilesCount', () => {
  it('shows total only when all visible', () => {
    expect(formatFilesCount(10, 10)).toBe('(10)')
  })

  it('shows visible of total when filtered', () => {
    expect(formatFilesCount(3, 10)).toBe('(3 of 10)')
  })

  it('shows 0 of total when none visible', () => {
    expect(formatFilesCount(0, 5)).toBe('(0 of 5)')
  })
})

describe('buildNoFilesMessage', () => {
  it('includes status name when filtering', () => {
    expect(buildNoFilesMessage('approved')).toBe('No approved files found.')
  })

  it('uses generic message for "all"', () => {
    expect(buildNoFilesMessage('all')).toBe('No files found.')
  })
})

describe('initializeStatusFilter (DOM integration)', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <select id="status-filter">
        <option value="all">All</option>
        <option value="approved">Approved</option>
        <option value="rejected">Rejected</option>
        <option value="nonexistent">Nonexistent</option>
      </select>
      <span id="files-count"></span>
      <div>
        <ul id="files-list">
          <li data-status="approved">file1.ts</li>
          <li data-status="rejected">file2.ts</li>
          <li data-status="approved">file3.ts</li>
        </ul>
      </div>
    `
  })

  it('sets initial files count on init', () => {
    initializeStatusFilter()
    const count = document.getElementById('files-count')!
    expect(count.textContent).toBe('(3)')
  })

  it('filters files when status changes', () => {
    initializeStatusFilter()
    const select = document.getElementById(
      'status-filter'
    ) as HTMLSelectElement
    select.value = 'approved'
    select.dispatchEvent(new Event('change'))

    const items = document.querySelectorAll('#files-list li')
    expect(items[0].classList.contains('hidden')).toBe(false)
    expect(items[1].classList.contains('hidden')).toBe(true)
    expect(items[2].classList.contains('hidden')).toBe(false)

    expect(document.getElementById('files-count')!.textContent).toBe(
      '(2 of 3)'
    )
  })

  it('shows no-files message when all hidden', () => {
    initializeStatusFilter()
    const select = document.getElementById(
      'status-filter'
    ) as HTMLSelectElement
    select.value = 'nonexistent'
    select.dispatchEvent(new Event('change'))

    const message = document.getElementById('no-files-message')
    expect(message).not.toBeNull()
    expect(message!.textContent).toBe('No nonexistent files found.')
  })

  it('removes no-files message when items become visible', () => {
    initializeStatusFilter()
    const select = document.getElementById(
      'status-filter'
    ) as HTMLSelectElement

    select.value = 'nonexistent'
    select.dispatchEvent(new Event('change'))
    expect(document.getElementById('no-files-message')).not.toBeNull()

    select.value = 'all'
    select.dispatchEvent(new Event('change'))
    expect(document.getElementById('no-files-message')).toBeNull()
  })

  it('does nothing when status-filter element is missing', () => {
    document.body.innerHTML = ''
    expect(() => initializeStatusFilter()).not.toThrow()
  })
})
