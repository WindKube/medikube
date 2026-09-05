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

// T089: the symptom list row states the episode count and most recent date.
func TestTheSymptomRowStatesTheEpisodeCountAndMostRecentDate(t *testing.T) {
	t.Parallel()

	occurredAt, err := time.Parse(time.RFC3339, "2026-01-01T09:00:00Z")
	require.NoError(t, err)

	lastOccurred, err := time.Parse(time.RFC3339, "2026-03-01T09:00:00Z")
	require.NoError(t, err)

	symptom := clinical.Symptom{
		ID:             "sym1",
		Name:           "Headache",
		Severity:       clinical.SeverityModerate,
		OccurredAt:     clinical.NewInstant(occurredAt),
		EpisodeCount:   4,
		LastOccurredAt: clinical.NewInstant(lastOccurred),
	}

	view := records.NewSymptomView(symptom, records.SymptomLinks{Detail: "/" + kind.Symptom.Segment() + "/sym1"})
	tree := viewstest.Render(t, records.SymptomRow(view), "tbody")

	row := tree.One(t, viewstest.Tag("tr"))
	text := viewstest.Text(row)

	assert.Contains(t, text, "4", "the row does not state the episode count")
	assert.Contains(t, text, view.LastOccurredAt, "the row does not state the most recent occurrence")
}
