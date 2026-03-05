import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  buildBreadcrumbHtml,
  buildListingHtml,
  buildSelectButtonState,
  initializeFileExplorer,
} from './file-explorer'
import type { BrowseResponse } from './file-explorer'

// ── Pure function tests ──────────────────────────────────────────

describe('buildBreadcrumbHtml', () => {
  it('renders root-only breadcrumb for "/"', () => {
    const html = buildBreadcrumbHtml('/')
    expect(html).toContain('data-testid="crumb-root"')
    expect(html).toContain('data-path="/"')
    expect(html).not.toContain('browse-crumb-sep')
  })

  it('renders clickable segments for a deep path', () => {
    const html = buildBreadcrumbHtml('/home/user/projects')
    expect(html).toContain('data-testid="crumb-root"')
    // First two segments are clickable buttons
    expect(html).toContain('data-path="/home"')
    expect(html).toContain('data-path="/home/user"')
    // Last segment is current (not a button)
    expect(html).toContain('browse-crumb-current')
    expect(html).toContain('projects')
  })

  it('renders last segment as non-clickable current', () => {
    const html = buildBreadcrumbHtml('/var/log')
    expect(html).toContain('<span class="browse-crumb-current">log</span>')
    // "var" should be a clickable button
    expect(html).toContain('data-path="/var"')
  })

  it('escapes HTML in path segments', () => {
    const html = buildBreadcrumbHtml('/home/<script>')
    expect(html).not.toContain('<script>')
    expect(html).toContain('&lt;script&gt;')
  })

  it('renders separators between segments', () => {
    const html = buildBreadcrumbHtml('/a/b/c')
    const sepCount = (html.match(/browse-crumb-sep/g) || []).length
    expect(sepCount).toBe(3)
  })
})

describe('buildListingHtml', () => {
  it('renders empty message when no entries', () => {
    const data: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [],
    }
    const html = buildListingHtml(data)
    expect(html).toContain('No subdirectories found.')
  })

  it('renders parent directory (..) entry', () => {
    const data: BrowseResponse = {
      current_path: '/home/user',
      parent_path: '/home',
      is_git_repo: false,
      entries: [
        { name: 'projects', path: '/home/user/projects', is_git_repo: false },
      ],
    }
    const html = buildListingHtml(data)
    expect(html).toContain('data-path="/home"')
    expect(html).toContain('browse-item-parent')
    expect(html).toContain('..')
  })

  it('does not render parent when parent equals current', () => {
    const data: BrowseResponse = {
      current_path: '/',
      parent_path: '/',
      is_git_repo: false,
      entries: [
        { name: 'home', path: '/home', is_git_repo: false },
      ],
    }
    const html = buildListingHtml(data)
    expect(html).not.toContain('browse-item-parent')
  })

  it('renders directory entries', () => {
    const data: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [
        { name: 'docs', path: '/home/docs', is_git_repo: false },
        { name: 'src', path: '/home/src', is_git_repo: false },
      ],
    }
    const html = buildListingHtml(data)
    expect(html).toContain('data-path="/home/docs"')
    expect(html).toContain('data-path="/home/src"')
    expect(html).toContain('docs')
    expect(html).toContain('src')
  })

  it('renders git repo entries with badge and special icon', () => {
    const data: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [
        { name: 'my-repo', path: '/home/my-repo', is_git_repo: true },
      ],
    }
    const html = buildListingHtml(data)
    expect(html).toContain('browse-item-repo')
    expect(html).toContain('browse-icon-repo')
    expect(html).toContain('browse-badge')
    expect(html).toContain('git')
    expect(html).toContain('data-is-repo="true"')
  })

  it('marks non-repo entries with data-is-repo=false', () => {
    const data: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [
        { name: 'docs', path: '/home/docs', is_git_repo: false },
      ],
    }
    const html = buildListingHtml(data)
    expect(html).toContain('data-is-repo="false"')
    expect(html).not.toContain('browse-item-repo')
  })

  it('escapes HTML in entry names and paths', () => {
    const data: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [
        { name: '<img>', path: '/home/<img>', is_git_repo: false },
      ],
    }
    const html = buildListingHtml(data)
    expect(html).not.toContain('<img>')
    expect(html).toContain('&lt;img&gt;')
  })
})

