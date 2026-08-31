// The browser gate's configuration: two viewports, one instance, no retries.
//
// T267 is where this file's task number lives; it arrives here because T183
// needs it to run at all. When T267 lands it adds the remaining projects and
// `e2e/routes.ts`'s generated page list, and the two blocks below stay as they
// are.
import { defineConfig, devices } from '@playwright/test';

const address = process.env.MEDIKUBE_E2E_ADDR ?? '127.0.0.1:8091';

export const baseURL = process.env.MEDIKUBE_E2E_BASE_URL ?? `http://${address}`;

export default defineConfig({
  testDir: '.',
  fullyParallel: true,

  // Constitution VIII: "a flaky assertion is fixed or removed, never retried
  // into passing". A retry budget is how a gate stops being evidence.
  retries: 0,

  // One worker in CI. The whole run shares one instance holding one seeded
  // fixture, so parallel workers would be several browsers reading a database
  // any of them may be writing to — and the failure that produces looks like a
  // rendering bug rather than like a race.
  workers: process.env.CI ? 1 : undefined,

  forbidOnly: !!process.env.CI,
  timeout: 30_000,
  expect: { timeout: 10_000 },

  reporter: process.env.CI
    ? [['github'], ['html', { open: 'never' }], ['list']]
    : [['list']],

  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },

  // Both viewports of contracts/pages.md and SC-003. Every spec runs twice,
  // once under each, because "it renders" and "it renders on a phone" are two
  // different claims and only one of them is usually checked.
  projects: [
    {
      name: 'desktop-1440x900',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
    {
      name: 'mobile-390x844',
      use: { ...devices['Desktop Chrome'], viewport: { width: 390, height: 844 } },
    },
  ],

  webServer: {
    command: 'node ./instance.mjs',
    // A route that answers 200 without a session and without touching a
    // record: readiness here means "the router is up and the migrations ran",
    // and asking a page would make the wait depend on the thing being tested.
    url: `${baseURL}/api/collections/users/auth-methods`,
    timeout: 60_000,
    reuseExistingServer: !process.env.CI,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
