// T268. The browser gate's page inventory, produced by the application
// itself: `medikube routes --json`, shelled out to at Playwright's collection
// phase, before any browser starts. This list IS the application's own route
// table (internal/httproute/routes.go) — nothing here is hand-maintained, and
// nothing here CAN drift, because a page missing a Landmark or a SmokeURL
// cannot even boot (internal/httproute/registry.go's describePage, FR-067).
//
// A new page therefore ships with a render-gate case for free: add a row to
// the Go table, run `task build`, and every spec that imports `pageRoutes`
// grows by one case with no TypeScript touched.
import { execFileSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import type { AccountKey } from './fixtures';
import type { Landmark } from './gate';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

// Overridable for a build placed somewhere else (e.g. a packaged binary in
// CI); the default is where instance.mjs already looks (T268 reuses that
// path rather than inventing a second one).
const binary = process.env.MEDIKUBE_BIN ?? resolve(repositoryRoot, 'medikube');

type RawRoute = {
  op_id: string;
  method: string;
  path: string;
  kind: string;
  auth: string;
  landmark?: string;
  smoke_url?: string;
  summary: string;
};

export type PageRoute = {
  opID: string;
  path: string;
  landmark: Landmark;
  smokeURL: string;
  auth: 'public' | 'user' | 'admin';
};

// parseLandmark turns internal/cli's wire form — `role[name="X"]`, verbatim
// off the Route's Landmark field — into the shape gate.ts's open() takes. Any
// other shape is a contract change nobody told this file about.
function parseLandmark(opID: string, raw: string): Landmark {
  const match = /^(\w+)\[name="(.*)"\]$/.exec(raw);
  if (!match) {
    throw new Error(
      `e2e: ${opID}'s landmark ${JSON.stringify(raw)} is not of the form role[name="X"], which is the only shape the gate can address`,
    );
  }

  const role = match[1];
  if (role !== 'region' && role !== 'article' && role !== 'form' && role !== 'main') {
    throw new Error(`e2e: ${opID}'s landmark role ${JSON.stringify(role)} is not one the gate can address`);
  }

  return { role, name: match[2] };
}

function loadRoutes(): RawRoute[] {
  let out: string;
  try {
    out = execFileSync(binary, ['routes', '--json'], { encoding: 'utf8' });
  } catch (cause) {
    throw new Error(
      `e2e: could not run ${binary} routes --json — run \`task build\` first (\`task test:e2e\` and \`task smoke\` already do), or set MEDIKUBE_BIN to point at a built binary`,
      { cause },
    );
  }

  const parsed: unknown = JSON.parse(out);
  if (!Array.isArray(parsed) || parsed.length === 0) {
    throw new Error('e2e: medikube routes --json returned no routes at all — the inventory is broken');
  }

  return parsed as RawRoute[];
}

// pageRoutes IS the browser gate's target list (FR-067, SC-009). Built once,
// at module load, so a route that cannot boot fails Playwright's collection
// phase rather than quietly narrowing the run to whatever did start.
export const pageRoutes: PageRoute[] = loadRoutes()
  .filter((route) => route.kind === 'page' && route.method === 'GET')
  .map((route) => {
    if (!route.landmark || !route.smoke_url) {
      // internal/httproute's describePage panics on either being empty, so
      // reaching this means the binary under test predates that guard.
      throw new Error(`e2e: page ${route.op_id} carries no landmark or smoke_url; internal/httproute should have refused to boot`);
    }

    return {
      opID: route.op_id,
      path: route.path,
      landmark: parseLandmark(route.op_id, route.landmark),
      smokeURL: route.smoke_url,
      auth: route.auth as PageRoute['auth'],
    };
  });

if (pageRoutes.length === 0) {
  throw new Error('e2e: medikube routes --json listed no page routes — the browser gate would smoke nothing');
}

// credentialFor picks the stored session (or its absence) a route's Auth
// column calls for. It throws rather than guesses for anything else, because
// a page under an Auth this file does not recognise is a page the gate would
// otherwise silently visit with the wrong — or no — credential.
export function credentialFor(route: Pick<PageRoute, 'opID' | 'auth'>): AccountKey {
  switch (route.auth) {
    case 'public':
      return 'anonymous';
    case 'user':
      return 'populated';
    default:
      throw new Error(
        `e2e: ${route.opID} declares auth ${JSON.stringify(route.auth)}, which no fixture in e2e/auth.ts covers; add one rather than skip the page`,
      );
  }
}
