// T183. The browser gate for contracts/pages.md's P4 and P5, at both viewports,
// plus the keyboard pass SC-014 asks for.
//
// Every case here runs twice — once per project in playwright.config.ts — so
// "it renders" and "it renders on a phone" are two results and not one.
//
// Phase 004 adds the recovery pages' cases to this file (T223p) and reuses
// gate.ts's seven assertions rather than restating them.
import { fixtures } from './fixtures';
import { open, watch } from './gate';
import { expect, test } from './auth';

import type { Locator, Page } from '@playwright/test';

const listLandmark = { role: 'region', name: 'Medications' } as const;
const detailLandmark = { role: 'article', name: 'Medication' } as const;
// contracts/pages.md fixes this one too: the delete confirmation is a rendered
// element with its own landmark and not a window.confirm, because a browser
// dialog is invisible to this gate (FR-028).
const confirmLandmark = { role: 'region', name: 'Confirm delete' } as const;

function rowsOf(landmark: Locator): Locator {
  return landmark.locator('tbody tr');
}

// --- P1, P2 and P6: the account surface (T223) ----------------------------
//
// The three pages a person meets before and after everything else. Each case
// runs gate.ts's seven assertions through open() and then asserts the one thing
// about the page that only a browser can see: what is actually rendered, in
// which order, and which of two states the account is in.

test.describe('P1 — sign in', () => {
  test.use({ signedInAs: 'anonymous' });

  test('asks for an address and a password and offers the way on to both other doors', async ({ page }) => {
    const form = await open(page, {
      path: fixtures.pages.login.path,
      title: fixtures.title(fixtures.titles.login),
      landmark: fixtures.pages.login.landmark,
    });

    // FR-005 is two controls. Anything else on a sign-in form is either a
    // third credential or something the caller should not be choosing.
    await expect(form.locator('input')).toHaveCount(2);
    await expect(form.locator('input[type="email"]')).toHaveCount(1);
    await expect(form.locator('input[type="password"]')).toHaveCount(1);

    // FR-027: a password is never rendered back into the page, not even the
    // one just typed. The form arrives with nothing in it either way.
    await expect(form.locator('input[type="password"]')).toHaveValue('');

    await expect(form.locator(`a[href="${fixtures.pages.register.path}"]`)).toBeVisible();
    await expect(form.getByRole('button', { name: fixtures.titles.login })).toBeVisible();
  });

  test('says a session ran out only when one did (FR-008)', async ({ page }) => {
    const plain = await open(page, {
      path: fixtures.pages.login.path,
      title: fixtures.title(fixtures.titles.login),
      landmark: fixtures.pages.login.landmark,
    });

    // The explanation is not a decoration: telling somebody who simply
    // navigated here that their session expired is telling them something
    // untrue about their account.
    await expect(plain.locator(`#${fixtures.ids.sessionExpired}`)).toHaveCount(0);

    const expired = await open(page, {
      path: fixtures.pages.login.path + fixtures.expiredQuery,
      title: fixtures.title(fixtures.titles.login),
      landmark: fixtures.pages.login.landmark,
    });

    const explanation = expired.locator(`#${fixtures.ids.sessionExpired}`);
    await expect(explanation).toBeVisible();
    expect((await explanation.innerText()).trim()).not.toBe('');
  });
});

