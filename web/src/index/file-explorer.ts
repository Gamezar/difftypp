/**
 * File explorer modal for browsing and selecting Git repositories.
 *
 * Provides a modal-based directory browser that fetches listings from
 * the /api/browse endpoint, renders breadcrumb navigation, and lets
 * users pick a Git repository path.
 */

import { escapeHtml } from '../shared/dom-utils'

// ── Types ────────────────────────────────────────────────────────

export interface BrowseEntry {
  name: string
  path: string
  is_git_repo: boolean
}

export interface BrowseResponse {
  current_path: string
  parent_path: string
  is_git_repo: boolean
  entries: BrowseEntry[]
}

export interface SelectButtonState {
  disabled: boolean
  text: string
  className: string
}

// ── Pure functions (exported for testing) ────────────────────────

/**
 * Build breadcrumb HTML from a directory path.
 * Each segment except the last is a clickable button; the last is a
 * static span showing the current directory.
 */
export function buildBreadcrumbHtml(currentPath: string): string {
  const parts = currentPath.split('/').filter((p) => p !== '')
  let html =
    '<button class="browse-crumb" data-path="/" data-testid="crumb-root">/</button>'

  let built = ''
  for (let i = 0; i < parts.length; i++) {
    built += '/' + parts[i]
    const isLast = i === parts.length - 1

    html += '<span class="browse-crumb-sep">/</span>'
    if (isLast) {
      html +=
        '<span class="browse-crumb-current">' +
        escapeHtml(parts[i]) +
        '</span>'
    } else {
      html +=
        '<button class="browse-crumb" data-path="' +
        escapeHtml(built) +
        '">' +
        escapeHtml(parts[i]) +
        '</button>'
    }
  }

  return html
}

/**
 * Build listing HTML from a browse API response.
 * Renders parent (..) entry, directory entries, and git repo entries
 * with appropriate icons and badges.
 */
export function buildListingHtml(data: BrowseResponse): string {
  if (!data.entries || data.entries.length === 0) {
    return '<p class="text-gray-400 text-sm p-4">No subdirectories found.</p>'
  }

  let html = '<ul class="browse-list">'

  if (data.parent_path && data.parent_path !== data.current_path) {
    html +=
      '<li class="browse-item browse-item-parent" data-path="' +
      escapeHtml(data.parent_path) +
      '">'
    html += '<span class="browse-icon">&#x1F4C1;</span>'
    html += '<span class="browse-name">..</span>'
    html += '</li>'
  }

  for (const entry of data.entries) {
    let classes = 'browse-item'
    if (entry.is_git_repo) {
      classes += ' browse-item-repo'
    }

    html +=
      '<li class="' +
      classes +
      '" data-path="' +
      escapeHtml(entry.path) +
      '" data-is-repo="' +
      entry.is_git_repo +
      '">'

    if (entry.is_git_repo) {
      html += '<span class="browse-icon browse-icon-repo">&#x1F4E6;</span>'
    } else {
      html += '<span class="browse-icon">&#x1F4C1;</span>'
    }

    html += '<span class="browse-name">' + escapeHtml(entry.name) + '</span>'

    if (entry.is_git_repo) {
      html += '<span class="browse-badge">git</span>'
    }

    html += '</li>'
  }

  html += '</ul>'
  return html
}

/**
 * Compute the select button's display state based on current path
 * and whether it's a git repository.
 */
export function buildSelectButtonState(
  currentPath: string,
  isGitRepo: boolean
): SelectButtonState {
  if (isGitRepo) {
    return {
      disabled: false,
      text: currentPath + ' (git repository)',
      className: 'text-sm text-green-600 truncate flex-1',
    }
  }
  return {
    disabled: false,
    text: currentPath,
    className: 'text-sm text-gray-500 truncate flex-1',
  }
}

// ── DOM-interactive initialization ───────────────────────────────

