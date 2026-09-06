// T019, US1-1/US1-7/SC-002: the settings page's own language control actually
// changes what a person sees, and the choice sticks — through the same load,
// a reload, and a fresh sign-in.
//
// A fresh account rather than one of the three shared ones (a11y.spec.ts's own
// reason): this spec leaves the account in Polish partway through, and the
// shared accounts are relied on to stay English by every other spec in the
// gate. randomUUID keeps it unique across the two viewport projects, which
// run against the same instance at once.
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { fixtures } from './fixtures';
import { open } from './gate';
import { expect, test } from './auth';

import type { Page } from '@playwright/test';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

function polish(id: string): string {
  const data = readFileSync(resolve(repositoryRoot, 'internal/i18n/locales/active.pl.toml'), 'utf8');
  const escaped = id.replace(/\./g, '\\.');
  const found = new RegExp(`\\[${escaped}\\][^[]*?\\nother = "([^"]*)"`).exec(data);
  if (!found) {
    throw new Error(`e2e: active.pl.toml declares no ${id}`);
  }
  return found[1];
}

const profilePl = polish('settings.profile_title');
const settingsPl = polish('nav.settings');
const saveChangesPl = polish('action.save_changes');

async function newAccount(page: Page): Promise<{ email: string; password: string }> {
  const email = `settings-locale-${randomUUID().slice(0, 8)}@example.test`;

  const registered = await page.request.post(fixtures.registerPath, {
    data: { email, name: 'Locale Rehearsal', password: fixtures.password },
  });
  expect(registered.status(), await registered.text()).toBe(201);

  return { email, password: fixtures.password };
}

test.describe('the settings page language control', () => {
  test.use({ signedInAs: 'anonymous' });

  test('choosing Polski changes the page in place, and the choice survives a reload and a fresh sign-in', async ({
    page,
  }) => {
    const account = await newAccount(page);

    const settings = await open(page, {
      path: fixtures.pages.settings.path,
      title: fixtures.title(fixtures.titles.settings),
      landmark: fixtures.pages.settings.landmark,
    });

    const profile = settings.getByRole('form', { name: fixtures.settingsLandmarks.profile });
    const language = profile.getByRole('combobox', { name: 'Language' });
    await expect(language, 'the settings page offers no language control').toBeVisible();

    await language.selectOption({ label: 'Polski' });
    await profile.getByRole('button', { name: 'Save changes' }).click();

    // Same response, no navigation: the re-rendered form already carries the
    // Polish label and aria-label (settings.language.label), which is what
    // proves this came back on the PATCH itself rather than a later reload.
    // The form's own landmark is Polish too once it comes back, so it is
    // found by its Polish name from here on.
    const profilePolish = settings.getByRole('form', { name: profilePl });
    const languagePl = profilePolish.getByRole('combobox', { name: 'Język' });
    await expect(languagePl, 'the re-rendered form is still in English').toBeVisible();
    await expect(languagePl).toHaveValue('pl');
    await expect(profilePolish).toContainText('Język');

    // Reload: the choice is a stored account preference, not a page state.
    await page.reload();
    const afterReload = page
      .getByRole('region', { name: settingsPl })
      .getByRole('form', { name: profilePl });
    await expect(afterReload.getByRole('combobox', { name: 'Język' })).toHaveValue('pl');

    // Sign out and back in: still Polish, because it lives on the account and
    // not on the browser (FR-045's own reasoning, applied to the language).
    const loggedOut = await page.request.post(fixtures.logoutPath);
    expect(loggedOut.status()).toBe(204);

    const signedIn = await page.request.post(fixtures.signInPath, {
      data: { email: account.email, password: account.password },
    });
    expect(signedIn.status(), await signedIn.text()).toBe(200);

    await page.goto(fixtures.pages.settings.path);
    const afterSignIn = page.getByRole('form', { name: profilePl });
    const languageAfterSignIn = afterSignIn.getByRole('combobox', { name: 'Język' });
    await expect(languageAfterSignIn, 'the language reverted to English after signing back in').toBeVisible();
    await expect(languageAfterSignIn).toHaveValue('pl');

    // Switch back: English is a language choice like any other, and this is
    // what proves the control keeps working once it is not showing the
    // default any more.
    await languageAfterSignIn.selectOption({ label: 'English' });
    await afterSignIn.getByRole('button', { name: saveChangesPl }).click();

    const afterSwitchBack = page.getByRole('form', { name: fixtures.settingsLandmarks.profile });
    const languageEn = afterSwitchBack.getByRole('combobox', { name: 'Language' });
    await expect(languageEn, 'the re-rendered form did not switch back to English').toBeVisible();
    await expect(languageEn).toHaveValue('en');
  });
});
