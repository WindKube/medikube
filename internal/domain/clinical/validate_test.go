package clinical

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func mustDate(t *testing.T, text string) domain.Date {
	t.Helper()

	date, err := domain.ParseDate(text)
	require.NoError(t, err)

	return date
}

// The smallest medication data-model §2 accepts: a name and a state. Every
// other field is optional, which is the "a medication with only a name" edge
// case the seed and the render tests both exercise.
func minimalMedication() Medication {
	return Medication{
		ID:        "med0000000001",
		PatientID: "mkpat0000001",
		Name:      "Levothyroxine",
		Status:    TherapyStatusActive,
	}
}

func refusals(t *testing.T, err error) []domain.FieldError {
	t.Helper()

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid, "Validate must return a *domain.ValidationError")

	return invalid.Fields
}

// The one refusal a single-fault case expects, unwrapped so the assertion reads
// as the rule it is testing.
func onlyRefusal(t *testing.T, err error) domain.FieldError {
	t.Helper()

	fields := refusals(t, err)
	require.Len(t, fields, 1, "expected exactly one refusal, got %v", fields)

	return fields[0]
}

func TestAValidMedicationIsAccepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		make func() Medication
	}{
		{
			name: "only a name and a state",
			make: minimalMedication,
		},
		{
			name: "every field filled in",
			make: func() Medication {
				m := minimalMedication()
				m.AlternativeName = "Euthyrox"
				m.Type = MedicationTypePrescription
				m.Dosage = "75 mcg"
				m.Frequency = "once daily before breakfast"
				m.Route = MedicationRouteOral
				m.Indication = "hypothyroidism"
				m.StartedOn = mustDate(t, "2026-03-01")
				m.EndedOn = mustDate(t, "2026-09-30")
				m.SideEffects = "palpitations in the first fortnight"
				m.Notes = "blood test due in October"

				return m
			},
		},
		{
			name: "a single-day course, started and ended the same day",
			make: func() Medication {
				m := minimalMedication()
				m.StartedOn = mustDate(t, "2026-03-01")
				m.EndedOn = mustDate(t, "2026-03-01")

				return m
			},
		},
		{
			name: "a course beginning next week",
			make: func() Medication {
				m := minimalMedication()
				m.StartedOn = mustDate(t, "2099-12-31")

				return m
			},
		},
		{
			name: "an end date with no start date",
			make: func() Medication {
				m := minimalMedication()
				m.EndedOn = mustDate(t, "2026-03-01")

				return m
			},
		},
		{
			name: "every state the ladder publishes",
			make: func() Medication {
				m := minimalMedication()
				m.Status = TherapyStatusCancelled

				return m
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// OrNil returns an untyped nil, so a valid record must satisfy
			// err != nil being false and not merely hold an empty error.
			assert.NoError(t, test.make().Validate())
		})
	}
}

func TestNameIsRequired(t *testing.T) {
	t.Parallel()

	// data-model §2: the name is trimmed before it is measured, so whitespace
	// is not a name however much of it there is.
	for _, name := range []string{"", " ", "   ", "\t", "\n", " \t\n "} {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			t.Parallel()

			m := minimalMedication()
			m.Name = name

			refusal := onlyRefusal(t, m.Validate())
			assert.Equal(t, "name", refusal.Field)
			assert.Equal(t, domain.CodeRequired, refusal.Code)
			assert.NotEmpty(t, refusal.Message)
		})
	}
}

