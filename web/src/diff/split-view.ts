/**
 * Split-view column resizer.
 *
 * The side-by-side diff table splits the space left by the line-number gutters
 * between its two content columns. This module lets the user drag the divider
 * between the two sides to change that split, persists the choice in
 * localStorage, and restores it on load. A double-click resets to even.
 *
 * The two content <col>s are sized directly (as percentages) rather than via a
 * CSS custom property, because Chromium does not re-evaluate a <col> width whose
 * calc() references a custom property when that property changes. The
 * --split-ratio property (0-100, left side) is still updated so the resizer
 * handle — an ordinary element, where calc(var()) works — can position itself
 * with plain CSS.
 */

const RATIO_KEY = 'diffty:split-ratio'
const MIN_RATIO = 15
const MAX_RATIO = 85
const DEFAULT_RATIO = 50
const FALLBACK_NUM_WIDTH = 56 // px, used only if a gutter cell can't be measured

/** Clamp a value to the inclusive [min, max] range. */
function clamp(val: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, val))
}

/**
 * Compute the left-side ratio (percentage, clamped) for a pointer at clientX,
 * given the wrapper's bounding rect and the width of one line-number gutter.
 * The two gutters (one per side) are excluded so the ratio describes only how
 * the remaining content space is divided. Exported for testing.
 */
export function computeSplitRatio(
  clientX: number,
  wrapperRect: { left: number; width: number },
  numWidth: number,
  min: number = MIN_RATIO,
  max: number = MAX_RATIO
): number {
  const contentArea = wrapperRect.width - 2 * numWidth
  if (contentArea <= 0) return DEFAULT_RATIO
  const raw = ((clientX - wrapperRect.left - numWidth) / contentArea) * 100
  return clamp(raw, min, max)
}

/** Read a persisted ratio, or null when absent/invalid/unavailable. */
export function readStoredRatio(): number | null {
  try {
    const raw = localStorage.getItem(RATIO_KEY)
    if (raw === null) return null
    const n = parseFloat(raw)
    return Number.isFinite(n) ? clamp(n, MIN_RATIO, MAX_RATIO) : null
  } catch {
    return null
  }
}

function storeRatio(n: number): void {
  try {
    localStorage.setItem(RATIO_KEY, String(Math.round(n * 10) / 10))
  } catch {
    // localStorage may be unavailable (private mode, disabled) — ignore.
  }
}

/**
 * Wire up the split-view resizer. No-op when the split table isn't present, so
 * it's safe to call unconditionally from the diff page bootstrap.
 */
export function initializeSplitResizer(options?: {
  signal?: AbortSignal
}): void {
  const wrapper = document.querySelector(
    '.diff-split-wrapper'
  ) as HTMLElement | null
  if (!wrapper) return
  const resizer = wrapper.querySelector(
    '.diff-split-resizer'
  ) as HTMLElement | null
  if (!resizer) return
  const table = wrapper.querySelector(
    '.diff-table-split'
  ) as HTMLElement | null
  const colLeft = wrapper.querySelector(
    'col.diff-col-left'
  ) as HTMLElement | null
  const colRight = wrapper.querySelector(
    'col.diff-col-right'
  ) as HTMLElement | null
  const listenerOpts = options?.signal ? { signal: options.signal } : undefined

  // Apply a ratio to both the columns (authoritative widths) and the
  // --split-ratio property (drives the resizer handle's CSS position).
  function applyRatio(ratio: number): void {
    wrapper!.style.setProperty('--split-ratio', String(ratio))
    if (colLeft) colLeft.style.width = ratio + '%'
    if (colRight) colRight.style.width = 100 - ratio + '%'
  }

  const stored = readStoredRatio()
  if (stored !== null) applyRatio(stored)

  // Measure a real gutter cell so the maths track the rendered layout rather
  // than assuming a fixed rem-to-px conversion.
  function gutterWidth(): number {
    const cell = table?.querySelector('.diff-line-num') as HTMLElement | null
    if (!cell) return FALLBACK_NUM_WIDTH
    const w = cell.getBoundingClientRect().width
    return w > 0 ? w : FALLBACK_NUM_WIDTH
  }

  let dragging = false

  function onMove(e: MouseEvent): void {
    if (!dragging) return
    const ratio = computeSplitRatio(
      e.clientX,
      wrapper!.getBoundingClientRect(),
      gutterWidth()
    )
    applyRatio(ratio)
  }

  function stop(): void {
    if (!dragging) return
    dragging = false
    resizer!.classList.remove('dragging')
    document.body.style.userSelect = ''
    const cur = parseFloat(wrapper!.style.getPropertyValue('--split-ratio'))
    if (Number.isFinite(cur)) storeRatio(cur)
  }

  resizer.addEventListener(
    'mousedown',
    (e) => {
      e.preventDefault()
      dragging = true
      resizer.classList.add('dragging')
      // Suppress text selection while dragging across the diff.
      document.body.style.userSelect = 'none'
    },
    listenerOpts
  )
  document.addEventListener('mousemove', onMove, listenerOpts)
  document.addEventListener('mouseup', stop, listenerOpts)

  // Double-click restores the even split.
  resizer.addEventListener(
    'dblclick',
    () => {
      applyRatio(DEFAULT_RATIO)
      storeRatio(DEFAULT_RATIO)
    },
    listenerOpts
  )
}
