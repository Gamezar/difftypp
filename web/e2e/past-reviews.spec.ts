/**
 * End-to-end tests for the Past Reviews feature.
 *
 * Each test creates a temporary Git repository, registers it with the
 * running diffty server, performs review operations via the UI, and
 * verifies that the past-reviews sidebar behaves correctly.
 *
 * Tests are serial — they share a single browser context and must not
 * run in parallel because they mutate server-side storage.
 */

import { test, expect, Page } from '@playwright/test'
import { execFileSync } from 'child_process'
import { mkdtempSync, rmSync, writeFileSync, existsSync } from 'fs'
import { tmpdir, homedir } from 'os'
import { join } from 'path'
import { PORT } from '../playwright.config'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DIFFTY_HOME = join(homedir(), '.difftypp')
const BASE_URL = `http://localhost:${PORT}`

/**
 * Sanitize a path the same way Go's sanitizeRepoPath does:
 * replace path separators and colons with underscores.
 */
function sanitizePath(p: string): string {
  return p.replace(/[/:]/g, '_')
}

// ---------------------------------------------------------------------------
// Git helpers (execFileSync with array args — no shell, no injection risk)
// ---------------------------------------------------------------------------

const GIT_ENV = {
  ...process.env,
  GIT_AUTHOR_NAME: 'test',
  GIT_AUTHOR_EMAIL: 'test@test.com',
  GIT_COMMITTER_NAME: 'test',
  GIT_COMMITTER_EMAIL: 'test@test.com',
}

function git(repoDir: string, ...args: string[]): void {
  execFileSync('git', args, { cwd: repoDir, stdio: 'pipe', env: GIT_ENV })
}

function gitOutput(repoDir: string, ...args: string[]): string {
  return execFileSync('git', args, { cwd: repoDir, stdio: 'pipe', env: GIT_ENV }).toString().trim()
}

// ---------------------------------------------------------------------------
// Factory helpers
// ---------------------------------------------------------------------------

/** Create a temporary Git repo with an initial commit and return its path. */
function createTestRepo(prefix: string): string {
  const dir = mkdtempSync(join(tmpdir(), `diffty-e2e-${prefix}-`))
  git(dir, 'init')
  // Rename the default branch to 'main' (compatible with git < 2.28 which lacks -b)
  git(dir, 'checkout', '-b', 'main')
  writeFileSync(join(dir, 'README.md'), '# Test Repo\n')
  git(dir, 'add', '.')
  git(dir, 'commit', '-m', 'Initial commit')
  return dir
}

/** Register a repo with the diffty server via POST /api/repository/add */
async function registerRepo(page: Page, repoPath: string): Promise<void> {
  const response = await page.request.post(`${BASE_URL}/api/repository/add`, {
    form: { path: repoPath },
  })
  // The handler redirects to / on success (303)
  expect(response.status()).toBeLessThan(400)
}

/**
 * Create a branch review by:
 * 1. Navigating to the diff view for the given branch pair
 * 2. Adding a comment on the first file
 * 3. Submitting the review
 *
 * Returns the commit hashes used for the review.
 */
async function createBranchReview(
  page: Page,
  repoPath: string,
  sourceBranch: string,
  targetBranch: string,
  commentBody: string,
): Promise<{ sourceCommit: string; targetCommit: string }> {
  // Get commit hashes for these branches
  const sourceCommit = gitOutput(repoPath, 'rev-parse', sourceBranch)
  const targetCommit = gitOutput(repoPath, 'rev-parse', targetBranch)

  // Navigate to compare and submit form to get to diff
  const compareUrl = `/compare?repo=${encodeURIComponent(repoPath)}&mode=branches`
  await page.goto(compareUrl)

  // Fill and submit the branch comparison form
  await page.locator('[data-testid="select-source-branch"]').selectOption(sourceBranch)
  await page.locator('[data-testid="select-target-branch"]').selectOption(targetBranch)
  await page.locator('[data-testid="btn-compare-branches"]').click()

  // Wait for diff page to load (auto-redirects to first file)
  await page.waitForURL(/\/diff\?/)

  // Add a comment — click on a diff line to position cursor, then use keyboard
  // The comment system uses the cursor position; let's use the API directly instead
  const diffUrl = page.url()
  const urlObj = new URL(diffUrl, BASE_URL)
  const file = urlObj.searchParams.get('file') ?? ''

  // Add comment via API (more reliable than UI interaction in e2e)
  const addCommentResponse = await page.request.post(
    `${BASE_URL}/api/review/comment?repo=${encodeURIComponent(repoPath)}&source=${encodeURIComponent(sourceBranch)}&target=${encodeURIComponent(targetBranch)}&source_commit=${encodeURIComponent(sourceCommit)}&target_commit=${encodeURIComponent(targetCommit)}&mode=branches`,
    {
      form: {
        file_path: file,
        start_line: '1',
        end_line: '1',
        side: 'right',
        body: commentBody,
      },
    },
  )
  expect(addCommentResponse.status()).toBeLessThan(400)

  // Submit the review via API
  const submitResponse = await page.request.post(
    `${BASE_URL}/api/review/submit?repo=${encodeURIComponent(repoPath)}&source=${encodeURIComponent(sourceBranch)}&target=${encodeURIComponent(targetBranch)}&source_commit=${encodeURIComponent(sourceCommit)}&target_commit=${encodeURIComponent(targetCommit)}&mode=branches`,
  )
  expect(submitResponse.status()).toBeLessThan(400)

  return { sourceCommit, targetCommit }
}

