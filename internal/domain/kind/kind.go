package kind

// Kind is one clinical record kind. Its value is the enum spelling — singular
// snake_case — which is what a select field, an audit target and an OpenAPI
// discriminator carry.
//
// Nothing switches on a Kind. internal/records dispatches through its registry,
// and a `switch k` anywhere else is the open/closed violation Principle II
// names: adding the fourteenth kind must change no code that already exists.
type Kind string

// The kinds this build knows. Phase 003 adds thirteen and phase 004 adds the
// fifteenth; each is one line here plus one row in the table below.
const (
	Medication       Kind = "medication"
	Allergy          Kind = "allergy"
	Condition        Kind = "condition"
	Encounter        Kind = "encounter"
	Procedure        Kind = "procedure"
	Treatment        Kind = "treatment"
	Symptom          Kind = "symptom"
	Vitals           Kind = "vitals"
	Immunization     Kind = "immunization"
	Injury           Kind = "injury"
	Insurance        Kind = "insurance"
	Equipment        Kind = "equipment"
	EmergencyContact Kind = "emergency_contact"
	FamilyMember     Kind = "family_member"
)

// entry is a kind's full identity. Upstream carried three spellings of one
// record kind in a single API; MediKube derives all of them from this row, so a
// route, a page and a collection cannot drift apart.
type entry struct {
	kind Kind
	// Plural kebab-case, and not derived from the enum value: `insurance` and
	// `family-history` are deliberately not mechanical plurals, which is
	// exactly why this is declared rather than computed (research D-05).
	segment string
	// The PocketBase collection. It matches the segment today and is a separate
	// column because it will not always: a collection is a schema name and a
	// segment is a URL, and renaming one is not licence to rename the other.
	collection string
	// The two ARIA landmarks a kind's list and detail pages assert
	// (contracts/pages.md §2, data-model §3): `region[name="…"]` and
	// `article[name="…"]`, spelled out here in full so a page's render test and
	// this table cannot drift into two different strings for one kind.
	listLandmark   string
	detailLandmark string
}

// THE declaration. Every spelling of a record kind in MediKube starts here.
var registry = []entry{
	{
		kind: Medication, segment: "medications", collection: "medications",
		listLandmark: `region[name="Medications"]`, detailLandmark: `article[name="Medication"]`,
	},
	{
		kind: Allergy, segment: "allergies", collection: "allergies",
		listLandmark: `region[name="Allergies"]`, detailLandmark: `article[name="Allergy"]`,
	},
	{
		kind: Condition, segment: "conditions", collection: "conditions",
		listLandmark: `region[name="Conditions"]`, detailLandmark: `article[name="Condition"]`,
	},
	{
		kind: Encounter, segment: "encounters", collection: "encounters",
		listLandmark: `region[name="Encounters"]`, detailLandmark: `article[name="Encounter"]`,
	},
	{
		kind: Procedure, segment: "procedures", collection: "procedures",
		listLandmark: `region[name="Procedures"]`, detailLandmark: `article[name="Procedure"]`,
	},
	{
		kind: Treatment, segment: "treatments", collection: "treatments",
		listLandmark: `region[name="Treatments"]`, detailLandmark: `article[name="Treatment"]`,
	},
	{
		kind: Symptom, segment: "symptoms", collection: "symptoms",
		listLandmark: `region[name="Symptoms"]`, detailLandmark: `article[name="Symptom episode"]`,
	},
	{
		kind: Vitals, segment: "vitals", collection: "vitals",
		listLandmark: `region[name="Measurements"]`, detailLandmark: `article[name="Measurement set"]`,
	},
	{
		kind: Immunization, segment: "immunizations", collection: "immunizations",
		listLandmark: `region[name="Vaccinations"]`, detailLandmark: `article[name="Vaccination"]`,
	},
	{
		kind: Injury, segment: "injuries", collection: "injuries",
		listLandmark: `region[name="Injuries"]`, detailLandmark: `article[name="Injury"]`,
	},
	{
		kind: Insurance, segment: "insurance", collection: "insurances",
		listLandmark: `region[name="Insurance"]`, detailLandmark: `article[name="Insurance policy"]`,
	},
	{
		kind: Equipment, segment: "equipment", collection: "equipment",
		listLandmark: `region[name="Equipment"]`, detailLandmark: `article[name="Equipment"]`,
	},
	{
		kind: EmergencyContact, segment: "emergency-contacts", collection: "emergency_contacts",
		listLandmark: `region[name="Emergency contacts"]`, detailLandmark: `article[name="Emergency contact"]`,
	},
	{
		kind: FamilyMember, segment: "family-history", collection: "family_members",
		listLandmark: `region[name="Family history"]`, detailLandmark: `article[name="Relative"]`,
	},
}