describe('buildSelectButtonState', () => {
  it('returns git repo state when path is a git repo', () => {
    const state = buildSelectButtonState('/home/repo', true)
    expect(state.disabled).toBe(false)
    expect(state.text).toBe('/home/repo (git repository)')
    expect(state.className).toContain('text-green-600')
  })

  it('returns non-repo state when path is not a git repo', () => {
    const state = buildSelectButtonState('/home/docs', false)
    expect(state.disabled).toBe(false)
    expect(state.text).toBe('/home/docs')
    expect(state.className).toContain('text-gray-500')
  })
})

// ── DOM integration tests ────────────────────────────────────────

describe('initializeFileExplorer', () => {
  let ac: AbortController

  beforeEach(() => {
    ac = new AbortController()
    document.body.innerHTML = `
      <input id="repo-path" type="text" value="" />
      <button id="btn-browse">Browse</button>
      <div id="browse-modal" style="display: none">
        <button id="browse-close">×</button>
        <div id="browse-breadcrumb"></div>
        <div id="browse-listing"></div>
        <span id="browse-selected-path"></span>
        <button id="browse-select" disabled>Select</button>
      </div>
    `
  })

  afterEach(() => {
    ac.abort()
  })

  it('opens modal on browse button click', () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '/',
          parent_path: '/',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const browseBtn = document.getElementById('btn-browse')!
    browseBtn.click()

    const modal = document.getElementById('browse-modal')!
    expect(modal.style.display).toBe('flex')

    vi.unstubAllGlobals()
  })

  it('closes modal on close button click', () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '/',
          parent_path: '/',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const modal = document.getElementById('browse-modal')!
    modal.style.display = 'flex'

    const closeBtn = document.getElementById('browse-close')!
    closeBtn.click()
    expect(modal.style.display).toBe('none')

    vi.unstubAllGlobals()
  })

  it('closes modal on overlay click', () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '/',
          parent_path: '/',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const modal = document.getElementById('browse-modal')!
    modal.style.display = 'flex'

    // Click on modal itself (overlay)
    modal.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(modal.style.display).toBe('none')

    vi.unstubAllGlobals()
  })

  it('does not close modal when clicking inside content', () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '/',
          parent_path: '/',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const modal = document.getElementById('browse-modal')!
    modal.style.display = 'flex'

    // Click on child element (not the overlay)
    const listing = document.getElementById('browse-listing')!
    listing.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(modal.style.display).toBe('flex')

    vi.unstubAllGlobals()
  })

  it('closes modal on Escape key', () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '/',
          parent_path: '/',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const modal = document.getElementById('browse-modal')!
    modal.style.display = 'flex'

    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    )
    expect(modal.style.display).toBe('none')

    vi.unstubAllGlobals()
  })

  it('does not close modal on Escape when already hidden', () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '/',
          parent_path: '/',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const modal = document.getElementById('browse-modal')!
    expect(modal.style.display).toBe('none')

    // Escape on hidden modal — should be no-op (no error)
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })
    )
    expect(modal.style.display).toBe('none')

    vi.unstubAllGlobals()
  })

  it('fetches directory on browse and renders listing', async () => {
    const mockData: BrowseResponse = {
      current_path: '/home/user',
      parent_path: '/home',
      is_git_repo: false,
      entries: [
        { name: 'projects', path: '/home/user/projects', is_git_repo: false },
        { name: 'my-repo', path: '/home/user/my-repo', is_git_repo: true },
      ],
    }
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockData),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const browseBtn = document.getElementById('btn-browse')!
    browseBtn.click()

    // Wait for async fetch to resolve
    await vi.waitFor(() => {
      const listing = document.getElementById('browse-listing')!
      expect(listing.innerHTML).toContain('projects')
    })

    expect(fetchSpy).toHaveBeenCalledWith('/api/browse')

    const listing = document.getElementById('browse-listing')!
    expect(listing.innerHTML).toContain('my-repo')
    expect(listing.innerHTML).toContain('browse-item-repo')

    const breadcrumb = document.getElementById('browse-breadcrumb')!
    expect(breadcrumb.innerHTML).toContain('data-testid="crumb-root"')
    expect(breadcrumb.innerHTML).toContain('user')

    vi.unstubAllGlobals()
  })

  it('passes path from input to fetch when browsing', async () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '/opt',
          parent_path: '/',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    const input = document.getElementById('repo-path') as HTMLInputElement
    input.value = '/opt'

    initializeFileExplorer({ signal: ac.signal })

    const browseBtn = document.getElementById('btn-browse')!
    browseBtn.click()

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith(
        '/api/browse?path=' + encodeURIComponent('/opt')
      )
    })

    vi.unstubAllGlobals()
  })

  it('select button copies path to input and closes modal', async () => {
    const mockData: BrowseResponse = {
      current_path: '/home/my-repo',
      parent_path: '/home',
      is_git_repo: true,
      entries: [],
    }
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockData),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    // Open modal and fetch
    document.getElementById('btn-browse')!.click()

    await vi.waitFor(() => {
      const selectedPath = document.getElementById('browse-selected-path')!
      expect(selectedPath.textContent).toContain('/home/my-repo')
    })

    // Click select
    const selectBtn = document.getElementById('browse-select')!
    selectBtn.click()

    const input = document.getElementById('repo-path') as HTMLInputElement
    expect(input.value).toBe('/home/my-repo')
    expect(document.getElementById('browse-modal')!.style.display).toBe('none')

    vi.unstubAllGlobals()
  })

  it('displays error when fetch fails', async () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: false,
      text: () => Promise.resolve('Not found'),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    document.getElementById('btn-browse')!.click()

    await vi.waitFor(() => {
      const listing = document.getElementById('browse-listing')!
      expect(listing.innerHTML).toContain('Error:')
      expect(listing.innerHTML).toContain('Not found')
    })

    vi.unstubAllGlobals()
  })

  it('navigates when clicking a directory item', async () => {
    const firstResponse: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [
        { name: 'user', path: '/home/user', is_git_repo: false },
      ],
    }
    const secondResponse: BrowseResponse = {
      current_path: '/home/user',
      parent_path: '/home',
      is_git_repo: false,
      entries: [],
    }
    const fetchSpy = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(firstResponse),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(secondResponse),
      })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })
    document.getElementById('btn-browse')!.click()

    await vi.waitFor(() => {
      const listing = document.getElementById('browse-listing')!
      expect(listing.innerHTML).toContain('user')
    })

    // Click on the "user" directory item (not the parent ".." entry)
    const item = document.querySelector(
      '.browse-item[data-path="/home/user"]'
    ) as HTMLElement
    item.click()

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledTimes(2)
    })

    expect(fetchSpy).toHaveBeenLastCalledWith(
      '/api/browse?path=' + encodeURIComponent('/home/user')
    )

    vi.unstubAllGlobals()
  })

  it('navigates when clicking a breadcrumb', async () => {
    const firstResponse: BrowseResponse = {
      current_path: '/home/user',
      parent_path: '/home',
      is_git_repo: false,
      entries: [],
    }
    const secondResponse: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [],
    }
    const fetchSpy = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(firstResponse),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(secondResponse),
      })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })
    document.getElementById('btn-browse')!.click()

    await vi.waitFor(() => {
      const breadcrumb = document.getElementById('browse-breadcrumb')!
      expect(breadcrumb.innerHTML).toContain('data-path="/home"')
    })

    // Click on "home" breadcrumb
    const crumb = document.querySelector(
      '.browse-crumb[data-path="/home"]'
    ) as HTMLElement
    crumb.click()

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledTimes(2)
    })

    expect(fetchSpy).toHaveBeenLastCalledWith(
      '/api/browse?path=' + encodeURIComponent('/home')
    )

    vi.unstubAllGlobals()
  })

  it('shows loading state while fetching', () => {
    const fetchSpy = vi.fn().mockReturnValue(new Promise(() => {}))
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })
    document.getElementById('btn-browse')!.click()

    const listing = document.getElementById('browse-listing')!
    expect(listing.innerHTML).toContain('Loading...')

    vi.unstubAllGlobals()
  })

  it('does nothing when select is clicked with no path', () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          current_path: '',
          parent_path: '',
          is_git_repo: false,
          entries: [],
        }),
    })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })

    const modal = document.getElementById('browse-modal')!
    modal.style.display = 'flex'

    const selectBtn = document.getElementById('browse-select')!
    selectBtn.click()

    // Modal should stay open since there's no valid path
    expect(modal.style.display).toBe('flex')

    vi.unstubAllGlobals()
  })
})

