package clinical

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain"
)

// The smallest procedure data-model §4.4 accepts (T062, FR-024).
func minimalProcedure() Procedure {
	return Procedure{
		ID: "mkprc0000001", PatientID: "mkpat0000001",
		Name: "Skin biopsy", OccurredOn: dateOrPanic("2026-03-01"), Status: OrderStatusCompleted,
	}
}

func TestProcedureValidate(t *testing.T) {
	t.Parallel()

	future := dateOrPanic("2099-01-01")

	tests := []struct {
		name  string
		build func() Procedure
		want  []domain.FieldError
	}{
		{name: "a minimal procedure is accepted", build: minimalProcedure},
		{
			name: "every field filled in is accepted",
			build: func() Procedure {
				p := minimalProcedure()
				p.Type, p.Code, p.Description = ProcedureTypeSurgical, "45378", "details"
				p.Outcome, p.Setting = ProcedureOutcomeSuccessful, ProcedureSettingOutpatient
				p.Complications, p.DurationMin = "none", 45
				p.Anesthesia, p.AnesthesiaNotes = AnesthesiaGeneral, "well tolerated"
				p.Notes = "nothing else"

				return p
			},
		},
		{
			name:  "the name is required",
			build: func() Procedure { p := minimalProcedure(); p.Name = "  "; return p },
			want:  []domain.FieldError{{Field: "name", Code: domain.CodeRequired}},
		},
		{
			name:  "the name has a documented maximum",
			build: func() Procedure { p := minimalProcedure(); p.Name = strings.Repeat("a", 301); return p },
			want:  []domain.FieldError{{Field: "name", Code: domain.CodeTooLong}},
		},
		{
			name:  "the date it happened is required",
			build: func() Procedure { p := minimalProcedure(); p.OccurredOn = domain.Date{}; return p },
			want:  []domain.FieldError{{Field: "occurred_on", Code: domain.CodeRequired}},
		},
		{
			name:  "a status is required",
			build: func() Procedure { p := minimalProcedure(); p.Status = ""; return p },
			want:  []domain.FieldError{{Field: "status", Code: domain.CodeRequired}},
		},
		{
			name:  "an unpublished status is refused",
			build: func() Procedure { p := minimalProcedure(); p.Status = "done"; return p },
			want:  []domain.FieldError{{Field: "status", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "an unpublished type is refused",
			build: func() Procedure { p := minimalProcedure(); p.Type = "cosmetic"; return p },
			want:  []domain.FieldError{{Field: "type", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "an unpublished outcome is refused",
			build: func() Procedure { p := minimalProcedure(); p.Outcome = "mixed"; return p },
			want:  []domain.FieldError{{Field: "outcome", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "an unpublished setting is refused",
			build: func() Procedure { p := minimalProcedure(); p.Setting = "home"; return p },
			want:  []domain.FieldError{{Field: "setting", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "an unpublished anesthesia is refused",
			build: func() Procedure { p := minimalProcedure(); p.Anesthesia = "twilight"; return p },
			want:  []domain.FieldError{{Field: "anesthesia", Code: domain.CodeInvalidValue}},
		},
		{
			name:  "a negative duration is refused",
			build: func() Procedure { p := minimalProcedure(); p.DurationMin = -1; return p },
			want:  []domain.FieldError{{Field: "duration_minutes", Code: domain.CodeOutOfRange}},
		},
		{
			// FR-025: a future date is accepted while the procedure is still
			// ordered or scheduled.
			name: "a future date is accepted for a scheduled procedure",
			build: func() Procedure {
				p := minimalProcedure()
				p.OccurredOn, p.Status = future, OrderStatusScheduled

				return p
			},
		},
		{
			name: "a future date is accepted for an ordered procedure",
			build: func() Procedure {
				p := minimalProcedure()
				p.OccurredOn, p.Status = future, OrderStatusOrdered

				return p
			},
		},
		{
			// FR-025's other half: completed cannot have happened in the future.
			name: "a future date is refused for a completed procedure",
			build: func() Procedure {
				p := minimalProcedure()
				p.OccurredOn, p.Status = future, OrderStatusCompleted

				return p
			},
			want: []domain.FieldError{{Field: "occurred_on", Code: CodeNotFuture}},
		},
		{
			name: "two simultaneous violations are both reported, in column order",
			build: func() Procedure {
				p := minimalProcedure()
				p.Name = ""
				p.DurationMin = -5

				return p
			},
			want: []domain.FieldError{
				{Field: "name", Code: domain.CodeRequired},
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

func TestProcedureScheduledIsOrderedOrScheduledOnly(t *testing.T) {
	t.Parallel()

	for _, status := range OrderStatuses() {
		want := status == OrderStatusOrdered || status == OrderStatusScheduled

		assert.Equal(t, want, Procedure{Status: status}.Scheduled(), "status %s", status)
	}
}
