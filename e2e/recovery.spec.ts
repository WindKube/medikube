// T223p, SC-016 and Phase Exit Criterion 8. Password recovery, the whole way
// through, in a browser.
//
// What makes this case worth its cost is the message. A recovery token is a JWT
// signed from the account's own tokenKey and the collection's reset secret, and
// PocketBase persists no row for it — so a test that minted its own token would
// be asserting that a token this application never sent still works, which is
// the one thing about recovery nobody doubts. The link here is read out of a
// real message, delivered over a real SMTP conversation, to e2e/mailsink.mjs.
//
// Every case below works on an account of its own, created for it. The three
// seeded accounts are off limits: a recovery ends their sessions and replaces
// their password, and every other case in this run is signed in as one of them.
import { randomUUID } from 'node:crypto';

import { fixtures } from './fixtures';
import { open } from './gate';
import { mailSinkURL } from './playwright.config';
import { expect, test } from './auth';

import type { APIRequestContext } from '@playwright/test';

// What the sink captured: the envelope of the conversation and the data, decoded
// out of the quoted-printable every part of one of these messages is written in.
type Captured = {
  from: string;
  to: string[];
  raw: string;
  text: string;
  receivedAt: string;
};

async function messagesTo(request: APIRequestContext, address: string): Promise<Captured[]> {
  const response = await request.get(`${mailSinkURL}/messages?to=${encodeURIComponent(address)}`);

  expect(
    response.ok(),
    `the mail sink at ${mailSinkURL} did not answer. It is started by e2e/instance.mjs, so this is a run ` +
      'against an instance that harness did not boot',
  ).toBe(true);

  return ((await response.json()) as { messages: Captured[] }).messages;
}

// A JSON Web Token: three base64url segments, each long enough that nothing
// else in a MIME message — a boundary, a Message-ID, a stylesheet — collides
// with it.
const tokenPattern = /[\w-]{16,}\.[\w-]{16,}\.[\w-]{16,}/g;

