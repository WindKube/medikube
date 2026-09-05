package symptom

import (
	"context"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// aggregateRow is one GROUP BY (patient, LOWER(name)) row: FR-031's episode
// count and most recent occurrence, derived on read and never stored
// (research: data-model §4.6, SC-016).
type aggregateRow struct {
	Key   string `db:"agg_key"`
	Count int    `db:"agg_count"`
	Last  string `db:"agg_last"`
}

// Aggregate computes every distinct name's episode count and most recent
// occurrence for one patient, in one query, keyed by the lower-cased name so
// names differing only in case group together.
func Aggregate(ctx context.Context, app core.App, patientID string) (map[string]clinical.Symptom, error) {
	var rows []aggregateRow

	q := "SELECT LOWER(" + fieldName + ") AS agg_key, COUNT(*) AS agg_count, MAX(" + fieldOccurredAt + ") AS agg_last" +
		" FROM " + kind.Symptom.Collection() +
		" WHERE " + fieldPatient + " = {:patient}" +
		" GROUP BY LOWER(" + fieldName + ")"

	if err := app.DB().NewQuery(q).Bind(map[string]any{"patient": patientID}).WithContext(ctx).All(&rows); err != nil {
		return nil, fmt.Errorf("aggregating %s for %s: %w", kind.Symptom, patientID, err)
	}

	result := make(map[string]clinical.Symptom, len(rows))

	for _, row := range rows {
		last, err := parseStoredInstant(row.Last)
		if err != nil {
			return nil, err
		}

		result[row.Key] = clinical.Symptom{EpisodeCount: row.Count, LastOccurredAt: last}
	}

	return result, nil
}

// AggregateOne is Aggregate scoped to a single name — what a single Get needs
// rather than the whole patient's aggregate.
func AggregateOne(ctx context.Context, app core.App, patientID, name string) (clinical.Symptom, error) {
	var row aggregateRow

	q := "SELECT COUNT(*) AS agg_count, MAX(" + fieldOccurredAt + ") AS agg_last" +
		" FROM " + kind.Symptom.Collection() +
		" WHERE " + fieldPatient + " = {:patient} AND LOWER(" + fieldName + ") = {:name}"

	if err := app.DB().NewQuery(q).
		Bind(map[string]any{"patient": patientID, "name": strings.ToLower(name)}).
		WithContext(ctx).One(&row); err != nil {
		return clinical.Symptom{}, fmt.Errorf("aggregating %s %q for %s: %w", kind.Symptom, name, patientID, err)
	}

	last, err := parseStoredInstant(row.Last)
	if err != nil {
		return clinical.Symptom{}, err
	}

	return clinical.Symptom{EpisodeCount: row.Count, LastOccurredAt: last}, nil
}

func parseStoredInstant(raw string) (clinical.Instant, error) {
	if raw == "" {
		return clinical.Instant{}, nil
	}

	parsed, err := types.ParseDateTime(raw)
	if err != nil {
		return clinical.Instant{}, fmt.Errorf("store/symptom: %q is not a stored instant: %w", raw, err)
	}

	return clinical.NewInstant(parsed.Time().UTC()), nil
}
