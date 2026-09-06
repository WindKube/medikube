// T032 (US1-1/US1-2/US1-3, SC-001): a Polish account's own browser tour —
// settings, the patient list, a record list, a record detail, tags, search
// and the timeline — each renders `<html lang="pl">`, a Polish `<title>`, no
// English nav label and no console error. routes.gate.spec.ts already runs
// contracts/pages.md's seven generic assertions against every page in
// English; this file is what a browser alone proves once the account's own
// language is Polish, so it does not reuse gate.ts's open() — that helper's
// shell-landmark names (`Primary`, …) are asserted in English on purpose,
// which is exactly the assumption a Polish account breaks.
//
// A fresh account rather than one of the three shared ones, for
// settings-locale.spec.ts's own reason: this run leaves the account in
// Polish, and Account A is relied on to stay English by every other spec in
// the gate (including locale.spec.ts's own English-account row below).
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { fixtures } from './fixtures';
import { watch } from './gate';
import { expect, test } from './auth';

import type { Page } from '@playwright/test';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

function goString(relative: string, name: string): string {
  const source = readFileSync(resolve(repositoryRoot, relative), 'utf8');
  const found = new RegExp(`\\b${name}\\s*=\\s*"([^"]*)"`).exec(source);
  if (!found) {
    throw new Error(`e2e: ${relative} no longer declares ${name}`);
  }
  return found[1];
}

// tomlOther reads one message id's "other" text straight out of the
// catalogue TOML, the same way internal/web/page/locale_render_test.go (T030)
// does server-side: there is no other way to list a translated string
// without importing internal/i18n, which this file cannot do.
function tomlOther(locale: 'en' | 'pl', id: string): string {
  const path = resolve(repositoryRoot, `internal/i18n/locales/active.${locale}.toml`);
  const data = readFileSync(path, 'utf8');
  const escaped = id.replace(/\./g, '\\.');
  const found = new RegExp(`\\[${escaped}\\][^[]*?\\nother = "([^"]*)"`).exec(data);
  if (!found) {
    throw new Error(`e2e: active.${locale}.toml declares no ${id}`);
  }
  return found[1];
}

function polishTitle(id: string): string {
  return tomlOther('pl', id) + goString('internal/web/views/shell/props.go', 'SuffixSeparator') + goString(
    'internal/web/views/shell/props.go',
    'ProductName',
  );
}

async function newPolishAccount(page: Page): Promise<{ patientID: string }> {
  const email = `locale-tour-${randomUUID().slice(0, 8)}@example.test`;

  const registered = await page.request.post(fixtures.registerPath, {
    data: { email, name: 'Locale Tour', password: fixtures.password },
  });
  expect(registered.status(), await registered.text()).toBe(201);

  const patched = await page.request.patch('/api/v1/me', { data: { locale: 'pl' } });
  expect(patched.status(), await patched.text()).toBe(200);

  const me = await page.request.get('/api/v1/me');
  expect(me.status(), await me.text()).toBe(200);
  const body = (await me.json()) as { active_patient: { id: string } | null };
  expect(body.active_patient, 'a freshly registered account has no self patient').not.toBeNull();

  return { patientID: (body.active_patient as { id: string }).id };
}

// englishNavLabels are every nav.* value active.en.toml carries, at the time
// of writing (the same set locale_render_test.go's isNavID collects
// server-side). A Polish page must contain none of them.
function englishNavLabels(): string[] {
  const path = resolve(repositoryRoot, 'internal/i18n/locales/active.en.toml');
  const data = readFileSync(path, 'utf8');

  const labels: string[] = [];
  for (const block of data.split('\n\n')) {
    const id = /^\[(nav\.[a-zA-Z0-9_.]+)\]/.exec(block);
    if (!id) continue;
    const other = /(?:^|\n)other = "([^"]*)"/.exec(block);
    if (other) labels.push(other[1]);
  }
  return labels;
}

