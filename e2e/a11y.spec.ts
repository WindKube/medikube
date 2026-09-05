// T256. Every interactive element of every page (e2e/routes.ts's own
// inventory) is reachable by Tab, in document order, from the skip link
// onward — and wherever Tab lands, there is something visible to see it by
// (FR-048, SC-014).
//
// The scope is the whole document, not just `main`: the shell's skip link and
// its primary navigation are controls too, and they are on every page rather
// than on the handful smoke.spec.ts's SC-014 section already drives through a
// full user journey. This file only ever asks "can a keyboard reach it, and
// can a person see where it is" — nothing about what pressing it does.
import { describe, focusableControls, reached, tabThrough } from './keyboard';
import { credentialFor, pageRoutes } from './routes';
import { expect, test } from './auth';

for (const route of pageRoutes) {
  test.describe(`${route.opID} — keyboard reachability`, () => {
    test.use({ signedInAs: credentialFor(route) });

    test('every interactive element is reachable by Tab and shows a focus indicator', async ({ page }) => {
      await page.goto(route.smokeURL);

      const expected = await focusableControls(page, 'body');
      expect(expected.length, `${route.opID} offers no focusable controls at all`).toBeGreaterThan(0);

      // A few steps of headroom over the control count: Tab can land on the
      // same identified element more than once (a native date input's shadow
      // parts, see keyboard.ts), and the walk still has to reach the last one.
      const walk = await tabThrough(page, expected.length + 8);

      expect(walk[0]?.label, `${route.opID}: the first Tab does not reach the skip link`).toBe('Skip to content');

      const remaining = reached(walk, expected);
      expect(remaining, `${route.opID}: never reached by Tab, in order: ${remaining.join(', ')}`).toEqual([]);

      for (const control of walk) {
        expect(control.indicator, `${route.opID}: no focus indicator on ${describe(control)}`).toBe(true);
      }
    });
  });
}
