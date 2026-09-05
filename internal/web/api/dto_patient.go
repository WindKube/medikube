package api

import (
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
	"medikube/internal/web"
)

// The wire spellings of patients.md's members, matching the JSON tags so a
// rename on the wire and not here is a compile error.
const (
	PatientMemberFirstName    = "first_name"
	PatientMemberLastName     = "last_name"
	PatientMemberBirthDate    = "birth_date"
	PatientMemberSex          = "sex"
	PatientMemberBloodType    = "blood_type"
	PatientMemberHeightCM     = "height_cm"
	PatientMemberWeightKG     = "weight_kg"
	PatientMemberAddress      = "address"
	PatientMemberRelationship = "relationship_to_owner"
	PatientMemberPractitioner = "primary_practitioner"
)

// PatientSummary is a list row.
type PatientSummary struct {
	ID           string  `json:"id"`
	FirstName    string  `json:"first_name"`
	LastName     string  `json:"last_name"`
	BirthDate    *string `json:"birth_date"`
	Age          *string `json:"age"`
	IsSelfRecord bool    `json:"is_self_record"`
	Relationship string  `json:"relationship_to_owner,omitempty"`
	PhotoURL     *string `json:"photo_url"`
	UpdatedAt    string  `json:"updated_at"`
}

// Display is computed from the actor's own unit_system. The recorded value
// never changes (FR-007) — only its rendering does.
type Display struct {
	UnitSystem string `json:"unit_system"`
	Height     string `json:"height,omitempty"`
	Weight     string `json:"weight,omitempty"`
}

// PractitionerRef is the primary practitioner's own display, embedded rather
// than an id alone so a chart does not need a second round trip to show who
// it is.
type PractitionerRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Specialty string `json:"specialty,omitempty"`
}

// Patient is the detail shape.
type Patient struct {
	PatientSummary

	Sex                 string           `json:"sex,omitempty"`
	BloodType           string           `json:"blood_type,omitempty"`
	HeightCm            *float64         `json:"height_cm"`
	WeightKg            *float64         `json:"weight_kg"`
	Address             string           `json:"address,omitempty"`
	PrimaryPractitioner *PractitionerRef `json:"primary_practitioner"`
	Display             Display          `json:"display"`
}

// PatientCreate is the create body. No `owner`, no `is_self_record` — FR-002
// and FR-004 are therefore enforced by shape (unknown fields are rejected).
type PatientCreate struct {
	FirstName    string   `json:"first_name"`
	LastName     string   `json:"last_name"`
	BirthDate    string   `json:"birth_date"`
	Sex          string   `json:"sex,omitempty"`
	BloodType    string   `json:"blood_type,omitempty"`
	HeightCm     *float64 `json:"height_cm,omitempty"`
	WeightKg     *float64 `json:"weight_kg,omitempty"`
	Address      string   `json:"address,omitempty"`
	Relationship string   `json:"relationship_to_owner,omitempty"`
	Practitioner string   `json:"primary_practitioner,omitempty"`
}

// PatientPatch is the partial update. HeightCm/WeightKg/Practitioner use
// web.Optional so absent-versus-explicit-null survives the round trip
// (mirrors MedicationPatch's own two dates).
type PatientPatch struct {
	FirstName    *string `json:"first_name,omitempty"`
	LastName     *string `json:"last_name,omitempty"`
	BirthDate    *string `json:"birth_date,omitempty"`
	Sex          *string `json:"sex,omitempty"`
	BloodType    *string `json:"blood_type,omitempty"`
	Address      *string `json:"address,omitempty"`
	Relationship *string `json:"relationship_to_owner,omitempty"`

	HeightCm     web.Optional[float64] `json:"height_cm,omitzero"`
	WeightKg     web.Optional[float64] `json:"weight_kg,omitzero"`
	Practitioner web.Optional[string]  `json:"primary_practitioner,omitzero"`
}

// NewPatientSummary renders the list shape. photoURL is nil when the patient
// has no photograph.
func NewPatientSummary(p person.Patient, photoURL *string) PatientSummary {
	return PatientSummary{
		ID:           p.ID,
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		BirthDate:    wirePatientDate(p.BirthDate),
		Age:          wireAge(p.BirthDate),
		IsSelfRecord: p.IsSelfRecord,
		Relationship: string(p.RelationshipToOwner),
		PhotoURL:     photoURL,
		UpdatedAt:    wireInstant(p.UpdatedAt),
	}
}

