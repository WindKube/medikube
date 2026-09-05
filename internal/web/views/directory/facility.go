package directory

import (
	"medikube/internal/domain/directory"
	"medikube/internal/web/views/records"
)

// FacilitySegment is contracts/pages.md's P5/P6 path segment.
const FacilitySegment = "facilities"

const (
	FieldFacilityKind         = "kind"
	FieldFacilityName         = "name"
	FieldFacilityBrand        = "brand"
	FieldFacilityStreet       = "street"
	FieldFacilityCity         = "city"
	FieldFacilityRegion       = "region"
	FieldFacilityPostalCode   = "postal_code"
	FieldFacilityCountry      = "country"
	FieldFacilityPhone        = "phone"
	FieldFacilityFax          = "fax"
	FieldFacilityEmail        = "email"
	FieldFacilityWebsite      = "website"
	FieldFacilityPortalURL    = "portal_url"
	FieldFacilityHours        = "hours"
	FieldFacilityOpen24h      = "open_24h"
	FieldFacilityDriveThrough = "drive_through"
	FieldFacilityServices     = "services"
	FieldFacilityNotes        = "notes"
)

var facilityFieldLabels = map[string]string{
	FieldFacilityKind:         "Kind",
	FieldFacilityName:         "Name",
	FieldFacilityBrand:        "Brand",
	FieldFacilityStreet:       "Street",
	FieldFacilityCity:         "City",
	FieldFacilityRegion:       "Region",
	FieldFacilityPostalCode:   "Postal code",
	FieldFacilityCountry:      "Country",
	FieldFacilityPhone:        "Phone",
	FieldFacilityFax:          "Fax",
	FieldFacilityEmail:        "Email",
	FieldFacilityWebsite:      "Website",
	FieldFacilityPortalURL:    "Patient portal",
	FieldFacilityHours:        "Hours",
	FieldFacilityOpen24h:      "Open 24 hours",
	FieldFacilityDriveThrough: "Drive-through",
	FieldFacilityServices:     "Services",
	FieldFacilityNotes:        "Notes",
}

func FacilityFieldLabel(field string) string {
	if label, known := facilityFieldLabels[field]; known {
		return label
	}

	return field
}

var facilityKindLabels = map[directory.FacilityKind]string{
	directory.FacilityKindPractice: "Practice",
	directory.FacilityKindPharmacy: "Pharmacy",
	directory.FacilityKindHospital: "Hospital",
	directory.FacilityKindLab:      "Laboratory",
	directory.FacilityKindImaging:  "Imaging centre",
	directory.FacilityKindOther:    "Other",
}

func FacilityKindLabel(value directory.FacilityKind) string {
	if value == "" {
		return ""
	}

	if label, known := facilityKindLabels[value]; known {
		return label
	}

	return string(value)
}

func FacilityKindOptions(selected directory.FacilityKind) []records.Option {
	published := directory.FacilityKinds()
	options := make([]records.Option, 0, len(published))

	for _, value := range published {
		options = append(options, records.Option{
			Value:    string(value),
			Label:    FacilityKindLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

type FacilityLinks struct {
	Detail string
	Record string
}

type FacilityView struct {
	ID           string
	Kind         string
	KindValue    string
	Name         string
	Brand        string
	Street       string
	City         string
	Region       string
	PostalCode   string
	Country      string
	Phone        string
	Fax          string
	Email        string
	Website      string
	PortalURL    string
	Hours        string
	Open24h      bool
	DriveThrough bool
	Services     string
	Notes        string

	UsagePractitioners int
	UsageRecords       int

	Version string
	Links   FacilityLinks
}

func NewFacilityView(f directory.Facility, links FacilityLinks) FacilityView {
	return FacilityView{
		ID:           f.ID,
		Kind:         FacilityKindLabel(f.Kind),
		KindValue:    string(f.Kind),
		Name:         f.Name,
		Brand:        f.Brand,
		Street:       f.Street,
		City:         f.City,
		Region:       f.Region,
		PostalCode:   f.PostalCode,
		Country:      f.Country,
		Phone:        f.Phone,
		Fax:          f.Fax,
		Email:        f.Email,
		Website:      f.Website,
		PortalURL:    f.PortalURL,
		Hours:        f.Hours,
		Open24h:      f.Open24h,
		DriveThrough: f.DriveThrough,
		Services:     f.Services,
		Notes:        f.Notes,
		Version:      f.Version,
		Links:        links,
	}
}

func (f FacilityView) Value(field string) string {
	switch field {
	case FieldFacilityKind:
		return f.KindValue
	case FieldFacilityName:
		return f.Name
	case FieldFacilityBrand:
		return f.Brand
	case FieldFacilityStreet:
		return f.Street
	case FieldFacilityCity:
		return f.City
	case FieldFacilityRegion:
		return f.Region
	case FieldFacilityPostalCode:
		return f.PostalCode
	case FieldFacilityCountry:
		return f.Country
	case FieldFacilityPhone:
		return f.Phone
	case FieldFacilityFax:
		return f.Fax
	case FieldFacilityEmail:
		return f.Email
	case FieldFacilityWebsite:
		return f.Website
	case FieldFacilityPortalURL:
		return f.PortalURL
	case FieldFacilityHours:
		return f.Hours
	case FieldFacilityServices:
		return f.Services
	case FieldFacilityNotes:
		return f.Notes
	default:
		return ""
	}
}

func (f FacilityView) KindOptions() []records.Option {
	return FacilityKindOptions(directory.FacilityKind(f.KindValue))
}

func (f FacilityView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldFacilityKind, Value: f.Kind},
		{Field: FieldFacilityBrand, Value: f.Brand},
		{Field: FieldFacilityStreet, Value: f.Street},
		{Field: FieldFacilityCity, Value: f.City},
		{Field: FieldFacilityRegion, Value: f.Region},
		{Field: FieldFacilityPostalCode, Value: f.PostalCode},
		{Field: FieldFacilityCountry, Value: f.Country},
		{Field: FieldFacilityPhone, Value: f.Phone},
		{Field: FieldFacilityFax, Value: f.Fax},
		{Field: FieldFacilityEmail, Value: f.Email},
		{Field: FieldFacilityWebsite, Value: f.Website},
		{Field: FieldFacilityPortalURL, Value: f.PortalURL},
		{Field: FieldFacilityHours, Value: f.Hours},
		{Field: FieldFacilityServices, Value: f.Services, Multiline: true},
		{Field: FieldFacilityNotes, Value: f.Notes, Multiline: true},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}

		entry.Label = FacilityFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

type FacilityListProps struct {
	Facilities []FacilityView
	CreateHref string
	NextHref   string
}

type FacilityDetailProps struct {
	Facility FacilityView
}

type FacilityFormProps struct {
	FormID     string
	New        bool
	OnSubmit   string
	CancelHref string
	Facility   FacilityView
	Errors     FieldErrors

	// Notice is set when the form was re-rendered from the server's current
	// values after a stale If-Match.
	Notice string
}

func (f FacilityFormProps) Label() string {
	if f.New {
		return "Add a facility"
	}

	return "Edit facility"
}

func (f FacilityFormProps) SubmitLabel() string {
	if f.New {
		return "Add facility"
	}

	return "Save changes"
}
