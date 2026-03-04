/**
 * Cursor position persistence via URL hash.
 *
 * Pure functions for serializing/deserializing cursor state
 * to/from the URL hash fragment, enabling cursor position
 * preservation across page reloads (e.g., after comment operations).
 */

export interface CursorPosition {
  index: number
  side: 'left' | 'right'
}

/**
 * Serialize cursor position into a URL hash string (without the leading #).
 * Returns null if the cursor index is invalid (negative).
 */
export function serializeCursorHash(
  index: number,
  side: string
): string | null {
  if (index < 0) return null
  return `cursor=${index}&side=${side || 'right'}`
}

/**
 * Parse a URL hash string into a CursorPosition.
 * Returns null if the hash is empty or contains invalid data.
 */
export function parseCursorHash(hash: string): CursorPosition | null {
  const cleaned = hash.replace(/^#/, '')
  if (!cleaned) return null

  const params = new URLSearchParams(cleaned)
  const idx = parseInt(params.get('cursor') || '', 10)
  const side = params.get('side') || 'right'

  if (isNaN(idx) || idx < 0) return null

  return { index: idx, side: side as 'left' | 'right' }
}

/**
 * Save the current cursor position into the URL hash.
 * Uses history.replaceState to avoid adding to the history stack.
 */
export function saveCursorToHash(index: number, side: string): void {
  const hash = serializeCursorHash(index, side)
  if (hash) {
    history.replaceState(null, '', '#' + hash)
  }
}

/**
 * Restore cursor position from the URL hash and clean it.
 * Returns the parsed position or null if not present.
 */
export function restoreCursorFromHash(): CursorPosition | null {
  const result = parseCursorHash(window.location.hash)
  if (result) {
    // Clean hash after reading
    history.replaceState(
      null,
      '',
      window.location.pathname + window.location.search
    )
  }
  return result
}
