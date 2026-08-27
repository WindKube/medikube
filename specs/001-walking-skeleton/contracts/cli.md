# Contract: the operator command surface

Requirements covered: FR-041, FR-051, FR-053, FR-058, FR-059, FR-060, FR-064, FR-065, SC-008,
SC-013.

One binary, no sidecar scripts, no Node in the runtime image.

PocketBase's `RootCmd` is a **real `*cobra.Command`** (`pocketbase.go:29`,
`Command: &cobra.Command{...}`) and `app.RootCmd.AddCommand(cmd)` works exactly as in any Cobra
program. Two traps, both real:

1. **`pocketbase.NewWithConfig` with `DefaultDataDir` unset produces `pb_data` next to the
   binary** — in a distroless image that is a read-only layer. `MEDIGO_DATA_DIR` is required and
   validated at boot.
2. **PocketBase parses `--dir`, `--encryptionEnv` and `--dev` from `os.Args` in `NewWithConfig`,
   before Cobra runs.** An unrecognised MediGo flag placed before the subcommand is consumed by
   that pre-parse and vanishes. Every MediGo flag is defined on its own subcommand, never
   globally.

---

## `medigo serve`

PocketBase's own, unchanged, plus MediGo's `OnServe` bindings. `--http` and `--https` behave as
documented upstream.

**At boot, in order**, and the process **exits non-zero** rather than serving if any step fails:

1. config parsed and validated from the environment (`caarlos0/env`) — FR-051;
2. migrations applied (`Automigrate` is off in production; `serve` runs them explicitly);
3. **the lockdown assertion** — every non-system collection has five nil API rules, `Batch` is
   disabled, and the `-1019` guard middleware is bound. Refuses to start otherwise (FR-035, and
   the reason this is an assertion rather than a hope);
4. **the protected-files assertion** — every `FileField` in every collection has
   `Protected: true`. Refuses to start otherwise;
5. PocketBase settings written from MediGo's validated config: rate limits, token TTLs,
   `Logs.MaxDays = 1`;
6. the admin-UI hardening warning, if applicable (below).

**A single startup line**, at info, naming the version, the data directory, the listen address and
the applied migration count. Nothing else at info before the first request (FR-053).

**The admin-UI warning is loud, at `warn`, and fires when any of four conditions holds**: the
superuser IP allowlist is empty, superuser MFA is off, `MFA.Rule` is non-empty (a *partial*
rollout, which reads as "on" and is not), or the admin UI is reachable on a public interface. The
admin UI ships enabled in production and hardened; the warning is what makes "hardened" checkable
at 3am (research D-32).

`SuperuserIPs` lives on `Settings()`; `MFA.Enabled` lives on the **superusers collection**, not on
settings — two different places, and a check that looks in one finds nothing (VERIFIED FACT 10).
MFA also requires **at least two** enabled auth methods on that collection or it silently does
nothing.

---

## `medigo migrate [up|down|history-sync|collections]`

PocketBase's migrate command with MediGo's migrations registered. Every migration in this phase is
**reversible by construction** — `migrations.Register(up, down, filename)` takes both halves and
the collection-snapshot form generates the down automatically (FACT 8).

`migrate down` is refused when `MEDIGO_ENV=production` unless `--force` is passed, because the
down of migration 3 drops `audit_events` (FR-059).

---

## `medigo seed`

Creates the demo accounts and their medications described in
[data-model.md](../data-model.md) §6. **Idempotent** — running it twice produces the same state
and does not duplicate rows.

**Refuses to run when `MEDIGO_ENV=production`**, no `--force` escape (FR-060). Seed data in a
medical production database is indistinguishable from real data once it is in there.

Prints one line per account created or skipped. Exits non-zero if the database is not migrated.

---

## `medigo routes [--json]`

Prints the route inventory from the one declarative registry. **This is the only place the route
table exists** — PocketBase's router is a private field with no introspection API, so the OpenAPI
document, the Playwright smoke list and this command all read the same `Registry.Routes()` and
cannot drift (FACT/C15, FR-065).

Default output is a human table: method, path, auth, summary, landmark. `--json` is the machine
form the smoke run consumes.

**The route-inventory gate** is a Go test, not this command: it asserts every registered route
appears in the generated OpenAPI document and every documented operation is registered, and fails
on either asymmetry.

---

## `medigo openapi [--out FILE]`

Writes the OpenAPI 3.1 document generated from the registry to stdout or `--out`.

**The generated document is committed**, and CI regenerates it and fails on any diff (FR-064,
Principle IX). A generator whose output nobody reviews is a generator that can start emitting
nonsense unnoticed.

The generator **marshals then re-loads** through `openapi3.NewLoader()` before writing, because
that is the only way to prove the document is valid as JSON — `Discriminator.Mapping` is
`map[string]openapi3.MappingRef`, a struct, and an in-memory `Validate()` alone will accept
structures that do not round-trip (FACT 9, kin-openapi v0.144.0).

---

## `medigo healthcheck [--addr]`

Dials `http://127.0.0.1:{port}/api/v1/readyz`, exits `0` on `200`, non-zero otherwise, prints
nothing on success. This exists because **the distroless runtime image has no shell, no `curl` and
no `wget`**, so a Dockerfile `HEALTHCHECK` cannot be expressed any other way (FR-058). See
[health.md](./health.md).

The image itself declares **no** `HEALTHCHECK` — house pattern; orchestrators do their own
probing — but the command is what those probes invoke.

---

## Removed

**`app.RootCmd.RemoveCommand` is called for PocketBase's `superuser` sub-commands that are not
needed and for anything that would let an operator bypass the lockdown.** Specifically, the
built-in destructive helpers are not exposed; superuser creation stays, because the first boot
needs it.

---

## Output contract, for all commands

- **Human output goes to stdout; diagnostics go to the zerolog stream** — one stream, structured,
  Principle VI. A command never writes its own ad-hoc log format.
- **Every command exits non-zero on failure**, with the reason on stderr in one line and the
  detail in the log stream.
- **No command prints a secret**, including in an error path. The config validator's failure
  message names the offending variable and never its value (FR-041).
- **SC-008**: a person with the repository and Docker reaches a running instance with seeded data
  in under ten minutes using only `task` targets documented in
  [../quickstart.md](../quickstart.md) — which is why `seed` is a first-class command and not a
  SQL file somebody is expected to find.
