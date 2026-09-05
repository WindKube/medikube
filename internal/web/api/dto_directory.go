package api

import (
	"medikube/internal/domain/directory"
	facilitysvc "medikube/internal/service/facility"
	practitionersvc "medikube/internal/service/practitioner"
	"medikube/internal/web"
)

// The wire spellings of every practitioner and facility member. Declared once,
// as dto_medication.go's own constants are, so a field renamed on the wire and
// not in a refusal is a compile error rather than a silently mismatched
// message.
const (
	MemberSpecialty    = "specialty"
	MemberFacility     = "facility"
	MemberPhone        = "phone"
	MemberEmail        = "email"
	MemberWebsite      = "website"
	MemberFacilityKind = "kind"
	MemberBrand        = "brand"
	MemberStreet       = "street"
	MemberCity         = "city"
	MemberRegion       = "region"
	MemberPostalCode   = "postal_code"
	MemberCountry      = "country"
	MemberFax          = "fax"
	MemberPortalURL    = "portal_url"
	MemberHours        = "hours"
	MemberOpen24h      = "open_24h"
	MemberDriveThrough = "drive_through"
	MemberServices     = "services"
)

// FacilityRef is what a practitioner's response names its facility with:
// enough to render a link and a badge, never the whole record.
type FacilityRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// PractitionerUsage answers FR-040 for one practitioner without a second round
// trip.
type PractitionerUsage struct {
	Patients int `json:"patients"`
	Records  int `json:"records"`
}

// PractitionerSummary is what listPractitioners returns.
type PractitionerSummary struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Specialty string       `json:"specialty,omitempty"`
	Facility  *FacilityRef `json:"facility"`
	UpdatedAt string       `json:"updated_at"`
}

// Practitioner is what getPractitioner and the two writes return.
type Practitioner struct {
	PractitionerSummary
	Phone   string            `json:"phone,omitempty"`
	Email   string            `json:"email,omitempty"`
	Website string            `json:"website,omitempty"`
	Notes   string            `json:"notes,omitempty"`
	Usage   PractitionerUsage `json:"usage"`
}

// PractitionerCreate is createPractitioner's body. There is no owner and no
// id: unknown members are rejected by the decoder, so a body naming either is
// refused before any handler decides anything (FR-032).
type PractitionerCreate struct {
	Name      string `json:"name"`
	Specialty string `json:"specialty,omitempty"`
	Facility  string `json:"facility,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Email     string `json:"email,omitempty"`
	Website   string `json:"website,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

// PractitionerPatch is updatePractitioner's body. Facility uses web.Optional,
// this codebase's mechanism for the three-state absent/clear/set the contract
// documents (in its own literal spelling) as `**string`.
type PractitionerPatch struct {
	Name      *string              `json:"name,omitempty"`
	Specialty *string              `json:"specialty,omitempty"`
	Facility  web.Optional[string] `json:"facility,omitzero"`
	Phone     *string              `json:"phone,omitempty"`
	Email     *string              `json:"email,omitempty"`
	Website   *string              `json:"website,omitempty"`
	Notes     *string              `json:"notes,omitempty"`
}

// FacilityUsage is a facility's own count: how many practitioners and
// medications point at it, which is what a delete would silently orphan.
type FacilityUsage struct {
	Practitioners int `json:"practitioners"`
	Records       int `json:"records"`
}

// FacilitySummary is what listFacilities returns.
type FacilitySummary struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Brand     string `json:"brand,omitempty"`
	City      string `json:"city,omitempty"`
	Phone     string `json:"phone,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// Facility is what getFacility and the two writes return.
type Facility struct {
	FacilitySummary
	Street       string        `json:"street,omitempty"`
	Region       string        `json:"region,omitempty"`
	PostalCode   string        `json:"postal_code,omitempty"`
	Country      string        `json:"country,omitempty"`
	Fax          string        `json:"fax,omitempty"`
	Email        string        `json:"email,omitempty"`
	Website      string        `json:"website,omitempty"`
	PortalURL    string        `json:"portal_url,omitempty"`
	Hours        string        `json:"hours,omitempty"`
	Open24h      bool          `json:"open_24h"`
	DriveThrough bool          `json:"drive_through"`
	Services     string        `json:"services,omitempty"`
	Notes        string        `json:"notes,omitempty"`
	Usage        FacilityUsage `json:"usage"`
}

// FacilityCreate is createFacility's body.
type FacilityCreate struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Brand        string `json:"brand,omitempty"`
	Street       string `json:"street,omitempty"`
	City         string `json:"city,omitempty"`
	Region       string `json:"region,omitempty"`
	PostalCode   string `json:"postal_code,omitempty"`
	Country      string `json:"country,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Fax          string `json:"fax,omitempty"`
	Email        string `json:"email,omitempty"`
	Website      string `json:"website,omitempty"`
	PortalURL    string `json:"portal_url,omitempty"`
	Hours        string `json:"hours,omitempty"`
	Open24h      bool   `json:"open_24h,omitempty"`
	DriveThrough bool   `json:"drive_through,omitempty"`
	Services     string `json:"services,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// FacilityPatch is updateFacility's body. Every field is a plain pointer:
// unlike a practitioner's facility, none of these has a third "clear it"
// state to carry — the empty string already means "unset" for every one of
// them.
type FacilityPatch struct {
	Kind         *string `json:"kind,omitempty"`
	Name         *string `json:"name,omitempty"`
	Brand        *string `json:"brand,omitempty"`
	Street       *string `json:"street,omitempty"`
	City         *string `json:"city,omitempty"`
	Region       *string `json:"region,omitempty"`
	PostalCode   *string `json:"postal_code,omitempty"`
	Country      *string `json:"country,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	Fax          *string `json:"fax,omitempty"`
	Email        *string `json:"email,omitempty"`
	Website      *string `json:"website,omitempty"`
	PortalURL    *string `json:"portal_url,omitempty"`
	Hours        *string `json:"hours,omitempty"`
	Open24h      *bool   `json:"open_24h,omitempty"`
	DriveThrough *bool   `json:"drive_through,omitempty"`
	Services     *string `json:"services,omitempty"`
	Notes        *string `json:"notes,omitempty"`
}

