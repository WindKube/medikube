// The browser gate's configuration: two viewports, one instance, no retries.
//
// T267 is where this file's task number lives; it arrives here because T183
// needs it to run at all. When T267 lands it adds the remaining projects and
// `e2e/routes.ts`'s generated page list, and the two blocks below stay as they
// are.
import { defineConfig, devices } from '@playwright/test';

const address = process.env.MEDIKUBE_E2E_ADDR ?? '127.0.0.1:8091';

export const baseURL = process.env.MEDIKUBE_E2E_BASE_URL ?? `http://${address}`;

// The mail sink's loopback endpoint (T223p). The default is mailsink.mjs's, in
// the shape the instance's own address is written above — two files declaring
// one default is what this file and instance.mjs already do for MEDIKUBE_E2E_ADDR.
export const mailSinkURL = `http://${process.env.MEDIKUBE_E2E_MAIL_ADDR ?? '127.0.0.1:8026'}`;

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
    // T223. One sign-in per account for the whole run, through MediKube's own
    // route, stored and reused. It is a project rather than a fixture because
    // both viewports depend on it: signing in per test would be forty sign-ins
    // against a route limited to ten guest attempts a minute (FR-006).
    {
      name: 'setup',
      testMatch: /.*\.setup\.ts/,
    },
    // The specs, and only the specs: without this the setup file is also an
    // ordinary test in both viewport projects, re-signing in halfway through
    // the run and rewriting the state the other cases are using.
    {
      name: 'desktop-1440x900',
      testMatch: /.*\.spec\.ts/,
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
      dependencies: ['setup'],
    },
    {
      name: 'mobile-390x844',
      testMatch: /.*\.spec\.ts/,
      use: { ...devices['Desktop Chrome'], viewport: { width: 390, height: 844 } },
      dependencies: ['setup'],
    },
  ],

  webServer: {
    command: 'node ./instance.mjs',
    // The mail sink's endpoint, which instance.mjs opens LAST — after the
    // router answers, after the migrations, and after the instance's SMTP
    // settings have been pointed at the sink. Readiness therefore means the
    // whole environment is assembled, not merely that the router is up: a
    // recovery case that started a moment early would find mail unconfigured
    // and read the refusal as a broken page. Asking a page instead would make
    // the wait depend on the thing being tested.
    url: `${mailSinkURL}/messages`,
    timeout: 60_000,
    reuseExistingServer: !process.env.CI,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
