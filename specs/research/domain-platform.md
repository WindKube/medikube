# MediKeep NON-CLINICAL / PLATFORM domain model

Source: `/Users/krzysztof.wiatrzyk/private/monorepo/medikeep-mcp/internal/spec/openapi.json`
(`info.title = MediKeep`, `info.version = 0.69.0`, 376 paths / 500 operations / 325 schemas).
Everything below is derived from the OpenAPI document only (schemas, field constraints, parameter defaults,
operation descriptions). Where the spec is silent I say so explicitly — MediKeep's FastAPI generator emits
bare `type: string` for most enums, so a lot of the "enums" only exist in prose.

## 0. Endpoint census (platform subsystems only)

| Subsystem | Prefix | Ops |
|---|---|---|
| Auth (incl. SSO) | `/api/v1/auth/*` | 11 |
| Users + preferences | `/api/v1/users/*` | 5 |
| Patients (legacy self-record + photo + per-patient clinical fan-out) | `/api/v1/patients/*` | 21 |
| Patient management (multi-patient) | `/api/v1/patient-management/*` | 10 |
| Patient sharing | `/api/v1/patient-sharing/*` | 10 |
| Family-history sharing | `/api/v1/family-history-sharing/*` | 10 |
| Family members + conditions | `/api/v1/family-members/*` | 11 |
| Invitations | `/api/v1/invitations/*` | 7 |
| Entity files (polymorphic) | `/api/v1/entity-files/*` | 16 |
| Lab-result files (legacy, non-polymorphic) | `/api/v1/lab-result-files/*` | 18 |
| Custom reports | `/api/v1/custom-reports/*` | 9 |
| Export | `/api/v1/export/*` | 4 |
| Admin backups | `/api/v1/admin/backups/*` | 16 |
| Admin dashboard | `/api/v1/admin/dashboard/*` | 9 |
| Admin generic model CRUD | `/api/v1/admin/models/*` | 9 |
| Admin restore | `/api/v1/admin/restore/*` | 4 |
| Admin trash | `/api/v1/admin/trash/*` | 4 |
| Admin activity log | `/api/v1/admin/activity-log/*` | 3 |
| Admin user management | `/api/v1/admin/user-management/*` | 3 |
| Admin bulk ops | `/api/v1/admin/bulk/*` | 2 |
| Admin maintenance (test library) | `/api/v1/admin/maintenance/*` | 3 |
| Notifications | `/api/v1/notifications/*` | 11 |
| Search | `/api/v1/search/` | 1 |
| Tags | `/api/v1/tags/*` | 9 |
| System / frontend-logs / utils | `/api/v1/system,frontend-logs,utils` | 10 |
| **Platform total** | | **~215 of 500** |

Out of scope by locked decision but present upstream: `paperless` (11), `papra` (5), lab OCR parse.

---

## 1. Auth & users

### 1.1 `users` collection (inferred columns)

`User` response schema (`password_hash` explicitly excluded):

| Field | Type | Req | Nullable | Default | Notes |
|---|---|---|---|---|---|
| `id` | int | yes | no | — | serial PK |
| `username` | string | yes | no | — | login identifier (login is username, not email) |
| `email` | string(email) | yes | no | — | |
| `full_name` | string | yes | no | — | single denormalized display name |
| `role` | string | yes | no | — | **no enum in spec**; prose shows `"user"`; admin paths imply `"admin"` |
| `must_change_password` | bool | no | no | `false` | forced-rotation flag, also echoed in the login response |
| `is_active` | bool | no | no | `true` | soft disable |
| `last_login_at` | datetime | no | yes | — | |
| `auth_method` | string | no | yes | — | prose example `"local"`; SSO sets something else |
| `external_id` | string | no | yes | — | subject id at the IdP |
| `sso_provider` | string | no | yes | — | |
| `sso_metadata` | object (free-form) | no | yes | — | raw IdP claims |
| `last_sso_login` | datetime | no | yes | — | |
| `account_linked_at` | datetime | no | yes | — | when local ⇄ SSO link happened |
| `sso_linking_preference` | string | no | yes | — | persisted answer to the conflict prompt |

Implied but not in any response DTO: `active_patient_id` (required by `/patient-management/switch` +
`/active/current`), `password_hash`.

### 1.2 Registration & gating

- `POST /api/v1/auth/register` — body `UserRegistration{username*, email*, full_name*, password*, first_name?, last_name?}`.
  Returns `User`. Two documented invariants:
  1. **`role` is deliberately absent** from the registration DTO — "all self-registered users get `role='user'`.
     This prevents privilege escalation via the registration endpoint. See: GHSA-xx23-8fx5-ph4q finding 1."
  2. "A basic patient record is automatically created for the user" → registration is a 2-entity transaction
     (user + self-record patient).
  `first_name`/`last_name` are accepted at registration but are **not** user columns — they feed the auto-created patient.
- `GET /api/v1/auth/registration-status` — unauthenticated, returns whether registration is open.
  The switch itself lives in admin settings: `RetentionSettingsResponse.allow_user_registration: bool` (required),
  writable via `POST /api/v1/admin/backups/settings/retention` (yes — registration gating is stored in the
  *backup retention* settings blob; pure organic growth).
- `POST /api/v1/admin/user-management/users/create` — `AdminUserCreateRequest{username*, password*, email*, full_name*, first_name?, last_name?, role="user", link_patient_id?}`.
  Admin-only, bypasses the registration gate, **can set `role`**. `link_patient_id` transfers an existing
  patient record to the new user "instead of auto-creating a blank patient. The original owner receives edit
  access via PatientShare" — i.e. an ownership *transfer* primitive that emits a share as compensation.

### 1.3 Login / logout / password

- `POST /api/v1/auth/login` — `application/x-www-form-urlencoded`, OAuth2-password shape
  (`grant_type?, username*, password*, scope="", client_id?, client_secret?`).
  → `Token{access_token*, token_type*, session_timeout_minutes?=30, must_change_password?=false}`.
  JWT, stateless. Note the response carries UI state (`session_timeout_minutes`, `must_change_password`).
- `POST /api/v1/auth/logout` — no-op confirmation; "JWT is stateless… client is responsible for discarding".
  **There is no server-side session or revocation list.**
- `POST /api/v1/auth/change-password` — `ChangePasswordRequest{currentPassword*, newPassword*}`
  (camelCase, unlike every other DTO). Requires current password. Presumably clears `must_change_password`.
- `POST /api/v1/admin/models/users/{user_id}/reset-password` — `AdminResetPasswordRequest{new_password*}`,
  admin-only, no current-password check. Presumably sets `must_change_password=true`.
- **No self-service password reset / forgot-password flow exists at all.** No email verification either.

### 1.4 SSO (hand-rolled OAuth2 + account linking)

| Endpoint | Body | Purpose |
|---|---|---|
| `GET /auth/sso/config` | — | public: is SSO enabled, provider info for the login screen |
| `POST /auth/sso/initiate?return_url=` | — | starts the flow, returns the authorize URL |
| `POST /auth/sso/callback` | `SSOCallbackRequest{code*, state*}` | code exchanged in a POST body deliberately — "to prevent exposure in backend URL parameters, browser history, and server logs" |
| `POST /auth/sso/resolve-conflict` | `SSOConflictRequest{temp_token*, action*, preference*}` | resolves "an SSO identity matches an existing local account" |
| `POST /auth/sso/resolve-github-link` | `GitHubLinkRequest{temp_token*, username*, password*}` | GitHub has no verified email → prove local account ownership with credentials |
| `POST /auth/sso/test-connection` | — | "(for admin use)" but **no admin security declared** |

State machine, callback outcome:

```
callback(code,state)
  ├─ no local account, no conflict ......... create user (auth_method=sso) → Token
  ├─ external_id already linked ............ Token, bump last_sso_login
  └─ email/username collides with a local account
        → temp_token + CONFLICT
             ├─ resolve-conflict{action, preference}   (action/preference values NOT enumerated in spec)
             └─ resolve-github-link{username,password} → account_linked_at set, sso_linking_preference persisted
```

`action` and `preference` legal values are nowhere in the document. Likely `action ∈ {link, create_separate}`,
`preference ∈ {sso, local, both}` — treat as unspecified.

### 1.5 User preferences — the exact set

`GET/PUT /api/v1/users/me/preferences`. This is a **1:1 table** (`id`, `user_id`, `created_at`, `updated_at` are
all required in the response), not a JSON blob.

| Field | Type | Default | In `UserPreferencesUpdate`? | Notes |
|---|---|---|---|---|
| `unit_system` | string | — (**required** in response) | yes | prose elsewhere: `imperial` \| `metric` |
| `session_timeout_minutes` | int? | `120` | yes | "controls the frontend inactivity timer only. JWT expiry is fixed at server config and does not change with this preference" — a **cosmetic** security control |
| `language` | string? | `"en"` | yes | no enum |
| `date_format` | string? | `"mdy"` | yes | no enum; `mdy`/`dmy`/`ymd` implied |
| `paperless_enabled` | bool? | `false` | yes | out of MediGo scope |
| `paperless_url` | string? | — | yes | |
| `paperless_api_token` | string? | — | **no** (write-only via separate path) | returned as raw field *and* as `paperless_has_token` |
| `paperless_username` | string? | — | yes | |
| `paperless_password` | string? | — | yes | plaintext-shaped secret in a GET response DTO |
| `default_storage_backend` | string? | `"local"` | yes | `local` \| `paperless` \| `papra` |
| `paperless_auto_sync` | bool? | `false` | yes | |
| `paperless_sync_tags` | bool? | `true` | yes | |
| `papra_enabled` | bool? | `false` | yes | |
| `papra_url` | string? | — | yes | |
| `papra_api_token` | string? | — | yes | |
| `papra_organization_id` | string? | — | yes | |
| `paperless_has_token` | bool | `false` | response-only | computed presence flag |
| `paperless_has_credentials` | bool | `false` | response-only | computed |
| `papra_has_token` | bool | `false` | response-only | computed |

