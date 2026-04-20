import { defineConfig, devices } from '@playwright/test'

/** Port used by the diffty server during e2e tests. */
export const PORT = 10103

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: 'list',
  timeout: 30_000,
  use: {
    baseURL: `http://localhost:${PORT}`,
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: `go run ../cmd/diffty -port ${PORT}`,
    port: PORT,
    reuseExistingServer: !process.env.CI,
    timeout: 15_000,
  },
})
