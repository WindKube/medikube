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
import type { Landmark } from './gate';

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

function goBool(source: string, relative: string, name: string): boolean {
  const found = new RegExp(`\\b${name}\\s*=\\s*(true|false)`).exec(source);
  if (!found) {
    throw new Error(`e2e: ${relative} no longer declares ${name}`);
  }
  return found[1] === 'true';
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
const routesGo = 'internal/httproute/routes.go';
const sessionGo = 'internal/web/session.go';
const accountPageGo = 'internal/web/page/accounts.go';
const loginPageGo = 'internal/web/page/login.go';
const authViewGo = 'internal/web/views/auth/props.go';
const settingsViewGo = 'internal/web/views/settings/props.go';
const recoveryGo = 'internal/domain/identity/recovery.go';

const identifiers = goSource(fixturesGo);
const kinds = goSource(kindGo);
const shell = goSource(shellGo);
const domIDs = goSource(idsGo);
const routes = goSource(routesGo);
const session = goSource(sessionGo);
const accountPages = goSource(accountPageGo);
const loginPage = goSource(loginPageGo);
const authView = goSource(authViewGo);
const settingsView = goSource(settingsViewGo);
const recovery = goSource(recoveryGo);

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

// A route's Path and a page's smoke target are read out of the route table
// rather than typed here, for the reason httproute.Handle panics without them:
// the table is what the router serves and what the inventory gate checks, so a
// page addressed by any other spelling is a page this gate is not testing.
//
// A Path is written as a chain of literals and identifiers declared in the same
// file — `base + "/login"` — so the expression is resolved rather than matched.
// The nearest DECLARATION ABOVE the row wins, which is what Go's own scoping
// does with the several `base :=` in that file.
function goExpression(expression: string, before: number): string {
  return expression
    .split('+')
    .map((term) => {
      const trimmed = term.trim();
      const literal = /^"([^"]*)"$/.exec(trimmed);

      return literal ? literal[1] : goIdentifier(trimmed, before);
    })
    .join('');
}

function goIdentifier(name: string, before: number): string {
  const declarations = new RegExp(`\\b${name}\\s*(?::=|=)\\s*([^\\n]+?)\\s*$`, 'gm');

  let declared: { value: string; at: number } | null = null;
  for (let found = declarations.exec(routes); found !== null; found = declarations.exec(routes)) {
    if (found.index > before) break;
    declared = { value: found[1], at: found.index };
  }

  if (declared === null) {
    throw new Error(`e2e: ${routesGo} declares no ${name} above the row that uses it`);
  }

  return goExpression(declared.value, declared.at);
}

function goRoutePath(opID: string): string {
  const row = new RegExp(`OpID:\\s*"${opID}"\\s*,[^\\n]*?Path:\\s*([^,\\n]+)`).exec(routes);
  if (!row) {
    throw new Error(`e2e: ${routesGo} has no ${opID} route, so the gate is driving an operation nobody serves`);
  }

  return goExpression(row[1], row.index);
}

// A page's smoke target: the URL the inventory publishes and the landmark it
// promises there. Both halves come off the same row, so a page whose landmark
// was renamed fails here with its own name in the message rather than as an
// empty locator in whichever case happened to look for it first.
function goPageTarget(opID: string): PageTarget {
  const row = new RegExp(
    `OpID:\\s*"${opID}"\\s*,[\\s\\S]{0,600}?Landmark:\\s*\`(\\w+)\\[name="([^"]+)"\\]\`\\s*,\\s*SmokeURL:\\s*([^,\\n]+)`,
  ).exec(routes);
  if (!row) {
    throw new Error(`e2e: ${routesGo} has no smoke target for ${opID}`);
  }

  const role = row[1];
  if (role !== 'region' && role !== 'article' && role !== 'form' && role !== 'main') {
    throw new Error(`e2e: ${opID}'s landmark role ${role} is not one the gate can address`);
  }

  return { path: goExpression(row[3], row.index), landmark: { role, name: row[2] } };
}

export type PageTarget = { path: string; landmark: Landmark };

export type AccountKey = 'populated' | 'counterparty' | 'empty' | 'anonymous';

// SignedInAccount is every key that HAS a session. `anonymous` is the absence
// of one and therefore has no stored state and no address.
export type SignedInAccount = Exclude<AccountKey, 'anonymous'>;