// facilityRefFor builds the ref a practitioner's response names its facility
// with, or nil when the practitioner has none.
func facilityRefFor(f *directory.Facility) *FacilityRef {
	if f == nil {
		return nil
	}

	return &FacilityRef{ID: f.ID, Name: f.Name, Kind: string(f.Kind)}
}

// practitionerSummary renders the list shape.
func practitionerSummary(p directory.Practitioner, facilityRef *FacilityRef) *PractitionerSummary {
	return &PractitionerSummary{
		ID:        p.ID,
		Name:      p.Name,
		Specialty: string(p.Specialty),
		Facility:  facilityRef,
		UpdatedAt: wireInstant(p.UpdatedAt),
	}
}

// practitionerDetail renders the full shape, usage included.
func practitionerDetail(p directory.Practitioner, facilityRef *FacilityRef, usage practitionersvc.Usage) *Practitioner {
	return &Practitioner{
		PractitionerSummary: *practitionerSummary(p, facilityRef),
		Phone:               p.Phone,
		Email:               p.Email,
		Website:             p.Website,
		Notes:               p.Notes,
		Usage:               PractitionerUsage{Patients: usage.Patients, Records: usage.Records},
	}
}

// practitionerDraft reads a create body into the domain entity. The four
// server-owned fields are left zero: the service overwrites them and the DTO
// has no member for any of them anyway.
func practitionerDraft(create PractitionerCreate) directory.Practitioner {
	return directory.Practitioner{
		Name:       create.Name,
		Specialty:  directory.Specialty(create.Specialty),
		FacilityID: create.Facility,
		Phone:      create.Phone,
		Email:      create.Email,
		Website:    create.Website,
		Notes:      create.Notes,
	}
}

// practitionerPatch reads an update body. Facility comes through web.Optional:
// absent leaves it alone, an explicit null clears it, a value sets it.
func practitionerPatch(incoming PractitionerPatch) practitionersvc.Patch {
	patch := practitionersvc.Patch{
		Name:      incoming.Name,
		Specialty: convert[directory.Specialty](incoming.Specialty),
		Phone:     incoming.Phone,
		Email:     incoming.Email,
		Website:   incoming.Website,
		Notes:     incoming.Notes,
	}

	if incoming.Facility.Present() {
		value, _ := incoming.Facility.Get()
		patch.FacilityID = &value
	}

	return patch
}

// facilitySummary renders the list shape.
func facilitySummary(f directory.Facility) *FacilitySummary {
	return &FacilitySummary{
		ID:        f.ID,
		Kind:      string(f.Kind),
		Name:      f.Name,
		Brand:     f.Brand,
		City:      f.City,
		Phone:     f.Phone,
		UpdatedAt: wireInstant(f.UpdatedAt),
	}
}

// facilityDetail renders the full shape, usage included.
func facilityDetail(f directory.Facility, usage facilitysvc.Usage) *Facility {
	return &Facility{
		FacilitySummary: *facilitySummary(f),
		Street:          f.Street,
		Region:          f.Region,
		PostalCode:      f.PostalCode,
		Country:         f.Country,
		Fax:             f.Fax,
		Email:           f.Email,
		Website:         f.Website,
		PortalURL:       f.PortalURL,
		Hours:           f.Hours,
		Open24h:         f.Open24h,
		DriveThrough:    f.DriveThrough,
		Services:        f.Services,
		Notes:           f.Notes,
		Usage:           FacilityUsage{Practitioners: usage.Practitioners, Records: usage.Records},
	}
}

func facilityDraft(create FacilityCreate) directory.Facility {
	return directory.Facility{
		Kind:         directory.FacilityKind(create.Kind),
		Name:         create.Name,
		Brand:        create.Brand,
		Street:       create.Street,
		City:         create.City,
		Region:       create.Region,
		PostalCode:   create.PostalCode,
		Country:      create.Country,
		Phone:        create.Phone,
		Fax:          create.Fax,
		Email:        create.Email,
		Website:      create.Website,
		PortalURL:    create.PortalURL,
		Hours:        create.Hours,
		Open24h:      create.Open24h,
		DriveThrough: create.DriveThrough,
		Services:     create.Services,
		Notes:        create.Notes,
	}
}

func facilityPatch(incoming FacilityPatch) facilitysvc.Patch {
	return facilitysvc.Patch{
		Kind:         convert[directory.FacilityKind](incoming.Kind),
		Name:         incoming.Name,
		Brand:        incoming.Brand,
		Street:       incoming.Street,
		City:         incoming.City,
		Region:       incoming.Region,
		PostalCode:   incoming.PostalCode,
		Country:      incoming.Country,
		Phone:        incoming.Phone,
		Fax:          incoming.Fax,
		Email:        incoming.Email,
		Website:      incoming.Website,
		PortalURL:    incoming.PortalURL,
		Hours:        incoming.Hours,
		Open24h:      incoming.Open24h,
		DriveThrough: incoming.DriveThrough,
		Services:     incoming.Services,
		Notes:        incoming.Notes,
	}
}

// wireInstant, convert and orderedRefusal are declared in dto_medication.go
// and shared as unexported package members: same package, same rendering
// rules, one definition.
