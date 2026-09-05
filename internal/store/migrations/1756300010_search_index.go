package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// SearchIndexCollection stores no content the source row does not already
// store (data-model §5.3): it is a maintained index, written by the
// post-commit hooks internal/service/search binds, and read by US8's search.
const SearchIndexCollection = "search_index"

const (
	searchFieldPatient    = "patient"
	searchFieldKind       = "kind"
	searchFieldRecordID   = "record_id"
	searchFieldTitle      = "title"
	searchFieldBody       = "body"
	searchFieldOccurredOn = "occurred_on"
	searchFieldTags       = "tags"
)

const (
	searchTitleMax    = 500
	searchBodyMax     = 8000
	searchRecordIDLen = 15
)

const (
	searchRecordIndex      = "uniq_search_record"
	searchPatientKindIndex = "idx_search_patient_kind"
)

func init() {
	register(searchIndexUp, searchIndexDown)
}

func searchIndexUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection := core.NewBaseCollection(SearchIndexCollection)
	lockRules(collection)

	// The authorization anchor. Cascade is what makes FR-087 true here: a
	// deleted patient's index rows do not outlive the records they describe.
	collection.Fields.Add(&core.RelationField{
		Name:          searchFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      searchFieldKind,
		Required:  true,
		MaxSelect: 1,
		Values:    kindSegments(),
	})
	collection.Fields.Add(&core.TextField{
		Name:     searchFieldRecordID,
		Required: true,
		Min:      searchRecordIDLen,
		Max:      searchRecordIDLen,
	})
	collection.Fields.Add(&core.TextField{
		Name:     searchFieldTitle,
		Required: true,
		Max:      searchTitleMax,
	})
	collection.Fields.Add(&core.TextField{
		Name: searchFieldBody,
		Max:  searchBodyMax,
	})
	collection.Fields.Add(&core.DateField{Name: searchFieldOccurredOn})
	collection.Fields.Add(&core.RelationField{
		Name:         searchFieldTags,
		MaxSelect:    0,
		CollectionId: tags.Id,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	collection.AddIndex(searchRecordIndex, true, searchFieldKind+", "+searchFieldRecordID, "")
	collection.AddIndex(searchPatientKindIndex, false,
		searchFieldPatient+", "+searchFieldKind+", "+searchFieldOccurredOn+", id", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", SearchIndexCollection, err)
	}

	return nil
}

func searchIndexDown(app core.App) error {
	return deleteCollection(app, SearchIndexCollection)
}

// kindSegments is the fourteen registered record kinds' enum values
// (data-model §5.3: "kind — the 14 kind values"), read from the one table
// rather than spelled here.
func kindSegments() []string {
	values := make([]string, 0, len(kind.Kinds()))
	for _, k := range kind.Kinds() {
		values = append(values, k.Enum())
	}

	return values
}
