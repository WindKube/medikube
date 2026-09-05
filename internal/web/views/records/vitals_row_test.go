package records_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

// T089: the vitals row renders only the measurements present, and shows bmi
// only when both height and weight are present.
func TestTheVitalsRowRendersOnlyTheMeasurementsPresent(t *testing.T) {
	t.Parallel()

	recordedAt, err := time.Parse(time.RFC3339, "2026-01-01T09:00:00Z")
	require.NoError(t, err)

	weight, systolic := 70.0, 120.0

	cases := []struct {
		name        string
		v           clinical.Vitals
		wantBMI     bool
		notWantWord string
	}{
		{
			name:        "one measurement recorded, no bmi",
			v:           clinical.Vitals{ID: "v1", RecordedAt: clinical.NewInstant(recordedAt), SystolicMmHg: &systolic},
			wantBMI:     false,
			notWantWord: "BMI",
		},
		{
			name:    "both height and weight present, bmi is shown",
			v:       fullVitalsSet(recordedAt, weight),
			wantBMI: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			view := records.NewVitalsView(testCase.v, records.VitalsLinks{Detail: "/" + kind.Vitals.Segment() + "/" + testCase.v.ID})
			tree := viewstest.Render(t, records.VitalsRow(view), "tbody")
			text := viewstest.Text(tree.One(t, viewstest.Tag("tr")))

			if testCase.wantBMI {
				assert.Contains(t, text, "BMI", "bmi is present when both height and weight are recorded")
			} else {
				assert.NotContains(t, text, "BMI", "bmi must not render when it was not derived")
			}

			if testCase.notWantWord != "" && !testCase.wantBMI {
				assert.NotContains(t, text, "Weight", "an unrecorded measurement must not render")
			}
		})
	}
}

func fullVitalsSet(recordedAt time.Time, weight float64) clinical.Vitals {
	height := 175.0

	return clinical.Vitals{
		ID: "v2", RecordedAt: clinical.NewInstant(recordedAt),
		WeightKg: &weight, HeightCm: &height,
	}
}
