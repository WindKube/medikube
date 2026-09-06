package clinical

import (
	"errors"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
	"medikube/internal/domain/person"
)

// data-model §4.13's bounds (FR-052, FR-054).
const (
	familyMemberNameMax = 100
	familyMemberYearMin = 1850
	familyMemberYearMax = 2200
)

// FamilyMember is one relative recorded against a person's family history
// (data-model §4.13, FR-052). It reuses person.Sex — data-model §4.13 says
// so explicitly, rather than declaring a second vocabulary for the same idea.
type FamilyMember struct {
	ID        string
	PatientID string

	Name         string
	Relationship FamilyRelationship
	Sex          person.Sex
	BirthYear    *int
	DeathYear    *int
	IsDeceased   bool
	Conditions   []FamilyCondition

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

// Validate enforces FR-052: name and relationship are required; sex, when
// present, is one of the shared vocabulary; birth_year and death_year, when
// present, fall within the plausible range, and a death year earlier than the
// birth year is refused with both fields reported (FR-054, US10-3).
func (f FamilyMember) Validate() error {
	var invalid domain.ValidationError

	if name := strings.TrimSpace(f.Name); name == "" {
		invalid.Add("name", domain.CodeRequired, "a name is required")
	} else {
		checkLength(&invalid, "name", "the name", name, familyMemberNameMax)
	}

	switch {
	case f.Relationship == "":
		invalid.Add("relationship", domain.CodeRequired, "a relationship is required")
	case !f.Relationship.Valid():
		invalid.Add("relationship", domain.CodeInvalidValue, "not one of the relationships MediKube accepts")
	}

	if f.Sex != "" && !f.Sex.Valid() {
		invalid.Add("sex", domain.CodeInvalidValue, "not one of the values MediKube accepts")
	}

	checkFamilyYear(&invalid, "birth_year", f.BirthYear)
	checkFamilyYear(&invalid, "death_year", f.DeathYear)

	if f.BirthYear != nil && f.DeathYear != nil && *f.DeathYear < *f.BirthYear {
		invalid.Add("death_year", CodeEndBeforeStart, "the year of death is before the year of birth")
		invalid.Add("birth_year", CodeEndBeforeStart, "the year of death is before the year of birth")
	}

	var conditionErrs *domain.ValidationError
	if errors.As(ValidateFamilyConditions(f.Conditions), &conditionErrs) {
		invalid.Fields = append(invalid.Fields, conditionErrs.Fields...)
	}

	return invalid.OrNil()
}

func checkFamilyYear(invalid *domain.ValidationError, field string, year *int) {
	if year == nil {
		return
	}

	if *year < familyMemberYearMin || *year > familyMemberYearMax {
		invalid.Addf(field, domain.CodeOutOfRange, "%s accepts a year between %d and %d", field, familyMemberYearMin, familyMemberYearMax)
	}
}

// MarshalZerologObject emits the two identifiers and nothing else: a
// relative's name, and every condition they had, is PHI (constitution VII).
func (f FamilyMember) MarshalZerologObject(e *zerolog.Event) {
	e.Str("family_member_id", f.ID).Str("patient_id", f.PatientID)
}
