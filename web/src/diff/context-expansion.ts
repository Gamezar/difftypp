/**
 * GitHub-style "expand context" for the diff view.
 *
 * Each collapsed region of unchanged lines is represented by a hunk-header row
 * carrying data-gap-* attributes. Bounded gaps (between two hunks) get two
 * controls — expand-down grows from the top of the gap (the hunk above) and
 * expand-up grows from the bottom of the gap (the hunk below) — so context can
 * be pulled in from whichever hunk is actually being read, not just from the
 * top every time. The unbounded trailing gap (end of file) only supports
 * expand-down since there is no lower hunk to anchor against. Clicking a
 * control fetches the next chunk of hidden lines from /api/file-context and
 * splices them into the diff table as real context rows, so cursor navigation
 * and inline comments work on them too. A separate "×" control collapses the
 * region back.
 */

// Lines revealed per click — incremental expansion, like GitHub.
const CHUNK = 20

// GitHub's "unfold" octicon (chevrons pointing apart) — used where only one
// expand direction is possible (the trailing end-of-file gap).
const UNFOLD_SVG =
  '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M8.177.677l2.896 2.896a.25.25 0 0 1-.177.427H8.75v1.25a.75.75 0 0 1-1.5 0V4H5.104a.25.25 0 0 1-.177-.427L7.823.677a.25.25 0 0 1 .354 0zM7.25 10.75a.75.75 0 0 1 1.5 0V12h2.146a.25.25 0 0 1 .177.427l-2.896 2.896a.25.25 0 0 1-.354 0l-2.896-2.896A.25.25 0 0 1 5.104 12H7.25v-1.25zm-5-2a.75.75 0 0 0 0-1.5h-.5a.75.75 0 0 0 0 1.5h.5zM6 8a.75.75 0 0 1-.75.75h-.5a.75.75 0 0 1 0-1.5h.5A.75.75 0 0 1 6 8zm2.25.75a.75.75 0 0 0 0-1.5h-.5a.75.75 0 0 0 0 1.5h.5zM12 8a.75.75 0 0 1-.75.75h-.5a.75.75 0 0 1 0-1.5h.5A.75.75 0 0 1 12 8zm2.25.75a.75.75 0 0 0 0-1.5h-.5a.75.75 0 0 0 0 1.5h.5z"/></svg>'

// Directional chevrons for bounded gaps — point toward the hunk each control
// pulls context from.
const FOLD_DOWN_SVG =
  '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M4 6L12 6L8 10Z"/></svg>'
const FOLD_UP_SVG =
  '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M4 10L12 10L8 6Z"/></svg>'

// A close/remove "×" octicon — visually distinct from the expand control so it
// reads as "undo this expansion" rather than "expand more".
const COLLAPSE_SVG =
  '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z"/></svg>'

export interface GapBounds {
  rightStart: number
  rightEnd: number | null // null for the bottom gap (extent unknown)
  leftStart: number
}

export type ExpandDirection = 'down' | 'up'

/**
 * Compute the 1-based inclusive right-side line range to fetch for the next
 * chunk. 'down' grows from the top of the remaining gap (toward the hunk
 * below); 'up' grows from the bottom of the remaining gap (toward the hunk
 * above) and is only meaningful for bounded gaps. The bottom gap (null
 * rightEnd) always takes a full chunk from its start, regardless of
 * direction, since there's no lower boundary to anchor 'up' against.
 * Returns null when nothing is hidden.
 */
export function computeFetchRange(
  bounds: GapBounds,
  chunk: number = CHUNK,
  direction: ExpandDirection = 'down'
): { start: number; end: number } | null {
  const { rightStart, rightEnd } = bounds
  if (rightStart < 1) return null
  if (rightEnd === null) return { start: rightStart, end: rightStart + chunk - 1 }
  if (rightStart > rightEnd) return null
  if (direction === 'up') {
    return { start: Math.max(rightStart, rightEnd - chunk + 1), end: rightEnd }
  }
  return { start: rightStart, end: Math.min(rightEnd, rightStart + chunk - 1) }
}

interface FileContextResponse {
  start: number
  lines: string[]
  eof: boolean
}

let groupCounter = 0

/** Assign (once) a stable group id to an expander row so its revealed rows
 *  can be located later for ordered insertion. */
