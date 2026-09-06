package api

import (
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/kinds"
	"medikube/internal/service/immunization"
	"medikube/internal/web"
)

// The wire spellings of every immunization member, mirroring dto_medication.go's
// MemberX constants.
const (
	ImmunizationMemberID             = "id"
	ImmunizationMemberKind           = "kind"
	ImmunizationMemberPatient        = "patient"
	ImmunizationMemberPractitioner   = "practitioner"
	ImmunizationMemberFacility       = "facility"
	ImmunizationMemberVaccineName    = "vaccine_name"
	ImmunizationMemberTradeName      = "trade_name"
	ImmunizationMemberAdministeredOn = "administered_on"
	ImmunizationMemberDoseNumber     = "dose_number"
	ImmunizationMemberLotNumber      = "lot_number"
	ImmunizationMemberManufacturer   = "manufacturer"
	ImmunizationMemberSite           = "site"
	ImmunizationMemberRoute          = "route"
	ImmunizationMemberExpiresOn      = "expires_on"
	ImmunizationMemberCreatedAt      = "created_at"
	ImmunizationMemberUpdatedAt      = "updated_at"
)

// immunizationMembers is data-model §4.8's column order: the order
// clinical.Immunization.Validate checks the rules in and the order refusals
// are sorted into before they leave.
var immunizationMembers = []string{
	ImmunizationMemberPatient,
	ImmunizationMemberVaccineName,
	ImmunizationMemberTradeName,
	ImmunizationMemberAdministeredOn,
	ImmunizationMemberDoseNumber,
	ImmunizationMemberLotNumber,
	ImmunizationMemberManufacturer,
	ImmunizationMemberSite,
	ImmunizationMemberRoute,
	ImmunizationMemberExpiresOn,
	ImmunizationMemberPractitioner,
	ImmunizationMemberFacility,
}

// ImmunizationSummary is what the list operations return.
type ImmunizationSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	VaccineName    string   `json:"vaccine_name"`
	AdministeredOn *string  `json:"administered_on"`
	DoseNumber     *int     `json:"dose_number,omitempty"`
	UpdatedAt      string   `json:"updated_at"`
	Basis          []string `json:"basis"`
}

func (s *ImmunizationSummary) SetBasis(basis []string) { s.Basis = basis }