// NewPatient renders the detail shape. system is the actor's own
// unit_system, for Display; practitioner is nil when none is set or none was
// resolved.
func NewPatient(p person.Patient, photoURL *string, system identity.UnitSystem, practitioner *PractitionerRef) Patient {
	return Patient{
		PatientSummary:      NewPatientSummary(p, photoURL),
		Sex:                 string(p.Sex),
		BloodType:           string(p.BloodType),
		HeightCm:            wireMeasure(p.HeightCM),
		WeightKg:            wireMeasure(p.WeightKG),
		Address:             p.Address,
		PrimaryPractitioner: practitioner,
		Display: Display{
			UnitSystem: string(system),
			Height:     person.FormatHeight(p.HeightCM, system),
			Weight:     person.FormatWeight(p.WeightKG, system),
		},
	}
}

// Draft reads a create body into a domain draft. owner and is_self_record are
// never set here — the DTO has no member for either, and the service
// overwrites both regardless of what the caller passed (FR-002, FR-004).
func (c PatientCreate) Draft() (person.Patient, error) {
	var invalid domain.ValidationError

	birthDate := readPatientDate(&invalid, PatientMemberBirthDate, ptrOrNil(c.BirthDate))

	draft := person.Patient{
		FirstName:             c.FirstName,
		LastName:              c.LastName,
		BirthDate:             birthDate,
		Sex:                   person.Sex(c.Sex),
		BloodType:             person.BloodType(c.BloodType),
		HeightCM:              derefFloat(c.HeightCm),
		WeightKG:              derefFloat(c.WeightKg),
		Address:               c.Address,
		RelationshipToOwner:   person.RelationshipToOwner(c.Relationship),
		PrimaryPractitionerID: c.Practitioner,
	}

	// draft carries every field submitted regardless of the date error, so a
	// Datastar re-render of a rejected create still shows what was typed
	// (FR-027) rather than a form emptied by the one field that failed.
	if err := invalid.OrNil(); err != nil {
		return draft, err
	}

	return draft, nil
}

// ToServicePatch reads an update body into a service patch.
func (p PatientPatch) ToServicePatch() (patient.Patch, error) {
	var invalid domain.ValidationError

	var birthDate *domain.Date
	if p.BirthDate != nil {
		parsed := readPatientDate(&invalid, PatientMemberBirthDate, p.BirthDate)
		birthDate = &parsed
	}

	if err := invalid.OrNil(); err != nil {
		return patient.Patch{}, err
	}

	patch := patient.Patch{
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		BirthDate:    birthDate,
		Sex:          convertSex(p.Sex),
		BloodType:    convertBloodType(p.BloodType),
		Address:      p.Address,
		Relationship: convertRelationship(p.Relationship),
	}

	if p.HeightCm.Present() {
		value, given := p.HeightCm.Get()
		patch.HeightCM = ptrPtr(given, value)
	}

	if p.WeightKg.Present() {
		value, given := p.WeightKg.Get()
		patch.WeightKG = ptrPtr(given, value)
	}

	if p.Practitioner.Present() {
		value, given := p.Practitioner.Get()
		patch.PrimaryPractitioner = ptrPtrString(given, value)
	}

	return patch, nil
}

func wirePatientDate(date domain.Date) *string {
	if date.IsZero() {
		return nil
	}

	rendered := date.String()

	return &rendered
}

func wireAge(birth domain.Date) *string {
	if birth.IsZero() {
		return nil
	}

	age := person.AgeAt(birth, time.Now().UTC())
	if !age.Recorded() {
		return nil
	}

	rendered := age.String()

	return &rendered
}

func wireMeasure(value float64) *float64 {
	if value == 0 {
		return nil
	}

	return &value
}

func readPatientDate(invalid *domain.ValidationError, member string, raw *string) domain.Date {
	if raw == nil {
		return domain.Date{}
	}

	parsed, err := domain.ParseDate(*raw)
	if err != nil {
		invalid.Add(member, domain.CodeInvalidDate, "a date is a real calendar day written YYYY-MM-DD")

		return domain.Date{}
	}

	return parsed
}

func ptrOrNil(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}

	return *value
}

func convertSex(value *string) *person.Sex {
	if value == nil {
		return nil
	}

	converted := person.Sex(*value)

	return &converted
}

func convertBloodType(value *string) *person.BloodType {
	if value == nil {
		return nil
	}

	converted := person.BloodType(*value)

	return &converted
}

func convertRelationship(value *string) *person.RelationshipToOwner {
	if value == nil {
		return nil
	}

	converted := person.RelationshipToOwner(*value)

	return &converted
}

func ptrPtr(given bool, value float64) **float64 {
	var inner *float64

	if given {
		inner = &value
	}

	return &inner
}

func ptrPtrString(given bool, value string) **string {
	var inner *string

	if given {
		inner = &value
	}

	return &inner
}
