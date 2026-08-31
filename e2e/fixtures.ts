// The fixture's identifiers, read out of the Go source that declares them.
//
// Nothing here is written twice. `internal/testsupport/fixtures.go` is what
// every Go test addresses a seeded record by, and `internal/domain/kind` is the
// one declaration of a kind's URL spelling — so a renamed constant or a moved
// segment fails this file loudly at collection time instead of turning a
// browser assertion into a 404 that reads like a broken page.
//
// It is a text scan and not a parser on purpose: a parser for one language
// written in another is a dependency, and the shapes below are `Name = "value"`
// lines in a file the compiler already checks.
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

function goSource(relative: string): string {
  const path = resolve(repositoryRoot, relative);
  try {
    return readFileSync(path, 'utf8');
  } catch (cause) {
    throw new Error(`e2e: cannot read ${path}, which is where the fixture identifiers are declared`, { cause });
  }
}

function goString(source: string, relative: string, name: string): string {
  const found = new RegExp(`\\b${name}\\s*=\\s*"([^"]*)"`).exec(source);
  if (!found) {
    throw new Error(`e2e: ${relative} no longer declares ${name}, so the browser gate is addressing a record nobody seeds`);
  }
  return found[1];
}

function goInt(source: string, relative: string, name: string): number {
  const found = new RegExp(`\\b${name}\\s*=\\s*(\\d+)`).exec(source);
  if (!found) {
    throw new Error(`e2e: ${relative} no longer declares ${name}`);
  }
  return Number(found[1]);
}

const fixturesGo = 'internal/testsupport/fixtures.go';
const kindGo = 'internal/domain/kind/kind.go';
const shellGo = 'internal/web/views/shell/props.go';
const idsGo = 'internal/web/views/ids/ids.go';

const identifiers = goSource(fixturesGo);
const kinds = goSource(kindGo);
const shell = goSource(shellGo);
const domIDs = goSource(idsGo);

// The kind's URL spelling comes off its row in the kind table rather than being
// typed here, for the same reason internal/httproute reads it back: the page
// path, the collection and the {kind} segment of the API are one declaration
// (research D-05).
function segmentOf(kind: string): string {
  const row = new RegExp(`\\{\\s*kind:\\s*${kind}\\s*,[^}]*segment:\\s*"([^"]+)"`).exec(kinds);
  if (!row) {
    throw new Error(`e2e: ${kindGo} has no row for ${kind}, so its pages have no address`);
  }
  return row[1];
}

function enumOf(kind: string): string {
  const declared = new RegExp(`\\b${kind}\\s+Kind\\s*=\\s*"([^"]+)"`).exec(kinds);
  if (!declared) {
    throw new Error(`e2e: ${kindGo} no longer declares ${kind}`);
  }
  return declared[1];
}

const segment = segmentOf('Medication');

// The DOM ids internal/web/views/ids publishes, composed the way that package
// composes them: the kind's enum spelling, then the element's role. They are
// the contract the SSE stream patches against and the only handle some
// elements have, so the gate reads both halves rather than spelling either.
function domID(role: string, ...parts: string[]): string {
  const suffix = goString(domIDs, idsGo, `role${role[0].toUpperCase()}${role.slice(1)}`);

  return [enumOf('Medication'), suffix, ...parts].join('-');
}

export type AccountKey = 'populated' | 'counterparty' | 'empty' | 'anonymous';

export const fixtures = {
  password: goString(identifiers, fixturesGo, 'Password'),

  // Named for the role each plays in the gate rather than for the person, so a
  // case says what it is exercising. Account C is the one with nothing, and
  // that is what makes the empty state a path the run takes rather than a
  // branch somebody asserted once (research D-39).
  accounts: {
    populated: goString(identifiers, fixturesGo, 'AccountAEmail'),
    counterparty: goString(identifiers, fixturesGo, 'AccountBEmail'),
    empty: goString(identifiers, fixturesGo, 'AccountCEmail'),
  } as Record<Exclude<AccountKey, 'anonymous'>, string>,

  counts: {
    populated: goInt(identifiers, fixturesGo, 'AccountAMedicationCount'),
    counterparty: goInt(identifiers, fixturesGo, 'AccountBMedicationCount'),
    empty: goInt(identifiers, fixturesGo, 'AccountCMedicationCount'),
  },

  // The row with a name and a state and nothing else. It is the one a detail
  // view that assumes a dose or a date breaks on.
  partialRecordID: goString(identifiers, fixturesGo, 'NameOnlyMedicationID'),
  // The row whose name is right-to-left text with markup characters in it. An
  // unescaped template renders an element instead of showing the name.
  escapingRecordID: goString(identifiers, fixturesGo, 'ScriptedMedicationID'),

  // The empty state's own element. contracts/pages.md turns on it being
  // INSIDE the list's landmark rather than instead of it, and it carries no
  // role of its own, so its id is the only way to say which of the region's
  // two create actions is the empty state's.
  emptyStateID: domID('empty'),

  listPath: `/${segment}`,
  detailPath: (id: string) => `/${segment}/${id}`,

  // contracts/pages.md's title column is "{page} — MediKube", and
  // internal/web/views/shell declares both halves as constants saying in so
  // many words that a Playwright assertion matches the whole string. So it is
  // composed from them rather than spelled here, and a change to either one is
  // a failure with a reason instead of nine mismatched titles.
  title: (page: string) =>
    page === ''
      ? goString(shell, shellGo, 'ProductName')
      : page + goString(shell, shellGo, 'SuffixSeparator') + goString(shell, shellGo, 'ProductName'),
} as const;
