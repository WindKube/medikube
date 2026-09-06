// T177. Smoke cases for US8's one page, at both viewports (every case here
// runs once per project in playwright.config.ts). open() carries
// contracts/pages.md's seven assertions; what is asserted here on top of
// that is the three states contracts/pages.md §5 and US8 scenario 2 name: no
// term entered yet, a term that matched nothing, and a person with nothing
// recorded at all.
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { open } from './gate';
import { expect, test } from './auth';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

function goString(relative: string, name: string): string {
  const source = readFileSync(resolve(repositoryRoot, relative), 'utf8');
  const found = new RegExp(`\\b${name}\\s*=\\s*"([^"]*)"`).exec(source);
  if (!found) {
    throw new Error(`e2e: ${relative} no longer declares ${name}`);
  }
  return found[1];
}

const fixturesGo = 'internal/testsupport/fixtures.go';
const shellGo = 'internal/web/views/shell/props.go';

function title(page: string): string {
  return page + goString(shellGo, 'SuffixSeparator') + goString(shellGo, 'ProductName');
}

// Account A's self-record: a person with records of several kinds, seeded
// with a medication named "Paracetamol" (internal/testsupport/seed/seed.go)
// that a term-matched case can look for.
const populatedPatientID = goString(fixturesGo, 'AccountAPatientSelfID');

// Account C's self-record: the fixture's one patient with nothing recorded
// of any kind (research D-39), which is what makes "nothing recorded yet"
// a path this run takes rather than a branch asserted once.
const emptyPatientID = goString(fixturesGo, 'AccountCPatientSelfID');

const searchLandmark = { role: 'search' as const };

test.describe('the search page', () => {
  test('no term entered yet reads as an invitation, not a failure', async ({ page }) => {
    const own = await open(page, {
      path: `/search?patient=${populatedPatientID}`,
      title: title('Search'),
      landmark: searchLandmark,
    });

    await expect(own).toContainText('Type a term');
  });

  test('a term matching nothing of this person reads "nothing matched"', async ({ page }) => {
    const own = await open(page, {
      path: `/search?patient=${populatedPatientID}&q=zzqxnonsensetermnobodyhas`,
      title: title('Search'),
      landmark: searchLandmark,
    });

    await expect(own).toContainText('Nothing matched');
  });

  test('a term matching this person\'s own record finds it', async ({ page }) => {
    const own = await open(page, {
      path: `/search?patient=${populatedPatientID}&q=paracetamol`,
      title: title('Search'),
      landmark: searchLandmark,
    });

    await expect(own).toContainText('Paracetamol');
  });
});

test.describe('the search page, an account with nothing recorded', () => {
  test.use({ signedInAs: 'empty' });

  test('a person with nothing recorded reads "nothing recorded yet"', async ({ page }) => {
    const own = await open(page, {
      path: `/search?patient=${emptyPatientID}&q=paracetamol`,
      title: title('Search'),
      landmark: searchLandmark,
    });

    await expect(own).toContainText('Nothing recorded yet');
  });
});
