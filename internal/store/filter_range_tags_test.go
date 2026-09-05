package store

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

func buildWhere(t *testing.T, schema Schema, query Query) (string, dbx.Params) {
	t.Helper()

	built, err := schema.Build(query)
	require.NoError(t, err)

	params := dbx.Params{}
	var where string
	if built.Where != nil {
		where = built.Where.Build(nil, params)
	}

	return where, params
}

// T033: date range (GTE/LTE), the status set (OpOneOf, already covered
// elsewhere in this package), and tags narrowing (?tags=&match=any|all).
func TestDateRangeIsTwoConditionsANDed(t *testing.T) {
	t.Parallel()

	schema := MedicationSchema()

	where, params := buildWhere(t, schema, Query{Conditions: []Condition{
		GTE(medicationFieldStartedOn, "2026-01-01"),
		LTE(medicationFieldStartedOn, "2026-03-01"),
	}})

	assert.Equal(t, "([[started_on]] >= {:mk0}) AND ([[started_on]] <= {:mk1})", where)
	assert.Equal(t, dbx.Params{"mk0": "2026-01-01", "mk1": "2026-03-01"}, params)
}

func TestTagsMatchAnyIsADisjunctionOfMembership(t *testing.T) {
	t.Parallel()

	schema := NewSchema(kind.Medication.Collection(), Column{Name: "tags"})

	where, params := buildWhere(t, schema, Query{Conditions: []Condition{AnyOf("tags", "t1", "t2")}})

	assert.Equal(t, `(([[tags]] LIKE {:mk0} ESCAPE '\' OR [[tags]] LIKE {:mk1} ESCAPE '\'))`, where)
	assert.Equal(t, dbx.Params{"mk0": `%"t1"%`, "mk1": `%"t2"%`}, params)
}

func TestTagsMatchAllIsAConjunctionOfMembership(t *testing.T) {
	t.Parallel()

	schema := NewSchema(kind.Medication.Collection(), Column{Name: "tags"})

	where, _ := buildWhere(t, schema, Query{Conditions: []Condition{AllOf("tags", "t1", "t2")}})

	assert.Equal(t, `(([[tags]] LIKE {:mk0} ESCAPE '\' AND [[tags]] LIKE {:mk1} ESCAPE '\'))`, where)
}

func TestATagIDContainingAWildcardMatchesOnlyItself(t *testing.T) {
	t.Parallel()

	schema := NewSchema(kind.Medication.Collection(), Column{Name: "tags"})

	_, params := buildWhere(t, schema, Query{Conditions: []Condition{AnyOf("tags", "50%_id")}})

	assert.Equal(t, `%"50\%\_id"%`, params["mk0"])
}

func TestAnOperatorWithNoValuesIsRefused(t *testing.T) {
	t.Parallel()

	schema := NewSchema(kind.Medication.Collection(), Column{Name: "tags"})

	_, err := schema.Build(Query{Conditions: []Condition{{Columns: []string{"tags"}, Op: OpAnyOf}}})
	require.ErrorIs(t, err, ErrInvalidQuery)
}
