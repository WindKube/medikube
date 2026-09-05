package clinical

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain"
)

// The smallest treatment data-model §4.5 accepts (T063, FR-027).
func minimalTreatment() Treatment {
	return Treatment{
		ID: "mktrt0000001", PatientID: "mkpat0000001",
		Name: "Physical therapy", StartedOn: dateOrPanic("2026-03-01"),
	}
}

func TestTreatmentValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() Treatment
		want  []domain.FieldError
	}{
		{name: "a minimal treatment is accepted", build: minimalTreatment},
		{
			name: "every field filled in is accepted",
			build: func() Treatment {
				tr := minimalTreatment()
				tr.Type, tr.Setting = "physiotherapy", TreatmentSettingOutpatient
				tr.Description = "supervised sessions"
				tr.EndedOn = dateOrPanic("2026-04-01")
				tr.Frequency, tr.Dosage = "twice weekly", "45 minutes"
				tr.ExpectedOutcome = "improved mobility"
				tr.Status = TherapyStatusActive
				tr.Encounters = []string{"mkenc0000002"}
				tr.Equipment = []string{"mkequ0000001"}
				tr.Notes = "nothing else"

				return tr
			},
		},
		{
			name:  "the name is required",
			build: func() Treatment { tr := minimalTreatment(); tr.Name = "  "; return tr },
			want:  []domain.FieldError{{Field: "name", Code: domain.CodeRequired}},
		},
		{
			name:  "the name has a documented maximum",
			build: func() Treatment { tr := minimalTreatment(); tr.Name = strings.Repeat("a", 301); return tr },
			want:  []domain.FieldError{{Field: "name", Code: domain.CodeTooLong}},
		},
		{
			name:  "an unpublished setting is refused",
			build: func() Treatment { tr := minimalTreatment(); tr.Setting = "clinic"; return tr },
			want:  []domain.FieldError{{Field: "setting", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "an unpublished status is refused",
			build: func() Treatment { tr := minimalTreatment(); tr.Status = "ongoing"; return tr },
			want:  []domain.FieldError{{Field: "status", Code: domain.CodeInvalidValue}},
		},
		{
			// FR-013: an end before the start is refused, on the end date.
			name: "an end date before its start date is refused",
			build: func() Treatment {
				tr := minimalTreatment()
				tr.StartedOn = dateOrPanic("2026-03-10")
				tr.EndedOn = dateOrPanic("2026-03-01")

				return tr
			},
			want: []domain.FieldError{{Field: "ended_on", Code: CodeEndBeforeStart}},
		},
		{
			name: "a single-day course, started and ended the same day, is accepted",
			build: func() Treatment {
				tr := minimalTreatment()
				tr.EndedOn = tr.StartedOn

				return tr
			},
		},
		{
			name: "two simultaneous violations are both reported, in column order",
			build: func() Treatment {
				tr := minimalTreatment()
				tr.Name = ""
				tr.StartedOn = dateOrPanic("2026-03-10")
				tr.EndedOn = dateOrPanic("2026-03-01")

				return tr
			},
			want: []domain.FieldError{
				{Field: "name", Code: domain.CodeRequired},
				{Field: "ended_on", Code: CodeEndBeforeStart},
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