var (
	byKind       = make(map[Kind]entry, len(registry))
	bySegment    = make(map[string]Kind, len(registry))
	byCollection = make(map[string]Kind, len(registry))
	ordered      = make([]Kind, 0, len(registry))
)

// A duplicate is a wiring mistake that would make one of the two kinds
// unreachable or point it at the other's rows. kind_test.go asserts the same
// thing; this refuses to boot at all, because the second reader of this file is
// a phase-003 author adding thirteen rows in one sitting.
func init() {
	for _, declared := range registry {
		if _, duplicate := byKind[declared.kind]; duplicate {
			panic("kind: " + string(declared.kind) + " is declared twice")
		}
		if other, duplicate := bySegment[declared.segment]; duplicate {
			panic("kind: " + string(declared.kind) + " and " + string(other) + " share the path segment " + declared.segment)
		}
		if other, duplicate := byCollection[declared.collection]; duplicate {
			panic("kind: " + string(declared.kind) + " and " + string(other) + " share the collection " + declared.collection)
		}
		byKind[declared.kind] = declared
		bySegment[declared.segment] = declared.kind
		byCollection[declared.collection] = declared.kind
		ordered = append(ordered, declared.kind)
	}
}

// Kinds returns every declared kind in declaration order. It is a copy: the
// registry is read by the router, the OpenAPI builder and the migrations, and
// none of them may reorder it for the others.
func Kinds() []Kind {
	return append([]Kind(nil), ordered...)
}

// Enum is the value as it is stored, published in OpenAPI and written into an
// audit row.
func (k Kind) Enum() string { return string(k) }

// Segment is the URL spelling: /api/v1/records/{segment} and /{segment}.
// An undeclared kind has none, and the generic handler answers 404 rather than
// inventing one.
func (k Kind) Segment() string { return byKind[k].segment }

func (k Kind) Collection() string { return byKind[k].collection }

// ListLandmark and DetailLandmark are the two ARIA landmarks a kind's list and
// detail pages assert, spelled `region[name="…"]` and `article[name="…"]`.
func (k Kind) ListLandmark() string   { return byKind[k].listLandmark }
func (k Kind) DetailLandmark() string { return byKind[k].detailLandmark }

func (k Kind) Valid() bool {
	_, declared := byKind[k]
	return declared
}

// FromEnum and the two lookups below it answer with the zero Kind when they do
// not know the spelling, so a caller that ignores the second return value gets
// something that fails Valid() rather than something that looks declared.
func FromEnum(enum string) (Kind, bool) {
	if k := Kind(enum); k.Valid() {
		return k, true
	}
	return "", false
}

// FromSegment is exact. PocketBase has done no trailing-slash or case
// normalisation since v0.23, so `/records/Medications` is a different path and
// answering it here would create a second spelling of the kind.
func FromSegment(segment string) (Kind, bool) {
	k, known := bySegment[segment]
	return k, known
}

func FromCollection(collection string) (Kind, bool) {
	k, known := byCollection[collection]
	return k, known
}
