/**
 * Status filter for file list in diff view.
 *
 * Filters file list items by their review status (data-status attribute)
 * and updates the visible file count badge.
 */

export interface FilterResult {
  visibleCount: number
  totalCount: number
}

/**
 * Determine which items should be visible given a status filter value.
 * Pure function — operates on data, not DOM.
 *
 * @param statuses - Array of status values for each item (parallel to items)
 * @param selectedStatus - The filter value ('all' or a specific status)
 * @returns Array of booleans, true = visible
 */
export function computeVisibility(
  statuses: string[],
  selectedStatus: string
): boolean[] {
  return statuses.map(
    (status) => selectedStatus === 'all' || status === selectedStatus
  )
}

/**
 * Format the file count display text.
 *
 * @returns e.g. "(10)" when all visible, "(3 of 10)" when filtered
 */
export function formatFilesCount(visible: number, total: number): string {
  if (visible === total) {
    return `(${total})`
  }
  return `(${visible} of ${total})`
}

/**
 * Build the "no files found" message text.
 */
export function buildNoFilesMessage(selectedStatus: string): string {
  if (selectedStatus !== 'all') {
    return `No ${selectedStatus} files found.`
  }
  return 'No files found.'
}

/**
 * Initialize the status filter on the diff page.
 * Wires up the select element to filter file list items.
 */
export function initializeStatusFilter(): void {
  const statusFilter = document.getElementById(
    'status-filter'
  ) as HTMLSelectElement | null
  if (!statusFilter) return

  updateFilesCountDOM()

  statusFilter.addEventListener('change', () => {
    const selectedStatus = statusFilter.value
    const filesList = document.getElementById('files-list')
    if (!filesList) return

    const files = Array.from(filesList.querySelectorAll('li'))
    const statuses = files.map(
      (f) => f.getAttribute('data-status') || ''
    )
    const visibility = computeVisibility(statuses, selectedStatus)

    let visibleCount = 0
    files.forEach((file, i) => {
      if (visibility[i]) {
        file.classList.remove('hidden')
        visibleCount++
      } else {
        file.classList.add('hidden')
        file.classList.remove('bg-gray-100')
      }
    })

    // Manage "no files" message
    const existing = document.getElementById('no-files-message')
    if (visibleCount === 0) {
      if (!existing) {
        const message = document.createElement('p')
        message.id = 'no-files-message'
        message.className = 'text-gray-500 py-4 text-center'
        message.textContent = buildNoFilesMessage(selectedStatus)
        filesList.parentNode?.appendChild(message)
      }
    } else if (existing) {
      existing.remove()
    }

    updateFilesCountDOM()
  })
}

/**
 * Update the files count badge in the DOM.
 */
function updateFilesCountDOM(): void {
  const filesList = document.getElementById('files-list')
  const filesCount = document.getElementById('files-count')
  if (!filesList || !filesCount) return

  const visibleFiles = filesList.querySelectorAll('li:not(.hidden)').length
  const totalFiles = filesList.querySelectorAll('li').length

  filesCount.textContent = formatFilesCount(visibleFiles, totalFiles)
}
