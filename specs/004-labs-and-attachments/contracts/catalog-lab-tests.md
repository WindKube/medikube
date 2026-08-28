# Contract: the standardized test catalogue

**Operations added: 1.** Shared design contract §2.3 entry 44, moved here from phase 002, which
never built the catalogues.

```
GET /api/v1/catalog/lab-tests   listCatalogLabTests
```

**There is no create, update or delete operation, and that is the enforcement of FR-037.** A gate
test asserts the route registry contains no method other than `GET` under `/api/v1/catalog`. All
five PocketBase API rules on `catalog_lab_tests` are `nil`, so the auto-CRUD subtree is
superuser-only as well.

---

## 1. `GET /api/v1/catalog/lab-tests`

| Parameter | Notes |
|---|---|
| `q` | case-insensitive substring over the standard **name**, the **alternative names** (`synonyms`) and the **standard code** (FR-038, FR-039) |
| `category` | comma list from `LabCategory` (FR-038) |
| `loinc` | exact match on the standard code |
| `common` | `true` restricts to `is_common` entries (FR-038) |
| `limit`, `cursor`, `count` | standard; `limit` default 25, max 100 |

**No `patient` parameter exists.** This is the one resource in MediKube whose list is not
patient-scoped, because it contains nothing about anyone (FR-043).

`200`:

```json
{ "loaded": true,
  "items": [
    { "id": "cat0000000000002",
      "loinc_code": "2160-0",
      "name": "Creatinine [Mass/volume] in Serum or Plasma",
      "short_name": "Creatinine",
      "default_unit": "umol/L",
      "category": "blood_work",
      "synonyms": ["Creatinine", "CREA", "Serum creatinine"],
      "is_common": true,
      "ref_low": 60.0,
      "ref_high": 110.0 }
  ],
  "next_cursor": null }
```

### 1.1 `loaded` — a state, not a count

`loaded` is `false` **only** when the collection holds zero rows, that is when the instance has
never had the catalogue extract applied. It is `true` whenever the catalogue exists, including when
a particular query matches nothing.

| Situation | Response | What the UI says |
|---|---|---|
| catalogue present, query matches entries | `loaded: true`, non-empty `items` | the suggestions |
| catalogue present, query matches nothing | `loaded: true`, `items: []` | "nothing matched that" (FR-039, US4 scenario 2) |
| catalogue never loaded on this instance | `loaded: false`, `items: []` | "the standard test catalogue has not been loaded on this instance"; manual entry continues to work (edge case: environment failures) |

Those two messages are **different strings**, and a templ render test asserts they are.

### 1.2 Authorization

Any authenticated account holder. Unauthenticated is `401`. There is **no** patient dimension, no
owner filter and no way to name another account; reading the catalogue discloses nothing about
anyone (FR-043), and a test asserts that two different accounts receive byte-identical responses
for the same query.

A superuser sees the same rows. No audit row is written for a catalogue read: it is not patient
data, and writing one per keystroke-batch would flood the trail with entries that name nobody.

### 1.3 Performance

SC-014 allows **1 s** from the third typed character. The extract is ~2,000 rows; `q` is served by
`idx_catalog_lab_tests_name` for the name and a scan of the small `synonyms` array. The three-
character minimum (FR-039) is enforced in the domain, not in the query: a `q` shorter than three
characters returns `400 bad_request`, code `query_too_short`, so the UI cannot accidentally ask for
the whole catalogue on every keystroke.

---

## 2. Choosing an entry

Choosing a catalogue entry is not an operation. The client copies the entry's `name`,
`default_unit`, `category`, `ref_low` and `ref_high` into the component form and sends the entry's
`id` as `catalog_test` in the lab result payload (FR-040, FR-041).

Two contractual consequences:

- **Every one of those four values remains editable before saving** (FR-040, US4 scenario 3). They
  are ordinary fields on `ComponentInput`; the catalogue supplies defaults, not constraints.
- **`catalog_test` is what makes two spellings one series** (FR-041, US4 scenario 4). It is
  recorded on the component and is the first half of the trend grouping key
  (`lab-components.md` §1).

A component may carry `catalog_test: null` and any `test_name` at all; it saves without complaint
and trends in its own right (FR-042, US4 scenario 6).

---

## 3. Loading and amending the catalogue

The extract lives at `assets/catalog/lab-tests.json`, is embedded with `embed.FS`, and is applied
by a **migration** whose `up` performs an idempotent upsert keyed on `loinc_code`
(data-model §7, research D-11). Running it twice produces the same row count, which its migration
test asserts.

Amending the catalogue means shipping a new extract and a new migration. There is no
administrative UI for it, no import endpoint and no `medikube` subcommand — the catalogue is
reference data that ships with the instance (FR-036, FR-037).
