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

/** A commit as returned by the recent-commits API. */
export interface Commit {
  hash: string
  subject: string
}

/**
 * Controls a live commit-selection list: it owns the selection state and can
 * re-render its rows in place (preserving selection and scroll) when new
 * commits arrive.
 */
export interface CommitSelectorController {
  /** Hashes of the rows currently rendered, top to bottom. */
  currentHashes(): string[]
  /** Replace the rows with `commits`, preserving selection and scroll. */
  refresh(commits: Commit[]): void
}

/** shortHash truncates a commit hash to 8 chars, matching the server's helper. */
export function shortHash(hash: string): string {
  return hash.length > 8 ? hash.slice(0, 8) : hash
}

/**
 * buildCommitRow creates a commit <li> matching the server-rendered markup so
 * refreshed rows are indistinguishable from the initial server render.
 */
export function buildCommitRow(commit: Commit): HTMLLIElement {
  const li = document.createElement('li')
  li.setAttribute('data-hash', commit.hash)
  li.setAttribute('data-testid', 'commit-row')
  li.className = `${BASE_CLASSES} hover:bg-gray-50`

  const code = document.createElement('code')
  code.className =
    'text-xs bg-gray-100 px-1.5 py-0.5 rounded font-mono text-gray-600 shrink-0 pointer-events-none'
  code.textContent = shortHash(commit.hash)

  const subject = document.createElement('span')
  subject.className = 'text-gray-700 truncate flex-1 pointer-events-none'
  subject.textContent = commit.subject

  li.append(code, subject)
  return li
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
 * Wires click listeners on each commit row inside `#commits-list` and keeps the
 * `#target` / `#source` inputs in sync. Returns a controller for refreshing the
 * list in place, or null when there is no commit list on the page.
 */
export function initializeCommitSelector(): CommitSelectorController | null {
  const targetInput = document.getElementById('target') as HTMLInputElement | null
  const sourceInput = document.getElementById('source') as HTMLInputElement | null
  const commitsList = document.getElementById('commits-list')
  if (!commitsList) return null

  // The selection survives re-renders; rows are re-queried each pass so a
  // refreshed list stays wired to the same selection state.
  let selection: CommitSelection = { target: '', source: '' }

  const currentRows = () =>
    Array.from(commitsList.querySelectorAll<HTMLElement>('li[data-hash]'))

  function updateAll(): void {
    for (const row of currentRows()) {
      const hash = row.getAttribute('data-hash') ?? ''
      applyRowState(row, computeRowState(hash, selection))
    }
    if (selection.target && targetInput) targetInput.value = selection.target
    if (selection.source && sourceInput) sourceInput.value = selection.source
  }

  function bindRows(): void {
    for (const row of currentRows()) {
      row.addEventListener('click', () => {
        const hash = row.getAttribute('data-hash') ?? ''
        selection = computeNextSelection(hash, selection)
        updateAll()
      })
    }
  }

  bindRows()
  updateAll()

  return {
    currentHashes(): string[] {
      return currentRows().map(row => row.getAttribute('data-hash') ?? '')
    },
    refresh(commits: Commit[]): void {
      const previous = new Set(this.currentHashes())
      const scrollTop = commitsList.scrollTop

      commitsList.replaceChildren(...commits.map(buildCommitRow))
      bindRows()
      updateAll()

      // Mark freshly arrived rows after updateAll, which rewrites className and
      // would otherwise strip the flash class. The flash is a one-shot CSS hint.
      for (const row of currentRows()) {
        const hash = row.getAttribute('data-hash') ?? ''
        if (!previous.has(hash)) row.classList.add('commit-row-new')
      }
      commitsList.scrollTop = scrollTop
    },
  }
}