// statePath is where auth.setup.ts leaves one account's signed-in state and
// where auth.ts picks it up. Outside version control and inside e2e/, so a
// stale credential is removed with the rest of the run's leavings.
export function statePath(account: SignedInAccount): string {
  return resolve(dirname(fileURLToPath(import.meta.url)), '.auth', `${account}.json`);
}

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

  // Whether each account's address is confirmed. The settings page renders one
  // of two states from this, and the gate visits both — so it reads the seed's
  // own answer rather than assuming which account is which (FR-075).
  confirmed: {
    populated: goBool(identifiers, fixturesGo, 'AccountAConfirmed'),
    counterparty: goBool(identifiers, fixturesGo, 'AccountBConfirmed'),
    empty: goBool(identifiers, fixturesGo, 'AccountCConfirmed'),
  } as Record<SignedInAccount, boolean>,

  // MediKube's own sign-in, which is what auth.setup.ts drives. The session is
  // this cookie and nothing else (research D-15): there is no token in the
  // body, so a gate that could not read the cookie could not sign in at all.
  signInPath: goRoutePath('login'),
  sessionCookieName: goString(session, sessionGo, 'SessionCookieName'),

  // The three operations the recovery flow drives (T223p). Read out of the
  // route table for the reason the sign-in is: an address composed here is an
  // address nothing serves.
  registerPath: goRoutePath('register'),
  recoveryPath: goRoutePath('requestPasswordReset'),
  recoveryConfirmPath: goRoutePath('confirmPasswordReset'),

  // The six account pages, each with the landmark its route promises.
  pages: {
    login: goPageTarget('loginPage'),
    register: goPageTarget('registerPage'),
    settings: goPageTarget('settingsPage'),
    forgotPassword: goPageTarget('forgotPasswordPage'),
    resetPassword: goPageTarget('resetPasswordPage'),
    verifyEmail: goPageTarget('verifyEmailPage'),
  },

  // The titles those pages set, read from the package that sets them rather
  // than from contracts/pages.md's table: the table is the requirement and this
  // is what the requirement is checked against.
  titles: {
    login: goString(accountPages, accountPageGo, 'loginTitle'),
    register: goString(accountPages, accountPageGo, 'registerTitle'),
    settings: goString(accountPages, accountPageGo, 'settingsTitle'),
    forgotPassword: goString(accountPages, accountPageGo, 'forgotPasswordTitle'),
    resetPassword: goString(accountPages, accountPageGo, 'resetPasswordTitle'),
    // contracts/pages.md gives P9 a tab that does not repeat its landmark: the
    // region is named "Email confirmation" and the title says "Confirm your
    // address", so reading both from the source is the only way this gate
    // asserts the pair rather than one of them twice.
    verifyEmail: goString(accountPages, accountPageGo, 'verifyEmailTitle'),
  },

  // FR-008's query. A person whose session ran out is told so; everybody else
  // is not, and the difference is one parameter.
  expiredQuery: `?${goString(loginPage, loginPageGo, 'ParamReason')}=${goString(loginPage, loginPageGo, 'ReasonExpired')}`,

  // Elements with no role of their own, addressed by the id the view publishes.
  ids: {
    sessionExpired: goString(authView, authViewGo, 'SessionExpiredID'),
    registrationRules: goString(authView, authViewGo, 'PasswordRulesID'),
    newPasswordRules: goString(authView, authViewGo, 'NewPasswordRulesID'),
    // FR-074's two states, both rendered INSIDE their page's landmark rather
    // than as an error view, and neither carrying a role of its own.
    linkDead: goString(authView, authViewGo, 'LinkDeadID'),
    mailUnconfigured: goString(authView, authViewGo, 'MailUnconfiguredID'),
    emailConfirmed: goString(settingsView, settingsViewGo, 'EmailConfirmedID'),
    emailUnconfirmed: goString(settingsView, settingsViewGo, 'EmailUnconfirmedID'),
    passwordRules: goString(settingsView, settingsViewGo, 'PasswordRulesID'),
    holdings: goString(settingsView, settingsViewGo, 'HoldingsID'),
  },

  // The inner landmarks of the settings page. contracts/pages.md gives P6 one
  // region; FR-011, FR-005 and FR-013 are three forms inside it.
  settingsLandmarks: {
    profile: goString(settingsView, settingsViewGo, 'ProfileLandmark'),
    password: goString(settingsView, settingsViewGo, 'PasswordLandmark'),
    dangerZone: goString(settingsView, settingsViewGo, 'DangerZoneLandmark'),
    deleteAccount: goString(settingsView, settingsViewGo, 'DeleteAccountLandmark'),
  },

  // FR-013's exact words. Read from the domain, because the form asking for one
  // phrase while the service compares another is a delete nobody can complete.
  deleteConfirmationPhrase: goString(recovery, recoveryGo, 'DeleteConfirmationPhrase'),

  listPath: `/${segment}`,
  detailPath: (id: string) => `/${segment}/${id}`,

  // US1's three other kinds (T059): one seeded row each, addressed the same
  // way medication's own listPath/detailPath are — off the kind table's
  // segment, never spelled here.
  allergy: {
    listPath: `/${segmentOf('Allergy')}`,
    detailPath: (id: string) => `/${segmentOf('Allergy')}/${id}`,
    seededID: goString(identifiers, fixturesGo, 'CriticalAllergyID'),
  },
  condition: {
    listPath: `/${segmentOf('Condition')}`,
    detailPath: (id: string) => `/${segmentOf('Condition')}/${id}`,
    seededID: goString(identifiers, fixturesGo, 'ResolvedConditionID'),
  },
  emergencyContact: {
    listPath: `/${segmentOf('EmergencyContact')}`,
    detailPath: (id: string) => `/${segmentOf('EmergencyContact')}/${id}`,
    seededID: goString(identifiers, fixturesGo, 'PrimaryContactID'),
  },

  // P8's address for a link that actually works, as opposed to
  // pages.resetPassword.path, which is the route's deliberately dead smoke
  // token. The pattern and the name of its placeholder are both read back —
  // internal/web/page/accounts.go declares PathToken precisely because a
  // handler reading a differently spelled parameter answers the dead-link
  // state to everybody, which looks exactly like a page that works.
  resetPasswordPath: (token: string) =>
    goRoutePath('resetPasswordPage').replace(`{${goString(accountPages, accountPageGo, 'PathToken')}}`, token),

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
