// The instance the browser gate drives.
//
// It serves the committed fixture — the same data directory every Go test
// clones, and the same rows `medikube seed` writes, which
// internal/testsupport/fixtures_test.go is the gate for. A copy, never the
// original: the server writes to it, and a smoke run that left the fixture
// dirty would make the next `go test` compare yesterday's data.
//
// `medikube seed` cannot be used instead, because there is no seed subcommand
// yet (T288 registers it and internal/cli/seed.go is the behaviour it will
// register). When it lands this file becomes `migrate` + `seed` against an
// empty directory and nothing else here changes.
//
// It also assembles the instance's OUTGOING MAIL, because T223p's recovery
// flow is not testable without it: the link a person follows exists only in a
// message. The sink is e2e/mailsink.mjs; this file starts it, points the
// instance at it through the admin API — SMTP is PocketBase's own settings
// store and not a MEDIKUBE_ variable (internal/platform/pb/mail.go says why) —
// and only then opens the endpoint Playwright waits on, so a spec can never
// run against an instance whose mail is still unconfigured.
import { cpSync, existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { spawn } from 'node:child_process';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { smtpAddress, splitAddress, startMailSink } from './mailsink.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '..');

const binary = resolve(root, 'medikube');
const fixture = resolve(root, 'internal/testdata/pb_data');
// Outside the working tree, so a smoke run leaves nothing behind for the next
// `git status` or the next `go test` to trip over.
const instanceDir = mkdtempSync(join(tmpdir(), 'medikube-e2e-'));
const dataDir = join(instanceDir, 'pb_data');

const address = process.env.MEDIKUBE_E2E_ADDR ?? '127.0.0.1:8091';
const publicURL = `http://${address}`;

function refuse(message) {
  process.stderr.write(`e2e: ${message}\n`);
  process.exit(1);
}

// The superuser credential, read out of the Go file that seeds it rather than
// written here, for the reason e2e/fixtures.ts reads every other identifier out
// of the same place: a credential spelled twice is a credential that stops
// working silently.
const identifiers = 'internal/testsupport/fixtures.go';

function goString(name) {
  const source = readFileSync(resolve(root, identifiers), 'utf8');
  const found = new RegExp(`\\b${name}\\s*=\\s*"([^"]*)"`).exec(source);
  if (!found) {
    refuse(`${identifiers} no longer declares ${name}, so the instance cannot be configured`);
  }

  return found[1];
}

if (!existsSync(binary)) {
  refuse(`no binary at ${binary} — run \`task build\` first (\`task test:e2e\` does)`);
}

if (!existsSync(fixture)) {
  refuse(`no fixture at ${fixture} — it is committed, so this is a broken checkout`);
}

cpSync(fixture, dataDir, { recursive: true });

const sink = startMailSink();
await sink.listen();

const server = spawn(binary, ['serve'], {
  stdio: 'inherit',
  env: {
    ...process.env,
    // Not production: the lockdown assertion and the settings write both run
    // either way, but production additionally demands an https public URL.
    MEDIKUBE_ENV: 'development',
    MEDIKUBE_DATA_DIR: dataDir,
    MEDIKUBE_HTTP_ADDR: address,
    MEDIKUBE_PUBLIC_URL: publicURL,
    // Open, because the sign-up form is what the gate asserts FR-004's
    // published rules against. /register is served either way — a closed
    // instance renders the explanation in the same landmark rather than
    // disappearing (research D-15) — and that branch is covered in
    // internal/web/page/accounts_test.go, where it costs no second instance.
    MEDIKUBE_AUTH_REGISTRATION_OPEN: 'true',
    MEDIKUBE_LOG_LEVEL: 'warn',
  },
});

function clean() {
  sink.close();
  rmSync(instanceDir, { recursive: true, force: true });
}

server.on('exit', (code, signal) => {
  clean();
  process.exit(code ?? (signal ? 1 : 0));
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => server.kill(signal));
}

// A route that answers 200 without a session and without touching a record:
// the same one the gate used to wait on, asked here so the settings write below
// happens the moment the router is up.
async function waitForRouter(deadline) {
  for (;;) {
    try {
      const response = await fetch(`${publicURL}/api/collections/users/auth-methods`);
      if (response.ok) {
        return;
      }
    } catch {
      // Not listening yet. The deadline below is the only thing that gives up.
    }

    if (Date.now() > deadline) {
      refuse(`the instance did not answer on ${address} — see the server output above`);
    }

    await new Promise((wake) => setTimeout(wake, 100));
  }
}

// Outgoing mail, written through the admin API and not through a migration:
// PocketBase's settings store is the platform's, MediKube writes it at boot
// from the environment and has no knob for SMTP (ApplySettings leaves it
// alone), so this is a property of the GATE's instance rather than of the
// application.
//
// Meta.AppURL comes with it. It is what {APP_URL} in the message resolves to,
// and PocketBase's default is localhost:8090 — a link to an instance nobody is
// running.
async function configureMail() {
  const credentials = {
    identity: goString('SuperuserEmail'),
    password: goString('Password'),
  };

  const authenticated = await fetch(`${publicURL}/api/collections/_superusers/auth-with-password`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(credentials),
  });

  if (!authenticated.ok) {
    refuse(`the seeded superuser could not sign in (${authenticated.status}); is the fixture seeded?`);
  }

  const { token } = await authenticated.json();
  const smtp = splitAddress(smtpAddress);

  const written = await fetch(`${publicURL}/api/settings`, {
    method: 'PATCH',
    headers: { 'content-type': 'application/json', authorization: token },
    body: JSON.stringify({
      smtp: { enabled: true, host: smtp.host, port: smtp.port, tls: false, username: '', password: '' },
      meta: { appUrl: publicURL },
    }),
  });

  if (!written.ok) {
    refuse(`the instance would not point its mail at the sink (${written.status}): ${await written.text()}`);
  }
}

await waitForRouter(Date.now() + 45_000);
await configureMail();
await sink.publish();