// FR-017 and the "free text at exactly its documented limit" edge case. The
// limits are pinned here as literals from data-model §2 — reading them from the
// implementation would assert only that the implementation equals itself.
func TestEveryFreeTextMaximumIsEnforcedAtItsDocumentedLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		field string
		label string
		limit int
		set   func(*Medication, string)
	}{
		{
			field: "name", label: "name", limit: 200,
			set: func(m *Medication, s string) { m.Name = s },
		},
		{
			field: "alternative_name", label: "alternative name", limit: 200,
			set: func(m *Medication, s string) { m.AlternativeName = s },
		},
		{
			field: "dosage", label: "dose", limit: 200,
			set: func(m *Medication, s string) { m.Dosage = s },
		},
		{
			field: "frequency", label: "often", limit: 100,
			set: func(m *Medication, s string) { m.Frequency = s },
		},
		{
			field: "indication", label: "reason", limit: 300,
			set: func(m *Medication, s string) { m.Indication = s },
		},
		{
			field: "side_effects", label: "side effects", limit: 1000,
			set: func(m *Medication, s string) { m.SideEffects = s },
		},
		{
			field: "notes", label: "notes", limit: 5000,
			set: func(m *Medication, s string) { m.Notes = s },
		},
	}

	for _, test := range tests {
		t.Run(test.field+" at exactly its limit is accepted", func(t *testing.T) {
			t.Parallel()

			m := minimalMedication()
			test.set(&m, strings.Repeat("a", test.limit))

			assert.NoError(t, m.Validate())
		})

		t.Run(test.field+" one character over is refused", func(t *testing.T) {
			t.Parallel()

			m := minimalMedication()
			test.set(&m, strings.Repeat("a", test.limit+1))

			refusal := onlyRefusal(t, m.Validate())
			assert.Equal(t, test.field, refusal.Field)
			assert.Equal(t, domain.CodeTooLong, refusal.Code)
			assert.Contains(t, refusal.Message, strconv.Itoa(test.limit),
				"FR-017: the refusal names the limit")
			assert.Contains(t, refusal.Message, test.label,
				"FR-017: the refusal names the field")
		})

		// data-model §2: the boundary is code points, not bytes, so a field in
		// a non-Latin script is not silently a third of the length.
		t.Run(test.field+" measures code points and not bytes", func(t *testing.T) {
			t.Parallel()

			atLimit := strings.Repeat("薬", test.limit)
			require.Greater(t, len(atLimit), test.limit, "the fixture must be multi-byte to test anything")

			m := minimalMedication()
			test.set(&m, atLimit)
			assert.NoError(t, m.Validate())

			over := minimalMedication()
			test.set(&over, strings.Repeat("薬", test.limit+1))
			assert.Equal(t, domain.CodeTooLong, onlyRefusal(t, over.Validate()).Code)
		})
	}
}

// FR-016 at the domain layer: a value outside a published set is refused, never
// stored as free text. The optional selects also accept absence, which is a
// different thing from a value nobody publishes.
func TestUnpublishedEnumValuesAreRefused(t *testing.T) {
	t.Parallel()

	t.Run("type", func(t *testing.T) {
		t.Parallel()

		for _, value := range []MedicationType{"vitamin", "Prescription", "OTC", "over-the-counter", " "} {
			m := minimalMedication()
			m.Type = value

			refusal := onlyRefusal(t, m.Validate())
			assert.Equal(t, "type", refusal.Field)
			assert.Equal(t, domain.CodeInvalidValue, refusal.Code)
		}
	})

	t.Run("an absent type is not a refusal", func(t *testing.T) {
		t.Parallel()

		m := minimalMedication()
		m.Type = ""

		assert.NoError(t, m.Validate())
	})

	t.Run("route", func(t *testing.T) {
		t.Parallel()

		for _, value := range []MedicationRoute{"by_mouth", "Oral", "iv", "injection", " "} {
			m := minimalMedication()
			m.Route = value

			refusal := onlyRefusal(t, m.Validate())
			assert.Equal(t, "route", refusal.Field)
			assert.Equal(t, domain.CodeInvalidValue, refusal.Code)
		}
	})

	t.Run("an absent route is not a refusal", func(t *testing.T) {
		t.Parallel()

		m := minimalMedication()
		m.Route = ""

		assert.NoError(t, m.Validate())
	})

	t.Run("status", func(t *testing.T) {
		t.Parallel()

		for _, value := range []TherapyStatus{"canceled", "Active", "paused", "finished", "draft"} {
			m := minimalMedication()
			m.Status = value

			refusal := onlyRefusal(t, m.Validate())
			assert.Equal(t, "status", refusal.Field)
			assert.Equal(t, domain.CodeInvalidValue, refusal.Code)
		}
	})

	// A state is required, and an absent one is reported as absent rather than
	// as an unrecognised value: the form shows a different message for each.
	t.Run("an absent status is required, not invalid", func(t *testing.T) {
		t.Parallel()

		m := minimalMedication()
		m.Status = ""

		refusal := onlyRefusal(t, m.Validate())
		assert.Equal(t, "status", refusal.Field)
		assert.Equal(t, domain.CodeRequired, refusal.Code)
	})
}

// FR-018. Equality is accepted — a single-day course is a real prescription.
func TestAnEndDateBeforeItsStartDateIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		startedOn string
		endedOn   string
		refused   bool
	}{
		{name: "one day before", startedOn: "2026-03-02", endedOn: "2026-03-01", refused: true},
		{name: "a year before", startedOn: "2026-03-01", endedOn: "2025-03-01", refused: true},
		{name: "one month before", startedOn: "2026-03-01", endedOn: "2026-02-28", refused: true},
		{name: "the same day", startedOn: "2026-03-01", endedOn: "2026-03-01"},
		{name: "one day after", startedOn: "2026-03-01", endedOn: "2026-03-02"},
		{name: "no end date", startedOn: "2026-03-01"},
		{name: "no start date", endedOn: "2026-03-01"},
		{name: "neither"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			m := minimalMedication()
			m.StartedOn = mustDate(t, test.startedOn)
			m.EndedOn = mustDate(t, test.endedOn)

			err := m.Validate()
			if !test.refused {
				assert.NoError(t, err)

				return
			}

			refusal := onlyRefusal(t, err)
			assert.Equal(t, "ended_on", refusal.Field)
			assert.Equal(t, CodeEndBeforeStart, refusal.Code)
		})
	}
}