/**
 * Initialize the file explorer modal on the index page.
 *
 * Wires up the Browse button, modal overlay, breadcrumb navigation,
 * directory listing clicks, select button, and keyboard dismiss.
 *
 * @param options - Optional config; pass { signal } from an AbortController
 *                  to tear down document-level listeners (useful in tests).
 */
export function initializeFileExplorer(
  options?: { signal?: AbortSignal }
): void {
  const modal = document.getElementById('browse-modal')
  const listing = document.getElementById('browse-listing')
  const breadcrumb = document.getElementById('browse-breadcrumb')
  const selectedPathEl = document.getElementById('browse-selected-path')
  const selectBtn = document.getElementById(
    'browse-select'
  ) as HTMLButtonElement | null
  const repoPathInput = document.getElementById(
    'repo-path'
  ) as HTMLInputElement | null

  if (!modal || !listing || !breadcrumb || !selectedPathEl || !selectBtn) {
    return
  }

  const listenerOpts = options?.signal
    ? { signal: options.signal }
    : undefined

  let currentPath = ''
  let currentIsGitRepo = false

  function closeModal(): void {
    if (modal) modal.style.display = 'none'
  }

  function updateSelectButton(): void {
    const state = buildSelectButtonState(currentPath, currentIsGitRepo)
    if (selectBtn) selectBtn.disabled = state.disabled
    if (selectedPathEl) {
      selectedPathEl.textContent = state.text
      selectedPathEl.className = state.className
    }
  }

  function wireBreadcrumbClicks(): void {
    if (!breadcrumb) return
    const crumbs = breadcrumb.querySelectorAll('.browse-crumb')
    for (let j = 0; j < crumbs.length; j++) {
      crumbs[j].addEventListener('click', function (this: HTMLElement) {
        fetchDirectory(this.getAttribute('data-path') || '/')
      })
    }
  }

  function wireListingClicks(): void {
    if (!listing) return
    const items = listing.querySelectorAll('.browse-item')
    for (let j = 0; j < items.length; j++) {
      items[j].addEventListener('click', function (this: HTMLElement) {
        fetchDirectory(this.getAttribute('data-path') || '/')
      })
    }
  }

  function fetchDirectory(path: string): void {
    let url = '/api/browse'
    if (path) {
      url += '?path=' + encodeURIComponent(path)
    }

    if (listing) {
      listing.innerHTML =
        '<p class="text-gray-400 text-sm p-4">Loading...</p>'
    }

    fetch(url)
      .then((resp) => {
        if (!resp.ok) {
          return resp.text().then((t) => {
            throw new Error(t)
          })
        }
        return resp.json()
      })
      .then((data: BrowseResponse) => {
        currentPath = data.current_path
        currentIsGitRepo = data.is_git_repo

        if (breadcrumb) {
          breadcrumb.innerHTML = buildBreadcrumbHtml(data.current_path)
          wireBreadcrumbClicks()
        }

        if (listing) {
          listing.innerHTML = buildListingHtml(data)
          wireListingClicks()
        }

        updateSelectButton()
      })
      .catch((err: Error) => {
        if (listing) {
          listing.innerHTML =
            '<p class="text-red-500 text-sm p-4">Error: ' +
            escapeHtml(err.message) +
            '</p>'
        }
      })
  }

  // Browse button opens the modal
  document
    .getElementById('btn-browse')
    ?.addEventListener('click', () => {
      modal.style.display = 'flex'
      const startPath = repoPathInput?.value || ''
      fetchDirectory(startPath)
    }, listenerOpts)

  // Close button
  document
    .getElementById('browse-close')
    ?.addEventListener('click', closeModal, listenerOpts)

  // Click on overlay closes modal
  modal.addEventListener('click', (e) => {
    if (e.target === modal) closeModal()
  }, listenerOpts)

  // Escape key closes modal
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && modal.style.display !== 'none') {
      closeModal()
    }
  }, listenerOpts)

  // Select button copies current path to input
  selectBtn.addEventListener('click', () => {
    if (currentPath) {
      if (repoPathInput) repoPathInput.value = currentPath
      closeModal()
    }
  }, listenerOpts)
}
