# Quickstart: Localisation (phase 007)

How to run this phase's work locally and verify it by hand. Everything is a `task`.

---

## 0. Generate, build, run

```bash
task gen     # go tool templ generate + tailwind -> internal/web/static/app.css
task build
task run
```

`task gen` must run before anything else in this phase touches a `.templ` file: every literal
this phase moves into the catalogue lives inside a `.templ` source, and `templ generate` is what
turns `i18n.T(ctx, "...")` calls in those sources into the `*_templ.go` the binary actually
builds.

---

## 1. Run the catalogue gates

```bash
task test:i18n
```

Three build-time tests, all ordinary `go test` under `internal/i18n` (research D-08,
`data-model.md` §4):

| Test | Fails when |
|---|---|
| `catalogue_test.go` (invariant a) | a shipped language is missing an id `active.en.toml` has — names the id and the language |
| `catalogue_test.go` (invariant b) | a shipped language has an id `active.en.toml` lacks — names the id and the language |
| `reference_test.go` (invariant c) | some `internal/web/**` source asks for an id no language defines — names the id and the file:line asking for it |

`task check` runs `test:i18n` as part of the ordinary gate sequence — there is nothing new to
wire into CI, the task exists so a failure here is named rather than surfacing as a generic
`go test ./...` failure buried in the middle of the suite.

---

## 2. Verify by hand — switching language in settings

1. Sign in, open **Settings**.
2. Choose **Polski** from the language control, save.
3. The settings screen itself re-renders in Polish on that same response (FR-002) — confirm no
   full-page flash of English text before the Polish appears.
4. Open the browser's page-inspector and confirm `<html lang="pl">` (FR-007).
5. Visit the pages the browser gate (`e2e/fixtures.ts`) lists; confirm no English
   application-owned text remains anywhere on a representative page of every screen family
   (spec SC-001).
6. Open a record whose fields were typed in English; confirm the field *values* are unchanged
   while the labels around them are Polish (spec US1 scenario 3).
7. Sign out, sign back in; confirm Polish is still in force (spec US1 scenario 7). Switch back to
   English and confirm the reverse.

---

## 3. Verify by hand — pre-sign-in language (curl)

The sign-in page's route is `loginPage`, `GET /login` (`internal/httproute/routes.go:512-517`);
sign-up is `registerPage`, `GET /register` (`routes.go:519-524`). Neither requires a session, so
`Accept-Language` alone decides the language (FR-003, research D-04):

```bash
curl -s -H 'Accept-Language: pl' http://localhost:8090/login | grep -o '<html[^>]*lang="[^"]*"'
# -> <html lang="pl">

curl -s -H 'Accept-Language: fr' http://localhost:8090/login | grep -o '<html[^>]*lang="[^"]*"'
# -> <html lang="en">   (fr is not shipped; falls back to English, spec US2 scenario 2)
```

## 4. Verify by hand — changing the account's language over the API

`PATCH /api/v1/me` is `OpID: updateMe` (`routes.go:254,260-264`):

```bash
curl -s -X PATCH http://localhost:8090/api/v1/me \
  -H 'Content-Type: application/json' \
  -H "Cookie: $SESSION_COOKIE" \
  -d '{"locale":"pl"}'
```

Confirm the response's own body already reflects the new locale, and a subsequent
`GET /api/v1/me` shows `"locale":"pl"` unconditionally of `Accept-Language`. A locale not shipped
(`{"locale":"xx"}`) refuses with the same `422 validation_failed` envelope
(`contracts/records-clinical.md`'s shape) naming `locale`, `error.invalid_value`-derived message —
the domain's shape check (`^[a-z]{2}(-[A-Za-z]{2})?$`) would accept `xx`, but the identity
service's membership check (research D-10) does not.

---

## 5. Add a language in five minutes

1. Copy the reference file: `cp internal/i18n/locales/active.en.toml
   internal/i18n/locales/active.de.toml`.
2. Translate every value; keep every id, every `description`, and every plural form the German
   CLDR rule uses (`internal/plural/rule_gen.go`'s `de` entry — verify which forms it declares
   before assuming `one`/`other` alone is enough).
3. `task test:i18n` — this either passes (the file is complete and has no surplus) or names
   exactly what is missing or extra.
4. `task run`; German now appears in the settings language control and is matched from a browser
   sending `Accept-Language: de`. **Nothing else was edited** (spec US3 scenario 1, FR-010) —
   confirmed by `git status` showing only the one new file.

To find what is still untranslated partway through step 2, `goi18n merge` diffs a target file
against the English source and writes out what is missing (flags verified against
`goi18n/merge_command.go` in the module cache: `-sourceLanguage` default `en`, `-outdir` default
`.`, `-format` default `toml`):

```bash
go run github.com/nicksnyder/go-i18n/v2/goi18n@v2.6.1 merge -sourceLanguage en \
  internal/i18n/locales/active.en.toml internal/i18n/locales/active.de.toml
```

This writes `active.de.toml` (the ids both files already agree on, merged) and, only if something
is still missing, `translate.de.toml` (the empty ids left to fill in) into the current directory —
`goi18n/marshal.go:writeFile` names the output `<label>.<lang>.<format>`, which is why the
merged-active output lands under the same name this phase's own files already use. Move
`translate.de.toml`'s remaining ids into `active.de.toml` by hand, translate them, delete
`translate.de.toml`, and re-run `task test:i18n`.

---

## 6. Things that will waste your afternoon if nobody tells you

- `task gen`'s templ step must run **after** editing a `.templ` file and **before** `task
  test:i18n` — the reference scan (invariant c) reads `.templ` source text directly, not the
  generated `.go`, so it does not need `gen` to have run first; but if you are checking a page in
  the browser and it still shows the old English literal, you forgot `task gen`, not a catalogue
  problem.
- The reference scan explicitly excludes `*_templ.go`: an `i18n.T(ctx, "...")` call that only
  exists in generated output (never in a committed `.templ`/`.go` source) is not something a
  contributor could have written, so it is deliberately invisible to the scan.
- A phrase file's language tag comes from the **filename**, not from any field inside the file
  (research D-02, `i18n/parse.go:parsePath`) — renaming `active.de.toml` to `de.active.toml`
  silently changes nothing about validity but breaks `parsePath`'s "second to last dot" rule and
  the file will not load under the tag you expect. Keep the `active.<lang>.toml` shape exactly.