// FR-027, which is the reason ValidationError holds a slice. Eleven rules broken
// at once come back as eleven refusals in the order the form renders them, not
// as whichever one the implementation happened to check first.
func TestEveryViolationComesBackInOneError(t *testing.T) {
	t.Parallel()

	broken := Medication{
		ID:              "med0000000001",
		PatientID:       "mkpat0000001",
		Name:            "   ",
		AlternativeName: strings.Repeat("a", 201),
		Type:            "vitamin",
		Dosage:          strings.Repeat("a", 201),
		Frequency:       strings.Repeat("a", 101),
		Route:           "by_mouth",
		Indication:      strings.Repeat("a", 301),
		StartedOn:       mustDate(t, "2026-03-02"),
		EndedOn:         mustDate(t, "2026-03-01"),
		Status:          "canceled",
		SideEffects:     strings.Repeat("a", 1001),
		Notes:           strings.Repeat("a", 5001),
	}

	fields := refusals(t, broken.Validate())

	type refusal struct{ field, code string }

	got := make([]refusal, 0, len(fields))
	for _, field := range fields {
		got = append(got, refusal{field: field.Field, code: field.Code})
	}

	assert.Equal(t, []refusal{
		{field: "name", code: domain.CodeRequired},
		{field: "alternative_name", code: domain.CodeTooLong},
		{field: "type", code: domain.CodeInvalidValue},
		{field: "dosage", code: domain.CodeTooLong},
		{field: "frequency", code: domain.CodeTooLong},
		{field: "route", code: domain.CodeInvalidValue},
		{field: "indication", code: domain.CodeTooLong},
		{field: "ended_on", code: CodeEndBeforeStart},
		{field: "status", code: domain.CodeInvalidValue},
		{field: "side_effects", code: domain.CodeTooLong},
		{field: "notes", code: domain.CodeTooLong},
	}, got)
}

// Constitution VII. Validate runs on a submission full of patient data and its
// error is what reaches the log, Sentry and the error envelope, so no message
// may quote what the person typed.
func TestNoRefusalQuotesWhatWasSubmitted(t *testing.T) {
	t.Parallel()

	submitted := []string{
		"Sertraline", "Zoloft", "vitamin", "50 mg", "twice daily",
		"by_mouth", "major depressive disorder", "canceled",
		"nausea", "started after the March appointment",
	}

	broken := Medication{
		Name:            strings.Repeat("Sertraline", 40),
		AlternativeName: strings.Repeat("Zoloft", 40),
		Type:            "vitamin",
		Dosage:          strings.Repeat("50 mg", 60),
		Frequency:       strings.Repeat("twice daily", 20),
		Route:           "by_mouth",
		Indication:      strings.Repeat("major depressive disorder", 20),
		Status:          "canceled",
		SideEffects:     strings.Repeat("nausea", 200),
		Notes:           strings.Repeat("started after the March appointment", 200),
	}

	err := broken.Validate()
	require.Error(t, err)

	for _, secret := range submitted {
		assert.NotContains(t, err.Error(), secret,
			"the error string reaches the log stream and must carry fields and codes only")

		for _, field := range refusals(t, err) {
			assert.NotContains(t, field.Message, secret,
				"%s quoted the submitted value back", field.Field)
		}
	}
}

// The type makes the "real calendar date" half of FR-018 unreachable from here:
// a domain.Date cannot hold 30 February, so the refusal happens when the text
// is parsed at the edge and Validate never sees a bad one.
func TestACalendarDateThatDoesNotExistIsRefusedBeforeItCanReachAMedication(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"2026-02-30", "2026-13-01", "2026-00-10", "01/03/2026", "2026-3-1"} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			_, err := domain.ParseDate(text)
			assert.Error(t, err)
		})
	}
}

// The clock is not a dependency of this entity. data-model §2 accepts a future
// start date outright, so Validate has nothing to ask a Clock about and cannot
// drift with the machine's date.
func TestValidateDoesNotConsultTheClock(t *testing.T) {
	t.Parallel()

	far := minimalMedication()
	far.StartedOn = mustDate(t, "9999-12-31")
	far.EndedOn = mustDate(t, "9999-12-31")
	assert.NoError(t, far.Validate())

	old := minimalMedication()
	old.StartedOn = mustDate(t, "1901-01-01")
	assert.NoError(t, old.Validate())
}