function ensureGroupId(anchor: HTMLElement): string {
  let id = anchor.getAttribute('data-expand-group-id')
  if (!id) {
    id = 'g' + groupCounter++
    anchor.setAttribute('data-expand-group-id', id)
  }
  return id
}

function makeControlButton(
  testId: string,
  action: 'expand' | 'collapse',
  svg: string,
  direction?: ExpandDirection
): HTMLButtonElement {
  const btn = document.createElement('button')
  btn.type = 'button'
  btn.className =
    action === 'collapse' ? 'diff-expand-btn diff-collapse-btn' : 'diff-expand-btn'
  btn.setAttribute('data-testid', testId)
  btn.setAttribute('data-action', action)
  if (direction) btn.setAttribute('data-direction', direction)
  if (action === 'collapse') {
    btn.title = 'Collapse expanded lines'
  } else if (direction === 'up') {
    btn.title = 'Expand hidden lines above (toward this hunk)'
  } else if (direction === 'down') {
    btn.title = 'Expand hidden lines below'
  } else {
    btn.title = 'Expand hidden lines'
  }
  btn.innerHTML = svg
  return btn
}

/** Count how many context rows the given expander has revealed so far. */
function revealedCount(anchor: HTMLElement): number {
  const id = anchor.getAttribute('data-expand-group-id')
  if (!id || !anchor.parentElement) return 0
  return anchor.parentElement.querySelectorAll(`tr[data-expand-group="${id}"]`).length
}

/** True if the expander still has hidden lines left to reveal. */
function hasMoreToExpand(anchor: HTMLElement, isBottom: boolean): boolean {
  if (isBottom) return anchor.getAttribute('data-eof') !== '1'
  const rs = parseInt(anchor.getAttribute('data-gap-right-start') || '0', 10)
  const re = parseInt(anchor.getAttribute('data-gap-right-end') || '0', 10)
  return rs <= re
}

/**
 * Render the expander's controls for its current state: expand control(s)
 * while lines remain hidden, and a separate "×" collapse control once
 * anything has been revealed (so it can be undone). Bounded gaps get both an
 * expand-down and an expand-up control so context can be pulled toward
 * whichever adjacent hunk is being read; the bottom gap only supports
 * expand-down. Falls back to "…" if neither applies.
 */
function updateControls(anchor: HTMLElement, isBottom: boolean): void {
  const cell = anchor.querySelector('.diff-expand-cell')
  if (!cell) return
  const more = hasMoreToExpand(anchor, isBottom)
  const revealed = revealedCount(anchor)

  cell.textContent = ''
  if (more) {
    if (isBottom) {
      cell.appendChild(makeControlButton('expand-bottom', 'expand', UNFOLD_SVG))
    } else {
      cell.appendChild(makeControlButton('expand-down', 'expand', FOLD_DOWN_SVG, 'down'))
      cell.appendChild(makeControlButton('expand-up', 'expand', FOLD_UP_SVG, 'up'))
    }
  }
  if (revealed > 0) {
    cell.appendChild(
      makeControlButton(isBottom ? 'collapse-bottom' : 'collapse-all', 'collapse', COLLAPSE_SVG)
    )
  }
  if (!more && revealed === 0) cell.textContent = '...'
}

/** Build a context diff row matching the server-rendered structure exactly so
 *  cursor navigation and the comment system treat it like any other line. When
 *  `split` is set it produces the four-column side-by-side structure; otherwise
 *  the three-column unified structure. Context lines are identical on both
 *  sides, so the same text fills the left and right content cells.
 *  Exported for testing. */
