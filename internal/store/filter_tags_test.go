package store_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/testsupport/seed"
)

// taggedFixture is the one seeded row and tag internal/testsupport/seed/
// tags.go applies for a given kind (FR-085): both are needed to prove the
// narrowing, so this is data rather than something derivable from the kind
// table alone.
var taggedFixture = map[kind.Kind]struct {
	recordID string
	tag      string
}{
	kind.Medication:       {seed.NameOnlyID, seed.TagChronicID},
	kind.Immunization:     {seed.ImmunizationSampleID, seed.TagFlaggedID},
	kind.Injury:           {seed.InjuryAnkleID, seed.TagFollowUpID},
	kind.Insurance:        {seed.InsurancePrimaryID, seed.TagChronicID},
	kind.Equipment:        {seed.EquipmentOverdueID, seed.TagFlaggedID},
	kind.Symptom:          {seed.SymptomHeadacheOne, seed.TagFollowUpID},
	kind.Vitals:           {seed.VitalsOne, seed.TagChronicID},
	kind.Allergy:          {seed.CriticalAllergyID, seed.TagFlaggedID},
	kind.Condition:        {seed.ResolvedConditionID, seed.TagChronicID},
	kind.EmergencyContact: {seed.PrimaryContactID, seed.TagFollowUpID},
	kind.Encounter:        {seed.EncounterNameOnlyID, seed.TagChronicID},
	kind.Procedure:        {seed.ProcedureNameOnlyID, seed.TagFlaggedID},
	kind.Treatment:        {seed.TreatmentNameOnlyID, seed.TagFollowUpID},
	kind.FamilyMember:     {seed.FamilyMemberGrandmotherID, seed.TagChronicID},
}

// T156, FR-067: `?tags=a,b&match=any|all` narrows correctly on every kind
// this build registers. It iterates kind.Kinds() rather than a hand list, so
// a fifteenth kind that forgets its row in taggedFixture fails loudly instead
// of silently going untested. The fixture already carries the FR-085 wiring
// (internal/testsupport/seed/tags.go) — one seeded row of every registered
// kind carries at least one of account A's three tags — so this asserts the
// narrowing against real rows rather than against a schema built for the
// occasion.
func TestTagsNarrowEveryRegisteredKind(t *testing.T) {
	t.Parallel()

	app := testsupport.NewApp(t)

	for _, k := range kind.Kinds() {
		t.Run(k.Enum(), func(t *testing.T) {
			t.Parallel()

			fixture, declared := taggedFixture[k]
			require.True(t, declared, "%s has no row in taggedFixture", k.Enum())

			ids := tagQuery(t, app, k.Collection(), store.AnyOf("tags", fixture.tag))
			assert.Contains(t, ids, fixture.recordID)

			ids = tagQuery(t, app, k.Collection(), store.AnyOf("tags", "no-such-tag-id"))
			assert.NotContains(t, ids, fixture.recordID)
		})
	}
}

// The medication seeded by NameOnlyID carries both chronic and follow-up
// (internal/testsupport/seed/tags.go), which is what makes match=all
// distinguishable from match=any on the same row.
func TestTagsMatchAllNarrowsToRecordsCarryingEveryTag(t *testing.T) {
	t.Parallel()

	app := testsupport.NewApp(t)

	both := tagQuery(t, app, kind.Medication.Collection(),
		store.AllOf("tags", seed.TagChronicID, seed.TagFollowUpID))
	assert.Contains(t, both, seed.NameOnlyID)

	// account A's medication carries no "flagged" tag, so requiring it
	// alongside chronic excludes the row that match=any would still return.
	notFlagged := tagQuery(t, app, kind.Medication.Collection(),
		store.AllOf("tags", seed.TagChronicID, seed.TagFlaggedID))
	assert.NotContains(t, notFlagged, seed.NameOnlyID)

	any := tagQuery(t, app, kind.Medication.Collection(),
		store.AnyOf("tags", seed.TagChronicID, seed.TagFlaggedID))
	assert.Contains(t, any, seed.NameOnlyID)
}

func tagQuery(t *testing.T, app core.App, collection string, condition store.Condition) []string {
	t.Helper()

	schema := store.NewSchema(collection, store.Column{Name: "tags"})

	built, err := schema.Build(store.Query{Conditions: []store.Condition{condition}})
	require.NoError(t, err)

	var records []*core.Record
	require.NoError(t, built.Apply(app.RecordQuery(collection)).All(&records))

	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.Id)
	}

	return ids
}
