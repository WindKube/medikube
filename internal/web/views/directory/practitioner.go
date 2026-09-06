package directory

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
	"medikube/internal/i18n"
	"medikube/internal/web/views/components"
	"medikube/internal/web/views/records"
)

// PractitionerSegment is contracts/pages.md's P3/P4 path segment. Practitioners
// are not a kind.Kind (research D-05), so it is spelled here rather than read
// off internal/domain/kind.
const PractitionerSegment = "practitioners"

const (
	FieldPractitionerName      = "name"
	FieldPractitionerSpecialty = "specialty"
	FieldPractitionerFacility  = "facility"
	FieldPractitionerPhone     = "phone"
	FieldPractitionerEmail     = "email"
	FieldPractitionerWebsite   = "website"
	FieldPractitionerNotes     = "notes"
)

var practitionerFieldLabelIDs = map[string]string{
	FieldPractitionerName:      "field.practitioner.name",
	FieldPractitionerSpecialty: "field.practitioner.specialty",
	FieldPractitionerFacility:  "field.practitioner.facility",
	FieldPractitionerPhone:     "field.practitioner.phone",
	FieldPractitionerEmail:     "field.practitioner.email",
	FieldPractitionerWebsite:   "field.practitioner.website",
	FieldPractitionerNotes:     "field.practitioner.notes",
}

func PractitionerFieldLabel(ctx context.Context, field string) string {
	if id, known := practitionerFieldLabelIDs[field]; known {
		return i18n.T(ctx, id)
	}

	return field
}

// SpecialtyLabel renders the closed specialty vocabulary's own catalogue
// entry (enum.specialty.<value>), one literal i18n.T call site per value so
// reference_test.go's D-08 scan can see each one.
func SpecialtyLabel(ctx context.Context, value directory.Specialty) string {
	switch value {
	case "":
		return ""
	case directory.SpecialtyAllergyImmunology:
		return i18n.T(ctx, "enum.specialty.allergy_immunology")
	case directory.SpecialtyAnesthesiology:
		return i18n.T(ctx, "enum.specialty.anesthesiology")
	case directory.SpecialtyCardiology:
		return i18n.T(ctx, "enum.specialty.cardiology")
	case directory.SpecialtyDentistry:
		return i18n.T(ctx, "enum.specialty.dentistry")
	case directory.SpecialtyDermatology:
		return i18n.T(ctx, "enum.specialty.dermatology")
	case directory.SpecialtyEmergencyMedicine:
		return i18n.T(ctx, "enum.specialty.emergency_medicine")
	case directory.SpecialtyEndocrinology:
		return i18n.T(ctx, "enum.specialty.endocrinology")
	case directory.SpecialtyFamilyMedicine:
		return i18n.T(ctx, "enum.specialty.family_medicine")
	case directory.SpecialtyGastroenterology:
		return i18n.T(ctx, "enum.specialty.gastroenterology")
	case directory.SpecialtyGeneralSurgery:
		return i18n.T(ctx, "enum.specialty.general_surgery")
	case directory.SpecialtyGenetics:
		return i18n.T(ctx, "enum.specialty.genetics")
	case directory.SpecialtyGeriatrics:
		return i18n.T(ctx, "enum.specialty.geriatrics")
	case directory.SpecialtyGynecology:
		return i18n.T(ctx, "enum.specialty.gynecology")
	case directory.SpecialtyHematology:
		return i18n.T(ctx, "enum.specialty.hematology")
	case directory.SpecialtyHepatology:
		return i18n.T(ctx, "enum.specialty.hepatology")
	case directory.SpecialtyInfectiousDisease:
		return i18n.T(ctx, "enum.specialty.infectious_disease")
	case directory.SpecialtyInternalMedicine:
		return i18n.T(ctx, "enum.specialty.internal_medicine")
	case directory.SpecialtyNephrology:
		return i18n.T(ctx, "enum.specialty.nephrology")
	case directory.SpecialtyNeurology:
		return i18n.T(ctx, "enum.specialty.neurology")
	case directory.SpecialtyNeurosurgery:
		return i18n.T(ctx, "enum.specialty.neurosurgery")
	case directory.SpecialtyNutrition:
		return i18n.T(ctx, "enum.specialty.nutrition")
	case directory.SpecialtyObstetrics:
		return i18n.T(ctx, "enum.specialty.obstetrics")
	case directory.SpecialtyOccupationalTherapy:
		return i18n.T(ctx, "enum.specialty.occupational_therapy")
	case directory.SpecialtyOncology:
		return i18n.T(ctx, "enum.specialty.oncology")
	case directory.SpecialtyOphthalmology:
		return i18n.T(ctx, "enum.specialty.ophthalmology")
	case directory.SpecialtyOptometry:
		return i18n.T(ctx, "enum.specialty.optometry")
	case directory.SpecialtyOralSurgery:
		return i18n.T(ctx, "enum.specialty.oral_surgery")
	case directory.SpecialtyOrthopedics:
		return i18n.T(ctx, "enum.specialty.orthopedics")
	case directory.SpecialtyOtolaryngology:
		return i18n.T(ctx, "enum.specialty.otolaryngology")
	case directory.SpecialtyPainMedicine:
		return i18n.T(ctx, "enum.specialty.pain_medicine")
	case directory.SpecialtyPalliativeCare:
		return i18n.T(ctx, "enum.specialty.palliative_care")
	case directory.SpecialtyPathology:
		return i18n.T(ctx, "enum.specialty.pathology")
	case directory.SpecialtyPediatrics:
		return i18n.T(ctx, "enum.specialty.pediatrics")
	case directory.SpecialtyPhysicalTherapy:
		return i18n.T(ctx, "enum.specialty.physical_therapy")
	case directory.SpecialtyPlasticSurgery:
		return i18n.T(ctx, "enum.specialty.plastic_surgery")
	case directory.SpecialtyPodiatry:
		return i18n.T(ctx, "enum.specialty.podiatry")
	case directory.SpecialtyPsychiatry:
		return i18n.T(ctx, "enum.specialty.psychiatry")
	case directory.SpecialtyPsychology:
		return i18n.T(ctx, "enum.specialty.psychology")
	case directory.SpecialtyPulmonology:
		return i18n.T(ctx, "enum.specialty.pulmonology")
	case directory.SpecialtyRadiology:
		return i18n.T(ctx, "enum.specialty.radiology")
	case directory.SpecialtyRheumatology:
		return i18n.T(ctx, "enum.specialty.rheumatology")
	case directory.SpecialtyUrology:
		return i18n.T(ctx, "enum.specialty.urology")
	case directory.SpecialtyOther:
		return i18n.T(ctx, "enum.specialty.other")
	default:
		return string(value)
	}
}