// ---------------------------------------------------------------------------
// Cleanup helper: remove storage for a specific test repo
// ---------------------------------------------------------------------------

function cleanupRepoStorage(repoPath: string | undefined): void {
  if (!repoPath) return
  const safePath = sanitizePath(repoPath)

  // Remove review data
  const reviewsDir = join(DIFFTY_HOME, 'reviews', safePath)
  if (existsSync(reviewsDir)) {
    rmSync(reviewsDir, { recursive: true, force: true })
  }

  // Remove review index data
  const indexDir = join(DIFFTY_HOME, 'review-index', safePath)
  if (existsSync(indexDir)) {
    rmSync(indexDir, { recursive: true, force: true })
  }

  // Remove review state data
  const stateDir = join(DIFFTY_HOME, 'review-state', safePath)
  if (existsSync(stateDir)) {
    rmSync(stateDir, { recursive: true, force: true })
  }
}

// ---------------------------------------------------------------------------
// Page Object Model
// ---------------------------------------------------------------------------

class DiffPage {
  constructor(private page: Page) {}

  async goto(
    repoPath: string,
    sourceBranch: string,
    targetBranch: string,
    sourceCommit: string,
    targetCommit: string,
    mode: string,
    file?: string,
  ): Promise<void> {
    let url = `/diff?repo=${encodeURIComponent(repoPath)}&source=${encodeURIComponent(sourceBranch)}&target=${encodeURIComponent(targetBranch)}&source_commit=${encodeURIComponent(sourceCommit)}&target_commit=${encodeURIComponent(targetCommit)}&mode=${encodeURIComponent(mode)}`
    if (file) {
      url += `&file=${encodeURIComponent(file)}`
    }
    await this.page.goto(url)
    // Handle auto-redirect to first file
    await this.page.waitForURL(/\/diff\?/)
  }

  getPanel() {
    return this.page.locator('[data-testid="past-reviews-panel"]')
  }

  getEntries() {
    return this.page.locator('[data-testid="past-review-entry"]')
  }

  getDeleteAllButton() {
    return this.page.locator('[data-testid="btn-delete-all-past"]')
  }

  getDeleteButtons() {
    return this.page.locator('[data-testid="btn-delete-past-review"]')
  }

  getViewOriginalLinks() {
    return this.page.locator('[data-testid="past-reviews-panel"] a:has-text("View original")')
  }

  getEphemeralLabels() {
    return this.page.locator('[data-testid="past-reviews-panel"] span:has-text("Ephemeral diff")')
  }

  async deletePastReview(index: number): Promise<void> {
    const btn = this.getDeleteButtons().nth(index)
    await btn.click()
    // The delete triggers window.location.reload() after fetch completes
    await this.page.waitForLoadState('load')
  }

