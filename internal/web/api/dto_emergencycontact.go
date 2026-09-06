package api

import (
	"fmt"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/emergencycontact"
)

const (
	MemberRelationship = "relationship"
	MemberPhoneAlt     = "phone_alt"
	MemberAddress      = "address"
	MemberIsPrimary    = "is_primary"
	MemberIsActive     = "is_active"
)

// DisplacedRef is research D-16's explanation: what a create or an update
// that set is_primary also changed.
type DisplacedRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type EmergencyContactSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name         string `json:"name"`
	Relationship string `json:"relationship"`
	Phone        string `json:"phone"`
	IsPrimary    bool   `json:"is_primary"`
	IsActive     bool   `json:"is_active"`
	UpdatedAt    string `json:"updated_at"`
}

type EmergencyContact struct {
	EmergencyContactSummary

	Patient  string `json:"patient"`
	PhoneAlt string `json:"phone_alt,omitempty"`
	Email    string `json:"email,omitempty"`
	Address  string `json:"address,omitempty"`
	Notes    string `json:"notes,omitempty"`

	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`

	// Displaced is set only on the create or the update response that caused
	// it (research D-16), never on a read.
	Displaced *DisplacedRef `json:"displaced,omitempty"`
}

type EmergencyContactCreate struct {
	Patient      string   `json:"patient"`
	Name         string   `json:"name"`
	Relationship string   `json:"relationship"`
	Phone        string   `json:"phone"`
	PhoneAlt     string   `json:"phone_alt,omitempty"`
	Email        string   `json:"email,omitempty"`
	Address      string   `json:"address,omitempty"`
	IsPrimary    bool     `json:"is_primary,omitempty"`
	IsActive     *bool    `json:"is_active,omitempty"`
	Notes        string   `json:"notes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *EmergencyContactCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

type EmergencyContactPatch struct {
	Name         *string `json:"name,omitempty"`
	Relationship *string `json:"relationship,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	PhoneAlt     *string `json:"phone_alt,omitempty"`
	Email        *string `json:"email,omitempty"`
	Address      *string `json:"address,omitempty"`
	IsPrimary    *bool   `json:"is_primary,omitempty"`
	IsActive     *bool   `json:"is_active,omitempty"`
	Notes        *string `json:"notes,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *EmergencyContactPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

type EmergencyContactCodec struct{}

func EmergencyContactSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(EmergencyContactSummary) },
		NewDetail:  func() any { return new(EmergencyContact) },
		NewCreate:  func() any { return new(EmergencyContactCreate) },
		NewPatch:   func() any { return new(EmergencyContactPatch) },
	}
}

func EmergencyContactSearchFields(body any) (title, text string) {
	found, ok := body.(*EmergencyContact)
	if !ok {
		return "", ""
	}

	return found.Name, found.Address + " " + found.Notes
}

// EmergencyContactBasis narrows nothing beyond is_active/is_primary, which
// are query terms and not a per-row distinction.
func EmergencyContactBasis(any, records.Criteria) []string { return nil }

func (EmergencyContactCodec) Summary(c clinical.EmergencyContact) any {
	return &EmergencyContactSummary{
		ID:           c.ID,
		Kind:         kind.EmergencyContact.Enum(),
		Name:         c.Name,
		Relationship: string(c.Relationship),
		Phone:        c.Phone,
		IsPrimary:    c.IsPrimary,
		IsActive:     c.IsActive,
		UpdatedAt:    wireInstant(c.UpdatedAt),
	}
}

func (codec EmergencyContactCodec) Detail(c clinical.EmergencyContact) any {
	summary, _ := codec.Summary(c).(*EmergencyContactSummary)

	detail := &EmergencyContact{
		EmergencyContactSummary: *summary,
		Patient:                 c.PatientID,
		PhoneAlt:                c.PhoneAlt,
		Email:                   c.Email,
		Address:                 c.Address,
		Notes:                   c.Notes,
		Tags:                    nonNil(c.Tags),
		CreatedAt:               wireInstant(c.CreatedAt),
	}

	if c.DisplacedID != "" {
		detail.Displaced = &DisplacedRef{ID: c.DisplacedID, Kind: kind.EmergencyContact.Enum()}
	}

	return detail
}

func (EmergencyContactCodec) Draft(body any) (clinical.EmergencyContact, error) {
	create, ok := body.(*EmergencyContactCreate)
	if !ok {
		return clinical.EmergencyContact{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	isActive := true
	if create.IsActive != nil {
		isActive = *create.IsActive
	}

	return clinical.EmergencyContact{
		PatientID:    create.Patient,
		Name:         create.Name,
		Relationship: clinical.ContactRelationship(create.Relationship),
		Phone:        create.Phone,
		PhoneAlt:     create.PhoneAlt,
		Email:        create.Email,
		Address:      create.Address,
		IsPrimary:    create.IsPrimary,
		IsActive:     isActive,
		Notes:        create.Notes,
		Tags:         create.Tags,
	}, nil
}

func (EmergencyContactCodec) Patch(body any) (emergencycontact.Patch, error) {
	incoming, ok := body.(*EmergencyContactPatch)
	if !ok {
		return emergencycontact.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	return emergencycontact.Patch{
		Name:         incoming.Name,
		Relationship: convert[clinical.ContactRelationship](incoming.Relationship),
		Phone:        incoming.Phone,
		PhoneAlt:     incoming.PhoneAlt,
		Email:        incoming.Email,
		Address:      incoming.Address,
		IsPrimary:    incoming.IsPrimary,
		IsActive:     incoming.IsActive,
		Notes:        incoming.Notes,
		Tags:         incoming.Tags,
	}, nil
}