// SpecialtyOptions offers the vocabulary in the order it is published, with an
// unset "not recorded" entry the caller adds.
func SpecialtyOptions(ctx context.Context, selected directory.Specialty) []records.Option {
	published := directory.Specialties()
	options := make([]records.Option, 0, len(published))

	for _, value := range published {
		options = append(options, records.Option{
			Value:    string(value),
			Label:    SpecialtyLabel(ctx, value),
			Selected: value == selected,
		})
	}

	return options
}

// PractitionerLinks are the URLs one practitioner's views address.
type PractitionerLinks struct {
	Detail string
	Record string
}

// PractitionerView is one practitioner as its views render it.
type PractitionerView struct {
	ID string

	Name           string
	Specialty      string
	SpecialtyValue string
	FacilityID     string
	FacilityName   string
	Phone          string
	Email          string
	Website        string
	Notes          string

	UsagePatients int
	UsageRecords  int

	Version string
	Links   PractitionerLinks
}

func NewPractitionerView(ctx context.Context, p directory.Practitioner, facilityName string, links PractitionerLinks) PractitionerView {
	return PractitionerView{
		ID:             p.ID,
		Name:           p.Name,
		Specialty:      SpecialtyLabel(ctx, p.Specialty),
		SpecialtyValue: string(p.Specialty),
		FacilityID:     p.FacilityID,
		FacilityName:   facilityName,
		Phone:          p.Phone,
		Email:          p.Email,
		Website:        p.Website,
		Notes:          p.Notes,
		Version:        p.Version,
		Links:          links,
	}
}

func (p PractitionerView) Value(field string) string {
	switch field {
	case FieldPractitionerName:
		return p.Name
	case FieldPractitionerSpecialty:
		return p.SpecialtyValue
	case FieldPractitionerFacility:
		return p.FacilityID
	case FieldPractitionerPhone:
		return p.Phone
	case FieldPractitionerEmail:
		return p.Email
	case FieldPractitionerWebsite:
		return p.Website
	case FieldPractitionerNotes:
		return p.Notes
	default:
		return ""
	}
}

func (p PractitionerView) SpecialtyOptions(ctx context.Context) []records.Option {
	return SpecialtyOptions(ctx, directory.Specialty(p.SpecialtyValue))
}

// DetailEntry reuses records.DetailEntry so the two directories and the
// medication detail share one rendering rule for "unrecorded means absent"
// (FR-024).
type DetailEntry = records.DetailEntry

func (p PractitionerView) Entries(ctx context.Context) []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldPractitionerSpecialty, Value: p.Specialty},
		{Field: FieldPractitionerFacility, Value: p.FacilityName},
		{Field: FieldPractitionerPhone, Value: p.Phone},
		{Field: FieldPractitionerEmail, Value: p.Email},
		{Field: FieldPractitionerWebsite, Value: p.Website},
		{Field: FieldPractitionerNotes, Value: p.Notes, Multiline: true},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = PractitionerFieldLabel(ctx, entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

type PractitionerListProps struct {
	Practitioners []PractitionerView
	CreateHref    string
	NextHref      string
}

type PractitionerDetailProps struct {
	Practitioner PractitionerView
}

type PractitionerFormProps struct {
	FormID       string
	New          bool
	OnSubmit     string
	CancelHref   string
	Practitioner PractitionerView
	Errors       FieldErrors

	// Notice is set when the form was re-rendered from the server's current
	// values after a stale If-Match.
	Notice string
}

func (p PractitionerFormProps) Label(ctx context.Context) string {
	if p.New {
		return i18n.T(ctx, "directory.add_practitioner")
	}

	return i18n.T(ctx, "directory.edit_practitioner")
}

func (p PractitionerFormProps) SubmitLabel(ctx context.Context) string {
	if p.New {
		return i18n.T(ctx, "directory.add_practitioner_submit")
	}

	return i18n.T(ctx, "action.save_changes")
}

// FieldErrors is components' own type, aliased here as records.go aliases it,
// so every form in the application answers the same question about a
// refusal's shape.
type FieldErrors = components.FieldErrors

func NewFieldErrors(invalid *domain.ValidationError) FieldErrors {
	return components.NewFieldErrors(invalid)
}
