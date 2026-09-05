package directory

import "slices"

// FacilityKind is FR-034's classification for a place of care. One collection
// covers all six kinds (research D-24) rather than a table per kind, so the
// vocabulary lives here instead of six.
type FacilityKind string

const (
	FacilityKindPractice FacilityKind = "practice"
	FacilityKindPharmacy FacilityKind = "pharmacy"
	FacilityKindHospital FacilityKind = "hospital"
	FacilityKindLab      FacilityKind = "lab"
	FacilityKindImaging  FacilityKind = "imaging"
	FacilityKindOther    FacilityKind = "other"
)

// Specialty is FR-032/FR-033's fixed medical specialty vocabulary. It is a Go
// enum and not a collection (research D-23): the only consumer is a
// server-rendered form, and the values are compiled into the binary.
//
// Stored as the empty string when unset, never NULL — research D-25 depends on
// that to make the (owner, LOWER(name), specialty) uniqueness index behave.
type Specialty string

const (
	SpecialtyAllergyImmunology   Specialty = "allergy_immunology"
	SpecialtyAnesthesiology      Specialty = "anesthesiology"
	SpecialtyCardiology          Specialty = "cardiology"
	SpecialtyDentistry           Specialty = "dentistry"
	SpecialtyDermatology         Specialty = "dermatology"
	SpecialtyEmergencyMedicine   Specialty = "emergency_medicine"
	SpecialtyEndocrinology       Specialty = "endocrinology"
	SpecialtyFamilyMedicine      Specialty = "family_medicine"
	SpecialtyGastroenterology    Specialty = "gastroenterology"
	SpecialtyGeneralSurgery      Specialty = "general_surgery"
	SpecialtyGenetics            Specialty = "genetics"
	SpecialtyGeriatrics          Specialty = "geriatrics"
	SpecialtyGynecology          Specialty = "gynecology"
	SpecialtyHematology          Specialty = "hematology"
	SpecialtyHepatology          Specialty = "hepatology"
	SpecialtyInfectiousDisease   Specialty = "infectious_disease"
	SpecialtyInternalMedicine    Specialty = "internal_medicine"
	SpecialtyNephrology          Specialty = "nephrology"
	SpecialtyNeurology           Specialty = "neurology"
	SpecialtyNeurosurgery        Specialty = "neurosurgery"
	SpecialtyNutrition           Specialty = "nutrition"
	SpecialtyObstetrics          Specialty = "obstetrics"
	SpecialtyOccupationalTherapy Specialty = "occupational_therapy"
	SpecialtyOncology            Specialty = "oncology"
	SpecialtyOphthalmology       Specialty = "ophthalmology"
	SpecialtyOptometry           Specialty = "optometry"
	SpecialtyOralSurgery         Specialty = "oral_surgery"
	SpecialtyOrthopedics         Specialty = "orthopedics"
	SpecialtyOtolaryngology      Specialty = "otolaryngology"
	SpecialtyPainMedicine        Specialty = "pain_medicine"
	SpecialtyPalliativeCare      Specialty = "palliative_care"
	SpecialtyPathology           Specialty = "pathology"
	SpecialtyPediatrics          Specialty = "pediatrics"
	SpecialtyPhysicalTherapy     Specialty = "physical_therapy"
	SpecialtyPlasticSurgery      Specialty = "plastic_surgery"
	SpecialtyPodiatry            Specialty = "podiatry"
	SpecialtyPsychiatry          Specialty = "psychiatry"
	SpecialtyPsychology          Specialty = "psychology"
	SpecialtyPulmonology         Specialty = "pulmonology"
	SpecialtyRadiology           Specialty = "radiology"
	SpecialtyRheumatology        Specialty = "rheumatology"
	SpecialtyUrology             Specialty = "urology"

	// SpecialtyOther is FR-033's mandated catch-all. It MUST NOT be removed:
	// the vocabulary is closed, and this is what keeps a practitioner whose
	// specialty is not otherwise listed representable at all.
	SpecialtyOther Specialty = "other"
)

// One declaration per vocabulary, in the order the form offers it. Valid() and
// the accessor read the same slice, so a value cannot be accepted without being
// offered or offered without being accepted.
var (
	facilityKinds = []FacilityKind{
		FacilityKindPractice,
		FacilityKindPharmacy,
		FacilityKindHospital,
		FacilityKindLab,
		FacilityKindImaging,
		FacilityKindOther,
	}

	// specialties is data-model §2's list verbatim (research D-23). The Go
	// slice and the generated select field are built from this single source,
	// so the two cannot disagree.
	specialties = []Specialty{
		SpecialtyAllergyImmunology, SpecialtyAnesthesiology, SpecialtyCardiology,
		SpecialtyDentistry, SpecialtyDermatology, SpecialtyEmergencyMedicine,
		SpecialtyEndocrinology, SpecialtyFamilyMedicine, SpecialtyGastroenterology,
		SpecialtyGeneralSurgery, SpecialtyGenetics, SpecialtyGeriatrics,
		SpecialtyGynecology, SpecialtyHematology, SpecialtyHepatology,
		SpecialtyInfectiousDisease, SpecialtyInternalMedicine, SpecialtyNephrology,
		SpecialtyNeurology, SpecialtyNeurosurgery, SpecialtyNutrition,
		SpecialtyObstetrics, SpecialtyOccupationalTherapy, SpecialtyOncology,
		SpecialtyOphthalmology, SpecialtyOptometry, SpecialtyOralSurgery,
		SpecialtyOrthopedics, SpecialtyOtolaryngology, SpecialtyPainMedicine,
		SpecialtyPalliativeCare, SpecialtyPathology, SpecialtyPediatrics,
		SpecialtyPhysicalTherapy, SpecialtyPlasticSurgery, SpecialtyPodiatry,
		SpecialtyPsychiatry, SpecialtyPsychology, SpecialtyPulmonology,
		SpecialtyRadiology, SpecialtyRheumatology, SpecialtyUrology,
		SpecialtyOther,
	}
)

// FacilityKinds is the published vocabulary, in the order the form offers it,
// and so is Specialties below. They clone, as every such accessor in this
// codebase does, because a caller that sorted the result for one display
// would otherwise reorder every form, every OpenAPI enum and every Valid()
// along with it.
func FacilityKinds() []FacilityKind { return slices.Clone(facilityKinds) }
func Specialties() []Specialty      { return slices.Clone(specialties) }

// Valid is false for the empty string on FacilityKind — the field is required.
// Specialty's empty string is valid on its own terms: research D-25 requires
// an unset specialty to be stored as ” and not NULL, so Valid here answers
// "is this a real vocabulary member", and Patient/Practitioner.Validate is
// what distinguishes "unset" from "set to something unpublished".
func (k FacilityKind) Valid() bool { return slices.Contains(facilityKinds, k) }
func (s Specialty) Valid() bool    { return slices.Contains(specialties, s) }
