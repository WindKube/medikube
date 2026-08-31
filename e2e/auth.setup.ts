// T223. The gate's sessions, made once, through the door a person uses.
//
// Every signed-in case in every spec runs against a state produced here: an
// address and a password posted to MediKube's own sign-in, and whatever the
// application answers with. Nothing about the session is constructed by the
// gate — no minted token, no header, no cookie assembled by hand — so a build
// that stopped issuing a usable session would fail here rather than quietly
// keep working through a side door.
//
// It replaces the token that e2e/auth.ts used to mint through PocketBase's
// native route, and the change is not only tidiness. That route is not the one
// FR-005 is about; MediKube's is rate limited at ten guest attempts a minute
// (internal/platform/pb/settings.go), which a per-test sign-in would exhaust
// inside one project; and the session it hands back is a cookie the browser
// applies to navigations, which is the thing the pages actually run on.
import { expect, test as setup } from '@playwright/test';

import { fixtures, statePath, type SignedInAccount } from './fixtures';

const accounts = Object.keys(fixtures.accounts) as SignedInAccount[];

for (const account of accounts) {
  setup(`sign in as the ${account} account`, async ({ request }) => {
    const email = fixtures.accounts[account];

    const response = await request.post(fixtures.signInPath, {
      data: { email, password: fixtures.password },
    });

    expect(
      response.status(),
      `${email} could not sign in. Is the instance running against the committed fixture?`,
    ).toBe(200);

    // research D-15: the credential is the HttpOnly cookie and nothing else. A
    // token in this body is a token the content security policy's 'unsafe-eval'
    // would let an injected expression read, and it would still let this gate
    // sign in — so the assertion belongs here, where the answer is real.
    const body = (await response.json()) as Record<string, unknown>;
    expect(Object.keys(body), 'the sign-in answer carries a token a script could read').not.toContain('token');

    const state = await request.storageState({ path: statePath(account) });

    expect(
      state.cookies.map((cookie) => cookie.name),
      'the sign-in set no session cookie, so every case below would run anonymously and say nothing',
    ).toContain(fixtures.sessionCookieName);
  });
}
