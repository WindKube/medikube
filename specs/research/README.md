# specs/research/

The nine reference dossiers behind the MediKube specification suite. Each was produced by reading
real source — downloaded module code, the MediKeep original, the sibling projects in this
monorepo — rather than documentation, and each records the evidence for decisions the phase plans
then cite by name.

**These are reference, not contract.** Nothing here binds an implementer on its own. The binding
documents sit one level up.

## Precedence

When two documents disagree, the higher one wins:

1. [`../../.specify/memory/constitution.md`](../../.specify/memory/constitution.md) — v1.3.0
2. [`../VERIFIED-SOURCE-FACTS.md`](../VERIFIED-SOURCE-FACTS.md) — **overrides everything below it,
   including this directory.** Facts established by building and running real module source. Where
   a dossier and this file disagree, the dossier is stale.
3. [`../SHARED-DESIGN.md`](../SHARED-DESIGN.md) — the cross-phase design contract: the domain
   model, the route family, the conventions, the package layout. Authoritative on **design**; the
   six phase charters are authoritative on **allocation**.
4. The phase charters — `../00N-*/spec.md`, then `plan.md`
5. [`../DESIGN.md`](../DESIGN.md) — authoritative on visual questions only
6. This directory

## The dossiers

| File | What it covers |
|---|---|
| `RECONCILIATION.md` | Contradictions found **between** the other dossiers, and how each was settled. Read this before trusting any single dossier — several of its C-numbered findings overturn advice given elsewhere in this directory. |
| `HOUSE-PATTERNS.md` | How this monorepo builds Go services: `arc-ui` as MediKube's template project, the Dockerfile and Taskfile shape, the `.dockerignore` allowlist, `depguard` and `forbidigo` configuration. |
| `pocketbase.md` | PocketBase v0.40.1 as an embedded framework — hooks, routing, migrations, the file API, the test harness, and which unexported internals the suite depends on. |
| `frontend.md` | templ, Datastar v1, Tailwind v4, and the CSP consequences of each. |
| `observability.md` | zerolog, Sentry, Prometheus, OpenTelemetry, and the logger bridge into PocketBase. |
| `conventions.md` | Naming, errors, DTOs, pagination, and the API envelope. |
| `testing.md` | testify, the PocketBase test app, Playwright, and the gate design. |
| `domain-clinical.md` | The clinical record types as MediKeep modelled them, and what a rewrite should keep, change or drop. |
| `domain-platform.md` | Accounts, patients, sharing, invitations, reporting, export and the operator surface, same treatment. |

## A warning about staleness

These were written before the suite was planned, and the suite has since diverged from them by
recorded decision in several places — three cross-artifact analysis rounds moved things. Two
known examples, both settled against the dossier:

- `domain-platform.md` and `frontend.md` predate the Datastar v1 event-type reduction and the
  removal of PocketBase realtime from the design.
- `pocketbase.md` predates the Go 1.27 requirement, which `VERIFIED-SOURCE-FACTS.md` FACT 0
  establishes by compiling.

A dossier is evidence of what was true when it was read. `VERIFIED-SOURCE-FACTS.md` is evidence of
what is true now. Prefer the latter, always.

## Provenance

Moved into the repository on 2026-08-27 from a session scratchpad. They had been cited as binding
from a temporary path, which meant the suite's authoritative contract would have vanished with a
`/tmp` cleanup.
