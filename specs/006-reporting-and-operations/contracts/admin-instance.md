# Contract: the operator overview (operations 80–81)

Conventions, status codes, the actor matrix and the audit rules are in [README.md](./README.md).

**Every operation in this file, and in the two that follow, requires `role = admin`.** An account
without it receives `404` — indistinguishable from the route not existing — **and the attempt is
recorded** (FR-076, SC-010). There is no `403` here: a `403` would confirm the route exists.

**No operation in this file can read a person's records.** `internal/service/admin` declares no port
capable of returning one ([D-48](../research.md#d-48), FR-088, US5 AS-15). Reaching a person's
records requires the break-glass credential, whose every session appears in the trail as
`admin_session` (FR-120).

## The figure catalogue

Every figure below is declared once in `internal/service/admin/opsfig` with a **key**, a **label**, a
**definition** and a **unit**, and rendered from that table ([D-47](../research.md#d-47)). Three tests
fail the build: every DTO key exists in the table, every table entry is rendered on `/admin`, and
**every entry has a non-empty definition** (FR-080, SC-011). A fourth, reflection-based, asserts no
field of either DTO is a free-text type outside the enumerated `State` and `Version` cases (FR-086).

Permitted units: `Count`, `Bytes`, `Duration`, `Version`, `State`, `Moment`, `Setting`. **A name, a
date of birth, a diagnosis, a recorded value, a note, a tag name or a file name is not a figure**, and
there is no field on either DTO that could hold one.

---

## 80. `GET /api/v1/admin/stats` — what the instance holds

`operationId: getAdminStats`

Replaces nine untyped upstream admin-dashboard routes, three of which were unauthenticated.

**Response** `200`

```json
{ "computed_at": "2026-08-27T09:00:00Z",
  "accounts":     { "value": 4,   "definition": "…", "unit": "count", "computed_at": "…" },
  "administrators": { "value": 1, "definition": "…", "unit": "count", "computed_at": "…" },
  "disabled_accounts": { "value": 1, "…": "…" },
  "patients":     { "value": 7,   "…": "…" },
  "records":      { "value": 4211,"…": "…" },
  "records_by_kind": [ { "kind": "medication", "label": "Medications", "value": 311, "…": "…" } ],
  "documents":    { "value": 940, "…": "…" },
  "trashed_documents": { "value": 7,  "…": "…" },
  "trashed_bytes":     { "value": 14220110, "unit": "bytes", "…": "…" },
  "shares_active":     { "value": 3,  "…": "…" },
  "invitations_pending": { "value": 1, "…": "…" },
  "jobs_queued":  { "value": 0, "…": "…" },
  "jobs_running": { "value": 1, "…": "…" },
  "jobs_failed_30d": { "value": 2, "…": "…" },
  "database_bytes": { "value": 88129536, "unit": "bytes", "computed_at": "2026-08-27T08:45:00Z",
                      "age_seconds": 900, "refreshed": true },
  "document_bytes": { "value": 2411529216, "unit": "bytes", "computed_at": "…",
                      "age_seconds": 900, "refreshed": true } }
```

**Every figure carries its own `definition` and `computed_at`** (FR-080). Figures split by **cost**
([D-16](../research.md#d-16)):

- **live per request** — the indexed `COUNT`s above, so a figure an operator just changed moves,
  which is US5's own independent test ("take a backup and confirm the last-backup figure moves");
- **refreshed** — `database_bytes` (a stat of `data.db`, `-wal` and `auxiliary.db`) and
  `document_bytes` (a walk of `<DataDir>/storage`), recomputed by the `medikube_storage_refresh` job
  every 15 minutes and at boot, served from memory with `refreshed: true` and an `age_seconds`. A
  directory walk over a 200 GB document store is never on the request path: an operator dashboard
  must not be the thing that takes the instance down.

**On a brand-new instance every figure is `0` with its definition** — never a blank, never an error,
never anything a reader could mistake for a failure to compute it (FR-081, US5 AS-1).

`trashed_documents` and `trashed_bytes` are the whole of what this phase says about deleted documents
(FR-056, [D-14](../research.md#d-14)). They carry the window that applies and **no file name, no
description and no indication of which person any of them concerns**. The page renders them beside a
link to `/documents?deleted=true`, which is a pointer to where recovery already happens — **there is
no second recovery surface** (FR-057).

**Authorization**: `role = admin`, else `404` + `access_denied`.

**Scale**: SC-023 — the whole overview renders within **1 s** at the documented volumes.

**Audit**: none on success; one `access_denied` on refusal.

---

## 81. `GET /api/v1/admin/system` — how the instance is running

`operationId: getAdminSystem`

**Response** `200`

```json
{ "ready": true,
  "uptime_seconds": 918273,
  "version": "v1.4.2",
  "migrations": { "state": "up_to_date", "highest_applied": "1757xxx400_audit_vocab_ops" },
  "backup": { "last_success_at": "2026-08-26T02:00:00Z", "last_success_name": "…",
              "bytes": 88129536, "age_seconds": 111600, "warn_after_seconds": 604800,
              "state": "ok" },
  "posture": { "superuser_mfa": "off", "superuser_ip_allowlist": "unset",
               "smtp": "configured", "oauth2": "unconfigured",
               "warnings": ["superuser_mfa", "superuser_ip_allowlist"] },
  "retention": [ { "key": "export_days", "value": 7, "applies_to": "produced documents and export archives",
                   "job": "medikube_purge_artifacts",
                   "last_run_at": "2026-08-27T03:10:00Z", "last_success_at": "2026-08-27T03:10:00Z" },
                 { "key": "audit_days", "value": 730, "applies_to": "activity entries",
                   "job": "medikube_purge_audit", "…": "…" },
                 { "key": "trash_days", "value": 30, "applies_to": "deleted documents",
                   "job": "medikube_attachment_maintenance", "…": "…" } ],
  "limits": [ { "key": "report_max_records", "value": 5000 },
              { "key": "report_max_charts", "value": 12 },
              { "key": "report_min_chart_points", "value": 3 },
              { "key": "report_max_chart_points", "value": 200 },
              { "key": "export_max_bytes", "value": 10737418240 },
              { "key": "backup_keep", "value": 14 } ],
  "attention": [ { "what": "job_failed", "job": "medikube_purge_artifacts",
                   "at": "2026-08-26T03:10:00Z", "error_code": "storage_full" },
                 { "what": "export_failed", "job_id": "rec…", "at": "…",
                   "error_code": "interrupted" } ] }
```

Field by field, against the requirements:

| Field | Requirement | Source |
|---|---|---|
| `ready` | FR-077 | the same check `/readyz` performs |
| `uptime_seconds`, `version` | FR-077 | process start time; the ldflags-stamped version |
| `migrations` | FR-085 | `_migrations` versus the binary's registered list |
| `backup.state ∈ {ok, stale, never}` | FR-082 | `never` is a **warning**, never a blank or a zero (US5 AS-3); `stale` when `age_seconds > MEDIKUBE_BACKUP_WARN_AFTER`, stating the age (US5 AS-4) |
| `posture.superuser_mfa ∈ {on, off, partial}` | FR-083 | the superusers collection's `MFA.Enabled`; `partial` when `MFA.Rule` is non-empty, because a partial rollout means some superuser can sign in without a second factor ([D-17](../research.md#d-17)) |
| `posture.superuser_ip_allowlist ∈ {set, unset}` | FR-083 | `len(Settings().SuperuserIPs) > 0` |
| `posture.smtp ∈ {configured, unconfigured}` | FR-084 | `Settings().SMTP.Enabled` — phase 001's password recovery and confirmation and phase 005's invitations all refuse rather than pretend without it, and all three warn at boot |
| `posture.oauth2 ∈ {configured, unconfigured}` | FR-136 | any enabled provider in `Settings().OAuth2.Providers`. **Names only if it is ever expanded — never a client id and never a secret.** `unconfigured` is not a warning: an instance signing in with passwords alone is a normal instance ([auth-oauth2.md](./auth-oauth2.md)) |
| `posture.warnings[]` | FR-083, SC-012 | non-empty whenever MFA or the allowlist is missing; the page renders an **unmistakable** warning naming exactly what is missing and what to do about it, and it keeps appearing until it is fixed. The same warning is emitted at **every** boot |
| `retention[]` | FR-055, FR-059 | the configured value, what it applies to, the job that enforces it, and its **last run** and **last success** — two `MAX(occurred_at)` queries over `audit_events` ([D-43](../research.md#d-43)). Changing a window changes this value and the next run's behaviour, with no restart of anything else (FR-059, US8 AS-8) |
| `limits[]` | FR-087 | every configurable limit this phase introduces, so an operator never reads source to learn one |
| `attention[]` | FR-085, FR-058, US8 AS-7 | failed or abandoned work in the last 30 days: `job_failed` entries and `export_jobs` in `failed`. Each appears **exactly once** with what failed and when; the job is retried on its next scheduled run, never in a loop |

**The MFA warning says more than "MFA is off"**: PocketBase refuses to enable MFA unless the auth
collection has at least two auth methods enabled (`validation_mfa_not_enough_auths`), so the message
states that as the next step rather than leaving an operator to discover it
([D-17](../research.md#d-17)).

**Authorization**: `role = admin`, else `404` + `access_denied`.

**Audit**: none on success; one `access_denied` on refusal.
