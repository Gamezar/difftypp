/**
 * Shared polling primitives for the lightweight "has this changed?" loops on the
 * diff and compare pages (freshness fingerprint, recent-commits refresh).
 */

/** Interval between background polls, in milliseconds. */
export const POLL_INTERVAL_MS = 8000

/**
 * fetchJSON GETs a JSON endpoint and returns the parsed body, or null on any
 * non-OK response or transient/network/parse error — so pollers can simply keep
 * their existing state instead of treating a blip as a change. Callers are
 * responsible for validating the shape of the returned value.
 */
export async function fetchJSON<T>(url: string): Promise<T | null> {
  try {
    const res = await fetch(url, { headers: { Accept: 'application/json' } })
    if (!res.ok) return null
    return (await res.json()) as T
  } catch {
    return null
  }
}
