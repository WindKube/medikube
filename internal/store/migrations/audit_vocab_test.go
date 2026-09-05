package migrations

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/audit"
)

// T070a. The assertion is set equality, not containment, in both directions: a
// value the column declares but data-model §3 does not is as much a failure as
// a missing one, so a later phase cannot quietly widen the trail by editing a
// migration.
//
// This is the test every later phase's vocabulary migration extends and
// re-asserts against the complete expected set. A value a phase writes but no
// migration declared then shows up here as a red test rather than as a
// SelectField validation failure in production, on the first share or the first
// backup.
func TestAuditEventVocabularyIsExactlyWhatDataModelDeclares(t *testing.T) {
	t.Parallel()

	collection := auditCollection(t, newTestApp(t))

	// The counts are written out as literals on purpose. Comparing the column
	// against audit.Actions() alone would pass if somebody deleted a value from
	// both sides at once, which is exactly the "quietly narrow the trail" move
	// the numbers in data-model §3 exist to stop.
	cases := []struct {
		field    string
		expected []string
		count    int
	}{
		{field: auditFieldActorKind, expected: enumValues(audit.ActorKinds()), count: 4},
		{field: auditFieldAction, expected: enumValues(audit.Actions()), count: 21},
		{field: auditFieldTargetKind, expected: enumValues(audit.TargetKinds()), count: 25},
	}

	for _, testCase := range cases {
		t.Run(testCase.field, func(t *testing.T) {
			t.Parallel()

			field := collection.Fields.GetByName(testCase.field)
			require.NotNil(t, field, "the column does not exist")

			selectField, isSelect := field.(*core.SelectField)
			require.Truef(t, isSelect, "%s is a %s field, not a select", testCase.field, field.Type())

			require.Len(t, testCase.expected, testCase.count,
				"the domain vocabulary changed size; data-model §3 says %d", testCase.count)

			assert.ElementsMatch(t, testCase.expected, selectField.Values)
			assert.Len(t, selectField.Values, testCase.count)

			// Single-select. A multi-value audit column would let one row claim
			// two actions, and nothing downstream could tell which happened.
			assert.Equal(t, 1, selectField.MaxSelect)
			assert.True(t, selectField.Required, "every one of the three is a required column")
		})
	}
}

// The two column sizes the later phases depend on. Phase 006 writes 20–31
// character job names and ~40 character archive names into target_id, and
// data-model §3 works the longest case out to 58 — against a PocketBase record
// id of 15, which is what the column would otherwise have been sized for.
func TestAuditEventTextColumnsAreSizedForThePhasesThatWriteThem(t *testing.T) {
	t.Parallel()

	collection := auditCollection(t, newTestApp(t))

	cases := []struct {
		field    string
		max      int
		required bool
		why      string
	}{
		{
			field: auditFieldTargetID, max: audit.MaxTargetID, required: false,
			why: "phase 006 writes job and archive names here; data-model §3 sizes the longest at 58",
		},
		{
			field: auditFieldRequestID, max: audit.MaxRequestID, required: true,
			why: "required, so a row that correlates to nothing cannot be written (FR-054)",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.field, func(t *testing.T) {
			t.Parallel()

			field := collection.Fields.GetByName(testCase.field)
			require.NotNil(t, field)

			text, isText := field.(*core.TextField)
			require.Truef(t, isText, "%s is a %s field, not text", testCase.field, field.Type())

			assert.Equal(t, 64, testCase.max, "data-model §3 sizes both columns at 64")
			assert.Equal(t, testCase.max, text.Max, testCase.why)
			assert.Equal(t, testCase.required, text.Required, testCase.why)
		})
	}
}

// The negative half of data-model §3, and the reason that section calls the
// collection's defining property negative: there is no column here that a
// value, a name, a note or a diff could be written into. Each of these is named
// with the phase that adds it, so a name appearing early is a phase reaching
// backwards rather than a typo.
func TestAuditEventsHasNoColumnAValueCouldBeWrittenInto(t *testing.T) {
	t.Parallel()

	collection := auditCollection(t, newTestApp(t))

	for _, absent := range []string{"ip", "reason", "affected", "content", "detail", "message", "note"} {
		assert.Nilf(t, collection.Fields.GetByName(absent),
			"audit_events has an %s column; data-model §7 says which phase adds it, and it is not this one", absent)
	}

	// The complete column list, in the order data-model §3 declares it, plus
	// the three §1.0 puts on every collection. Asserted as a whole rather than
	// as absences, because the failure this catches is an addition nobody
	// listed above.
	expected := []string{
		core.FieldNameId,
		auditFieldOccurredAt,
		auditFieldActor,
		auditFieldActorKind,
		auditFieldAction,
		auditFieldTargetKind,
		auditFieldTargetID,
		auditFieldRequestID,
		fieldCreated,
		fieldUpdated,
	}

	assert.Equal(t, expected, collection.Fields.FieldNames())
}

func auditCollection(t *testing.T, app core.App) *core.Collection {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(auditEventsCollection)
	require.NoError(t, err)

	return collection
}

// The three indexes of data-model §3, created wide enough for phase 006's
// reader on day one. Each carries the tiebreaker 006's keyset paging needs to
// stay index-only, so 006 creates no audit index at all — the alternative puts
// six b-trees on the highest-write-volume collection in the instance, and 006's
// idx_audit_target would collide by name with the one here and fail outright.
func TestAuditIndexesAlreadyCarryPhase006sTiebreakers(t *testing.T) {
	t.Parallel()

	collection := auditCollection(t, newTestApp(t))

	cases := []struct {
		index   string
		columns string
	}{
		{index: auditOccurredIndex, columns: "(occurred_at DESC, id DESC)"},
		{index: auditActorTimeIndex, columns: "(actor, occurred_at DESC, id DESC)"},
		{index: auditTargetIndex, columns: "(target_kind, target_id, occurred_at DESC)"},
	}

	require.Len(t, collection.Indexes, len(cases),
		"a fourth audit index means a later phase added one this collection was meant to already have")

	for _, testCase := range cases {
		t.Run(testCase.index, func(t *testing.T) {
			t.Parallel()

			declared := indexNamed(t, collection, testCase.index)

			assert.Contains(t, declared, testCase.columns)
			assert.True(t,
				strings.HasSuffix(declared, "id DESC)") || strings.HasSuffix(declared, "occurred_at DESC)"),
				"a reader index with no tiebreaker pages unstably: %s", declared)
		})
	}
}
