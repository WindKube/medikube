package clinical

import "slices"

// MedicationType is FR-016's kind of medication. The set is closed: a value
// outside it is refused rather than stored as free text, at the domain layer
// here and at the storage layer by a SelectField carrying the same spellings.
type MedicationType string

const (
	MedicationTypePrescription MedicationType = "prescription"
	MedicationTypeOTC          MedicationType = "otc"
	MedicationTypeSupplement   MedicationType = "supplement"
	MedicationTypeHerbal       MedicationType = "herbal"
)

// MedicationRoute is the route of administration. The fourteen values are
// verbatim from SHARED-DESIGN §1.4 so phase 003 inherits the vocabulary
// unchanged rather than publishing a second, subtly different one.
type MedicationRoute string

const (
	MedicationRouteOral          MedicationRoute = "oral"
	MedicationRouteSublingual    MedicationRoute = "sublingual"
	MedicationRouteTopical       MedicationRoute = "topical"
	MedicationRouteTransdermal   MedicationRoute = "transdermal"
	MedicationRouteInhalation    MedicationRoute = "inhalation"
	MedicationRouteNasal         MedicationRoute = "nasal"
	MedicationRouteOphthalmic    MedicationRoute = "ophthalmic"
	MedicationRouteOtic          MedicationRoute = "otic"
	MedicationRouteRectal        MedicationRoute = "rectal"
	MedicationRouteVaginal       MedicationRoute = "vaginal"
	MedicationRouteIntramuscular MedicationRoute = "intramuscular"
	MedicationRouteSubcutaneous  MedicationRoute = "subcutaneous"
	MedicationRouteIntravenous   MedicationRoute = "intravenous"
	MedicationRouteOther         MedicationRoute = "other"
)

// TherapyStatus is the course-of-therapy ladder. It is named for the shape and
// not for the kind because phase 003 reuses it for treatments and equipment;
// naming it MedicationStatus would force a duplicate vocabulary then.
//
// It is not a state machine. A person may move a medication back from stopped
// to active to correct a mistake, and no requirement forbids it.
type TherapyStatus string

const (
	TherapyStatusActive    TherapyStatus = "active"
	TherapyStatusOnHold    TherapyStatus = "on_hold"
	TherapyStatusCompleted TherapyStatus = "completed"
	TherapyStatusStopped   TherapyStatus = "stopped"
	TherapyStatusCancelled TherapyStatus = "cancelled"
)

// One declaration per vocabulary, in the order the form offers it. Valid() and
// the accessor read the same slice, so a value cannot be accepted without being
// offered or offered without being accepted.
var (
	medicationTypes = []MedicationType{
		MedicationTypePrescription,
		MedicationTypeOTC,
		MedicationTypeSupplement,
		MedicationTypeHerbal,
	}

	medicationRoutes = []MedicationRoute{
		MedicationRouteOral,
		MedicationRouteSublingual,
		MedicationRouteTopical,
		MedicationRouteTransdermal,
		MedicationRouteInhalation,
		MedicationRouteNasal,
		MedicationRouteOphthalmic,
		MedicationRouteOtic,
		MedicationRouteRectal,
		MedicationRouteVaginal,
		MedicationRouteIntramuscular,
		MedicationRouteSubcutaneous,
		MedicationRouteIntravenous,
		MedicationRouteOther,
	}

	therapyStatuses = []TherapyStatus{
		TherapyStatusActive,
		TherapyStatusOnHold,
		TherapyStatusCompleted,
		TherapyStatusStopped,
		TherapyStatusCancelled,
	}
)

// MedicationTypes is the published set of kinds, in the order the form offers
// them. It clones, as the two accessors below do, because a caller that sorted
// the result for one display would otherwise reorder every form, every OpenAPI
// enum and every Valid() along with it.
func MedicationTypes() []MedicationType { return slices.Clone(medicationTypes) }

func MedicationRoutes() []MedicationRoute { return slices.Clone(medicationRoutes) }
func TherapyStatuses() []TherapyStatus    { return slices.Clone(therapyStatuses) }

// Valid is false for the empty string on all three. Absence is an optional
// field's business and belongs to Validate, which can tell "not filled in" from
// "filled in with something we do not publish" and reports them differently.
func (t MedicationType) Valid() bool  { return slices.Contains(medicationTypes, t) }
func (r MedicationRoute) Valid() bool { return slices.Contains(medicationRoutes, r) }
func (s TherapyStatus) Valid() bool   { return slices.Contains(therapyStatuses, s) }
