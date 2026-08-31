// contracts/pages.md's seven smoke assertions, in one place.
//
// Every page case in every phase runs all seven. They live here rather than in
// each spec so that adding a page is one call and so that assertion 3's
// non-empty clause — the load-bearing one, because a landmark containing
// nothing passes a naive presence check while the page is broken — cannot be
// left out of a new case by accident.
import { expect, type Locator, type Page, type Response } from '@playwright/test';

// The four shell landmarks of contracts/pages.md, present on every page,
// signed in or out.
const shellLandmarks = [
  { role: 'banner' as const },
  { role: 'navigation' as const, name: 'Primary' },
  { role: 'main' as const },
  { role: 'contentinfo' as const },
];

export type Landmark = {
  role: 'region' | 'article' | 'form' | 'main';
  name: string;
};

export type PageCase = {
  path: string;
  title: string;
  landmark: Landmark;
  status?: number;
};

// Problems collects the three "zero of these" assertions. Every listener is
// attached before the first navigation, because a console error emitted during
// load is the one a listener attached afterwards never sees.
type Problems = {
  console: string[];
  crashes: string[];
  csp: string[];
  network: string[];
};

// The browser asks for this on its own on every top-level navigation, and no
// page references it. Counting the answer as a failed request would make every
// case red for something the application never asked for.
const browserInitiated = /\/favicon\.ico$/;

export async function watch(page: Page): Promise<Problems> {
  const problems: Problems = { console: [], crashes: [], csp: [], network: [] };

  page.on('console', (message) => {
    const kind = message.type();
    if (kind === 'error' || kind === 'warning') {
      problems.console.push(`${kind}: ${message.text()}`);
    }
  });

  page.on('pageerror', (error) => problems.crashes.push(String(error)));

  page.on('requestfailed', (request) => {
    const failure = request.failure();
    problems.network.push(`${request.method()} ${request.url()} — ${failure?.errorText ?? 'failed'}`);
  });

  page.on('response', (response) => {
    if (response.status() < 400) return;
    if (browserInitiated.test(response.url())) return;
    // A subresource the page asked for and did not get. The document's own
    // status is asserted separately, so a deliberate 4xx page is not caught
    // here twice.
    if (response.request().resourceType() === 'document') return;
    problems.network.push(`${response.status()} ${response.url()}`);
  });

  // CSP violations do not surface as console messages Playwright can classify,
  // so the page reports them itself. Registered as an init script so it is in
  // place before the document's own markup is parsed.
  await page.exposeFunction('__medikubeCspViolation', (detail: string) => {
    problems.csp.push(detail);
  });

  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      const report = window as unknown as { __medikubeCspViolation?: (detail: string) => void };
      report.__medikubeCspViolation?.(
        `${event.violatedDirective} blocked ${event.blockedURI || '(inline)'}`,
      );
    });
  });

  return problems;
}

// open navigates and runs all seven assertions. It returns the page's own
// landmark so a case can go on to assert what is inside it.
export async function open(page: Page, expected: PageCase): Promise<Locator> {
  const problems = await watch(page);

  const response = await page.goto(expected.path);

  // 1 — the status is the expected one.
  expect(response, `no response for ${expected.path}`).not.toBeNull();
  expect((response as Response).status(), `status of ${expected.path}`).toBe(expected.status ?? 200);

  // 2 — the four shell landmarks are present, signed in or out.
  for (const landmark of shellLandmarks) {
    await expect(
      page.getByRole(landmark.role, landmark.name ? { name: landmark.name } : undefined),
      `${expected.path} is missing the ${landmark.role} landmark`,
    ).toBeVisible();
  }

  // 3 — the page's own landmark is present AND non-empty.
  const own = page.getByRole(expected.landmark.role, { name: expected.landmark.name });
  await expect(own, `${expected.path} is missing ${expected.landmark.role}[name="${expected.landmark.name}"]`).toBeVisible();
  expect(
    (await own.innerText()).trim(),
    `${expected.landmark.role}[name="${expected.landmark.name}"] rendered empty, which passes a presence check and fails the person`,
  ).not.toBe('');

  // 4 — the title matches.
  await expect(page).toHaveTitle(expected.title);

  // 5, 6, 7 — zero console errors or warnings, zero CSP violations, zero
  // failed network requests.
  expect(problems.console, `console output on ${expected.path}`).toEqual([]);
  expect(problems.crashes, `uncaught page failures on ${expected.path}`).toEqual([]);
  expect(problems.csp, `CSP violations on ${expected.path}`).toEqual([]);
  expect(problems.network, `failed requests on ${expected.path}`).toEqual([]);

  return own;
}