  async deleteAllPastReviews(): Promise<void> {
    const btn = this.getDeleteAllButton()
    await btn.click()
    await this.page.waitForLoadState('load')
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe.serial('Past Reviews — branches mode', () => {
  let repoDir: string | undefined
  let firstReview: { sourceCommit: string; targetCommit: string }

  test.beforeAll(async () => {
    repoDir = createTestRepo('branches')

    // Create a feature branch with changes
    git(repoDir, 'checkout', '-b', 'feature')
    writeFileSync(join(repoDir, 'hello.txt'), 'Hello from feature branch\n')
    git(repoDir, 'add', '.')
    git(repoDir, 'commit', '-m', 'Add hello.txt')

    // Go back to main
    git(repoDir, 'checkout', 'main')
  })

  test.afterAll(() => {
    cleanupRepoStorage(repoDir)
    if (repoDir) rmSync(repoDir, { recursive: true, force: true })
  })

  test('register repo and create first review', async ({ page }) => {
    await test.step('Register repository', async () => {
      await registerRepo(page, repoDir)
    })

    await test.step('Create first branch review with a comment', async () => {
      firstReview = await createBranchReview(page, repoDir, 'feature', 'main', 'First review comment')
    })
  })

  test('create a new commit and second review, verify past reviews panel', async ({ page }) => {
    await test.step('Add a new commit on the feature branch', async () => {
      git(repoDir, 'checkout', 'feature')
      writeFileSync(join(repoDir, 'hello.txt'), 'Hello from feature branch v2\n')
      git(repoDir, 'add', '.')
      git(repoDir, 'commit', '-m', 'Update hello.txt')
      git(repoDir, 'checkout', 'main')
    })

    await test.step('Create second review', async () => {
      await createBranchReview(page, repoDir, 'feature', 'main', 'Second review comment')
    })

    await test.step('Navigate to current diff and verify past reviews panel', async () => {
      const sourceCommit = gitOutput(repoDir, 'rev-parse', 'feature')
      const targetCommit = gitOutput(repoDir, 'rev-parse', 'main')

      const diffPage = new DiffPage(page)
      await diffPage.goto(repoDir, 'feature', 'main', sourceCommit, targetCommit, 'branches')

      // The panel should be visible with the first review as a past review
      await expect(diffPage.getPanel()).toBeVisible()
      await expect(diffPage.getEntries()).toHaveCount(1)

      // "View original" link should be present (branches mode is linkable)
      await expect(diffPage.getViewOriginalLinks()).toHaveCount(1)
    })
  })

  test('"View original" navigates to the old commit pair', async ({ page }) => {
    const sourceCommit = gitOutput(repoDir, 'rev-parse', 'feature')
    const targetCommit = gitOutput(repoDir, 'rev-parse', 'main')

    const diffPage = new DiffPage(page)
    await diffPage.goto(repoDir, 'feature', 'main', sourceCommit, targetCommit, 'branches')

    await test.step('Click "View original" link', async () => {
      const link = diffPage.getViewOriginalLinks().first()
      await link.click()
      await page.waitForURL(/\/diff\?/)
    })

    await test.step('Verify we are viewing the old commit pair', async () => {
      const url = new URL(page.url(), BASE_URL)
      // The old source commit should be our first review's source commit
      expect(url.searchParams.get('source_commit')).toBe(firstReview.sourceCommit)
      expect(url.searchParams.get('target_commit')).toBe(firstReview.targetCommit)
    })
  })

  test('delete a single past review', async ({ page }) => {
    // Create a third commit + review to have 2 past reviews
    await test.step('Create third commit and review', async () => {
      git(repoDir, 'checkout', 'feature')
      writeFileSync(join(repoDir, 'hello.txt'), 'Hello from feature branch v3\n')
      git(repoDir, 'add', '.')
      git(repoDir, 'commit', '-m', 'Update hello.txt v3')
      git(repoDir, 'checkout', 'main')
      await createBranchReview(page, repoDir, 'feature', 'main', 'Third review comment')
    })

    const sourceCommit = gitOutput(repoDir, 'rev-parse', 'feature')
    const targetCommit = gitOutput(repoDir, 'rev-parse', 'main')
    const diffPage = new DiffPage(page)
    await diffPage.goto(repoDir, 'feature', 'main', sourceCommit, targetCommit, 'branches')

    await test.step('Verify 2 past reviews exist', async () => {
      await expect(diffPage.getEntries()).toHaveCount(2)
    })

    await test.step('Delete one past review', async () => {
      await diffPage.deletePastReview(0)
    })

    await test.step('Verify only 1 past review remains', async () => {
      await expect(diffPage.getEntries()).toHaveCount(1)
    })
  })

  test('delete all past reviews', async ({ page }) => {
    const sourceCommit = gitOutput(repoDir, 'rev-parse', 'feature')
    const targetCommit = gitOutput(repoDir, 'rev-parse', 'main')
    const diffPage = new DiffPage(page)
    await diffPage.goto(repoDir, 'feature', 'main', sourceCommit, targetCommit, 'branches')

    await test.step('Verify at least 1 past review exists', async () => {
      const count = await diffPage.getEntries().count()
      expect(count).toBeGreaterThanOrEqual(1)
    })

    await test.step('Delete all past reviews', async () => {
      await diffPage.deleteAllPastReviews()
    })

    await test.step('Verify past reviews panel is gone', async () => {
      await expect(diffPage.getPanel()).toBeHidden()
    })
  })
})

test.describe.serial('Past Reviews — staged mode', () => {
  let repoDir: string | undefined

  test.beforeAll(async () => {
    repoDir = createTestRepo('staged')
  })

  test.afterAll(() => {
    cleanupRepoStorage(repoDir)
    if (repoDir) rmSync(repoDir, { recursive: true, force: true })
  })

  test('staged mode shows "Ephemeral diff" instead of "View original"', async ({ page }) => {
    await test.step('Register repository', async () => {
      await registerRepo(page, repoDir)
    })

    await test.step('Stage changes and create first review', async () => {
      writeFileSync(join(repoDir, 'staged.txt'), 'Staged content v1\n')
      git(repoDir, 'add', '.')

      // Navigate to staged diff
      await page.goto(`/diff?repo=${encodeURIComponent(repoDir)}&mode=staged`)
      await page.waitForURL(/\/diff\?/)

      // Get the commits from the URL (they are set by the server)
      const url = new URL(page.url(), BASE_URL)
      const sourceCommit = url.searchParams.get('source_commit') ?? gitOutput(repoDir, 'rev-parse', 'HEAD')
      const targetCommit = url.searchParams.get('target_commit') ?? `staged-${sourceCommit}`
      const file = url.searchParams.get('file') ?? 'staged.txt'

      // Add comment via API
      await page.request.post(
        `${BASE_URL}/api/review/comment?repo=${encodeURIComponent(repoDir)}&source=HEAD&target=staged&source_commit=${encodeURIComponent(sourceCommit)}&target_commit=${encodeURIComponent(targetCommit)}&mode=staged`,
        { form: { file_path: file, start_line: '1', end_line: '1', side: 'right', body: 'Staged review comment v1' } },
      )

      // Submit review
      await page.request.post(
        `${BASE_URL}/api/review/submit?repo=${encodeURIComponent(repoDir)}&source=HEAD&target=staged&source_commit=${encodeURIComponent(sourceCommit)}&target_commit=${encodeURIComponent(targetCommit)}&mode=staged`,
      )
    })

    await test.step('Commit the staged changes and stage new ones', async () => {
      git(repoDir, 'commit', '-m', 'Commit staged v1')
      writeFileSync(join(repoDir, 'staged.txt'), 'Staged content v2\n')
      git(repoDir, 'add', '.')
    })

    await test.step('Navigate to staged diff and verify Ephemeral diff label', async () => {
      await page.goto(`/diff?repo=${encodeURIComponent(repoDir)}&mode=staged`)
      await page.waitForURL(/\/diff\?/)

      const diffPage = new DiffPage(page)

      // The past review from v1 should show as "Ephemeral diff"
      await expect(diffPage.getPanel()).toBeVisible()
      await expect(diffPage.getEntries()).toHaveCount(1)
      await expect(diffPage.getEphemeralLabels()).toHaveCount(1)
      await expect(diffPage.getViewOriginalLinks()).toHaveCount(0)
    })
  })
})

test.describe.serial('Past Reviews — unstaged mode', () => {
  let repoDir: string | undefined

  test.beforeAll(async () => {
    repoDir = createTestRepo('unstaged')
  })

  test.afterAll(() => {
    cleanupRepoStorage(repoDir)
    if (repoDir) rmSync(repoDir, { recursive: true, force: true })
  })

  test('unstaged mode shows "Ephemeral diff" label', async ({ page }) => {
    await test.step('Register repository', async () => {
      await registerRepo(page, repoDir)
    })

    await test.step('Create unstaged changes and first review', async () => {
      writeFileSync(join(repoDir, 'unstaged.txt'), 'Unstaged content v1\n')
      // Don't stage — this is for unstaged mode

      // Navigate to unstaged diff
      await page.goto(`/diff?repo=${encodeURIComponent(repoDir)}&mode=unstaged`)
      await page.waitForURL(/\/diff\?/)

      const url = new URL(page.url(), BASE_URL)
      const sourceCommit = url.searchParams.get('source_commit') ?? gitOutput(repoDir, 'rev-parse', 'HEAD')
      const targetCommit = url.searchParams.get('target_commit') ?? `unstaged-${sourceCommit}`
      const file = url.searchParams.get('file') ?? 'unstaged.txt'

      // Add comment via API
      await page.request.post(
        `${BASE_URL}/api/review/comment?repo=${encodeURIComponent(repoDir)}&source=HEAD&target=unstaged&source_commit=${encodeURIComponent(sourceCommit)}&target_commit=${encodeURIComponent(targetCommit)}&mode=unstaged`,
        { form: { file_path: file, start_line: '1', end_line: '1', side: 'right', body: 'Unstaged review comment v1' } },
      )

      // Submit review
      await page.request.post(
        `${BASE_URL}/api/review/submit?repo=${encodeURIComponent(repoDir)}&source=HEAD&target=unstaged&source_commit=${encodeURIComponent(sourceCommit)}&target_commit=${encodeURIComponent(targetCommit)}&mode=unstaged`,
      )
    })

    await test.step('Stage and commit, then create new unstaged changes', async () => {
      git(repoDir, 'add', '.')
      git(repoDir, 'commit', '-m', 'Commit unstaged v1')
      writeFileSync(join(repoDir, 'unstaged.txt'), 'Unstaged content v2\n')
    })

    await test.step('Navigate to unstaged diff and verify Ephemeral diff', async () => {
      await page.goto(`/diff?repo=${encodeURIComponent(repoDir)}&mode=unstaged`)
      await page.waitForURL(/\/diff\?/)

      const diffPage = new DiffPage(page)

      await expect(diffPage.getPanel()).toBeVisible()
      await expect(diffPage.getEntries()).toHaveCount(1)
      await expect(diffPage.getEphemeralLabels()).toHaveCount(1)
      await expect(diffPage.getViewOriginalLinks()).toHaveCount(0)
    })
  })
})

test.describe.serial('Past Reviews — commits mode (negative test)', () => {
  let repoDir: string | undefined

  test.beforeAll(async () => {
    repoDir = createTestRepo('commits')

    // Create two commits to compare
    writeFileSync(join(repoDir, 'file.txt'), 'Content v1\n')
    git(repoDir, 'add', '.')
    git(repoDir, 'commit', '-m', 'Add file v1')

    writeFileSync(join(repoDir, 'file.txt'), 'Content v2\n')
    git(repoDir, 'add', '.')
    git(repoDir, 'commit', '-m', 'Update file v2')
  })

  test.afterAll(() => {
    cleanupRepoStorage(repoDir)
    if (repoDir) rmSync(repoDir, { recursive: true, force: true })
  })

  test('commits mode does not produce past reviews (unique key per commit pair)', async ({ page }) => {
    await test.step('Register repository', async () => {
      await registerRepo(page, repoDir)
    })

    // Get the two latest commit hashes
    const commitV2 = gitOutput(repoDir, 'rev-parse', 'HEAD')
    const commitV1 = gitOutput(repoDir, 'rev-parse', 'HEAD~1')

    await test.step('Create a review comparing commit v1 to v2', async () => {
      // In commits mode, source_branch is set to the full commit hash
      // So each different commit pair gets a unique review index key

      // Add comment via API (commits mode uses commit hash as branch)
      await page.request.post(
        `${BASE_URL}/api/review/comment?repo=${encodeURIComponent(repoDir)}&source=${encodeURIComponent(commitV1)}&target=${encodeURIComponent(commitV2)}&source_commit=${encodeURIComponent(commitV1)}&target_commit=${encodeURIComponent(commitV2)}&mode=commits`,
        { form: { file_path: 'file.txt', start_line: '1', end_line: '1', side: 'right', body: 'Commits mode comment' } },
      )

      // Submit review
      await page.request.post(
        `${BASE_URL}/api/review/submit?repo=${encodeURIComponent(repoDir)}&source=${encodeURIComponent(commitV1)}&target=${encodeURIComponent(commitV2)}&source_commit=${encodeURIComponent(commitV1)}&target_commit=${encodeURIComponent(commitV2)}&mode=commits`,
      )
    })

    await test.step('Navigate to same commit pair diff — no past reviews expected', async () => {
      const diffPage = new DiffPage(page)
      await diffPage.goto(repoDir, commitV1, commitV2, commitV1, commitV2, 'commits')

      // Past reviews panel should NOT be visible — the current review is
      // the only entry in the index, and it's filtered out as "current"
      await expect(diffPage.getPanel()).toBeHidden()
    })
  })
})
