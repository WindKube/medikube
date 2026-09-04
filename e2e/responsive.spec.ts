// T257. At 390px, no page (e2e/routes.ts's own inventory) scrolls
// horizontally, and every primary-navigation link stays reachable (FR-044).
//
// Runs in every project Playwright collects, and skips itself outside the
// mobile one: the assertion is specifically about the narrow viewport, and a
// desktop run of it would either pass vacuously or duplicate routes.gate's
// coverage for no reason.
import { credentialFor, pageRoutes } from './routes';
import { expect, test } from './auth';

const mobileProject = 'mobile-390x844';

for (const route of pageRoutes) {
  test.describe(`${route.opID} — 390px`, () => {
    test.use({ signedInAs: credentialFor(route) });

    test('does not scroll horizontally and keeps every nav link visible', async ({ page }, testInfo) => {
      test.skip(testInfo.project.name !== mobileProject, 'this assertion is only meaningful at the mobile viewport');

      await page.goto(route.smokeURL);

      const overflow = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(
        overflow.scrollWidth,
        `${route.opID} scrolls horizontally at 390px (${overflow.scrollWidth}px content in a ${overflow.clientWidth}px viewport)`,
      ).toBeLessThanOrEqual(overflow.clientWidth);

      const nav = page.getByRole('navigation', { name: 'Primary' });
      const links = await nav.getByRole('link').all();
      expect(links.length, `${route.opID}: the primary navigation offers no links at all`).toBeGreaterThan(0);

      for (const link of links) {
        await expect(link, `${route.opID}: a primary nav link is not visible at 390px`).toBeVisible();
      }
    });
  });
}