async function assertPolishPage(page: Page, path: string, expectedTitle: string): Promise<void> {
  const problems = await watch(page);

  const response = await page.goto(path);
  expect(response, `no response for ${path}`).not.toBeNull();
  expect(response!.status(), `status of ${path}`).toBe(200);

  await expect(page.locator('html')).toHaveAttribute('lang', 'pl');
  await expect(page).toHaveTitle(expectedTitle);

  const bodyText = await page.locator('body').innerText();
  for (const label of englishNavLabels()) {
    expect(bodyText, `${path} carries the English nav label "${label}"`).not.toContain(label);
  }

  expect(problems.console, `console output on ${path}`).toEqual([]);
  expect(problems.crashes, `uncaught page failures on ${path}`).toEqual([]);
  expect(problems.csp, `CSP violations on ${path}`).toEqual([]);
  expect(problems.network, `failed requests on ${path}`).toEqual([]);
}

test.describe('a Polish account touring the application', () => {
  test.use({ signedInAs: 'anonymous' });

  test('settings, the patient list, a record list, a record detail, tags, search and the timeline all render in Polish', async ({
    page,
  }) => {
    const { patientID } = await newPolishAccount(page);

    await assertPolishPage(page, fixtures.pages.settings.path, polishTitle('nav.settings'));
    await assertPolishPage(page, '/patients', polishTitle('nav.patients'));
    await assertPolishPage(page, `${fixtures.condition.listPath}?patient=${patientID}`, polishTitle('page.conditions.title'));

    // A record detail: created for THIS account's own patient, because
    // Account A's seeded condition belongs to an account this spec never
    // signs in as.
    const created = await page.request.post(`/api/v1/records/${fixtures.condition.listPath.slice(1)}`, {
      data: { patient: patientID, diagnosis: 'Wycieczka lokalizacyjna', status: 'active' },
    });
    expect(created.status(), await created.text()).toBe(201);
    const record = (await created.json()) as { id: string };

    await assertPolishPage(page, fixtures.condition.detailPath(record.id), 'Wycieczka lokalizacyjna' + goString(
      'internal/web/views/shell/props.go',
      'SuffixSeparator',
    ) + goString('internal/web/views/shell/props.go', 'ProductName'));

    await assertPolishPage(page, '/tags', polishTitle('page.tagsPage.title'));
    await assertPolishPage(page, '/search', polishTitle('page.searchPage.title'));
    await assertPolishPage(page, `/timeline?patient=${patientID}`, polishTitle('nav.timeline'));
  });
});

// T036 (US2-1, US2-3). A browser whose OWN locale is Polish, with no account
// and no stored session: the sign-in page is already in Polish (FR-003), and
// an account created from it is created Polish (FR-004) with no PATCH here —
// unlike newPolishAccount above, which sets the locale itself because that
// spec is about an ACCOUNT's language, not the browser's.
test.describe('a browser whose own language is Polish, before it has an account', () => {
  test.use({ signedInAs: 'anonymous', locale: 'pl-PL' });

  test('the sign-in page is Polish, and an account created from it is Polish from the first signed-in page', async ({
    page,
  }) => {
    const signInResponse = await page.goto(fixtures.pages.login.path);
    expect(signInResponse, 'no response for the sign-in page').not.toBeNull();
    expect(signInResponse!.status()).toBe(200);

    await expect(page.locator('html')).toHaveAttribute('lang', 'pl');
    await expect(page).toHaveTitle(polishTitle('action.sign_in'));
    await expect(page.getByText(tomlOther('pl', 'auth.forgot_password_link'))).toBeVisible();

    const email = `locale-browser-${randomUUID().slice(0, 8)}@example.test`;
    // page.request does not carry the browser context's locale, so the header
    // the browser itself would send is set by hand.
    const registered = await page.request.post(fixtures.registerPath, {
      data: { email, name: 'Locale Browser', password: fixtures.password },
      headers: { 'Accept-Language': 'pl-PL,pl;q=0.9' },
    });
    expect(registered.status(), await registered.text()).toBe(201);

    const me = await page.request.get('/api/v1/me');
    expect(me.status(), await me.text()).toBe(200);
    const body = (await me.json()) as { locale: string };
    expect(body.locale, "the account's own locale must be Polish (FR-004)").toBe('pl');

    await assertPolishPage(page, '/', polishTitle('nav.overview'));
  });
});
