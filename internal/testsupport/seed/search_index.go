package seed

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

const searchIndexCollection = "search_index"

// IndexRecord writes the search_index row a seeded record would have earned
// through the service; the seed writes rows directly, so nothing else does.
func IndexRecord(app core.App, k kind.Kind, recordID, patientID, title, body string, occurredOn domain.Date) error {
	collection, err := app.FindCollectionByNameOrId(searchIndexCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", searchIndexCollection, err)
	}

	record := core.NewRecord(collection)
	if err := app.RecordQuery(collection).AndWhere(dbx.HashExp{"kind": string(k), "record_id": recordID}).One(record); err != nil {
		record = core.NewRecord(collection)
	}

	record.Set("patient", patientID)
	record.Set("kind", string(k))
	record.Set("record_id", recordID)
	record.Set("title", title)
	record.Set("body", body)

	if occurredOn.IsZero() {
		record.Set("occurred_on", nil)
	} else {
		record.Set("occurred_on", occurredOn.UTC())
	}

	if err := app.Save(record); err != nil {
		return fmt.Errorf("indexing %s %s: %w", k, recordID, err)
	}

	return nil
}