// A URL in either part of the message. The closing bracket is excluded because
// the plain-text alternative PocketBase derives from the HTML writes the link
// as `[Reset password](url)`.
const linkPattern = /https?:\/\/[^\s"'<>()]+/g;

// recoveryToken reads the one token the message carries, and asserts it is
// carried in a link rather than printed as a value.
//
// The TOKEN and not the path, deliberately. data-model.md leaves
// VerificationTemplate and ResetPasswordTemplate at PocketBase's defaults, and
// that template addresses {APP_URL}/_/#/auth/confirm-password-reset/{TOKEN} —
// the admin UI's own route, not contracts/pages.md's P8. So the link a person
// receives today does not open the page this case then drives. Reading the
// token rather than the path is what lets this assert the flow that matters
// while that is settled, and it reads the same either way, so nothing here
// needs editing when the template is.
function recoveryToken(message: Captured): string {
  const tokens = [...new Set([...message.text.matchAll(tokenPattern)].map((found) => found[0]))];
  expect(tokens.length, `the message carries ${tokens.length} distinct tokens, not one:\n${message.text}`).toBe(1);

  const carrying = [...message.text.matchAll(linkPattern)].map((found) => found[0]).filter((link) => link.includes(tokens[0]));
  expect(carrying.length, 'the token is in the message but not inside a link anybody could follow').toBeGreaterThan(0);

  return tokens[0];
}

function newAccount(): { email: string; password: string } {
  // Unique per case, so the two viewport projects do not race for one account
  // and a rerun against a still-running instance does not collide with itself.
  return {
    email: `recovery-${randomUUID().slice(0, 8)}@example.test`,
    password: `first-password-${randomUUID().slice(0, 8)}`,
  };
}

test.describe('recovery, end to end', () => {
  // No stored session: this flow starts at a sign-up form and ends at a
  // sign-in form, and a case that arrived already signed in would prove
  // neither.
  test.use({ signedInAs: 'anonymous' });

  test('a link out of a real message sets the password, kills the old session and lets the account back in', async ({
    page,
  }) => {
    const account = newAccount();
    const recovered = `recovered-password-${randomUUID().slice(0, 8)}`;

    const registered = await page.request.post(fixtures.registerPath, {
      data: { email: account.email, name: 'Recovery Rehearsal', password: account.password },
    });
    expect(registered.status(), await registered.text()).toBe(201);

    // The session registration issued, used the way a person uses one. Asserted
    // here so that its death below is a page that stopped opening rather than a
    // token comparison.
    expect((await page.goto(fixtures.pages.settings.path))?.status()).toBe(200);

    const asked = await page.request.post(fixtures.recoveryPath, { data: { email: account.email } });
    expect(asked.status(), await asked.text()).toBe(202);

    // Read without waiting, and that is an assertion of its own:
    // internal/platform/pb/mail.go sends synchronously — deliberately, unlike
    // PocketBase's own route, which fires and forgets — so the message is in
    // the sink before the 202 was written. A poll here would quietly cover for
    // the day that stops being true.
    const messages = await messagesTo(page.request, account.email);
    expect(messages.length, `${account.email} was sent ${messages.length} messages, not one`).toBe(1);
    expect(messages[0].to, 'the recovery message went somewhere else as well').toEqual([account.email]);

    const token = recoveryToken(messages[0]);

    // P8 with a link that works: the form, not the dead-link state, and the
    // token carried back in a hidden control rather than read out of the
    // address bar by a script.
    const form = await open(page, {
      path: fixtures.resetPasswordPath(token),
      title: fixtures.title(fixtures.titles.resetPassword),
      landmark: fixtures.pages.resetPassword.landmark,
    });
    await expect(form.locator(`#${fixtures.ids.linkDead}`), 'a live link opened the dead-link state').toHaveCount(0);
    await expect(form.locator('input[type="password"]')).toHaveCount(2);
    await expect(form.locator(`#${fixtures.ids.newPasswordRules}`)).toBeVisible();
    await expect(form.locator(`input[type="hidden"][value="${token}"]`)).toHaveCount(1);

    const set = await page.request.post(fixtures.recoveryConfirmPath, {
      data: { token, password: recovered, password_confirm: recovered },
    });
    expect(set.status(), await set.text()).toBe(204);

    // FR-074's last sentence. The browser is still holding the cookie it was
    // given at registration; what changed is that the credential inside it
    // stopped working.
    expect(
      (await page.goto(fixtures.pages.settings.path))?.status(),
      'a session issued before the recovery survived it',
    ).toBe(403);

    // And the link is spent. The same address that rendered the form a moment
    // ago now renders FR-074's one refusal, with the offer to ask for another.
    const spent = await open(page, {
      path: fixtures.resetPasswordPath(token),
      title: fixtures.title(fixtures.titles.resetPassword),
      landmark: fixtures.pages.resetPassword.landmark,
    });
    const dead = spent.locator(`#${fixtures.ids.linkDead}`);
    await expect(dead, 'a link that has been used still opens the form').toBeVisible();
    await expect(dead.locator(`a[href="${fixtures.pages.forgotPassword.path}"]`)).toBeVisible();

    const signedIn = await page.request.post(fixtures.signInPath, {
      data: { email: account.email, password: recovered },
    });
    expect(signedIn.status(), `the password chosen through the link does not sign in: ${await signedIn.text()}`).toBe(200);
  });

  // FR-073's server-side half, which only a real sink can show: the two
  // requests are answered identically — internal/web/api/password_reset_test.go
  // compares those bytes — and the difference is entirely in what was sent.
  test('an address with no account is accepted and sent nothing (FR-073)', async ({ page }) => {
    const stranger = `nobody-${randomUUID().slice(0, 8)}@example.test`;

    const asked = await page.request.post(fixtures.recoveryPath, { data: { email: stranger } });
    expect(asked.status(), await asked.text()).toBe(202);

    expect(
      await messagesTo(page.request, stranger),
      'an address with no account was sent a message, which is the oracle FR-073 closes',
    ).toEqual([]);
  });
});
