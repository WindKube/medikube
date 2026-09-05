package directory

import (
	"medikube/internal/domain"
	"medikube/internal/domain/directory"
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

var practitionerFieldLabels = map[string]string{
	FieldPractitionerName:      "Name",
	FieldPractitionerSpecialty: "Specialty",
	FieldPractitionerFacility:  "Facility",
	FieldPractitionerPhone:     "Phone",
	FieldPractitionerEmail:     "Email",
	FieldPractitionerWebsite:   "Website",
	FieldPractitionerNotes:     "Notes",
}

func PractitionerFieldLabel(field string) string {
	if label, known := practitionerFieldLabels[field]; known {
		return label
	}

	return field
}

// SpecialtyLabel renders the enum's snake_case spelling as prose. Unknown to
// this map (there is none — the vocabulary is closed) falls back to the raw
// value, the same convention records.MedicationTypeLabel uses.
func SpecialtyLabel(value directory.Specialty) string {
	if value == "" {
		return ""
	}

	return specialtyLabels[value]
}

var specialtyLabels = buildSpecialtyLabels()

func buildSpecialtyLabels() map[directory.Specialty]string {
	labels := make(map[directory.Specialty]string, len(directory.Specialties()))
	for _, value := range directory.Specialties() {
		labels[value] = humanizeEnum(string(value))
	}

	return labels
}

// SpecialtyOptions offers the vocabulary in the order it is published, with an
// unset "not recorded" entry the caller adds.
func SpecialtyOptions(selected directory.Specialty) []records.Option {
	published := directory.Specialties()
	options := make([]records.Option, 0, len(published))

	for _, value := range published {
		options = append(options, records.Option{
			Value:    string(value),
			Label:    SpecialtyLabel(value),
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

func NewPractitionerView(p directory.Practitioner, facilityName string, links PractitionerLinks) PractitionerView {
	return PractitionerView{
		ID:             p.ID,
		Name:           p.Name,
		Specialty:      SpecialtyLabel(p.Specialty),
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

func (p PractitionerView) SpecialtyOptions() []records.Option {
	return SpecialtyOptions(directory.Specialty(p.SpecialtyValue))
}

// DetailEntry reuses records.DetailEntry so the two directories and the
// medication detail share one rendering rule for "unrecorded means absent"
// (FR-024).
type DetailEntry = records.DetailEntry

func (p PractitionerView) Entries() []DetailEntry {
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

		entry.Label = PractitionerFieldLabel(entry.Field)
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
}

func (p PractitionerFormProps) Label() string {
	if p.New {
		return "Add a practitioner"
	}

	return "Edit practitioner"
}

func (p PractitionerFormProps) SubmitLabel() string {
	if p.New {
		return "Add practitioner"
	}

	return "Save changes"
}

// FieldErrors is components' own type, aliased here as records.go aliases it,
// so every form in the application answers the same question about a
// refusal's shape.
type FieldErrors = components.FieldErrors

func NewFieldErrors(invalid *domain.ValidationError) FieldErrors {
	return components.NewFieldErrors(invalid)
}

// humanizeEnum turns "allergy_immunology" into "Allergy immunology" — good
// enough prose for a closed, compiled-in vocabulary with no punctuation to
// preserve.
func humanizeEnum(value string) string {
	out := []byte(value)
	for i, b := range out {
		if b == '_' {
			out[i] = ' '
		}
	}

	if len(out) > 0 && out[0] >= 'a' && out[0] <= 'z' {
		out[0] -= 'a' - 'A'
	}

	return string(out)
}