// Immunization is what the detail operations return: every recorded field of
// data-model §4.8 plus the created and last-changed instants.
type Immunization struct {
	ImmunizationSummary

	Patient      string `json:"patient"`
	Practitioner string `json:"practitioner,omitempty"`
	Facility     string `json:"facility,omitempty"`

	TradeName    string   `json:"trade_name,omitempty"`
	LotNumber    string   `json:"lot_number,omitempty"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Site         string   `json:"site,omitempty"`
	Route        string   `json:"route,omitempty"`
	ExpiresOn    *string  `json:"expires_on"`
	Tags         []string `json:"tags,omitempty"`
	CreatedAt    string   `json:"created_at"`
}

// GetTags implements records.Tagged so the search index stays in step with
// this record's tags on write (T164-T177 follow-up).
func (i *Immunization) GetTags() []string { return i.Tags }

// ImmunizationCreate is the create body. FR-039: DoseNumber is a plain *int —
// absent or zero means "not recorded" (zeroIsAbsent: an empty number control
// submits 0), and a negative is refused by clinical.Immunization.Validate.
type ImmunizationCreate struct {
	Patient        string   `json:"patient"`
	VaccineName    string   `json:"vaccine_name"`
	TradeName      string   `json:"trade_name,omitempty"`
	AdministeredOn *string  `json:"administered_on,omitempty"`
	DoseNumber     *int     `json:"dose_number,omitempty"`
	LotNumber      string   `json:"lot_number,omitempty"`
	Manufacturer   string   `json:"manufacturer,omitempty"`
	Site           string   `json:"site,omitempty"`
	Route          string   `json:"route,omitempty"`
	ExpiresOn      *string  `json:"expires_on,omitempty"`
	Practitioner   *string  `json:"practitioner,omitempty"`
	Facility       *string  `json:"facility,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *ImmunizationCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

// ImmunizationPatch is the partial update. AdministeredOn and ExpiresOn go
// through web.Optional the same way medication's two dates do (present with
// a value is a new day, present-and-null clears it, absent is silence).
// DoseNumber stays a plain *int: supplying it always sets it, and there is no
// stated requirement to distinguish "clear" from "set to absent" for it.
type ImmunizationPatch struct {
	VaccineName *string `json:"vaccine_name,omitempty"`
	TradeName   *string `json:"trade_name,omitempty"`

	AdministeredOn web.Optional[string] `json:"administered_on,omitzero"`

	DoseNumber   *int    `json:"dose_number,omitempty"`
	LotNumber    *string `json:"lot_number,omitempty"`
	Manufacturer *string `json:"manufacturer,omitempty"`
	Site         *string `json:"site,omitempty"`
	Route        *string `json:"route,omitempty"`

	ExpiresOn web.Optional[string] `json:"expires_on,omitzero"`

	Practitioner *string `json:"practitioner,omitempty"`
	Facility     *string `json:"facility,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *ImmunizationPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// ImmunizationCodec is the DTO boundary for immunizations: the only place a
// clinical.Immunization becomes a wire shape and the only place a wire shape
// becomes one.
type ImmunizationCodec struct{}

var _ kinds.Codec = ImmunizationCodec{}

// ImmunizationSchema is the four constructors the registry publishes.
func ImmunizationSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(ImmunizationSummary) },
		NewDetail:  func() any { return new(Immunization) },
		NewCreate:  func() any { return new(ImmunizationCreate) },
		NewPatch:   func() any { return new(ImmunizationPatch) },
	}
}

// ImmunizationSearchFields reads the two search_index columns off the wire
// DTO Record.Body carries: the vaccine name, and the trade name, lot number
// and manufacturer as the free-text detail (research D-11, mirroring
// MedicationSearchFields's name-plus-details precedent).
func ImmunizationSearchFields(body any) (title, text string) {
	immunization, ok := body.(*Immunization)
	if !ok {
		return "", ""
	}

	return immunization.VaccineName, immunization.TradeName + " " + immunization.Manufacturer + " " + immunization.LotNumber
}

// ImmunizationBasis narrows nothing: immunization publishes no filter beyond
// search and sort, so there is no per-row distinction to state a reason for.
// It is declared anyway, so the registry's completeness check has something
// to find.
func ImmunizationBasis(any, records.Criteria) []string { return nil }

// Summary renders the list shape.
func (ImmunizationCodec) Summary(i clinical.Immunization) any {
	return &ImmunizationSummary{
		ID:             i.ID,
		Kind:           kind.Immunization.Enum(),
		VaccineName:    i.VaccineName,
		AdministeredOn: wireDate(i.AdministeredOn),
		DoseNumber:     i.DoseNumber,
		UpdatedAt:      wireInstant(i.UpdatedAt),
	}
}

// Detail renders the full shape.
func (c ImmunizationCodec) Detail(i clinical.Immunization) any {
	summary, ok := c.Summary(i).(*ImmunizationSummary)
	if !ok {
		// Unreachable while Summary returns what it says it does.
		return &Immunization{}
	}

	return &Immunization{
		ImmunizationSummary: *summary,
		Patient:             i.PatientID,
		Practitioner:        i.PractitionerID,
		Facility:            i.FacilityID,
		TradeName:           i.TradeName,
		LotNumber:           i.LotNumber,
		Manufacturer:        i.Manufacturer,
		Site:                string(i.Site),
		Route:               string(i.Route),
		ExpiresOn:           wireDate(i.ExpiresOn),
		Tags:                i.Tags,
		CreatedAt:           wireInstant(i.CreatedAt),
	}
}

// Draft reads a create body.
func (ImmunizationCodec) Draft(body any) (clinical.Immunization, error) {
	create, ok := body.(*ImmunizationCreate)
	if !ok {
		return clinical.Immunization{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	administeredOn := readDate(&invalid, ImmunizationMemberAdministeredOn, create.AdministeredOn)
	expiresOn := readDate(&invalid, ImmunizationMemberExpiresOn, create.ExpiresOn)

	if err := orderedImmunizationRefusal(&invalid); err != nil {
		return clinical.Immunization{}, err
	}

	return clinical.Immunization{
		PatientID:      create.Patient,
		VaccineName:    create.VaccineName,
		TradeName:      create.TradeName,
		AdministeredOn: administeredOn,
		DoseNumber:     zeroIsAbsent(create.DoseNumber),
		LotNumber:      create.LotNumber,
		Manufacturer:   create.Manufacturer,
		Site:           clinical.ImmunizationSite(create.Site),
		Route:          clinical.ImmunizationRoute(create.Route),
		ExpiresOn:      expiresOn,
		PractitionerID: deref(create.Practitioner),
		FacilityID:     deref(create.Facility),
		Tags:           create.Tags,
	}, nil
}

// Patch reads an update body.
func (ImmunizationCodec) Patch(body any) (immunization.Patch, error) {
	incoming, ok := body.(*ImmunizationPatch)
	if !ok {
		return immunization.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := immunization.Patch{
		VaccineName:    incoming.VaccineName,
		TradeName:      incoming.TradeName,
		AdministeredOn: readOptionalDate(&invalid, ImmunizationMemberAdministeredOn, incoming.AdministeredOn),
		DoseNumber:     zeroClears(doubleIntPointer(incoming.DoseNumber)),
		LotNumber:      incoming.LotNumber,
		Manufacturer:   incoming.Manufacturer,
		Site:           convert[clinical.ImmunizationSite](incoming.Site),
		Route:          convert[clinical.ImmunizationRoute](incoming.Route),
		ExpiresOn:      readOptionalDate(&invalid, ImmunizationMemberExpiresOn, incoming.ExpiresOn),
		Practitioner:   incoming.Practitioner,
		Facility:       incoming.Facility,
		Tags:           incoming.Tags,
	}

	if err := orderedImmunizationRefusal(&invalid); err != nil {
		return immunization.Patch{}, err
	}

	return patch, nil
}

// doubleIntPointer lifts a supplied *int into immunization.Patch.DoseNumber's
// **int: a non-nil incoming pointer, whatever it points at, becomes "set the
// dose to this"; a nil one leaves the stored dose alone.
func doubleIntPointer(value *int) **int {
	if value == nil {
		return nil
	}

	return &value
}

// orderedImmunizationRefusal sorts the refusals into data-model §4.8's column
// order, the same discipline orderedRefusal applies for medication.
func orderedImmunizationRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(immunizationMembers, left.Field) - slices.Index(immunizationMembers, right.Field)
	})

	return invalid.OrNil()
}
