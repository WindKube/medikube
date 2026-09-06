# PocketBase upgrade checklist

MediKube embeds PocketBase rather than wrapping it, and in four places it reaches past a
public API because no public API exists. Each one is a place a PocketBase upgrade can break
MediKube **silently** — which is why every entry below records the symptom rather than only the
mechanism. Work this list before bumping the pinned version, and read the symptom column first:
it is what you will actually see.

Section 5 is a different kind of entry and is here for the same reason: three PocketBase
*behaviours* that MediKube's account layer depends on rather than reimplements. Nothing reaches
past an API for them, and they would still break silently.

Localisation (phase 007) adds nothing here: `e.Auth.GetString("locale")` and parsing
`Accept-Language` (`internal/web/localize.go`) are both public API, same shape as the existing
`themeField` read — nothing reaches past PocketBase for either.

Pinned at **PocketBase v0.40.1** (`go.mod`). Risk R8, cross-artifact CT-1.

Before touching the pinned version, also re-run
`internal/records/registry_completeness_test.go` and the per-collection lockdown scenarios in
`internal/platform/pb/lockdown*_test.go` — neither reaches past a public API the way the entries
below do, but both assert PocketBase-observable behaviour (every kind's API rules, every
collection's five rules being nil) that an upgrade can quietly change underneath MediKube without
touching a single line here.

## 1. The `pb.App` decorator — the log bridge, request path

**What.** `internal/logging/pbbridge.go` decorates the exported embedded `core.App` field on
`pocketbase.PocketBase` so `Logger()` resolves through MediKube's zerolog handler.

**Why there is no public API.** `core.BaseApp.initLogger` hardcodes its slog handler, so the
handler cannot be injected. Decorating the embedded field is the only seam. It works because
PocketBase does `event.App = app`, so `Logger()` resolves through the decorator dynamically at
call time (research D-29).

**Symptom if an upgrade breaks it.** Application logs keep flowing and look correct, but
PocketBase's own lines stop appearing in the zerolog stream — or start appearing twice, in
PocketBase's own format. Nothing errors. The bridge test in `internal/logging` is what fails.

**Check on upgrade.** That `core.App` is still an exported embedded field on
`pocketbase.PocketBase`, and that `event.App` is still assigned per event rather than captured
once at construction.

## 2. The `_logs` model hook — the log bridge, transaction path

**What.** `internal/logging/pblogs.go` binds `OnModelCreate("_logs")` and emits the record into
zerolog. It deliberately does **not** call `e.Next()`.

**Why there is no public API.** The decorator in (1) cannot see transaction-scoped logging:
`createTxApp` shallow-copies a `*BaseApp`, keeping the hardcoded internal logger, so every line
logged inside `RunInTransaction` bypasses the decorator entirely (research D-29). Intercepting
the model write is the second half of the bridge, and both halves are required.

`Logs.MaxDays` must be **1**, never 0. At 0 PocketBase disables its log writes altogether and
its own failures go nowhere at all — the setting reads like a retention knob and behaves like an
off switch (constitution Principle VI).

**Symptom if an upgrade breaks it.** Log lines written inside a transaction vanish. Everything
outside a transaction still logs, so the gap is invisible until you go looking for the record of
a write that failed. If `_logs` is renamed, the hook silently never fires.

**Check on upgrade.** That the internal collection is still called `_logs`, that
`OnModelCreate` still fires for it, and that `Logs.MaxDays = 1` still means retain-one-day.

## 3. The `WriteTimeout` override — SSE survival

**What.** An `OnServe` hook adjusts `se.Server.WriteTimeout` before the listener starts, and
every SSE handler goes through the mandatory `internal/web/stream.newStream()` helper, which
clears the per-connection deadline with
`http.NewResponseController(e.Response).SetWriteDeadline(time.Time{})`.

**Why there is no public API.** `apis/serve.go` constructs the server as a struct literal with
`WriteTimeout: 5 * time.Minute` and no configuration field. `datastar.NewSSE` sets
`Cache-Control`, `Content-Type` and `Connection` and flushes — it never touches the write
deadline (research D-34).

**Symptom if an upgrade breaks it.** Every long-lived stream dies at **exactly five minutes**
with a write error, and the browser reconnect-loops. It passes every test shorter than five
minutes, which is what makes it dangerous. SC-007 requires a view left open for sixty continuous
minutes to still be receiving updates, so the CI job that holds a stream open for more than five
minutes is the only thing standing between this and production.

**Check on upgrade.** That `tools/router.ResponseWriter` still implements
`Unwrap() http.ResponseWriter` — `SetWriteDeadline`, `Flush` and `Hijack` all pass through that
one method — and that the server is still built somewhere a hook can reach before `ListenAndServe`.

## 4. The copied `DefaultDBConnect` pragma string — otelsql

**What.** otelsql attaches through `pocketbase.Config.DBConnect`, which means MediKube supplies
the connection function and therefore carries its own copy of PocketBase's pragma string
(research D-30).

**Why there is no public API.** Overriding `DBConnect` replaces PocketBase's default wholesale;
there is no way to wrap it and keep the pragmas.

**Symptom if an upgrade breaks it.** Nothing fails. The database opens, the application runs,
and a pragma PocketBase started relying on — journal mode, busy timeout, foreign keys — is
quietly not set. It surfaces much later as lock contention or as a constraint that does not fire.

**Check on upgrade.** The copy is `pocketbasePragmas` in `internal/obs/db.go`, and
`internal/obs/db_test.go` checks it two ways, deliberately: one case reads the literal out of
PocketBase's own `core/db_connect.go` in the module cache and compares it byte for byte, and one
opens both connections and compares every pragma SQLite reports plus the `_defensive` flag's
observable effect. The first catches a reordering and `_defensive`, which no pragma listing
reports; the second catches a pragma PocketBase has *added*, which a check driven by the copy's
own contents cannot see. If either fails, PocketBase moved — work this entry before changing the
copy.

Note that MediKube only uses the copy when tracing is configured: with no OTLP endpoint,
`InstrumentedDBConnect` returns nil and PocketBase opens the database through its own function.
An untraced deployment cannot be hurt by this drifting.

## 4a. `deleteRefRecords`'s unset semantics — the `Required`/`CascadeDelete` matrix

**What.** `internal/store/migrations`' relation fields (`patients.owner`, `medications.patient`,
`users.active_patient`, `patients.primary_practitioner`, `medications.practitioner`,
`medications.pharmacy`, `practitioners.facility`, `practitioners.owner`, `facilities.owner`,
`audit_events.patient`) each set `Required`/`CascadeDelete` to an exact, tested value, and none of
MediKube's own code clears a reference on delete.

