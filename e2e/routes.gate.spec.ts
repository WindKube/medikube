// T260, T295. The generic half of the browser gate: contracts/pages.md's
// seven assertions (status, the four shell landmarks, the page's own landmark
// non-empty, title, zero console/CSP/network problems), run once per page in
// the application's OWN route inventory (e2e/routes.ts) and once for each of
// the two reachable error views, at both viewports (playwright.config.ts's
// two projects).
//
// The list is never hand-kept: a page routes.ts does not enumerate is a page
// e2e/httproute's own boot-time panic already refused to serve (FR-067), and
// a page whose credential or title this file cannot work out fails loudly,
// naming the op id, rather than being quietly skipped (T295 is what a page
// with no case at all looks like: it never reaches this file, and
// gate_test.go on the Go side is what keeps `routes --json` from silently
// stopping to list it).
//
// smoke.spec.ts is unchanged and keeps the page-specific assertions — what
// the record list contains, what the settings danger zone states, and so on.
// This file only ever runs the seven generic ones.
import { fixtures } from './fixtures';
import { open } from './gate';
import { credentialFor, pageRoutes, type PageRoute } from './routes';
import { expect, test } from './auth';

import type { Page } from '@playwright/test';

async function apiNameOf(page: Page, id: string): Promise<string> {
  const response = await page.request.get(`/api/v1/records/medications/${id}`);
  expect(response.ok(), `routes.gate: the API did not answer for record ${id}`).toBe(true);

  return ((await response.json()) as { name: string }).name;
}

// idOf reads the record id P5's SmokeURL is bound to, off the end of the URL
// itself, so nothing here needs a second copy of the kind's URL segment.
function idOf(smokeURL: string): string {
  return smokeURL.slice(smokeURL.lastIndexOf('/') + 1);
}

// titleFor is contracts/pages.md's title column, worked out generically:
// internal/web/page/errors.go titles the error views after their own
// landmark's name plus the product suffix, and seven of the nine pages do the
// same. The two that do not are named here, each with the reason — not
// guessed at, and not silently skipped:
//
//   - P5 (medicationDetailPage) titles itself after the RECORD, never the
//     landmark's fixed name ("Medication"), so its title is read off the same
//     API the page itself renders from.
//   - P9 (verifyEmailPage) is titled "Confirm your address" while its
//     landmark is named "Email confirmation" — contracts/pages.md's own row
//     gives the pair different words.
//
// Any OTHER page that does not follow the generic rule and has no entry here
// fails assertion 4 with its own op id in the mismatch, which is this file
// doing exactly the job contracts/pages.md's gate exists for.
async function titleFor(route: PageRoute, page: Page): Promise<string> {
  switch (route.opID) {
    case 'verifyEmailPage':
      return fixtures.title(fixtures.titles.verifyEmail);
    case 'medicationDetailPage':
      return fixtures.title(await apiNameOf(page, idOf(route.smokeURL)));
    default:
      return fixtures.title(route.landmark.name);
  }
}

for (const route of pageRoutes) {
  test.describe(`${route.opID} — the render gate`, () => {
    test.use({ signedInAs: credentialFor(route) });

    test('passes every one of contracts/pages.md\'s seven assertions', async ({ page }) => {
      await open(page, {
        path: route.smokeURL,
        title: await titleFor(route, page),
        landmark: route.landmark,
        status: 200,
      });
    });
  });
}

// --- The error views -------------------------------------------------------
//
// E1 (404) and E2 (403) are each reachable by a URL and run through the same
// seven assertions as any page. E3 (500) is not: contracts/pages.md records
// why — no route in a shipped build fails on purpose, so producing one for
// this gate would be a worse defect than an unsmoked error page. It is
// covered instead, at the templ layer, by internal/web/page/errors_test.go
// (T230). Its title, "Something went wrong — MediKube", follows the same
// generic rule as the two below and is asserted there.

test.describe('E1 — not found', () => {
  test.use({ signedInAs: 'anonymous' });

  test('a path nothing serves answers 404 inside the full shell (FR-033)', async ({ page }) => {
    // A sentinel distinct from every path the inventory actually serves,
    // checked here rather than assumed, so a future page cannot collide with
    // it in silence.
    const path = '/routes-gate-404-sentinel-for-smoke';
    expect(
      pageRoutes.some((route) => route.path === path || route.smokeURL === path),
      'the 404 probe path collides with a real page; pick another sentinel',
    ).toBe(false);

    await open(page, {
      path,
      title: fixtures.title('Not found'),
      landmark: { role: 'region', name: 'Not found' },
      status: 404,
    });
  });
});

test.describe('E2 — sign in required', () => {
  test.use({ signedInAs: 'anonymous' });

  test('a session page opened with no session renders the sign-in prompt, not a 404 (E2)', async ({ page }) => {
    // Any session-required page probes the same refusal: it is enforced once,
    // at the router (internal/httproute/registry.go's Bind), not per page.
    const sessionPage = pageRoutes.find((route) => route.auth === 'user');
    expect(sessionPage, 'routes.ts stopped listing any session-required page to probe E2 with').toBeTruthy();

    await open(page, {
      path: sessionPage!.smokeURL,
      title: fixtures.title('Sign in required'),
      landmark: { role: 'region', name: 'Sign in required' },
      status: 403,
    });
  });
});
