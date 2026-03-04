/**
 * Commit selector for the compare page.
 *
 * Allows users to select target and source commits by clicking
 * on commit rows. Selected commits populate hidden form inputs
 * used for branch/commit comparison.
 */

/** Selection state holding the target and source commit hashes. */
export interface CommitSelection {
  target: string
  source: string
}

/** Shared base Tailwind classes applied to every commit row. */
export const BASE_CLASSES =
  'flex items-center gap-2 py-1.5 px-2 text-sm rounded cursor-pointer transition-colors'

/**
 * Determine the visual role of a commit row given the current selection.
 *
 * @returns 'target' if the hash is the selected target,
 *          'source' if the hash is the selected source,
 *          'none' otherwise.
 */
export function computeRowState(
  hash: string,
  selection: CommitSelection
): 'target' | 'source' | 'none' {
  if (hash === selection.target) return 'target'
  if (hash === selection.source) return 'source'
  return 'none'
}

/**
 * Compute the next selection state after a commit row click.
 *
 * Toggle logic:
 *  - Clicking the current target clears it
 *  - Clicking the current source clears it
 *  - If target is empty, fill target
 *  - If source is empty, fill source
 *  - Otherwise the click is ignored (both slots occupied)
 */
export function computeNextSelection(
  hash: string,
  selection: CommitSelection
): CommitSelection {
  if (hash === selection.target) {
    return { target: '', source: selection.source }
  }
  if (hash === selection.source) {
    return { target: selection.target, source: '' }
  }
  if (!selection.target) {
    return { target: hash, source: selection.source }
  }
  if (!selection.source) {
    return { target: selection.target, source: hash }
  }
  return { target: selection.target, source: selection.source }
}

/** Badge style config keyed by selection role. */
const BADGE_STYLES: Record<'target' | 'source', { bg: string; text: string; label: string }> = {
  target: {
    bg: 'bg-blue-100',
    text: 'text-blue-700',
    label: 'Target',
  },
  source: {
    bg: 'bg-green-100',
    text: 'text-green-700',
    label: 'Source',
  },
}

/**
 * Create a badge <span> element for the given selection role.
 */
export function buildBadgeHtml(role: 'target' | 'source'): HTMLSpanElement {
  const style = BADGE_STYLES[role]
  const el = document.createElement('span')
  el.className =
    `commit-badge text-xs ${style.bg} ${style.text} px-1.5 py-0.5 rounded font-medium shrink-0`
  el.textContent = style.label
  return el
}

/** Row style config keyed by selection role. */
const ROW_STYLES: Record<'target' | 'source' | 'none', string> = {
  target: `${BASE_CLASSES} bg-blue-50 border-l-2 border-blue-400`,
  source: `${BASE_CLASSES} bg-green-50 border-l-2 border-green-400`,
  none: `${BASE_CLASSES} hover:bg-gray-50`,
}

/**
 * Apply visual state to a single commit row:
 * set CSS classes, remove stale badge, and append a fresh badge when selected.
 */
function applyRowState(row: HTMLElement, state: 'target' | 'source' | 'none'): void {
  const badge = row.querySelector('.commit-badge')
  if (badge) badge.remove()

  row.className = ROW_STYLES[state]

  if (state === 'target' || state === 'source') {
    row.appendChild(buildBadgeHtml(state))
  }
}

/**
 * Initialize the commit selector on the compare page.
 *
 * Wires click listeners on each commit row inside `#commits-list`
 * and keeps the `#target` / `#source` hidden inputs in sync.
 */
export function initializeCommitSelector(): void {
  const targetInput = document.getElementById('target') as HTMLInputElement | null
  const sourceInput = document.getElementById('source') as HTMLInputElement | null
  const commitsList = document.getElementById('commits-list')
  if (!commitsList) return

  let selection: CommitSelection = { target: '', source: '' }
  const rows = Array.from(commitsList.querySelectorAll<HTMLElement>('li[data-hash]'))

  function updateAll(): void {
    for (const row of rows) {
      const hash = row.getAttribute('data-hash') ?? ''
      applyRowState(row, computeRowState(hash, selection))
    }
    if (selection.target && targetInput) targetInput.value = selection.target
    if (selection.source && sourceInput) sourceInput.value = selection.source
  }

  for (const row of rows) {
    row.addEventListener('click', () => {
      const hash = row.getAttribute('data-hash') ?? ''
      selection = computeNextSelection(hash, selection)
      updateAll()
    })
  }
}