test.describe('P2 — create account', () => {
  test.use({ signedInAs: 'anonymous' });

  // The instance the gate drives has registration open (e2e/instance.mjs). The
  // closed configuration renders its explanation in the same landmark and is
  // asserted in internal/web/page/accounts_test.go, because covering it here
  // would mean a second instance for one branch.
  test('publishes the password rules before a password is chosen (FR-004)', async ({ page }) => {
    const form = await open(page, {
      path: fixtures.pages.register.path,
      title: fixtures.title(fixtures.titles.register),
      landmark: fixtures.pages.register.landmark,
    });

    const rules = form.locator(`#${fixtures.ids.registrationRules}`);
    await expect(rules, 'the rules a password is judged by are not on the page that asks for one').toBeVisible();
    expect((await rules.innerText()).trim()).not.toBe('');

    // Published means the control points at them, not merely that they are
    // somewhere on the page: a screen reader announces what aria-describedby
    // names and nothing else (FR-048).
    const password = form.locator('input[type="password"]');
    await expect(password).toHaveCount(1);
    expect(
      (await password.getAttribute('aria-describedby'))?.split(/\s+/) ?? [],
      'the password control does not point at the published rules',
    ).toContain(fixtures.ids.registrationRules);

    // FR-012: role, confirmation and the disabled flag are not the caller's to
    // choose, and the form that cannot ask for them is the enforcement.
    for (const forbidden of ['role', 'verified', 'disabled_at']) {
      await expect(form.locator(`[name="${forbidden}"]`), `the sign-up form offers ${forbidden}`).toHaveCount(0);
    }
  });
});

