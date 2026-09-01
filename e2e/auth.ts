// The signed-in browser context.
//
// A spec asks for an account by the role it plays — `test.use({ signedInAs:
// 'counterparty' })` — and gets a context carrying that account's session
// cookie, or, for 'anonymous', a context carrying nothing at all.
//
// The sessions themselves are made in auth.setup.ts, once per run, and stored;
// this file only chooses which one a case runs under. Signing in here instead
// would be one sign-in per test against a route deliberately limited to ten
// guest attempts a minute (FR-006), and the failure that produces is a 429 in
// the middle of an unrelated assertion.
import { test as base } from '@playwright/test';

import { statePath, type AccountKey } from './fixtures';

// signedInAs is the option a describe block sets. Its default is the populated
// account because that is the case most pages are about; 'anonymous' is a
// context with no credential at all, which is what the refusal cases need.
export const test = base.extend<{ signedInAs: AccountKey }>({
  signedInAs: ['populated', { option: true }],

  storageState: async ({ signedInAs }, use) => {
    // Empty rather than undefined: undefined would inherit whatever the
    // project's own storageState is, and an "anonymous" case that silently
    // carried a session is a refusal test that can no longer fail.
    await use(signedInAs === 'anonymous' ? { cookies: [], origins: [] } : statePath(signedInAs));
  },
});

export { expect } from '@playwright/test';