export function buildContextRow(
  rightNum: number,
  leftNum: number,
  content: string,
  groupId: string,
  split: boolean
): HTMLTableRowElement {
  const numClass =
    'diff-line-num text-right px-2 select-none w-12 cursor-pointer bg-gray-50 text-gray-400'

  const tr = document.createElement('tr')
  tr.className = 'diff-line diff-line-context diff-line-expanded'
  tr.setAttribute('data-left-num', String(leftNum))
  tr.setAttribute('data-right-num', String(rightNum))
  tr.setAttribute('data-line-type', 'context')
  tr.setAttribute('data-expand-group', groupId)

  const left = document.createElement('td')
  left.className = split ? numClass + ' diff-split-num-left' : numClass
  left.setAttribute('data-line-num', String(leftNum))
  left.setAttribute('data-side', 'left')
  left.textContent = leftNum > 0 ? String(leftNum) : ''

  const right = document.createElement('td')
  right.className = split
    ? numClass + ' diff-split-num-right diff-split-divider'
    : numClass
  right.setAttribute('data-line-num', String(rightNum))
  right.setAttribute('data-side', 'right')
  right.textContent = rightNum > 0 ? String(rightNum) : ''

  if (split) {
    const leftContent = document.createElement('td')
    leftContent.className =
      'diff-line-content diff-split-content-left px-3 whitespace-pre-wrap bg-white'
    leftContent.textContent = content

    const rightContent = document.createElement('td')
    rightContent.className =
      'diff-line-content diff-split-content-right px-3 whitespace-pre-wrap bg-white'
    rightContent.textContent = content

    // Column order: left num, left content, right num, right content.
    tr.append(left, leftContent, right, rightContent)
    return tr
  }

  const contentTd = document.createElement('td')
  contentTd.className = 'diff-line-content px-3 whitespace-pre-wrap bg-white'
  contentTd.textContent = content

  tr.append(left, right, contentTd)
  return tr
}

/**
 * Insert newly built rows (ascending right-num) so the diff table stays in
 * line-number order. The block is placed before the first existing sibling that
 * sorts after it: either an already-revealed row of the same group, or the
 * anchor row itself.
 */
function insertRows(
  anchor: HTMLElement,
  groupId: string,
  rows: HTMLTableRowElement[]
): void {
  const parent = anchor.parentElement
  if (!parent || rows.length === 0) return

  const lastRight = parseInt(
    rows[rows.length - 1].getAttribute('data-right-num') || '0',
    10
  )

  let ref: HTMLElement = anchor
  const existing = parent.querySelectorAll<HTMLElement>(
    `tr[data-expand-group="${groupId}"]`
  )
  for (const row of existing) {
    const rn = parseInt(row.getAttribute('data-right-num') || '0', 10)
    if (rn > lastRight) {
      ref = row
      break
    }
  }

  const frag = document.createDocumentFragment()
  rows.forEach((r) => frag.appendChild(r))
  parent.insertBefore(frag, ref)
}

function buildContextUrl(start: number, end: number): string {
  const cur = new URLSearchParams(window.location.search)
  const u = new URLSearchParams()
  u.set('repo', cur.get('repo') || '')
  u.set('source', cur.get('source') || '')
  u.set('target', cur.get('target') || '')
  u.set('source_commit', cur.get('source_commit') || '')
  u.set('target_commit', cur.get('target_commit') || '')
  u.set('mode', cur.get('mode') || 'branches')
  u.set('file', cur.get('file') || '')
  u.set('start', String(start))
  u.set('end', String(end))
  return '/api/file-context?' + u.toString()
}