**Why there is no public API.** `core/record_model.go`'s `deleteRefRecords` is unexported and does
three different things depending on the two booleans: non-cascade + non-required unsets the id and
re-saves the referencing record via `SaveNoValidate` (which still fires model hooks — clearing a
practitioner off 40 medications writes 40 update audit rows, correctly); non-cascade + required
with no other ids left fails the delete outright; cascade deletes the referencing record. There is
no way to ask PocketBase for this behaviour — it is a side effect of the two struct fields, read
from source rather than documented (research D-06, D-13's `assertions.go`).

**Symptom if an upgrade breaks it.** Silent and structural, not a crash: if a boolean's effective
behaviour changes, a delete either cascades where FR-040 requires the reference merely cleared
(losing data an upgrade should not lose), or a required-but-not-cascading relation starts failing
deletes that used to succeed. `internal/store/migrations/assertions_test.go`'s
`TestDeletingAnAccountDeletesItsMedicationsAndOutlivesItsAuditTrail` and its per-relation table are
what catch it.

**Check on upgrade.** Run `go test ./internal/store/migrations/...` and read
`assertions.go`'s cascade/required table against the matrix in research.md's D-06 before believing
the bump; a change to `deleteRefRecords`'s three branches is the one PocketBase upgrade this
checklist cannot pre-empt with a source-diff, since nothing here calls the unexported function
directly.

## 4b. The `thumbs_<filename>/` key layout — patient photo thumbnails

**What.** `internal/store/patient/photo.go` generates thumbnails eagerly, on upload, at
`<collectionId>/<recordId>/thumbs_<filename>/<size>_<filename>` — PocketBase's own naming for a
thumb it would otherwise create lazily on first request through `/api/files/`, which MediKube
never calls (constitution VII: no file-token URL).

**Why there is no public API.** Bypassing PB's file route to keep photos protected (research D-16)
means PB's lazy thumbnailer (`apis/file.go`) never runs, so MediKube's own upload path has to
reproduce the key PB would have used. The layout is read out of `apis/file.go` and
`core/field_file.go`'s `fsys.DeletePrefix(record.BaseFilesPath() + "/thumbs_" + filename + "/")` —
that prefix-delete is what makes replacing a photo actually remove the old thumbnails, and only
holds if MediKube's key matches it exactly.

**Symptom if an upgrade breaks it.** Nothing errors. If PocketBase's own thumb key format changes,
MediKube's hand-generated thumbs simply stop matching what a future PB-owned cleanup path expects,
and replacing a photo leaves the previous thumbnails on disk forever — a slow leak, not a failure.

**Check on upgrade.** Re-read `apis/file.go`'s thumb-key construction and `field_file.go`'s
`DeletePrefix` call against `internal/store/patient/photo.go`'s own key-building; a rename of
either the `thumbs_` prefix or the `<size>_<filename>` suffix is the whole of what to look for.

## 4c. The single-transaction migration runner — the medication re-attribution

**What.** `internal/store/migrations/1756200600_medications_repoint.go` re-attributes every
medication from `owner` to `patient` with one raw `UPDATE ... SET patient = (SELECT ...)`
statement, asserts zero rows are left unattributed, and only then flips the field to
`Required: true, CascadeDelete: true` — relying on `core/migrations_runner.go` wrapping every
pending migration in one `AuxRunInTransaction(RunInTransaction(...))` so a failed assertion rolls
back this migration **and every other one in the batch**, leaving no partially migrated state
(research D-13, CT-1).

**Why there is no public API.** There is no supported way to run a bulk `UPDATE` through
PocketBase's record API without loading and re-saving every row — which would fire
`OnRecordAfterUpdateSuccess` once per medication and write a spurious audit row for every one.
The safety this migration relies on (all-or-nothing across the whole batch) is a documented
implementation detail of PocketBase's migration runner, not a contract.

**Symptom if an upgrade breaks it.** If a future PocketBase version runs each migration in its own
transaction instead of one shared one, a failure in this migration's assertion step would leave
steps 1–3 committed and only this migration rolled back — a database with `medications.patient`
declared `Required` on rows that were never actually backfilled, which fails loudly on the very
next write rather than at migration time.

**Check on upgrade.** Read `core/migrations_runner.go`'s transaction wrapping before assuming it is
unchanged, and run `go test ./internal/store/migrations/...` — the repoint migration's own test
seeds unattributed medications and asserts the whole batch rolls back together.

## 4d. `RelationField.MaxSelect <= 1` reads as single-valued, not "unlimited"

**What.** Every multi-valued relation in this codebase — every kind's `tags` field, chief among
them — sets `MaxSelect` to a named constant (`unlimitedTags`) rather than `0`.

**Why there is no public API.** `RelationField.IsMultiple()` treats `MaxSelect <= 1` as a
single-value relation. `0` does not mean unlimited; it means "at most one", the same as `1`. There
is no PocketBase constant for "no cap" — the ceiling has to be a real number, chosen large enough
that no requirement caps it in practice (`internal/store/migrations/1756300000_tags.go`).

**Symptom if an upgrade breaks it.** If a future PocketBase version changes `IsMultiple`'s
threshold, or starts treating `0` as unlimited after all, a `tags` field silently becomes
single-valued (or vice versa): a record loses every tag but one on its next save, and no error
surfaces anywhere near the write.

**Check on upgrade.** `core.RelationField.IsMultiple` still treats `<= 1` as single-valued, and
`unlimitedTags`'s value is still comfortably above anything a record could carry. Every carrier's
own test (`medication_tags_test.go`, `family_member_tags_test.go`, `symptom_vitals_tags_test.go`,
`search_index_test.go`, and their siblings) asserts `MaxSelect == unlimitedTags` with this
reasoning in the assertion message.

## 5. Three account behaviours MediKube depends on rather than reimplements

**What.** `internal/store/identity` deliberately does not carry its own copy of any of these.

1. **`tokenKey` rotates by itself on a password change.** `core/record_model.go`'s
   `onRecordSaveExecute` re-randomises the key when the saved record's password changed and its
   key did not. `Authenticator.SetPassword` therefore calls `Record.SetPassword` and `Save` and
   performs **no rotation of its own** — one write cannot half-happen, and a second rotation on
   MediKube's side would make a PocketBase regression invisible by passing on MediKube's own
   line. A sign-out has no password change, so `EndSessions` calls `RefreshTokenKey` explicitly;
   that half is MediKube's.

2. **`PasswordField.Cost` is what a real credential costs.** It is `0` on `users`, which means
   `bcrypt.DefaultCost` — 10. `identity.DummyPasswordHash` is a fixed cost-10 hash, and the
   anti-enumeration equalisation of research D-17 only holds while the two agree.

3. **`core.PasswordFieldValue.Validate` is the comparison.** MediKube holds the value directly
   rather than through a record, because `Record.SetRaw("password", "<a hash>")` followed by
   `Record.ValidatePassword` compares **nothing**: `SetRaw` stores a plain string,
   `ValidatePassword` type-asserts for `*PasswordFieldValue`, and the assertion fails silently to
   `false` with no bcrypt run at all (measured against v0.40.1, and the opposite of what the
   `PasswordField` doc comment implies).

**Symptom if an upgrade breaks it.** (1) A password change stops ending other sessions: every
stolen token stays live for the rest of its seven days, and every ordinary test still passes
because the new password works. (2) The unknown-address sign-in path becomes cheaper than the
wrong-password one and the account-existence oracle is back behind two byte-identical `401`
bodies. (3) The dummy comparison stops running bcrypt at all, with the same result and no error
anywhere.

**Check on upgrade.** All three have tests, and they are the check:
`internal/web/stream/session_rotation_test.go` drives the real adapter into the real production
`Session` for (1), and `internal/store/identity`'s `TestTheDummyHashCostsWhatEveryRealHashCosts`
and `auth_internal_test.go` cover (2) and (3). Run
`go test ./internal/store/identity/... ./internal/web/stream/...` before believing the bump.

**Also worth knowing, because it is a trap rather than a dependency.**
`app.FindAuthRecordByEmail` is **case-sensitive** here. It searches case-insensitively only when
the single-column unique index on `email` carries `COLLATE NOCASE`, and PocketBase's stock index
does not; MediKube's `idx_users_email_lower` is on `LOWER(email)`, which `dbutils` does not
recognise as an index on `email` at all. Measured: it answers "no rows" for `AMARA@…` against an
account stored as `amara@…`. Every address lookup goes through `store.SameAddress` instead
(FR-003).
