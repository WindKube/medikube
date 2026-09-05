package clinical

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain"
)

// dateOrPanic is mustDate without a *testing.T, for the package-level table
// literals below that build outside any single subtest.
func dateOrPanic(text string) domain.Date {
	d, err := domain.ParseDate(text)
	if err != nil {
		panic(err)
	}

	return d
}

// The smallest encounter data-model §4.3 accepts (T061, FR-022).
func minimalEncounter() Encounter {
	return Encounter{
		ID: "mkenc0000001", PatientID: "mkpat0000001",
		Reason: "Annual check-up", OccurredOn: dateOrPanic("2026-03-01"),
	}
}

func TestEncounterValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() Encounter
		want  []domain.FieldError
	}{
		{name: "a minimal encounter is accepted", build: minimalEncounter},
		{
			name: "every field filled in is accepted",
			build: func() Encounter {
				e := minimalEncounter()
				e.VisitType, e.Priority = VisitTypeOffice, VisitPriorityRoutine
				e.Assessment, e.Plan, e.FollowUp = "stable", "continue", "six months"
				e.DurationMin = 20
				e.Notes = "nothing else to add"

				return e
			},
		},
		{
			name:  "the reason is required",
			build: func() Encounter { e := minimalEncounter(); e.Reason = "   "; return e },
			want:  []domain.FieldError{{Field: "reason", Code: domain.CodeRequired}},
		},
		{
			name:  "the reason has a documented maximum",
			build: func() Encounter { e := minimalEncounter(); e.Reason = strings.Repeat("a", 301); return e },
			want:  []domain.FieldError{{Field: "reason", Code: domain.CodeTooLong}},
		},
		{
			name:  "the date it happened is required",
			build: func() Encounter { e := minimalEncounter(); e.OccurredOn = domain.Date{}; return e },
			want:  []domain.FieldError{{Field: "occurred_on", Code: domain.CodeRequired}},
		},
		{
			name:  "an unpublished visit type is refused",
			build: func() Encounter { e := minimalEncounter(); e.VisitType = "housecall"; return e },
			want:  []domain.FieldError{{Field: "visit_type", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "an unpublished priority is refused",
			build: func() Encounter { e := minimalEncounter(); e.Priority = "asap"; return e },
			want:  []domain.FieldError{{Field: "priority", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "a negative duration is refused",
			build: func() Encounter { e := minimalEncounter(); e.DurationMin = -1; return e },
			want:  []domain.FieldError{{Field: "duration_minutes", Code: domain.CodeOutOfRange}},
		},
		{
			name: "two simultaneous violations are both reported, in column order",
			build: func() Encounter {
				e := minimalEncounter()
				e.Reason = ""
				e.DurationMin = -5

				return e
			},
			want: []domain.FieldError{
				{Field: "reason", Code: domain.CodeRequired},
				{Field: "duration_minutes", Code: domain.CodeOutOfRange},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.build().Validate()

			if len(test.want) == 0 {
				assert.NoError(t, err)

				return
			}

			got := refusals(t, err)
			assert.Len(t, got, len(test.want))

			for i, want := range test.want {
				if i >= len(got) {
					break
				}

				assert.Equal(t, want.Field, got[i].Field)
				assert.Equal(t, want.Code, got[i].Code)
				assert.NotEmpty(t, got[i].Message)
			}
		})
	}
}