test.describe('P6 — settings', () => {
  test('renders this account, its preferences and what deleting it would destroy', async ({ page }) => {
    const settings = await open(page, {
      path: fixtures.pages.settings.path,
      title: fixtures.title(fixtures.titles.settings),
      landmark: fixtures.pages.settings.landmark,
    });

    // FR-011, FR-005 and FR-013 are three forms and a region, and every one of
    // them is inside the page's own landmark rather than beside it.
    for (const name of [fixtures.settingsLandmarks.profile, fixtures.settingsLandmarks.password]) {
      await expect(settings.getByRole('form', { name }), `no form[name="${name}"] on the settings page`).toBeVisible();
    }
    await expect(settings.getByRole('region', { name: fixtures.settingsLandmarks.dangerZone })).toBeVisible();

    // The address is shown and cannot be edited here: this version has no
    // address-change flow, and a control that looks like one is a promise the
    // application does not keep.
    expect(await settings.innerText()).toContain(fixtures.accounts.populated);
    await expect(
      settings.locator('input[type="email"]'),
      'the settings page offers a control for an address it cannot change',
    ).toHaveCount(0);

    expect(fixtures.confirmed.populated, 'the seeded account stopped being the confirmed one').toBe(true);
    await expect(settings.locator(`#${fixtures.ids.emailConfirmed}`)).toBeVisible();
    await expect(settings.locator(`#${fixtures.ids.emailUnconfirmed}`)).toHaveCount(0);

    const holdings = settings.locator(`#${fixtures.ids.holdings}`);
    await expect(holdings).toBeVisible();
    expect(
      await holdings.innerText(),
      'the danger zone does not say how much this account would lose',
    ).toContain(String(fixtures.counts.populated));

    // FR-013: what will be destroyed is stated BEFORE the form that destroys
    // it. A consequence printed underneath the button is a consequence read
    // after the decision.
    const stated = await settings
      .getByRole('region', { name: fixtures.settingsLandmarks.dangerZone })
      .evaluate((zone, id) => {
        const consequence = zone.querySelector(`#${id}`);
        const form = zone.querySelector('form');
        if (!consequence || !form) return false;

        return (consequence.compareDocumentPosition(form) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
      }, fixtures.ids.holdings);
    expect(stated, 'the deletion form comes before the consequence it carries out').toBe(true);

    // FR-013 again: the phrase is exact, and the page has to say which one.
    const deletion = settings.getByRole('form', { name: fixtures.settingsLandmarks.deleteAccount });
    await expect(deletion).toBeVisible();
    expect(await deletion.innerText()).toContain(fixtures.deleteConfirmationPhrase);
    await expect(deletion.locator('input[type="password"]'), 'deletion asks for no password').toHaveCount(1);
  });

  test.describe('as the account whose address is not confirmed', () => {
    test.use({ signedInAs: 'empty' });

    test('says so and offers to send the confirmation again (FR-075)', async ({ page }) => {
      expect(
        fixtures.confirmed.empty,
        'every seeded address is confirmed, so this case is asserting the other branch',
      ).toBe(false);

      const settings = await open(page, {
        path: fixtures.pages.settings.path,
        title: fixtures.title(fixtures.titles.settings),
        landmark: fixtures.pages.settings.landmark,
      });

      const unconfirmed = settings.locator(`#${fixtures.ids.emailUnconfirmed}`);
      await expect(unconfirmed).toBeVisible();
      await expect(
        unconfirmed.getByRole('button'),
        'the page says the address is unconfirmed and offers no way to fix it',
      ).toBeVisible();

      await expect(settings.locator(`#${fixtures.ids.emailConfirmed}`)).toHaveCount(0);

      // The same page for an account that holds nothing: the danger zone still
      // states the consequence rather than rendering an empty list.
      const holdings = settings.locator(`#${fixtures.ids.holdings}`);
      await expect(holdings).toBeVisible();
      expect(await holdings.innerText()).toContain(String(fixtures.counts.empty));
    });
  });

  test.describe('with no session at all', () => {
    test.use({ signedInAs: 'anonymous' });

    test('is refused rather than served (E2)', async ({ page }) => {
      const response = await page.goto(fixtures.pages.settings.path);

      // 403 and not 404, for the same reason as the record list: whether this
      // instance has a settings page is not information about anybody, and a
      // 404 would make "no session" and "no such page" the same answer.
      expect(response?.status()).toBe(403);
    });
  });
});

test.describe('P4 — the record list', () => {
  test('lists what the account owns and nothing else', async ({ page }) => {
    const list = await open(page, {
      path: fixtures.listPath,
      title: fixtures.title('Medications'),
      landmark: listLandmark,
    });

    // The whole account fits in one page: internal/web's default limit is 25
    // and the fixture holds fewer, so a short list here is a missing row and
    // not a second page.
    await expect(rowsOf(list)).toHaveCount(fixtures.counts.populated);

    // The partial-data row is on the list, and a renderer that assumed a dose
    // or a start date would have had to invent one to get here.
    await expect(list.locator(`a[href="${fixtures.detailPath(fixtures.partialRecordID)}"]`)).toBeVisible();
  });

  test.describe('as the isolation counterparty', () => {
    test.use({ signedInAs: 'counterparty' });

    test('holds fewer rows, and not one belonging to the populated account', async ({ page }) => {
      const list = await open(page, {
        path: fixtures.listPath,
        title: fixtures.title('Medications'),
        landmark: listLandmark,
      });

      await expect(rowsOf(list)).toHaveCount(fixtures.counts.counterparty);
      await expect(list.locator(`a[href="${fixtures.detailPath(fixtures.partialRecordID)}"]`)).toHaveCount(0);
    });
  });

  test.describe('as the account with nothing recorded', () => {
    test.use({ signedInAs: 'empty' });

    test('renders its empty state INSIDE the landmark, with the create action (FR-029)', async ({ page }) => {
      const list = await open(page, {
        path: fixtures.listPath,
        title: fixtures.title('Medications'),
        landmark: listLandmark,
      });

      expect(fixtures.counts.empty, 'the fixture stopped leaving one account empty').toBe(0);
      await expect(rowsOf(list)).toHaveCount(0);

      // Inside, not instead of. A bare centred paragraph where the region
      // should be passes a presence check and fails the person, and this is
      // the case research D-39 exists for.
      const empty = list.locator(`#${fixtures.emptyStateID}`);
      await expect(empty, 'the empty state is not inside the landmark').toBeVisible();
      await expect(empty.getByRole('link'), 'the empty state offers no way to record the first one').toBeVisible();
    });
  });

  test.describe('with no session at all', () => {
    test.use({ signedInAs: 'anonymous' });

    test('is refused rather than served', async ({ page }) => {
      const response = await page.goto(fixtures.listPath);

      // 403 and not 404: the existence of the list is not information about
      // anybody. contracts/pages.md's E2 renders the sign-in prompt inside the
      // shell here; the composition root wires no error view yet (T263), so
      // this asserts the refusal and not yet the view.
      expect(response?.status()).toBe(403);
    });
  });
});

test.describe('P5 — one record', () => {
  test('shows the row that has nothing but a name, without empty placeholders', async ({ page }) => {
    const article = await open(page, {
      path: fixtures.detailPath(fixtures.partialRecordID),
      title: fixtures.title(await nameOf(page, fixtures.partialRecordID)),
      landmark: detailLandmark,
    });

    // FR-024: a field that was never filled in is left out entirely rather
    // than shown as a blank. A label with nothing beside it is the failure.
    const labels = await article.locator('dt').count();
    const values = await article.locator('dd').count();
    expect(labels, 'a label was rendered with no value beside it').toBe(values);
    for (const value of await article.locator('dd').allInnerTexts()) {
      expect(value.trim(), 'an empty value was rendered instead of being left out').not.toBe('');
    }

    await expect(page.getByRole(confirmLandmark.role, { name: confirmLandmark.name })).toBeAttached();
  });

  test('shows a name full of markup characters as text', async ({ page }) => {
    const article = await open(page, {
      path: fixtures.detailPath(fixtures.escapingRecordID),
      title: fixtures.title(await nameOf(page, fixtures.escapingRecordID)),
      landmark: detailLandmark,
    });

    // The fixture's name and notes contain an element and a script. If either
    // reached the browser as markup there would be one here.
    expect(await article.locator('b').count(), 'the name was rendered as markup').toBe(0);
    expect(await article.locator('script').count(), 'the notes were rendered as markup').toBe(0);
    expect(await article.innerText()).toContain('<b>');
  });
});

// nameOf reads the record's own name from the API, so the title assertion is
// not a copy of the fixture's prose. It uses the browser context's credential,
// which is the same account the page is rendered for.
async function nameOf(page: Page, id: string): Promise<string> {
  const response = await page.request.get(`/api/v1/records${fixtures.detailPath(id)}`);
  expect(response.ok(), `the API did not answer for ${id}`).toBe(true);

  return ((await response.json()) as { name: string }).name;
}

// --- SC-014: the keyboard ---------------------------------------------------
//
// "A person using only a keyboard can complete the whole of recording, editing
// and deleting a medication, at both viewports, without becoming unable to see
// where the focus is."
//
// What is asserted below is the half that is true of this build: every control
// of that sequence is reachable by Tab, in document order, and shows where the
// focus is. The other half — pressing them and having a record appear, change
// and vanish — needs the Datastar runtime, and the shell serves no script tag
// yet (internal/web/views/shell/document.templ says so and says why: there is
// no asset route to serve it from). The fixme at the bottom of this file is
// that gap, deliberately visible.

type Focused = {
  tag: string;
  id: string;
  label: string;
  indicator: boolean;
};

// descriptor is how a control seen in the DOM and the same control seen under
// focus are recognised as one thing, without marking up the page to do it.
function describe(control: Focused): string {
  return `${control.tag}#${control.id}:${control.label}`;
}

const focusable =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

async function focusableControls(page: Page, within: string): Promise<string[]> {
  return page.evaluate(
    ([selector, scope]) => {
      const root = document.querySelector(scope);
      if (!root) return [];

      return Array.from(root.querySelectorAll<HTMLElement>(selector))
        .filter((element) => element.offsetParent !== null || element.tagName === 'A')
        .map((element) => {
          const label = (element.getAttribute('aria-label') ?? element.textContent ?? '').trim().slice(0, 40);
          return `${element.tagName.toLowerCase()}#${element.id}:${label}`;
        });
    },
    [focusable, within] as const,
  );
}

async function tabThrough(page: Page, steps: number): Promise<Focused[]> {
  const walk: Focused[] = [];

  for (let step = 0; step < steps; step += 1) {
    await page.keyboard.press('Tab');

    const control = await page.evaluate(() => {
      const element = document.activeElement as HTMLElement | null;
      if (!element || element === document.body) return null;

      const style = getComputedStyle(element);

      return {
        tag: element.tagName.toLowerCase(),
        id: element.id,
        label: (element.getAttribute('aria-label') ?? element.textContent ?? '').trim().slice(0, 40),
        // Either is a visible indicator. Asserting one particular mechanism
        // would fail the day the stylesheet lands and replaces the browser's
        // ring with its own.
        indicator: (style.outlineStyle !== 'none' && style.outlineWidth !== '0px') || style.boxShadow !== 'none',
      };
    });

    if (control === null) break;

    // A native date input is four Tab stops — day, month, year and the
    // picker — and only the first of them draws the ring on the host: the rest
    // are inside a shadow tree no script can read, so the host reports no
    // outline while the browser is quite plainly showing one. Collapsing
    // consecutive stops on the same identified element counts the control
    // once, which is also what "reachable" means to the person tabbing.
    const previous = walk[walk.length - 1];
    if (previous && control.id !== '' && previous.id === control.id && previous.tag === control.tag) {
      continue;
    }

    walk.push(control);
  }

  return walk;
}

// reached checks that every expected control appears in the walk, in order. A
// control that is missing is unreachable by keyboard; one out of order is a
// focus order that does not follow the page.
function reached(walk: Focused[], expected: string[]): void {
  const seen = walk.map(describe);
  const remaining = [...expected];

  for (const control of seen) {
    if (remaining[0] === control) remaining.shift();
  }

  expect(remaining, `these controls were never reached by Tab, in order: ${remaining.join(', ')}`).toEqual([]);
}

test.describe('SC-014 — the keyboard', () => {
  test('reaches every control of the list page and its create form, showing where the focus is', async ({ page }) => {
    await watch(page);
    await page.goto(fixtures.listPath);

    const expected = await focusableControls(page, 'main');
    expect(expected.length, 'the page offers no controls at all').toBeGreaterThan(12);

    const walk = await tabThrough(page, expected.length + 8);

    expect(walk[0]?.label, 'the first Tab does not reach the skip link').toBe('Skip to content');
    reached(walk, expected);

    for (const control of walk) {
      expect(control.indicator, `no focus indicator on ${describe(control)}`).toBe(true);
    }
  });

  test('reaches edit, delete and the delete confirmation on a record', async ({ page }) => {
    await watch(page);
    await page.goto(fixtures.detailPath(fixtures.partialRecordID));

    const article = page.getByRole(detailLandmark.role, { name: detailLandmark.name });
    await expect(article.getByRole('link', { name: 'Edit' })).toBeVisible();

    const confirm = page.getByRole(confirmLandmark.role, { name: confirmLandmark.name });
    await expect(confirm.getByRole('button')).toHaveCount(2);

    const expected = await focusableControls(page, 'main');
    const walk = await tabThrough(page, expected.length + 8);

    reached(walk, expected);

    for (const control of walk) {
      expect(control.indicator, `no focus indicator on ${describe(control)}`).toBe(true);
    }
  });

  test.fixme(
    'records, edits and deletes a medication with the keyboard alone',
    async () => {
      // Not skipped because it is hard: skipped because it cannot pass yet and
      // saying so out loud is better than a gate that quietly does not cover
      // SC-014's verb. The forms carry `data-on:submit__prevent` and the
      // delete button `@delete(...)`, and nothing in the shell loads the
      // runtime that reads them — internal/web/views/shell/document.templ
      // renders no script tag because no route serves internal/web/static
      // (T261 adds both, T268 adds the generated page list this file will use).
      // Until then a keypress on the submit button is a native GET, which is
      // not what any of this is meant to do.
    },
  );
});