async function handleExpandClick(btn: HTMLButtonElement): Promise<void> {
  const anchor = btn.closest('tr.diff-hunk-header') as HTMLElement | null
  if (!anchor || btn.disabled) return

  const isBottom = anchor.classList.contains('diff-bottom-expander')
  const direction: ExpandDirection =
    btn.getAttribute('data-direction') === 'up' ? 'up' : 'down'

  const rightStart = parseInt(anchor.getAttribute('data-gap-right-start') || '0', 10)
  const leftStart = parseInt(anchor.getAttribute('data-gap-left-start') || '0', 10)
  const rightEndAttr = anchor.getAttribute('data-gap-right-end')
  const rightEnd =
    isBottom || rightEndAttr === null ? null : parseInt(rightEndAttr, 10)

  if (rightStart < 1) return

  const range = computeFetchRange({ rightStart, rightEnd, leftStart }, CHUNK, direction)
  if (!range) return

  // The left/right line-number offset is invariant across a gap of unchanged
  // lines, so a single offset maps every revealed right-num to its left-num.
  const offset = leftStart - rightStart

  btn.disabled = true
  let data: FileContextResponse
  try {
    const resp = await fetch(buildContextUrl(range.start, range.end))
    if (!resp.ok) throw new Error('status ' + resp.status)
    data = (await resp.json()) as FileContextResponse
  } catch (err) {
    btn.disabled = false
    alert('Failed to expand context — please try again.')
    return
  }

  const lines = data.lines || []
  if (lines.length === 0) {
    // Nothing left to reveal (e.g. last hunk already at EOF).
    btn.disabled = false
    if (isBottom) {
      // A bottom expander that never revealed anything is just noise — drop it.
      if (revealedCount(anchor) === 0) {
        anchor.remove()
        return
      }
      anchor.setAttribute('data-eof', '1')
      updateControls(anchor, true)
    }
    return
  }

  // Remember the original collapsed-state bounds once, so the whole region can
  // be re-folded later regardless of how many chunks get revealed from either
  // end.
  if (!anchor.hasAttribute('data-collapsed-right-start')) {
    anchor.setAttribute('data-collapsed-right-start', String(rightStart))
    anchor.setAttribute('data-collapsed-left-start', String(leftStart))
    if (rightEnd !== null) {
      anchor.setAttribute('data-collapsed-right-end', String(rightEnd))
    }
  }

  const groupId = ensureGroupId(anchor)
  const split =
    (anchor.closest('table') as HTMLElement | null)?.getAttribute(
      'data-view'
    ) === 'split'
  const rows = lines.map((content, i) => {
    const r = range.start + i
    return buildContextRow(r, r + offset, content, groupId, split)
  })
  insertRows(anchor, groupId, rows)

  // Shrink the remaining gap by the number of lines actually revealed, from
  // whichever end this chunk came from.
  const revealed = lines.length
  if (direction === 'up' && rightEnd !== null) {
    anchor.setAttribute('data-gap-right-end', String(rightEnd - revealed))
  } else {
    anchor.setAttribute('data-gap-right-start', String(range.start + revealed))
    anchor.setAttribute('data-gap-left-start', String(leftStart + revealed))
  }
  if (isBottom && (data.eof || revealed < range.end - range.start + 1)) {
    anchor.setAttribute('data-eof', '1')
  }

  btn.disabled = false
  updateControls(anchor, isBottom)

  // Let cursor navigation pick up the new rows.
  document.dispatchEvent(new CustomEvent('diff:rows-changed'))
}

function handleCollapseClick(btn: HTMLButtonElement): void {
  const anchor = btn.closest('tr.diff-hunk-header') as HTMLElement | null
  if (!anchor) return

  const isBottom = anchor.classList.contains('diff-bottom-expander')
  const groupId = anchor.getAttribute('data-expand-group-id')
  if (groupId) {
    anchor.parentElement
      ?.querySelectorAll(`tr[data-expand-group="${groupId}"]`)
      .forEach((r) => r.remove())
  }

  // Restore the original collapsed-state bounds captured at first expand.
  const rs = anchor.getAttribute('data-collapsed-right-start')
  const ls = anchor.getAttribute('data-collapsed-left-start')
  const re = anchor.getAttribute('data-collapsed-right-end')
  if (rs) anchor.setAttribute('data-gap-right-start', rs)
  if (ls) anchor.setAttribute('data-gap-left-start', ls)
  if (re) anchor.setAttribute('data-gap-right-end', re)
  anchor.removeAttribute('data-collapsed-right-start')
  anchor.removeAttribute('data-collapsed-left-start')
  anchor.removeAttribute('data-collapsed-right-end')
  anchor.removeAttribute('data-eof')

  updateControls(anchor, isBottom)
  document.dispatchEvent(new CustomEvent('diff:rows-changed'))
}

/**
 * Wire up expand-context controls on the diff page. Uses event delegation so it
 * keeps working after the expander cells re-render themselves.
 */
export function initializeContextExpansion(options?: {
  signal?: AbortSignal
}): void {
  const diffTable = document.querySelector('.diff-table') as HTMLElement | null
  if (!diffTable) return

  diffTable.addEventListener(
    'click',
    (e) => {
      const target = e.target as HTMLElement
      const btn = target.closest('.diff-expand-btn') as HTMLButtonElement | null
      if (!btn) return
      e.preventDefault()
      if (btn.getAttribute('data-action') === 'collapse') {
        handleCollapseClick(btn)
      } else {
        void handleExpandClick(btn)
      }
    },
    options?.signal ? { signal: options.signal } : undefined
  )
}
