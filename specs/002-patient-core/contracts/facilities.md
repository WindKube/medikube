# Contract: `/api/v1/facilities`

Five operations. Requirements covered: FR-034 … FR-040.

**One directory for practices, pharmacies, hospitals, laboratories, imaging centres and anything
else**, distinguished by `kind` (FR-034, research D-24). Upstream modelled practices and pharmacies
as two entities with two different treatments of the same six address concepts; MediGo has one
shape, one CRUD, one page, one search.

## DTOs

```go
type FacilitySummary struct {
    ID        string `json:"id"`
    Kind      string `json:"kind"`                // practice|pharmacy|hospital|lab|imaging|other
    Name      string `json:"name"`
    Brand     string `json:"brand,omitempty"`
    City      string `json:"city,omitempty"`
    Phone     string `json:"phone,omitempty"`
    UpdatedAt string `json:"updated_at"`
}

type Facility struct {
    FacilitySummary
    Street       string `json:"street,omitempty"`
    Region       string `json:"region,omitempty"`
    PostalCode   string `json:"postal_code,omitempty"`
    Country      string `json:"country,omitempty"`
    Fax          string `json:"fax,omitempty"`
    Email        string `json:"email,omitempty"`
    Website      string `json:"website,omitempty"`
    PortalURL    string `json:"portal_url,omitempty"`
    Hours        string `json:"hours,omitempty"`
    Open24h      bool   `json:"open_24h"`
    DriveThrough bool   `json:"drive_through"`
    Services     string `json:"services,omitempty"`
    Notes        string `json:"notes,omitempty"`
    Usage        Usage  `json:"usage"`            // detail only; see practitioners.md
}

type FacilityCreate struct { Kind, Name string; /* …every writable field… */ }
type FacilityPatch  struct { Kind, Name *string; /* …pointers… */ }
```

`owner` appears in neither write DTO. `Usage` here counts `practitioners (facility)` +
`medications (pharmacy)`.

---

## `listFacilities` — `GET /api/v1/facilities`

**Query**: `?q=` (substring of `name` and `brand`), `?kind=` (one of the six), `?limit=`,
`?cursor=`. **Sort**: `kind, name, id`.

**200** `{ "items": [FacilitySummary], "next_cursor": … }`.

Serves the directory page, the kind filter (FR-036) and the type-ahead behind every facility and
pharmacy picker (FR-039). One operation, not six.

| Status | When |
|---|---|
| 200 | always; empty directory → `items: []` |
| 422 | `?kind=` outside the vocabulary |
| 400 | bad cursor |
| 401 | no session |

---

## `createFacility` — `POST /api/v1/facilities`

**201** + `Location` + `ETag`.

| Status | When | Requirement |
|---|---|---|
| 201 | created | FR-034, US5-2 |
| 422 | `kind` missing or outside the vocabulary; `name` missing or >200; malformed email/website/portal | FR-034 |
| 401 | no session | |

**There is deliberately no uniqueness constraint on name.** FR-035 and US5-3: a chain's second
branch is a second entry with its own address and hours, and both must be offered. A test creates
two facilities with an identical name and different addresses and asserts both are stored and both
appear in the list — the mirror image of the practitioner uniqueness test, and just as necessary,
because it is the kind of rule somebody "fixes" by adding an index.

---

## `getFacility` — `GET /api/v1/facilities/{id}`

**200** `Facility` (with `usage`) + `ETag`. **404** not owned / absent. **401** unauthenticated.

## `updateFacility` — `PATCH /api/v1/facilities/{id}`

`If-Match` required. **200** / **412** / **422** / **404** / **401**, as elsewhere.

Changing `kind` is permitted — a "practice" that turns out to be a hospital is a correction, not a
new entity.

## `deleteFacility` — `DELETE /api/v1/facilities/{id}`

`If-Match` required. **204**.

References from `practitioners.facility` and `medications.pharmacy` are **unset, not cascaded**
(research D-06). The practitioner and the medication both survive.

| Status | When | Requirement |
|---|---|---|
| 204 | deleted; references cleared | FR-040 |
| 412 / 404 / 401 | as elsewhere | |

**Mandatory tests**
- A facility referenced by a practitioner and by a medication's `pharmacy` is deleted → both
  survive with an empty reference.
- Account B → 404 for every one of the five operations against Account A's ids (FR-037, SC-014).