// ── Additional coverage tests ────────────────────────────────────

describe('buildListingHtml — entries undefined guard', () => {
  it('returns "No subdirectories found" when entries is undefined', () => {
    const data = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: undefined,
    } as unknown as BrowseResponse
    const html = buildListingHtml(data)
    expect(html).toContain('No subdirectories found.')
  })
})

describe('initializeFileExplorer — additional coverage', () => {
  let ac: AbortController

  beforeEach(() => {
    ac = new AbortController()
  })

  afterEach(() => {
    ac.abort()
  })

  it('returns immediately without throwing when DOM elements are missing', () => {
    document.body.innerHTML = ''
    expect(() => initializeFileExplorer({ signal: ac.signal })).not.toThrow()
  })

  it('displays error when fetch rejects at network level', async () => {
    document.body.innerHTML = `
      <input id="repo-path" type="text" value="" />
      <button id="btn-browse">Browse</button>
      <div id="browse-modal" style="display: none">
        <button id="browse-close">x</button>
        <div id="browse-breadcrumb"></div>
        <div id="browse-listing"></div>
        <span id="browse-selected-path"></span>
        <button id="browse-select" disabled>Select</button>
      </div>
    `

    const fetchSpy = vi
      .fn()
      .mockRejectedValue(new TypeError('Failed to fetch'))
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })
    document.getElementById('btn-browse')!.click()

    await vi.waitFor(() => {
      const listing = document.getElementById('browse-listing')!
      expect(listing.innerHTML).toContain('Error:')
      expect(listing.innerHTML).toContain('Failed to fetch')
    })

    vi.unstubAllGlobals()
  })

  it('navigates to parent directory when clicking (..) item', async () => {
    document.body.innerHTML = `
      <input id="repo-path" type="text" value="" />
      <button id="btn-browse">Browse</button>
      <div id="browse-modal" style="display: none">
        <button id="browse-close">x</button>
        <div id="browse-breadcrumb"></div>
        <div id="browse-listing"></div>
        <span id="browse-selected-path"></span>
        <button id="browse-select" disabled>Select</button>
      </div>
    `

    const firstResponse: BrowseResponse = {
      current_path: '/home/user',
      parent_path: '/home',
      is_git_repo: false,
      entries: [
        { name: 'projects', path: '/home/user/projects', is_git_repo: false },
      ],
    }
    const secondResponse: BrowseResponse = {
      current_path: '/home',
      parent_path: '/',
      is_git_repo: false,
      entries: [
        { name: 'user', path: '/home/user', is_git_repo: false },
      ],
    }
    const fetchSpy = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(firstResponse),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(secondResponse),
      })
    vi.stubGlobal('fetch', fetchSpy)

    initializeFileExplorer({ signal: ac.signal })
    document.getElementById('btn-browse')!.click()

    await vi.waitFor(() => {
      const listing = document.getElementById('browse-listing')!
      expect(listing.innerHTML).toContain('browse-item-parent')
    })

    const parentItem = document.querySelector(
      '.browse-item-parent'
    ) as HTMLElement
    parentItem.click()

    await vi.waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledTimes(2)
    })

    expect(fetchSpy).toHaveBeenLastCalledWith(
      '/api/browse?path=' + encodeURIComponent('/home')
    )

    vi.unstubAllGlobals()
  })
})
