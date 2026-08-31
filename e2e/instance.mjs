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
import { cpSync, existsSync, mkdtempSync, rmSync } from 'node:fs';
import { spawn } from 'node:child_process';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '..');

const binary = resolve(root, 'medikube');
const fixture = resolve(root, 'internal/testdata/pb_data');
// Outside the working tree, so a smoke run leaves nothing behind for the next
// `git status` or the next `go test` to trip over.
const instanceDir = mkdtempSync(join(tmpdir(), 'medikube-e2e-'));
const dataDir = join(instanceDir, 'pb_data');

const address = process.env.MEDIKUBE_E2E_ADDR ?? '127.0.0.1:8091';

function refuse(message) {
  process.stderr.write(`e2e: ${message}\n`);
  process.exit(1);
}

if (!existsSync(binary)) {
  refuse(`no binary at ${binary} — run \`task build\` first (\`task test:e2e\` does)`);
}

if (!existsSync(fixture)) {
  refuse(`no fixture at ${fixture} — it is committed, so this is a broken checkout`);
}

cpSync(fixture, dataDir, { recursive: true });

const server = spawn(binary, ['serve'], {
  stdio: 'inherit',
  env: {
    ...process.env,
    // Not production: the lockdown assertion and the settings write both run
    // either way, but production additionally demands an https public URL.
    MEDIKUBE_ENV: 'development',
    MEDIKUBE_DATA_DIR: dataDir,
    MEDIKUBE_HTTP_ADDR: address,
    MEDIKUBE_PUBLIC_URL: `http://${address}`,
    // Open, because /register is one of the pages the gate visits and a closed
    // instance renders 404 there by design (contracts/pages.md).
    MEDIKUBE_AUTH_REGISTRATION_OPEN: 'true',
    MEDIKUBE_LOG_LEVEL: 'warn',
  },
});

function clean() {
  rmSync(instanceDir, { recursive: true, force: true });
}

server.on('exit', (code, signal) => {
  clean();
  process.exit(code ?? (signal ? 1 : 0));
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => server.kill(signal));
}
