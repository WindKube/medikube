package person

import (
	"strings"
	"time"
	"unicode/utf8"

	"medikube/internal/domain"
)

// This entity's own codes, spelled as contracts/README.md's error envelope
// spells them; the shared ones live in the domain package.
const (
	CodeDateInFuture = "date_in_future"
	CodeDateTooOld   = "date_too_old"
)

const (
	maxNameLength    = 100
	maxAddressLength = 500

	minHeightCM = 30
	maxHeightCM = 272
	minWeightKG = 0.5
	maxWeightKG = 450

	maxAgeYears = 150
)

// Validate reports every offending field at once (FR-003, US1-3: "together
// with every other invalid field in the same submission"). The rules run in
// data-model §3's order, which is the order the form renders them.
//
// first_name, last_name and birth_date are required for every patient except
// a server-provisioned self-record (research D-09): PocketBase cannot make
// them collection-required without making FR-005's automatic self-record
// unsatisfiable for a pre-existing account, so an empty value is refused here
// unless IsSelfRecord is set.
func (p Patient) Validate() error {
	var invalid domain.ValidationError

	switch first := strings.TrimSpace(p.FirstName); {
	case first == "" && !p.IsSelfRecord:
		invalid.Add("first_name", domain.CodeRequired, "a first name is required")
	case first != "":
		checkNameLength(&invalid, "first_name", "the first name", first)
	}

	switch last := strings.TrimSpace(p.LastName); {
	case last == "" && !p.IsSelfRecord:
		invalid.Add("last_name", domain.CodeRequired, "a last name is required")
	case last != "":
		checkNameLength(&invalid, "last_name", "the last name", last)
	}

	p.validateBirthDate(&invalid)

	if p.Sex != "" && !p.Sex.Valid() {
		invalid.Add("sex", domain.CodeInvalidValue, "not one of the values MediKube accepts")
	}
	if p.BloodType != "" && !p.BloodType.Valid() {
		invalid.Add("blood_type", domain.CodeInvalidValue, "not one of the values MediKube accepts")
	}
	if p.RelationshipToOwner != "" && !p.RelationshipToOwner.Valid() {
		invalid.Add("relationship_to_owner", domain.CodeInvalidValue, "not one of the values MediKube accepts")
	}

	if p.HeightCM != 0 && (p.HeightCM < minHeightCM || p.HeightCM > maxHeightCM) {
		invalid.Addf("height_cm", domain.CodeOutOfRange, "height must be between %d and %d cm", minHeightCM, maxHeightCM)
	}
	if p.WeightKG != 0 && (p.WeightKG < minWeightKG || p.WeightKG > maxWeightKG) {
		invalid.Addf("weight_kg", domain.CodeOutOfRange, "weight must be between %v and %v kg", minWeightKG, maxWeightKG)
	}

	if utf8.RuneCountInString(p.Address) > maxAddressLength {
		invalid.Addf("address", domain.CodeTooLong, "the address accepts at most %d characters", maxAddressLength)
	}

	return invalid.OrNil()
}

func (p Patient) validateBirthDate(invalid *domain.ValidationError) {
	if p.BirthDate.IsZero() {
		if !p.IsSelfRecord {
			invalid.Add("birth_date", domain.CodeRequired, "a birth date is required")
		}
		return
	}

	today, err := domain.NewDate(time.Now().UTC().Date())
	if err != nil {
		return
	}

	if p.BirthDate.After(today) {
		invalid.Add("birth_date", CodeDateInFuture, "the birth date is in the future")
		return
	}

	oldest, err := domain.NewDate(today.Year()-maxAgeYears, today.Month(), today.Day())
	if err != nil {
		// today's month/day has no equivalent maxAgeYears ago (29 February) —
		// widen by a day rather than refuse a birth date that is not actually
		// too old.
		oldest, err = domain.NewDate(today.Year()-maxAgeYears, today.Month(), today.Day()-1)
		if err != nil {
			return
		}
	}
	if p.BirthDate.Before(oldest) {
		invalid.Addf("birth_date", CodeDateTooOld, "the birth date is more than %d years ago", maxAgeYears)
	}
}

// The message names the field and its limit and never the text that broke it
// — that text is patient data and this message reaches the log (constitution
// VII).
func checkNameLength(invalid *domain.ValidationError, field, label, value string) {
	if utf8.RuneCountInString(value) > maxNameLength {
		invalid.Addf(field, domain.CodeTooLong, "%s accepts at most %d characters", label, maxNameLength)
	}
}
