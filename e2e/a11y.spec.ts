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
import { randomUUID } from 'node:crypto';

import { describe, focusableControls, reached, tabThrough } from './keyboard';
import { credentialFor, pageRoutes } from './routes';
import { fixtures } from './fixtures';
import { expect, test } from './auth';

import type { Page } from '@playwright/test';

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

// --- T163: the switcher, the create drawers and the delete confirmation ---
//
// A fresh account rather than one of the three shared ones (patient-switch.spec.ts's
// own reason): these cases open the delete confirmation, and a shared account's
// patients are shared with every other case in the run.
async function newAccount(page: Page): Promise<void> {
  const email = `a11y-${randomUUID().slice(0, 8)}@example.test`;
  const registered = await page.request.post(fixtures.registerPath, {
    data: { email, name: 'A11y Rehearsal', password: fixtures.password },
  });
  expect(registered.status(), await registered.text()).toBe(201);
}

async function newPatient(page: Page): Promise<{ id: string }> {
  const response = await page.request.post('/api/v1/patients', {
    data: {
      first_name: 'Keyboard',
      last_name: 'Rehearsal',
      birth_date: '1980-01-01',
      relationship_to_owner: 'other',
    },
  });
  expect(response.ok(), `creating the patient failed: ${await response.text()}`).toBe(true);
  return (await response.json()) as { id: string };
}

test.describe('T163 — the switcher, the create drawers and the delete confirmation', () => {
  test.use({ signedInAs: 'anonymous' });

  test('the active-patient switcher is reachable by keyboard and shows a focus indicator', async ({ page }) => {
    await newAccount(page);
    await newPatient(page);
    await page.goto('/patients');

    const switcher = page.getByRole('combobox', { name: 'Active patient' });
    await expect(switcher).toBeVisible();

    const expected = await focusableControls(page, 'body');
    const walk = await tabThrough(page, expected.length + 8);

    const match = walk.find((control) => control.tag === 'select' && control.label === 'Active patient');
    expect(match, 'the switcher was never reached by Tab').toBeDefined();
    expect(match?.indicator, 'the switcher shows no focus indicator').toBe(true);
  });

  test('reaches the patient delete confirmation by keyboard, with a visible focus indicator', async ({ page }) => {
    await newAccount(page);
    const patient = await newPatient(page);
    await page.goto(`/patients/${patient.id}`);

    await page.getByRole('button', { name: 'Delete' }).click();

    const confirm = page.getByRole('region', { name: 'Confirm delete' });
    await expect(confirm).toBeVisible();
    await expect(confirm.getByRole('button')).toHaveCount(2);

    await page.keyboard.press('Tab');
    const focused = confirm.getByRole('button', { name: 'Delete permanently' });
    await expect(focused).toBeFocused();

    const style = await focused.evaluate((element) => {
      const computed = getComputedStyle(element);
      return (computed.outlineStyle !== 'none' && computed.outlineWidth !== '0px') || computed.boxShadow !== 'none';
    });
    expect(style, 'Delete permanently shows no focus indicator').toBe(true);
  });

  // T159a-T162: the three "Add a ..." forms (internal/web/views/patients/form.templ,
  // internal/web/views/directory/practitioner_form.templ,
  // internal/web/views/directory/facility_form.templ) are now mounted on
  // their list pages, at the fragment id CreateHref already pointed at
  // (ids.PatientForm / ids.DirectoryForm). Each case opens the list page,
  // follows the "Add a ..." link, and asserts every field of the form is
  // reachable by Tab, in document order, with a visible focus indicator.
  const createForms: Array<{ label: string; path: string; formSelector: string }> = [
    { label: 'Add a person', path: '/patients', formSelector: '#patient-form' },
    { label: 'Add a practitioner', path: '/practitioners', formSelector: '#practitioners-form' },
    { label: 'Add a facility', path: '/facilities', formSelector: '#facilities-form' },
  ];

  for (const form of createForms) {
    test(`reaches every field of the ${form.label} form by keyboard, with a visible focus indicator`, async ({
      page,
    }) => {
      await newAccount(page);
      await page.goto(form.path);
      // A brand-new account's practitioner and facility lists are empty, so
      // the empty state's own action repeats this same label — .first()
      // picks the header's link, which is the one every account sees.
      await page.getByRole('link', { name: form.label, exact: true }).first().click();

      const expectedInForm = await focusableControls(page, form.formSelector);
      expect(expectedInForm.length, `${form.label}: the form offers no focusable controls at all`).toBeGreaterThan(0);

      // Steps cover the whole document, not only the form: it renders after
      // the list on the page (T159a-T162), so Tab reaches its own controls
      // only after the skip link, the nav, the list and its pagination.
      const expectedOnPage = await focusableControls(page, 'body');
      const walk = await tabThrough(page, expectedOnPage.length + 8);
      const remaining = reached(walk, expectedInForm);

      expect(remaining, `${form.label}: never reached by Tab, in order: ${remaining.join(', ')}`).toEqual([]);

      for (const control of walk.filter((candidate) => expectedInForm.includes(describe(candidate)))) {
        expect(control.indicator, `${form.label}: no focus indicator on ${describe(control)}`).toBe(true);
      }
    });
  }
});