**Real MediGo-relevant preferences: exactly four** — `unit_system`, `session_timeout_minutes`, `language`,
`date_format`. The other twelve are Paperless/Papra integration config (dropped) and should not be carried over.
Note the design smell: secrets are modelled as normal nullable string fields *and* as `*_has_*` booleans, so the
response schema advertises fields the API may or may not redact.

### 1.6 Login history

- `GET /api/v1/admin/user-management/users/{user_id}/login-history?page=1&per_page=20` — admin-only,
  response typed `any`. Description: "Get login history for a specific user **from the activity log**."
  → There is **no login-history table**; it is a filtered projection of `ActivityLog`. `users.last_login_at`
  is the only denormalized copy. Regular users cannot see their own login history.

### 1.7 Account deletion

`DELETE /api/v1/users/me` — hard cascade: user + patient record + medications, lab results, allergies,
conditions, procedures, immunizations, vitals, encounters, treatments, emergency contacts.
"WARNING: This action cannot be undone!" No soft delete, no export-before-delete, no grace period.

### 1.8 Authorization rules inferable for auth/users

- `HTTPBearer` on everything except: `/auth/login`, `/auth/register`, `/auth/registration-status`,
  `/auth/sso/config`, `/auth/sso/initiate`, `/auth/sso/callback`, `/auth/sso/resolve-*`,
  `/auth/sso/test-connection`, `/admin/dashboard/health-check`, `/admin/dashboard/stats-test`,
  `/export/formats`, `/system/*`, `/frontend-logs/{log,error,health}`, `/utils/timezone-info`.
