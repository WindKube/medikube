package person

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func validPatient(t *testing.T) Patient {
	t.Helper()
	birth, err := domain.NewDate(1990, time.March, 12)
	require.NoError(t, err)

	return Patient{
		ID:        "pat0000000001",
		OwnerID:   "usr0000000001",
		FirstName: "Amara",
		LastName:  "Okafor",
		BirthDate: birth,
	}
}

// FR-003, US1-3: a payload with four simultaneous faults reports all four
// fields[] entries in the one *domain.ValidationError, never just the first.
func TestPatientValidateReportsEveryOffendingFieldAtOnce(t *testing.T) {
	t.Parallel()

	future, err := domain.NewDate(2099, time.January, 1)
	require.NoError(t, err)

	patient := Patient{
		OwnerID:   "usr0000000001",
		FirstName: "",
		LastName:  "",
		BirthDate: future,
		HeightCM:  9999,
	}

	err = patient.Validate()
	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)

	fields := make(map[string]string, len(invalid.Fields))
	for _, f := range invalid.Fields {
		fields[f.Field] = f.Code
	}

	assert.Len(t, invalid.Fields, 4)
	assert.Equal(t, domain.CodeRequired, fields["first_name"])
	assert.Equal(t, domain.CodeRequired, fields["last_name"])
	assert.Equal(t, CodeDateInFuture, fields["birth_date"])
	assert.Equal(t, domain.CodeOutOfRange, fields["height_cm"])
}

func TestPatientValidate(t *testing.T) {
	t.Parallel()

	tooOld, err := domain.NewDate(1800, time.January, 1)
	require.NoError(t, err)
	future, err := domain.NewDate(2099, time.January, 1)
	require.NoError(t, err)

	tests := []struct {
		name       string
		mutate     func(p *Patient)
		wantFields []string
	}{
		{
			name:   "a fully valid patient",
			mutate: func(p *Patient) {},
		},
		{
			name: "empty first name",
			mutate: func(p *Patient) {
				p.FirstName = "   "
			},
			wantFields: []string{"first_name"},
		},
		{
			name: "first name too long",
			mutate: func(p *Patient) {
				p.FirstName = strings.Repeat("a", 101)
			},
			wantFields: []string{"first_name"},
		},
		{
			name: "empty last name",
			mutate: func(p *Patient) {
				p.LastName = ""
			},
			wantFields: []string{"last_name"},
		},
		{
			name: "last name too long",
			mutate: func(p *Patient) {
				p.LastName = strings.Repeat("b", 101)
			},
			wantFields: []string{"last_name"},
		},
		{
			name: "missing birth date",
			mutate: func(p *Patient) {
				p.BirthDate = domain.Date{}
			},
			wantFields: []string{"birth_date"},
		},
		{
			name: "birth date in the future",
			mutate: func(p *Patient) {
				p.BirthDate = future
			},
			wantFields: []string{"birth_date"},
		},
		{
			name: "birth date more than 150 years ago",
			mutate: func(p *Patient) {
				p.BirthDate = tooOld
			},
			wantFields: []string{"birth_date"},
		},
		{
			name: "self record may have no name or birth date",
			mutate: func(p *Patient) {
				p.FirstName = ""
				p.LastName = ""
				p.BirthDate = domain.Date{}
				p.IsSelfRecord = true
			},
		},
		{
			name: "invalid sex",
			mutate: func(p *Patient) {
				p.Sex = Sex("nonbinary")
			},
			wantFields: []string{"sex"},
		},
		{
			name: "invalid blood type",
			mutate: func(p *Patient) {
				p.BloodType = BloodType("ab")
			},
			wantFields: []string{"blood_type"},
		},
		{
			name: "invalid relationship to owner",
			mutate: func(p *Patient) {
				p.RelationshipToOwner = RelationshipToOwner("cousin")
			},
			wantFields: []string{"relationship_to_owner"},
		},
		{
			name: "height out of range",
			mutate: func(p *Patient) {
				p.HeightCM = 10
			},
			wantFields: []string{"height_cm"},
		},
		{
			name: "height at the maximum is accepted",
			mutate: func(p *Patient) {
				p.HeightCM = maxHeightCM
			},
		},
		{
			name: "weight out of range",
			mutate: func(p *Patient) {
				p.WeightKG = 1000
			},
			wantFields: []string{"weight_kg"},
		},
		{
			name: "weight at the minimum is accepted",
			mutate: func(p *Patient) {
				p.WeightKG = minWeightKG
			},
		},
		{
			name: "address too long",
			mutate: func(p *Patient) {
				p.Address = strings.Repeat("x", 501)
			},
			wantFields: []string{"address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			patient := validPatient(t)
			tt.mutate(&patient)

			err := patient.Validate()

			if len(tt.wantFields) == 0 {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)

			got := make([]string, 0, len(invalid.Fields))
			for _, f := range invalid.Fields {
				got = append(got, f.Field)
			}
			assert.ElementsMatch(t, tt.wantFields, got)
		})
	}
}
