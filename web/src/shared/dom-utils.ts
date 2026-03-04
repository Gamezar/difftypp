/**
 * escapeHtml prevents XSS by encoding HTML special characters.
 * Uses a temporary DOM element for safe escaping.
 */
export function escapeHtml(str: string): string {
  const div = document.createElement('div')
  div.textContent = str
  return div.innerHTML
}

/**
 * Shorthand for document.getElementById with type narrowing.
 * Returns null if the element does not exist.
 */
export function $<T extends HTMLElement>(id: string): T | null {
  return document.getElementById(id) as T | null
}

/**
 * Shorthand for querySelectorAll that returns an array.
 */
export function $$<T extends Element>(
  selector: string,
  root: ParentNode = document
): T[] {
  return Array.from(root.querySelectorAll<T>(selector))
}