- Privilege-escalation defences are DTO-shaped, not rule-shaped: `UserRegistration` omits `role`;
  `UserSelfUpdate` omits `role` and `is_active` ("only admins can change these. See GHSA-xx23-8fx5-ph4q
  finding 2"). Both are documented CVE remediations — i.e. upstream *did* have these holes.
- Admin-ness is asserted only in prose. There is no scope/permission model, just `role`.

---

## 2. Patient management & multi-patient

### 2.1 The ownership model (the core of the whole app)

```
users(1) ──owns──> (N) patients            patients.owner_user_id  (required, non-null)
users(1) ──active_patient_id──> patients   mutable pointer = "who am I looking at right now"
users(N) <──patient_shares──> (N) patients  is_active + expires_at + permission_level
patients(1) ──> (N) every clinical record   every clinical row is keyed by patient_id
```

- Every user owns **0..N** patient records and **at most one** of them is the *self-record*
  (`is_self_record: bool`, "Only one self-record per user is allowed"). Registration auto-creates it.
- Accessible set = `owned ∪ shared-with-me` (`GET /patient-management/?permission=view` returns
  `PatientListResponse{patients[], total_count, owned_count, shared_count}` — the counts prove the union).
- "Netflix-style switching": `POST /patient-management/switch {patient_id}` sets `users.active_patient_id`
  after checking access; `GET /patient-management/active/current` returns it or `null`
  ("null if no active patient is set **or if the active patient is no longer accessible**" — a revoked share
  must not strand the pointer, so resolution is lazy/validated, not FK-cascaded).
- The active patient is **server-side session state on the user row**, but nearly every clinical read *also*
  accepts an explicit `?patient_id=` override ("Patient ID for Phase 1 patient switching"). So two
  parallel mechanisms coexist: implicit active-patient and explicit per-request patient. That duplication is
  the single biggest source of the 500-endpoint sprawl.

### 2.2 Patient fields

`PatientResponse` (`app__api__v1__endpoints__patient_management__PatientResponse` — there are two
same-named `PatientResponse` schemas in the doc, itself a smell):

| Field | Type | Req | Nullable | Constraints / notes |
|---|---|---|---|---|
| `id` | int | yes | no | |
| `first_name` | string | yes | no | create: 1..100 |
| `last_name` | string | yes | no | create: 1..100 |
| `birth_date` | date | yes | no | "cannot be in the future or >150 years ago" |
| `gender` | string | yes | **yes** | valid: `M, F, MALE, FEMALE, OTHER, U, UNKNOWN` (from the 422 prose — seven aliases for three concepts) |
| `blood_type` | string | yes | **yes** | valid: `A+, A-, B+, B-, AB+, AB-, O+, O-` |
| `height` | number | yes | **yes** | **inches**, 1..108 |
| `weight` | number | yes | **yes** | **pounds**, 1..992 |
| `address` | string | yes | **yes** | ">= 5 characters if provided" |
| `physician_id` | int | yes | **yes** | FK practitioners |
| `relationship_to_self` | string | yes | **yes** | free text; "e.g. self, spouse, child" |
| `owner_user_id` | int | yes | no | ownership |
| `is_self_record` | bool | yes | no | ≤1 true per owner |
| `privacy_level` | string | yes | no | **never explained anywhere in the spec, and no endpoint writes it** |
| `permission_level` | string | no | yes | *computed for the caller* — `null` when the caller is the owner |

`PatientCreateRequest` = the above minus `id/owner_user_id/privacy_level/permission_level`, plus
`is_self_record: bool = false`. `PatientUpdateRequest` = all-optional, and notably **cannot change
`is_self_record`** — self-record status is create-time-only.

Storage is imperial-only (inches/pounds) with `unit_system` applied as a display/export concern
(`GET /export/data?unit_system=imperial|metric`).

### 2.3 Patient photo

Separate 1:1 table, not the generic file system:
`PatientPhotoResponse{id, patient_id, file_name, file_path, file_size?, mime_type?, original_name?, width?, height?, uploaded_by?, uploaded_at, updated_at}`.
`POST/GET/DELETE /api/v1/patients/{patient_id}/photo` + `GET .../photo/info`.
Rules from prose: JPEG/PNG/GIF/BMP, **max 15 MB**, "automatically replaces existing photo",
"processes image (resize, rotate, convert to JPEG)".

### 2.4 Stats

- `GET /patient-management/stats` → `any` ("counts and metadata about accessible patients").
- `GET /patients/me/dashboard-stats?patient_id=` → `PatientDashboardStats{patient_id, total_records,
  active_medications, total_lab_results, total_procedures, total_treatments, total_conditions,
  total_allergies, total_immunizations, total_encounters, total_vitals}` — 10 hardcoded counters,
  one per clinical type. Adding a record type means editing this DTO.
- `GET /patients/me/recent-activity?limit=10` and `GET /patients/recent-activity/?limit=10&patient_id=`
  → `UserRecentActivity{id, model_name, action, description, timestamp}[]`. Two endpoints, same shape;
  a projection of `ActivityLog` filtered to medical entities ("Excludes user management activities").

### 2.5 Legacy duplicate surface

`/api/v1/patients/me` (GET/PUT/POST/DELETE) + `/api/v1/patients/current` ("alias for /me") operate on
the self-record with a *different, thinner* DTO (`Patient` — no owner/self/privacy fields) than
`/patient-management/*`. Plus 9 `GET /api/v1/patients/{patient_id}/{clinical_type}/` fan-out routes that
duplicate `/{clinical_type}/patient/{patient_id}` routes on the clinical routers. Pure duplication:
**~21 of the 500 ops are a second-generation copy of the multi-patient API.**

### 2.6 Authorization rules

- create: any authenticated user; self-record uniqueness enforced.
- read one/list: "User must have access to this patient" (owner ∪ active share).
- update: "User must have **edit** permission".
- delete: "**Only the patient owner** can delete the record. This will also delete all associated medical
  records." → hard cascade, and share recipients with `edit`/`full` cannot delete.
- **`required_permission` is a client-supplied query parameter on 41 operations** (see §11) — the *caller*
  states which permission level the server should demand. Default is `"view"` even on writes
  (e.g. `POST /api/v1/patients/{patient_id}/medications/?required_permission=view`,
  `POST /api/v1/vitals/patient/{patient_id}/vitals/`, `POST /api/v1/vitals/patient/{patient_id}/import/execute`).
  If the server honours the parameter literally, a `view`-only grantee can write. **Do not reproduce this.**

---

## 3. Patient sharing — `/api/v1/patient-sharing/*`

### 3.1 The share record

`ShareResponse` / `ShareWithUserInfo` (table `patient_shares`):

| Field | Type | Req | Nullable | Notes |
|---|---|---|---|---|
| `id` | int | yes | no | |
| `patient_id` | int | yes | no | the shared resource |
| `shared_by_user_id` | int | yes | no | must be the patient owner at creation time |
| `shared_with_user_id` | int | yes | no | resolved from a username-or-email identifier |
| `permission_level` | string | yes | no | default `"view"`; documented values `view` \| `edit` \| `full`. **`full` is never defined** — no endpoint description distinguishes it from `edit`, and `delete` is owner-only, so `full` has no observable semantics |
| `custom_permissions` | object (free-form) | yes | **yes** | accepted on invite and update, echoed back, **never interpreted anywhere in the spec**. Dead field |
| `is_active` | bool | yes | no | revocation and expiry cleanup flip this to `false` — shares are soft-deleted |
| `expires_at` | datetime | yes | **yes** | `null` = never expires |
| `created_at` / `updated_at` | datetime | yes | no | |
| + `shared_with_username` / `shared_with_email` / `shared_with_full_name` | string? | no | yes | join-denormalized for the owner's "shared by me" UI |

### 3.2 Endpoints

| Op | Auth rule (from prose) | Notes |
|---|---|---|
| `POST /patient-sharing/` | "current user must own the patient" | **creates an invitation, not a share** — "CHANGED: Now creates invitation instead of direct share". Body `PatientShareInvitationRequest{patient_id* (≤2147483647), shared_with_user_identifier* (1..255, username or email), permission_level="view", expires_at?, custom_permissions?, message?, expires_hours?=168}`. Returns `{message, invitation_id, expires_at?, title}` |
| `POST /patient-sharing/bulk-invite` | "must own **all** patients" | `PatientShareBulkInvitationRequest` = same, with `patient_ids: int[]`. **One invitation covering N patients** → `{message, invitation_id, patient_count, expires_at?, title}`. Accept/reject is all-or-nothing |
| `PUT /patient-sharing/?patient_id=&shared_with_user_id=` | owner | `UpdateShareRequest{permission_level?, expires_at?, custom_permissions?}` → `ShareResponse`. Composite key in the query string, not a share id |
| `DELETE /patient-sharing/revoke` | owner | body `RevokeShareRequest{patient_id*, shared_with_user_id*}` — a DELETE with a required body |
| `DELETE /patient-sharing/remove-my-access/{patient_id}` | "current user must have **received** access… (not be the owner)" | recipient-initiated self-removal |
| `GET /patient-sharing/{patient_id}` | owner | `PatientSharesResponse{patient_id, shares: ShareWithUserInfo[], total_count}` |
| `GET /patient-sharing/shared-by-me` | self | typed `any` |
| `GET /patient-sharing/shared-with-me` | self | typed `any` |
| `GET /patient-sharing/stats/user` | self | `UserSharingStatsResponse{shared_by_me: int, shared_with_me: int}` |
| `POST /patient-sharing/cleanup-expired` | "Only available to admin users **in production**" | "deactivates expired shares" — so expiry is **not** evaluated at read time; it needs a sweeper. Environment-dependent authorization is a bug shape, not a feature |

### 3.3 Share state machine

```
(no share) ──invitation accepted──> ACTIVE(is_active=true, expires_at=T?)
ACTIVE ──owner: PUT ──────────────> ACTIVE (level / expiry / custom_permissions changed)
ACTIVE ──owner: DELETE revoke ────> INACTIVE (is_active=false)
ACTIVE ──recipient: remove-my-access──> INACTIVE
ACTIVE ──now > expires_at + sweeper─> INACTIVE
INACTIVE ─(no transition back; a new invitation creates a new/reactivated row)
```

Two distinct revocation actors (owner-revoke vs recipient-leave) are genuinely different operations and both
are needed. Expiry is lazy-swept, which means an expired share is still `is_active=true` until the cron runs —
**any read path that trusts `is_active` alone is a stale-access bug.**

---

## 4. Family-history sharing — `/api/v1/family-history-sharing/*`

### 4.1 The resource being shared

`family_members` hang off a patient (`FamilyMemberCreate.patient_id` required) and own `family_conditions`:

`FamilyMemberWithConditions`: `{id, name (1..100), relationship (free text), gender?, birth_year?, death_year?, is_deceased (bool), notes?, family_conditions: FamilyConditionBase[] = []}`

`FamilyConditionBase`: `{id, condition_name (2..200), diagnosis_age?, severity?, status?, condition_type?, notes?, icd10_code?}` — all of `severity`/`status`/`condition_type` are unenumerated strings.

### 4.2 The share record

There is no flat share DTO; it is nested as `ShareDetails`:

`FamilyMemberWithShare{family_member: FamilyMemberWithConditions, share_details: ShareDetails}`
`ShareDetails{shared_by: SharedByUser{id, name, email}, shared_at, sharing_note?, permission_level, invitation?}`

Differences from `patient_shares`, field by field:

| | patient share | family-history share |
|---|---|---|
| subject | `patient_id` | `family_member_id` |
| free-text note | `message` (on the invitation only) | `sharing_note` (**persisted on the share**, surfaced to the recipient forever) |
| permission levels | `view` \| `edit` \| `full` | `"view"` default, and the DTO says **"(view only for Phase 1.5)"** — effectively read-only |
| `expires_at` on the share | yes (nullable) | **absent** — only the *invitation* expires (`expires_hours=168`) |
| `custom_permissions` | present (dead) | absent |
| `is_active` | exposed | not exposed |
| back-reference to invitation | no | `ShareDetails.invitation: object?` |

### 4.3 Endpoints

| Op | Notes |
|---|---|
| `POST /{family_member_id}/shares` | `FamilyHistoryShareInvitationCreate{shared_with_identifier*, permission_level="view", sharing_note?, expires_hours?=168}` — creates an **invitation** |
| `POST /bulk-invite` | `FamilyHistoryBulkInvite{family_member_ids*[], shared_with_identifier*, permission_level="view", sharing_note?, expires_hours?=168}` — "Send ONE invitation to share multiple family members with one user" |
| `GET /{family_member_id}/shares` | "See who has access" (owner) |
| `DELETE /{family_member_id}/shares/{user_id}` | owner revoke — **path params, not a body** (contrast §3: `DELETE /patient-sharing/revoke` with a body) |
| `DELETE /shared-with-me/{family_member_id}/remove-access` | recipient self-removal |
| `GET /mine` | `OrganizedFamilyHistory{owned_family_history: FamilyMemberWithConditions[], shared_family_history: FamilyMemberWithShare[], summary: map<string,int>}` — the only strongly-typed endpoint in the router |
| `GET /my-own`, `GET /shared-with-me`, `GET /shared-by-me` | three untyped (`any`) slices of what `/mine` already returns |
| `GET /{family_member_id}/details` | "if user has access" — owner ∪ share |

### 4.4 Why two sharing systems exist, and what it costs

They differ in **scope of disclosure** and **direction of interest**:

- Patient sharing = *caregiver delegation*. Grantee sees an entire patient chart: meds, labs, vitals,
  encounters, insurance, files. `edit` is meaningful (a spouse maintaining your medication list).
- Family-history sharing = *pedigree exchange between relatives*. Grantee sees only `family_members` +
  `family_conditions` for one relative — the hereditary-risk subset. Nobody wants "share my whole chart with
  my cousin so she can see that Grandma had breast cancer at 42." `view`-only is deliberate.
- Consequently: family history has no expiry (a pedigree fact doesn't lapse), keeps a permanent
  `sharing_note` (the *reason* is part of the data), and is subject-scoped per family member rather than
  per chart.

That is a legitimate product distinction. What is **not** legitimate is that it was implemented as a second
parallel table, second router, second bulk-invite, second revoke shape, second remove-my-access, second
shared-by-me/shared-with-me pair — 20 operations where a single resource-generic share model would be ~6.

---

## 5. Invitations — `/api/v1/invitations/*`

### 5.1 The invitation record

`InvitationResponse` (table `invitations`) — a **single generic table for both sharing systems**:

| Field | Type | Req | Nullable | Notes |
|---|---|---|---|---|
| `id` | int | yes | no | |
| `sent_by_user_id` | int | yes | no | |
| `sent_to_user_id` | int | yes | no | resolved from the username-or-email identifier at send time → **you can only invite existing users; there is no invite-an-email-address flow** |
| `invitation_type` | string | yes | no | discriminator. Values not enumerated; from usage: `patient_share`, `family_history_share` (+ bulk variants or a flag inside `context_data`) |
| `status` | string | yes | no | not enumerated (see §5.2) |
| `title` | string | yes | no | pre-rendered display string, returned by the send endpoints |
| `message` | string | yes | **yes** | sender's note |
| `context_data` | object (free-form) | yes | no | **the actual payload**: patient_id(s) or family_member_id(s), permission_level, expires_at, custom_permissions, sharing_note |
| `expires_at` | datetime | yes | **yes** | from `expires_hours` (default 168 = 7 days; "1 hour to 1 year"; `None` = no expiration for family history) |
| `responded_at` | datetime | yes | **yes** | |
| `response_note` | string | yes | **yes** | |
| `created_at` / `updated_at` | datetime | yes | no | |
| `sent_by` / `sent_to` | object? | no | yes | denormalized user summaries |

`context_data` being untyped free-form JSON is what allows one table to serve both systems — and is also why
nothing about the invitation is validatable at the schema level.

### 5.2 Lifecycle / state machine

```
                     POST /patient-sharing/            POST /family-history-sharing/{id}/shares
                     POST /patient-sharing/bulk-invite POST /family-history-sharing/bulk-invite
                                     │
                                     ▼
                                  PENDING ──────────────────────────────────┐
                                     │                                      │
   recipient: POST {id}/respond      │                                      │ now > expires_at
   {response:"accepted"|"rejected",  │                                      │ + POST /invitations/cleanup
    response_note?}                  │                                      ▼
              ┌──────────────────────┴──────────┐                        EXPIRED
              ▼                                 ▼
          ACCEPTED  ──────────────────>     REJECTED
              │  (creates the share row:
              │   patient_shares or the
              │   family-history share)
              │
              │ sender: POST {id}/revoke  ("Revoke an accepted invitation
              │                            (specifically for family history shares)")
              ▼
           REVOKED  (+ underlying share deactivated)

   PENDING ── sender: DELETE /invitations/{id} ──> CANCELLED   ("Cancel a sent invitation")
```

- `responded_at` + `response_note` are only set by `respond`.
- Two terminal-ish paths for the sender: `DELETE {id}` cancels a *pending* invite; `POST {id}/revoke`
  undoes an *accepted* one. The prose scopes revoke to family history, meaning patient shares are revoked
  through `DELETE /patient-sharing/revoke` instead — the same logical action lives in two routers with
  two different shapes.
- `POST /invitations/cleanup` — "admin/maintenance endpoint", no admin security declared, marks expired
  invitations. Same lazy-sweep problem as share expiry.
- `GET /invitations/pending?invitation_type=` (recipient view), `GET /invitations/sent?invitation_type=`
  (sender view), `GET /invitations/summary` (untyped counts).

### 5.3 How invitations tie the two sharing systems together

Both sharing routers are *write-only* w.r.t. invitations: they create rows in `invitations` with a
type discriminator and stuff their parameters into `context_data`. The invitations router owns the entire
recipient-side lifecycle. On `accepted`, the invitations service must dispatch back into the correct sharing
service to materialise the share (1 row for a single invite, N rows for a bulk invite). So:

`sharing routers = producers`, `invitations router = state machine`, `sharing tables = materialised effect`.

That is actually a decent factoring; the mistake is duplicating the producer/effect side per resource type.

---

## 6. Files / attachments

MediKeep has **two coexisting file systems**, 34 operations combined.

### 6.1 Generic `entity_files` (polymorphic)

`EntityFileResponse`:

| Field | Type | Req | Nullable | Default | Notes |
|---|---|---|---|---|---|
| `id` | int | yes | no | | |
| `entity_type` | string | yes | no | | in the path it is a bare string; the *batch-count* DTO constrains it to `EntityType` enum: **`lab-result, insurance, visit, encounter, procedure, vitals, medication, immunization, allergy, condition, treatment, symptom, injury`** (13 values, note both `visit` **and** `encounter`) |
| `entity_id` | int | yes | no | | **no FK possible** — polymorphic pair |
| `file_name` | string | yes | no | | |
| `file_path` | string | yes | no | | server path, exposed to clients |
| `file_type` | string? | no | yes | | MIME |
| `file_size` | int? | no | yes | | bytes |
| `description` | string? | no | yes | | |
| `category` | string? | no | yes | | free text, no enum |
| `storage_backend` | string? | no | yes | `"local"` | `local` \| `paperless` \| `papra` |
| `paperless_document_id` | string? | no | yes | | |
| `paperless_task_uuid` | string? | no | yes | | async upload handle |
| `papra_document_id` | string? | no | yes | | |
| `papra_organization_id` | string? | no | yes | | |
| `sync_status` | string? | no | yes | `"synced"` | see state machine |
| `uploaded_at`, `created_at`, `updated_at`, `last_sync_at` | datetime? | no | yes | | four timestamps |

**Processing / sync state machine** (values from the `status` endpoint prose + `processing/update`):

```
pending ──(actual upload finishes)──> synced
   │                                    │
   │                                    └──(remote doc deleted)──> missing (surfaced by sync/paperless map)
   └──> processing ──(task completes)──> synced
                    ──(task fails)────> failed
```

- `POST /{entity_type}/{entity_id}/files/pending` creates a **record with no bytes**
  (`file_name*, file_size*, file_type*, description?, category?, storage_backend?`) so async uploads are trackable.
- `PUT /files/{file_id}/status` (form-encoded) `{actual_file_path*, sync_status="synced", paperless_document_id?}`
  closes the loop. `sync_status` documented as `'synced' | 'failed'` here, but `processing` and `pending`
  also exist → **4 states, no enum in the schema.**
- `POST /entity-files/processing/update` → `map<file_id, new_status>` — a client-triggered poll that
  reconciles every `processing` row. This is a background job exposed as an HTTP endpoint.

Endpoints: upload (`multipart`: `file*, description?, category?, storage_backend?`), list per entity,
get, delete, `PUT metadata` (form: `description?, category?`), download, view, `POST files/batch-counts`
(`FileBatchCountRequest{entity_type: EntityType, entity_ids: int[]}` → `[{entity_id, file_count}]`),
`POST {entity_type}/{entity_id}/cleanup?preserve_paperless=true` (→ `map<string,int>` stats),
`link-paperless` / `link-papra` (attach an already-remote doc without uploading), `sync/paperless`,
`sync/papra`.

**Download vs view:**

- `GET /files/{file_id}/download` → attachment disposition, works across local + paperless.
- `GET /files/{file_id}/view?token=<jwt>` → `Content-Disposition: inline`, and **accepts the JWT as a query
  parameter** "to enable opening files in new browser tabs where Authorization headers are not automatically
  included". A long-lived bearer token in a URL, for PHI. This is exactly the problem PocketBase's
  short-lived *file tokens* solve.

**Authorization hole, stated outright:**
`POST /{entity_type}/{entity_id}/cleanup` — "IMPORTANT: This endpoint assumes authorization has already been
performed by the calling endpoint that is deleting the entity. **No additional authorization checks are
performed here.**" It is a public authenticated route. Any user can delete any entity's files.

### 6.2 Legacy `lab_result_files`

`LabResultFileResponse{id, lab_result_id*, file_name*, file_path*, file_type?, file_size?, description?, uploaded_at?}`
— no storage backend, no sync, no category. 18 operations: full CRUD, `upload/{lab_result_id}` (multipart),
download, list-by-lab-result, list-by-patient (`?skip&limit&required_permission=view`),
`filter/by-type`, `filter/date-range`, `filter/recent?days=7`, `search/by-filename?filename_pattern=`,
`stats/count-by-lab-result/{id}`, `stats/batch-counts` (body: bare `int[]`),
`batch-operation` (`FileBatchOperation{file_ids*[], operation*, target_path?}` — `operation` is an
unenumerated string driving a server-side file move/delete: a path-traversal shape),
`lab-result/{id}/files` DELETE-all, `health/storage`.

Plus a **third** overlapping set on the clinical router: `GET/POST /lab-results/{id}/files` and
`DELETE /lab-results/{id}/files/{file_id}`.

**Storage health:** `GET /lab-result-files/health/storage` and `GET /admin/dashboard/storage-health`,
both typed `any`. Two endpoints, same concern, different routers.

---

## 7. Reporting & export

### 7.1 Custom reports — `/api/v1/custom-reports/*`

**Template model** (`ReportTemplate` / `ReportTemplateResponse`, table `report_templates`):

| Field | Type | Req | Default | Notes |
|---|---|---|---|---|
| `name` | string (≤255) | yes | | |
| `description` | string? | no | | |
| `selected_records` | `SelectiveRecordRequest[]` | no | `[]` | `{category*: string, record_ids*: int[]}` — **hard-coded record IDs are persisted in the template**, so a saved template silently rots as records are added/deleted |
| `trend_charts` | `TrendChartSelection?` | no | | see below |
| `is_public` | bool | no | `false` | never referenced by any read rule in the spec |
| `shared_with_family` | bool | no | `false` | a **third** sharing mechanism, boolean-only, no target list |
| `report_settings` | object? | no | | free-form "sorting, grouping, etc." |
| `id`, `user_id`, `created_at`, `updated_at` | | yes | | response-only |
| `created_by_name` | string? | no | | denormalized |

`TrendChartSelection{vital_charts: VitalChartRequest[], lab_test_charts: LabTestChartRequest[]}`

- `VitalChartRequest{vital_type* (a raw DB column name), date_from?, date_to?}`
- `LabTestChartRequest{test_name* (1..500), unit?, date_from?, date_to?}` — `unit` "scopes the trend to a
  single unit so values recorded in different units are not merged. Omit on legacy templates for
  backward-compat" (an upstream bug fix visible in the schema).

**Generate:** `POST /custom-reports/generate` — `CustomReportRequest{selected_records[], trend_charts?,
report_title?="Custom Medical Report", include_patient_info=true, include_profile_picture=true,
include_summary=true, date_range?{start_date?, end_date?}}` → PDF (response typed `any`). Synchronous.

**Supporting reads:**

- `GET /available-trend-data` → which vital types + lab test names actually have data (untyped).
- `POST /trend-chart-counts` (body `TrendChartSelection`) → per-chart data-point counts for the picker.
- `GET /data-summary` → `DataSummaryResponse{categories: map<string, CategorySummary>, total_records, last_updated?}`
  where `CategorySummary{count, records: RecordSummary[], has_more=false}` and
  `RecordSummary{id, title, date?, practitioner?, key_info*, status?}` — a generic
  "pick records for the report" projection over every clinical type. **This is the one genuinely good
  abstraction in the reporting subsystem** and should survive the rewrite.

CRUD: `GET/POST /templates`, `GET/PUT/DELETE /templates/{id}` → `TemplateActionResponse{success, message, template_id?}`.
Delete is a **soft delete** ("Delete (soft delete) a report template") — the only soft delete in the entire
document besides `patient_shares.is_active`.

### 7.2 Export — `/api/v1/export/*`

- `ExportFormat` enum: **`json`, `csv`, `pdf`**.
- `ExportScope` enum (17): `all, allergies, conditions, emergency_contacts, encounters, family_history,
  immunizations, injuries, insurance, lab_results, medications, pharmacies, practitioners, procedures,
  symptoms, treatments, vitals`. Note: **no `files`, no `report_templates`, no `tags`** — export is not a
  full account export.
- `GET /export/data?format=json&scope=all&start_date=&end_date=&include_files=false&include_patient_info=true&unit_system=imperial`
  — `include_files` is "PDF exports only".
- `POST /export/bulk` — `BulkExportRequest{scopes*[], format=json, start_date?, end_date?,
  include_patient_info=true, unit_system="imperial"}` → ZIP, one file per scope. Synchronous, no job model.
- `GET /export/formats` — **unauthenticated**, static capability list.
- `GET /export/summary` — per-category counts (untyped), overlaps `custom-reports/data-summary` and
  `patients/me/dashboard-stats`. Three endpoints computing the same counters.

No async export jobs, no download tokens, no expiry, no size guard. Everything is generated in-request.

---

## 8. Ops surface — `/api/v1/admin/*`

### 8.1 Dashboard

- `GET /admin/dashboard/stats` → `DashboardStats` — 15 required ints: `total_users, total_patients,
  total_practitioners, total_medications, total_lab_results, total_vitals, total_conditions,
  total_allergies, total_immunizations, total_procedures, total_treatments, total_encounters,
  recent_registrations, active_medications, pending_lab_results`.
- `GET /admin/dashboard/system-health` → `SystemHealth{database_status*, total_records*, last_backup?,
  system_uptime*, database_connection_test=true, memory_usage?, disk_usage?}` (uptime/memory/disk are strings).
- `GET /admin/dashboard/analytics-data?days=7|start_date&end_date&compare=false` → untyped;
  `compare=true` adds a previous-period `comparison` object.
- `GET /admin/dashboard/recent-activity?limit=20&action_filter=&entity_filter=` →
  `RecentActivity{id, model_name, action, description, timestamp, user_info?}[]`.
- `GET /admin/dashboard/storage-health`, `GET /admin/dashboard/system-metrics` → untyped.
- `GET /admin/dashboard/health-check` (**unauthenticated**, for monitoring),
  `GET /admin/dashboard/stats-test` (**unauthenticated**, "debug routing issues"),
  `GET /admin/dashboard/test-access` — three debug endpoints shipped in a v0.69 release.

### 8.2 Activity log (the audit trail)

`ActivityLogEntry{id, user_id?, username?, action*, entity_type*, entity_type_display*, entity_id?,
patient_id?, description*, timestamp?, ip_address?}` — note `patient_id` on every row: the audit log is
patient-scoped, which is what makes per-patient "recent activity" possible.

- `GET /admin/activity-log?page=1&per_page=50&search=&action=&entity_type=&user_id=&start_date=&end_date=`
  → `ActivityLogListResponse{items[], total, page, per_page, total_pages}`.
- `GET /admin/activity-log/export` → CSV, same filters minus paging.
- `GET /admin/activity-log/filters` → `ActivityLogFilters{actions: FilterOption[], entity_types: FilterOption[],
  users: UserFilterOption[]}` where `FilterOption{value: string, label: string}` and
  `UserFilterOption{value: int, label: string}` — server-driven filter dropdowns.
- `action` and `entity_type` values are never enumerated in the schema; from other DTOs the actions include
  at least `create, update, delete, view`, plus login events (§1.6).
- The same table backs `/patients/me/recent-activity`, `/patients/recent-activity/`,
  `/admin/dashboard/recent-activity`, and `/admin/user-management/users/{id}/login-history`
  — **one table, five read surfaces, four different DTOs.**

### 8.3 Backups (16 ops)

`BackupResponse{id, backup_type*, filename*, size_bytes*, status*, created_at* (string), description?}`.
`backup_type ∈ {database, files, full}` (from the three create endpoints); `status` not enumerated
(at least `completed`/`failed`, plus something in-progress).

| Op | Notes |
|---|---|
| `POST /admin/backups/create-database` | `BackupCreateRequest{description?}` — three near-identical endpoints instead of one `{type}` |
| `POST /admin/backups/create-files` | ditto |
| `POST /admin/backups/create-full` | "database + files" |
| `GET /admin/backups/` | list (untyped) |
| `GET /admin/backups/{id}/download` | admin only |
| `POST /admin/backups/{id}/verify` | integrity check |
| `DELETE /admin/backups/{id}` | record + file |
| `GET /admin/backups/export` | backup **history** as CSV |
| `POST /admin/backups/cleanup` | retention-policy sweep |
| `POST /admin/backups/cleanup-orphaned` | files on disk with no DB row |
| `POST /admin/backups/cleanup-all` | backups + orphans + old trash |
| `GET /admin/backups/retention/stats` | current stats + cleanup **preview** |
| `GET/POST /admin/backups/settings/retention` | `RetentionSettingsResponse{backup_retention_days*, trash_retention_days*, backup_min_count*, backup_max_count*, allow_user_registration*}`; update DTO all-optional. Retention is **days + min-count + max-count** (min-count protects against over-aggressive pruning) — and it also carries the registration flag |
| `GET/POST /admin/backups/settings/schedule` | `AutoBackupScheduleResponse{preset*, time_of_day*, day_of_week?, enabled*, last_run_at?, last_run_status?, last_run_error?, next_run_at?}`; update: `AutoBackupScheduleUpdate{preset*, time_of_day?, day_of_week?}`. `preset` unenumerated (`disabled/daily/weekly/monthly` implied — `enabled` is response-only and derived from it) |

Three cleanup variants exist because backups, orphaned files, and trash have separate retention clocks.

### 8.4 Restore (4 ops) — two-phase with a confirmation token

```
GET  /admin/restore/confirmation-token/{backup_id}   → {token, ...}  "extra security step for dangerous operations"
POST /admin/restore/preview/{backup_id}              → RestorePreviewResponse{backup_id, backup_type,
                                                        backup_created, backup_size, backup_description?,
                                                        warnings[], affected_data{}}
POST /admin/restore/execute/{backup_id}              body RestoreExecuteRequest{confirmation_token*}
                                                     → RestoreExecuteResponse{success, message, backup_id,
                                                        backup_type, safety_backup_id*, restore_completed,
                                                        warnings?}
POST /admin/restore/upload  (multipart file*)        → UploadBackupResponse{success, message, backup_id,
                                                        backup_type, backup_size, backup_description}
                                                     accepts .sql (db) and .zip (files/full)
```

The important behaviour: **execute always takes a safety backup first** (`safety_backup_id` is required in
the response). `preview` is a POST because it is expensive, not because it mutates. Upload registers an
external artefact as a normal backup row, after which the same token/preview/execute flow applies.

### 8.5 Trash (4 ops) — **files only, not records**

- `GET /admin/trash/` → `object[]` (untyped).
- `POST /admin/trash/restore?trash_path=&restore_path=` → `map<string,string>`.
- `DELETE /admin/trash/permanently-delete?trash_path=` → `map<string,string>`.
- `POST /admin/trash/cleanup` → `map<string,int>`, honours `trash_retention_days`.

The identity of a trash item is a **filesystem path passed as a query parameter** — no ids, no ownership,
no per-user scoping, no undelete of database rows. MediKeep's "trash" is a directory that deleted
attachments get moved to. **There is no soft delete for clinical records anywhere in the API** (`DELETE`
on every clinical resource is hard, and patient/user deletion cascades irreversibly). Anyone reading
"trash" in the requirements as "record-level undo" is misreading upstream.

### 8.6 Bulk ops (2) and generic model CRUD (9)

- `POST /admin/bulk/update` — `BulkUpdateRequest{model_name*, record_ids*[], update_data* (free-form object)}`
- `POST /admin/bulk/delete` — `BulkDeleteRequest{model_name*, record_ids*[]}`
- both → `BulkOperationResponse{success, affected_records, failed_records: int[], message}` (partial success).
- `/admin/models/*`: `GET /` (model names), `GET/POST /{model_name}/`, `GET/PUT/DELETE /{model_name}/{record_id}`,
  `GET /{model_name}/metadata` → `ModelMetadata{name, table_name, fields: ModelField[], relationships:
  map<string,string>, display_name, verbose_name_plural}` with `ModelField{name, type, nullable, primary_key,
  foreign_key?, max_length?, choices?}`, `GET /{model_name}/export` (CSV), `GET /{model_name}/` list with
  `?page&per_page&search` → `ModelListResponse{items: object[], total, page, per_page, total_pages}`.

This is a **hand-rolled generic admin panel** — reflection-driven metadata, untyped record bodies, bulk
mutate-by-model-name, CSV export per model. ~11 operations and a reflection layer that exist purely to
give admins a Django-admin-like UI. This is the single largest chunk of the ops surface that PocketBase
deletes outright.

### 8.7 Maintenance + system

- `POST /admin/maintenance/test-library/{reload,sync}`, `GET .../info` — reload the standardized lab-test
  catalog from disk and re-match components in batches ("to prevent memory issues and timeouts on large
  datasets"). Clinical-catalog concern, listed here because it is admin-gated.
- `GET /api/v1/system/{health,version,releases,log-level,log-rotation-config}` — all unauthenticated
  (`log-rotation-config` is described as an "admin endpoint" but declares no security), rate-limited
  "60 req/min per IP". `releases` proxies GitHub release notes for a "What's New" UI.
- `POST /api/v1/frontend-logs/{log,error,user-action}` + `GET /frontend-logs/health` — the React app
  ships its logs to the backend. `log` and `error` are **unauthenticated write endpoints**; only
  `user-action` requires a bearer token.

---

## 9. Search & tags

### 9.1 Unified search — `GET /api/v1/search/`

Parameters:

| Param | Type | Default | Constraints |
|---|---|---|---|
| `q` | string? | — | `minLength: 1`; **omitting it means "list all records"** (search and list are the same endpoint) |
| `types` | string[]? | — | "Filter by record types" — **values not enumerated anywhere** |
| `skip` | int | `0` | `minimum: 0` |
| `limit` | int | `20` | `maximum: 100`; semantics are "results **per type**", not per response |
| `sort` | string | `"relevance"` | prose: `relevance, date_desc, date_asc, title` |
| `date_from` / `date_to` | string? | — | ISO |
| `patient_id` | int? | — | "for Phase 1 patient switching"; falls back to the active patient |

Response `SearchResponse{query*, total_count*, results: map<string, SearchResultGroup>*, pagination: PaginationInfo*}`
with `SearchResultGroup{count*, items: any[]*}` and `PaginationInfo{skip*, limit*, has_more*}`.

Behavioural notes: results are **grouped by record type** (map keyed by type name), each group carries its
own `count`, and `items` is untyped — the client must switch on the map key to know what it got.
`pagination` is single-valued while `limit` is per-type, so `has_more` cannot be correct for a multi-type
query. `sort=relevance` over what is almost certainly SQL `ILIKE` is aspirational.

"Consolidates multiple API calls into a single efficient search" — this endpoint was retrofitted to
replace N per-type list calls, which is the right instinct.

### 9.2 Tag model — a denormalized string array **plus** a registry

Tags live in two places at once:

1. **On every clinical record**, as `tags: string[]?` (default `[]`, nullable). Present on
   Allergy, Condition, Encounter, Immunization, Injury, LabResult, MedicalEquipment, Medication,
   Procedure, Symptom, Treatment (Create/Update/Response/WithRelations variants) — 43 schemas carry it.
   No FK, no normalization, no per-tag metadata.
2. **A "user tags registry"** with an `id` and a `color` — implied by `POST /tags/create`
   (`TagCreateRequest{tag*}`) and `PATCH /tags/{tag_id}/color` (`TagColorUpdateRequest{color?: string}`,
   nullable, no format/pattern → any string accepted as a colour).

Because the string arrays are the source of truth, every rename/delete is an **O(all rows) rewrite**, which
is exactly why these endpoints exist:

| Op | Shape | Notes |
|---|---|---|
| `POST /tags/create` | `{tag}` | registry insert only |
| `PUT /tags/rename?old_tag=&new_tag=` | query params | "Rename a tag across all entities owned by the current user" |
| `PUT /tags/replace?old_tag=&new_tag=` | query params | "Replace one tag with another" — **functionally indistinguishable from rename** in the spec; presumably rename fails if `new_tag` exists and replace merges |
| `DELETE /tags/delete?tag=` | query param | "Delete a tag from all entities owned by the current user" |
| `PATCH /tags/{tag_id}/color` | `{color?}` | the only op keyed by tag **id** rather than by name |
| `GET /tags/autocomplete?q=*&limit=10` | → `string[]` | |
| `GET /tags/suggestions?entity_type=&limit=20` | → `string[]` | "based on what users have actually created" |
| `GET /tags/popular?entity_types=[…8 defaults…]&limit=20` | → `object[]` | default `entity_types = [lab_result, medication, condition, procedure, immunization, treatment, encounter, allergy]` |
| `GET /tags/search?tags=*&entity_types=&limit_per_entity=10&match_mode=any&patient_id=` | → `map<type, any[]>` | `match_mode ∈ {any (OR), all (AND)}`; "scoped to an accessible patient" |

Note the entity-type vocabulary drift: tags use `lab_result` (underscore), entity-files use `lab-result`
(hyphen), export scopes use `lab_results` (plural). Three spellings of the same concept in one API.

Tag mutations are scoped "to entities **owned by** the current user" — so a share recipient with `edit`
cannot rename tags, and tag colours are per-user while tag strings are per-record. Sharing a patient
therefore leaks the owner's tag vocabulary without the colours.

---

## 10. Cross-cutting observations (things the rewrite must consciously fix)

1. **`required_permission` as a client-supplied query parameter on 41 operations**, defaulting to `"view"`
   even on POST/DELETE (`/patients/{id}/medications/`, `/vitals/patient/{id}/vitals/`,
   `/vitals/patient/{id}/import/execute`, `/vitals/patient/{id}/import/{source}/date/{date}`).
   Authorization strength is negotiated by the caller. This must be a server-side property of the route.
2. **Lazy expiry.** Share and invitation expiry only take effect when a cleanup endpoint is invoked;
   nothing in the read path is documented to compare `expires_at` to now.
3. **Untyped responses everywhere.** ~40% of platform ops return `any` / bare `object` / `map<str,…>`,
   including every "shared-by-me / shared-with-me" list and most admin reads. An OpenAPI-from-Go-types
   approach fixes this by construction.
4. **Three sharing mechanisms** (patient shares, family-history shares, `report_templates.shared_with_family`
   + `is_public`) and **five projections of one activity table**.
5. **Duplicated resource surfaces**: `/patients/me` vs `/patient-management/*`; `entity-files` vs
   `lab-result-files` vs `/lab-results/{id}/files`; `export/summary` vs `custom-reports/data-summary` vs
   `patients/me/dashboard-stats`; `lab-result-files/health/storage` vs `admin/dashboard/storage-health`.
6. **Debug/unauthenticated endpoints in a release build**: `stats-test`, `test-access`, `sso/test-connection`,
   `frontend-logs/log|error` (unauthenticated writes), `system/log-rotation-config`.
7. **PHI in URLs**: `?token=<jwt>` for inline file viewing; `file_path` returned to clients;
   `trash_path` as an API identifier.
8. **Self-documented authz gap**: `entity-files/{type}/{id}/cleanup` performs no authorization at all.

---

## 11. Appendix — notifications (platform-adjacent, present upstream)

Not in the nine requested areas, but it is platform code and it is coupled to sharing, so worth recording.

- `ChannelType` enum: **`discord, email, gotify, ntfy, webhook`**.
- `EventType` enum: **`backup_completed, backup_failed, invitation_received, invitation_accepted,
  share_revoked, password_changed, medication_reminder_due`** — five of the seven are platform events, and
  three come straight out of the sharing/invitation state machines above.
- `ChannelCreate{name (1..100)*, channel_type*, config* (free-form), is_enabled=true}`;
  `ChannelResponse` adds `is_verified, last_test_at?, last_test_status?, last_used_at?,
  total_notifications_sent, config_valid=true, config_error?`; `ChannelWithConfigResponse` adds
  `config_masked` (secrets masked for edit forms — the pattern user preferences *should* have used).
- `PreferenceCreate{channel_id*, event_type*, is_enabled=true, remind_before_minutes?}`; the matrix read
  (`PreferenceMatrix{channels[], events[], preferences: map<event, map<channel, bool>>}`) is a nice
  UI-shaped projection.
- `HistoryResponse{id, event_type, title, message_preview?, channel_name?, channel_type?, status,
  attempt_count, error_message?, created_at, sent_at?}` — delivery has retries and a status; the
  medication-reminder test endpoint mentions an "idempotency dedup key" keyed on scheduled time.

---

# REIMAGINED MODEL RECOMMENDATIONS

Baseline: PocketBase v0.40.1 embedded as a Go framework, PB owns the router, every public route is a
hand-written `/api/v1` handler over interface-defined services, PB's `/api/collections/*` CRUD locked down.

## A. What PocketBase gives away for FREE — do NOT rebuild

### A1. Auth, identities, tokens, OAuth2 → deletes all 11 `/auth/*` ops and most of `/users/*`

PB's `users` auth collection natively provides: password auth by email *or* username, token issue/refresh,
per-collection token durations, email verification, **password reset**, email-change confirmation, MFA/OTP,
superuser impersonation, and **built-in OAuth2 providers including linking a provider to an existing
auth record**.

Delete outright:

- The entire hand-rolled SSO flow (`initiate`, `callback`, `resolve-conflict`, `resolve-github-link`,
  `test-connection`, `temp_token`, `sso_metadata`, `sso_linking_preference`, `account_linked_at`,
  `external_id`, `auth_method`). PB stores external identities in `_externalAuths` and its
  `authWithOAuth2` accepts a bearer token to *link* rather than create — that is precisely the
  "account conflict" machinery MediKeep hand-built, and it is a config toggle in PB.
- `must_change_password` as a bespoke flag → PB's password-reset flow + `verified`. Keep a boolean only if
  you truly need admin-forced rotation; it is one field, not a subsystem.
- Login history: PB records auth requests; combine with your own `activity_log` (§A5) for the domain view.
  Do not build a login-history table.

Keep bespoke and thin: `POST /api/v1/auth/login|logout|refresh` as *thin DTO wrappers* over PB's auth
(because the public API is hand-written), plus `GET /api/v1/meta/registration-status` reading a settings
record. That is ~5 endpoints replacing 16.

**User preferences: do not build a preferences subsystem.** Four real fields survive
(`unit_system`, `session_timeout_minutes`, `language`, `date_format`) — put them as fields directly on the
`users` collection. Two endpoints (`GET/PATCH /api/v1/me`) cover profile + preferences. Drop all twelve
Paperless/Papra fields with the integrations.

### A2. Files, storage, thumbnails, protected downloads → deletes ~34 ops down to ~0 bespoke ones

PB file fields give: local **or** S3 storage (config, not code), MIME-type and size validation,
`maxSelect` for multi-file fields, automatic deletion when the owning record is deleted, on-the-fly
**thumbnails** (`?thumb=100x100t`), `?download=1` to force attachment disposition, and **protected files
served only with a short-lived file token**.

Recommendation: **kill the polymorphic `entity_files` table.** Put a `files` (multi) file field directly on
each clinical collection that needs attachments. Consequences:

- `entity_type` + `entity_id` polymorphism → gone, replaced by real relations and real API rules.
- `batch-counts` (both variants) → gone; a count is `len(record.GetStringSlice("files"))`, returned inline
  with the record.
- `pending`/`processing`/`sync_status` state machine → gone with Paperless/Papra (out of scope). Uploads are
  synchronous multipart into a PB record.
- `download` vs `view?token=<jwt>` → PB's file endpoint with `?download=1` vs inline, authorized by the
  collection's `viewRule` plus a **file token** for protected files. This also fixes the
  "long-lived JWT in a URL" PHI leak: file tokens are short-lived and single-purpose.
- Patient photo "resize, rotate, convert to JPEG" → a PB image field + `?thumb=` presets. (Verify EXIF
  orientation handling at implementation time; if PB doesn't rotate, that is ~20 lines in an
  `OnRecordCreateRequest` hook, not a subsystem.)
- `lab-result-files/*` (18 ops), `entity-files/*` (16 ops), `/lab-results/{id}/files*` (3 ops),
  `filter/by-type`, `filter/date-range`, `filter/recent`, `search/by-filename`, `batch-operation`,
  `health/storage` → **all gone.** Filtering attachments is a query on the owning record.

Only genuinely bespoke bit: if you want a document library browsable *across* record types, add a real
`attachments` collection with **one nullable relation field per clinical collection** (referentially sound,
API-rule-expressible) rather than `entity_type`+`entity_id`. Do that only if the product needs it.

### A3. Admin UI + generic model CRUD → deletes ~20 ops and a reflection layer

PB's superuser dashboard at `/_/` already is the Django-admin MediKeep hand-built: collection browser,
per-record CRUD, filtering, relation navigation, schema editor, settings, backups, cron inspector.

Delete: all 9 `/admin/models/*` ops, `ModelMetadata`/`ModelField` reflection, both `/admin/bulk/*` ops
(PB's dashboard does multi-select delete; and PB's transactional `/api/batch` covers programmatic bulk),
`/admin/dashboard/stats-test`, `/admin/dashboard/test-access`, `/admin/dashboard/health-check`
(Prometheus + your own `/healthz`), `/admin/models/{model}/export` (PB dashboard exports; and see §B4).

Keep bespoke: the *product-facing* admin dashboard tiles (`DashboardStats`, `SystemHealth`,
`storage-health`) — but as **2 endpoints, not 9**, and typed. `_superusers` is PB's admin identity; keep a
`role` field on `users` only if you need a non-superuser "app admin" tier (recommended: yes, one field,
two values).

### A4. Backup / restore → deletes 20 ops down to ~4 thin ones

PB has backups **built in**: create/list/delete/download/upload/restore over `/api/backups`, a
cron-scheduled auto-backup with `maxKeep`, optional S3 backup storage, and a dashboard UI for all of it.
`core.App.CreateBackup(ctx, name)` / `RestoreBackup(ctx, name)` are callable from Go.

Delete: `create-database` / `create-files` / `create-full` as three endpoints (PB's backup is one archive:
`data.db` + `pb_public`/storage), `cleanup` / `cleanup-orphaned` / `cleanup-all` (PB's `maxKeep` prunes;
orphans can't accumulate because PB owns the directory), `retention/stats`, `settings/schedule`
(PB settings: cron expression + maxKeep), `backups/export` CSV, `confirmation-token/{id}`.

Keep bespoke, deliberately: **the safety-backup-before-restore behaviour** (`safety_backup_id`) and
**`restore/preview`**. PB will happily restore over your data; taking a snapshot first and showing the
operator what they are about to clobber is real value and is ~50 lines calling `CreateBackup` then
`RestoreBackup`. Budget: `POST /api/v1/admin/backups`, `GET /api/v1/admin/backups`,
`POST /api/v1/admin/backups/{id}/restore` (with preview + `confirm=true`), `POST /api/v1/admin/backups/upload`.

### A5. Authorization primitives → API rules as defence-in-depth, not as the public contract

Collection API rules (`listRule/viewRule/createRule/updateRule/deleteRule`) with `@request.auth.id`,
relation traversal, back-relations, `@collection.x`, and date comparison against `@now` can express the
**entire** sharing model declaratively, including expiry:

```
// patients.viewRule
owner = @request.auth.id ||
@collection.shares.patient ?= id &&
@collection.shares.grantee ?= @request.auth.id &&
@collection.shares.revoked_at ?= null &&
(@collection.shares.expires_at ?= null || @collection.shares.expires_at > @now)
```

Since the public API is hand-written, the authoritative check lives in a Go `Authorizer` interface — but
setting these rules anyway means a mistake in a handler cannot become a data breach, and it makes
**expiry evaluated at read time** instead of by a sweeper. That single change deletes
`patient-sharing/cleanup-expired` and `invitations/cleanup` from the *correctness* path (keep a cron job
purely for tidiness/analytics).

Also free: PB's **rate limiting** settings (replaces the hand-rolled "60 req/min per IP" on `/system/*`),
and PB's **realtime** subscriptions with API rules applied (locked decision already covers this) — which
is how "your invitation was accepted" reaches the UI without a notifications subsystem.

### A6. Logging / request audit

PB's `_logs` (separate SQLite DB, retention setting, dashboard viewer) covers request/error logging —
but the locked decision **disables `_logs` and bridges PB's slog into zerolog**, so this one is
deliberately forfeited. Consequence: the *request* log is zerolog/Sentry/OTel, and the **domain** audit
trail must be bespoke (§B3). Also delete all four `/frontend-logs/*` endpoints — a templ+Datastar frontend
has no React error boundaries to ship home, and two of them were unauthenticated write endpoints.

## B. What genuinely needs bespoke Go code

### B1. The sharing & permission model — bespoke, and **unify it to one model** (highest-value redesign)

PB gives no row-level grant table. Write one, once, resource-generic:

```go
// collection: shares
type Share struct {
    ID          string    // PB id
    ResourceKind string   // enum: "patient" | "family_member"   (select field, exactly 2 values)
    PatientID    string   // relation, set iff kind=patient
    FamilyMemberID string  // relation, set iff kind=family_member
    GrantorID    string   // relation -> users (must own the resource at create time)
    GranteeID    string   // relation -> users
    Level        string   // select: "view" | "edit"   -- DROP "full", it has no semantics
    Note         string   // survives from family-history's sharing_note; useful for both
    ExpiresAt    *time.Time // null = never; evaluated at read time
    RevokedAt    *time.Time // null = active; who revoked is recoverable from activity_log
    InvitationID string    // relation, provenance
}
```

Design decisions to lock in:

- **Two permission levels, `view` and `edit`.** `full` is undefined upstream; delete is owner-only anyway.
- **Drop `custom_permissions` entirely.** Free-form JSON that nothing reads is a liability.
- **`RevokedAt`/`ExpiresAt` timestamps instead of `is_active`** — you get "when" for free and expiry becomes
  a filter, not a cron job.
- **Family-history sharing keeps its narrower default** (`level=view`, subject = one family member) but
  shares the table, the invitation flow, and the revoke/leave endpoints. Product distinction preserved,
  20 endpoints become ~6.
- Two revocation actors remain distinct because they are: owner-revoke (`DELETE /shares/{id}`) and
  grantee-leave (`DELETE /shares/mine/{id}`).
- `report_templates.is_public` / `shared_with_family` — **delete**. If templates need sharing, they are a
  third `ResourceKind`, not two booleans.

Endpoint budget: `POST /api/v1/shares` (single or bulk via `resource_ids[]`),
`GET /api/v1/shares?direction=granted|received`, `PATCH /api/v1/shares/{id}`, `DELETE /api/v1/shares/{id}`,
`DELETE /api/v1/shares/mine/{id}`, `GET /api/v1/patients/{id}/shares`. **6 vs 20.**

### B2. Invitations — bespoke, but keep the good factoring and type the payload

Keep the single generic invitation table and the state machine
(`pending → accepted | rejected | cancelled | expired`, `accepted → revoked`). Fix two things:

- **Replace `context_data: object` with a typed discriminated payload** per `invitation_type`
  (Go structs + a `kind` select field). Free-form JSON is why nothing upstream is validatable.
- **Allow inviting an email address that has no account yet** (upstream requires `sent_to_user_id`, i.e.
  the recipient must already exist). PB's auth + email templates make an "accept and sign up" flow cheap.

Endpoints: `GET /api/v1/invitations?direction=received|sent`, `POST /api/v1/invitations/{id}/respond`,
`DELETE /api/v1/invitations/{id}`. **3 vs 7.** Expiry is a filter, not `/cleanup`.

### B3. Activity log — bespoke table, near-free population

PB's `_logs` is request logging and is disabled by decision; a patient-scoped domain audit trail
(`user_id, action, entity_type, entity_id, patient_id, description, ip, ts`) is genuinely bespoke.
But **populating it is near-free**: one `OnRecordAfterCreateSuccess` / `…UpdateSuccess` /
`…DeleteSuccess` hook per collection group writes the rows, so you do not sprinkle audit calls through
handlers. Consolidate the five upstream read surfaces into **one** endpoint with filters
(`GET /api/v1/activity?patient_id=&user_id=&action=&entity_type=&from=&to=&page=`) plus `?format=csv`.
Server-driven filter options become a static list generated from your Go enums, not an endpoint.
**2 ops vs 8.**

### B4. Reporting & export — fully bespoke, and worth doing properly

PB has nothing here. Keep:

- The **`RecordSummary` / `CategorySummary` projection** — upstream's one good abstraction. One endpoint:
  `GET /api/v1/reports/data-summary?patient_id=` doubling as the export summary and the dashboard counters
  (kills three duplicate counter endpoints).
- Template model, but **store selection criteria, not frozen `record_ids`** (category + filters + date
  range). Upstream templates rot; criteria-based ones don't.
- Trend charts: keep `unit` scoping on lab charts (it's a real bug fix), keep `trend-chart-counts` folded
  into `available-trend-data` as one endpoint.
- PDF generation: bespoke Go (`maroto`/`gofpdf`/headless-Chrome — decide separately). CSV/JSON: bespoke.
- **Add what upstream lacks**: an async job for large exports with a token-authorized, expiring download.
  Synchronous ZIP-of-everything in a request handler is a timeout waiting to happen. PB's `app.Cron()`
  plus a `jobs` collection covers this without new infrastructure.
- `ExportScope` should include `files` and be derived from the collection registry, not a hand-maintained
  17-value enum.

Endpoint budget: `GET /api/v1/reports/data-summary`, `GET /api/v1/reports/trend-data`,
`GET/POST /api/v1/reports/templates`, `GET/PATCH/DELETE /api/v1/reports/templates/{id}`,
`POST /api/v1/reports/generate`, `POST /api/v1/exports`, `GET /api/v1/exports/{id}`. **~10 vs 13**, but
typed, async, and non-rotting.

### B5. Trash / soft delete — **decide, don't inherit**

Upstream "trash" is a filesystem directory for deleted attachment bytes, keyed by path, admin-only, with
no record-level undo anywhere. PB has no soft delete either. Two honest options:

1. **No trash.** PB deletes a record's files with the record; nightly backups are the undo. This is
   coherent for a single-user self-hosted app and is what upstream effectively has.
2. **Real record-level soft delete**: a `deleted_at` timestamp on clinical collections, excluded by every
   list/view API rule and every service query, with `GET /api/v1/trash`,
   `POST /api/v1/trash/{collection}/{id}/restore`, `DELETE /api/v1/trash/{collection}/{id}`, and a cron
   purge honouring `trash_retention_days`. Cost: every query path must respect `deleted_at`, and PB's
   cascade-delete semantics have to be re-thought (a soft-deleted patient with live children).

**Recommendation: option 1 for v1**, because the *stated* upstream behaviour is a file-bytes trash and
nobody will miss a path-keyed admin file browser. If the product wants undo, do option 2 deliberately and
put `deleted_at` in the base model from day one — retrofitting it later touches every query.

### B6. Search — bespoke, and be honest about SQLite

PB gives you filter expressions per collection, not a cross-collection unified search. Keep one endpoint,
keep the grouped-by-type response (it's right), and fix the flaws:

- Split "list" from "search": `q` optional-means-list-everything is why `pagination` is incoherent.
- Make `types` a **typed enum** and pick **one spelling** for record-type identifiers across search, tags,
  files and exports (upstream has three: `lab_result` / `lab-result` / `lab_results`).
- Per-type `limit` with a per-type `has_more` in each group, not one global `PaginationInfo`.
- `sort=relevance` requires an actual ranking source. Verify whether the PocketBase build's SQLite has
  **FTS5** available before promising relevance; if not, either maintain a `search_index` table
  (record_ref, patient, kind, text, tokens) updated from PB record hooks, or ship
  `date_desc | date_asc | title` only and drop `relevance`. Do not claim relevance ranking over `LIKE`.

### B7. Tags — normalize, and 9 endpoints collapse to 3

The single highest-leverage small fix in the whole platform surface. Make tags a real collection
(`tags{ name, color, owner }`) and put a **multi-relation** `tags` field on each clinical collection.
Then, for free:

- **Rename** = update one `tags` record. Delete `PUT /tags/rename` **and** `PUT /tags/replace` (which was
  only ever a merge-on-collision workaround for string arrays).
- **Delete** = delete one record; PB's relation cascade removes it from every referencing record.
  Delete `DELETE /tags/delete?tag=`.
- **Colour** = a field on the tag, not a side table keyed by a different identity. Delete
  `PATCH /tags/{id}/color` as a special endpoint.
- **Autocomplete / suggestions / popular** = one filtered+sorted list endpoint over `tags` with a
  usage counter maintained in a hook (or a back-relation count).
- **Search by tags** = a normal filter on the record collections, so `GET /tags/search` merges into
  `GET /api/v1/search?tags=&match=any|all`.

Endpoint budget: `GET /api/v1/tags` (list/autocomplete/popular via params),
`POST /api/v1/tags`, `PATCH /api/v1/tags/{id}`, `DELETE /api/v1/tags/{id}`. **4 vs 9**, and colours now
apply to shared records too.

### B8. Multi-patient ownership & switching — bespoke, but simplify hard

PB has no notion of "acting as". Keep bespoke:

- `patients.owner` relation + `is_self_record` with a partial-unique constraint per owner.
- `users.active_patient` relation, validated on read (return null when access is gone).
- **Pick ONE mechanism.** Upstream carries both an active-patient pointer *and* `?patient_id=` on ~40
  clinical endpoints. Recommendation: **explicit `patient_id` on every scoped route** (path or required
  query), with `active_patient` kept purely as a UI convenience the frontend uses to fill it in. That
  removes the ambient-state coupling that generated most of the duplicate routes.
- Delete the entire `/api/v1/patients/me|current` legacy surface and the 9 `/patients/{id}/{type}/`
  fan-out routes: the self-record is just a patient whose `is_self_record` is true.
- `privacy_level` — delete. Nothing writes it and nothing documents it.
- Normalize `gender` to a 3–4 value select (upstream accepts seven aliases) and store height/weight in
  **SI units** with `unit_system` applied at the edge (upstream stores inches/pounds, which forces
  conversion into export *and* reporting *and* the UI).

### B9. Notifications — bespoke, but scope it

If notifications ship at all: PB gives SMTP settings + email templates + `app.Cron()`, so the `email`
channel is near-free. Discord/ntfy/gotify/webhook are ~30 lines each behind a `Notifier` interface. Keep
`config_masked` (upstream got this right) and keep the delivery-attempt/status history. Consider deferring
everything except email + the four sharing events, and lean on PB realtime for in-app notices.

## C. Consolidated endpoint budget for the platform half

| Subsystem | Upstream ops | Proposed | Mechanism |
|---|---|---|---|
| Auth + SSO | 11 | 4 | PB native auth + OAuth2 |
| Users + preferences | 5 | 3 | prefs as `users` fields |
| Patients + multi-patient + photo | 31 | 9 | one patient resource; PB file field for photo |
| Sharing (patient + family history) | 20 | 6 | one generic `shares` collection |
| Invitations | 7 | 3 | typed payload, expiry as filter |
| Files / attachments | 34 | 0–2 | **PB file fields** |
| Reporting + export | 13 | 10 | bespoke, async exports |
| Ops (dashboard/activity/backup/restore/trash/bulk/models) | 46 | 9 | PB admin UI + PB backups |
| Search | 1 | 1 | bespoke, typed, split list/search |
| Tags | 9 | 4 | normalized tags collection |
| System / frontend-logs / utils | 10 | 2 | `/healthz`, `/version`; drop frontend-logs |
| Notifications | 11 | 0–6 | defer; PB SMTP + realtime |
| **Total** | **~198** | **~51–57** | |

That leaves roughly 30–60 endpoints of the 80–120 budget for the clinical half — which is the right split,
since the clinical domain is where the actual product lives.

## D. The five things to state loudest

1. **Files: don't build a file subsystem.** PB file fields + thumbnails + protected file tokens delete 34
   operations, the `entity_type`/`entity_id` polymorphism, the pending/processing/sync state machine, the
   batch-count endpoints, and the `?token=<jwt>`-in-a-URL PHI leak.
2. **Ops: don't build an admin panel or a backup manager.** PB's dashboard + `/api/backups` + cron
   auto-backup delete ~37 operations and a reflection layer. Keep only safety-backup-before-restore and
   restore preview.
3. **Auth: don't build SSO.** PB's OAuth2 with account linking *is* the conflict-resolution flow MediKeep
   hand-rolled, and PB also brings password reset and email verification, which upstream simply lacks.
4. **Sharing is the one thing you must build well** — one resource-generic `shares` table with
   `view|edit`, timestamped revoke/expire, expiry enforced in the read path (and mirrored into PB API
   rules as defence-in-depth), one invitation state machine with typed payloads. 26 upstream ops → 9.
5. **Normalize tags and drop `required_permission` from the wire.** Tags-as-relations deletes 5 endpoints
   and the O(all-rows) rewrites; permission level is a property of the route, never a query parameter the
   caller chooses.
