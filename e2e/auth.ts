// The signed-in browser context.
//
// This phase has no sign-in page and no session cookie: `loginPage` and the
// `login` operation both answer 501 (cmd/medikube/main_test.go's stub list), and
// PocketBase's loadAuthToken reads the Authorization header and nothing else
// (apis/middlewares.go). So the gate mints a token through
// `nativeUserAuthWithPassword` — a route the registry declares and documents —
// and hands it to the browser as a request header, which Playwright applies to
// document navigations as well as to fetches.
//
// T223 replaces this with `e2e/auth.setup.ts` signing in through MediKube's own
// /api/v1/auth/login and storing the session. When it does, every spec below
// keeps working unchanged: they ask for an account by role, not for a token.
import { test as base, type APIRequestContext } from '@playwright/test';

import { fixtures, type AccountKey } from './fixtures';

const nativeSignIn = '/api/collections/users/auth-with-password';

async function tokenFor(request: APIRequestContext, email: string): Promise<string> {
  const response = await request.post(nativeSignIn, {
    data: { identity: email, password: fixtures.password },
  });

  if (!response.ok()) {
    throw new Error(
      `e2e: ${email} could not sign in (${response.status()}). Is the instance running against the committed fixture?`,
    );
  }

  const body = (await response.json()) as { token?: string };
  if (!body.token) {
    throw new Error(`e2e: ${nativeSignIn} answered without a token for ${email}`);
  }

  return body.token;
}

// signedInAs is the option a describe block sets. Its default is the populated
// account because that is the case most pages are about; 'anonymous' is a
// context with no credential at all, which is what the refusal cases need.
export const test = base.extend<{ signedInAs: AccountKey }>({
  signedInAs: ['populated', { option: true }],

  extraHTTPHeaders: async ({ playwright, baseURL, signedInAs }, use) => {
    if (signedInAs === 'anonymous') {
      await use({});
      return;
    }

    const request = await playwright.request.newContext({ baseURL });
    try {
      await use({ Authorization: await tokenFor(request, fixtures.accounts[signedInAs]) });
    } finally {
      await request.dispose();
    }
  },
});

export { expect } from '@playwright/test';
