package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// T073. Applied == registered is what readyz's `migrations` check reports, and
// it is the only signal that distinguishes "this build is running against the
// schema it expects" from "this build is running against last week's".
//
// The name is composed rather than written out because one of the three
// contains a kind's collection spelling, which lives in the kind table and
// nowhere else (research D-05).
func TestTheAppliedMigrationSetEqualsTheRegisteredSet(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	expected := []string{
		"1756100100_users_profile.go",
		"1756100200_" + kind.Medication.Collection() + ".go",
		"1756100300_audit_events.go",
		"1756200100_facilities.go",
		"1756200200_practitioners.go",
		"1756200300_patients.go",
		"1756200400_users_active_patient.go",
		"1756200500_audit_events_patient.go",
		"1756200600_" + kind.Medication.Collection() + "_repoint.go",
		"1756300000_" + TagsCollection + ".go",
		"1756300010_search_index.go",
		"1756300020_medication_tags.go",
		"1756300030_audit_vocab.go",
		"1756300100_" + kind.Allergy.Collection() + ".go",
		"1756300110_" + kind.Condition.Collection() + ".go",
		"1756300120_" + kind.EmergencyContact.Collection() + ".go",
		"1756400010_" + kind.Equipment.Collection() + ".go",
		"1756400020_" + kind.Insurance.Collection() + ".go",
		"1756400100_" + kind.Immunization.Collection() + ".go",
		"1756400200_" + kind.Injury.Collection() + ".go",
		"1756400300_" + kind.Symptom.Collection() + ".go",
		"1756400400_" + kind.Vitals.Collection() + ".go",
		"1756400500_" + kind.Encounter.Collection() + ".go",
		"1756400510_" + kind.Procedure.Collection() + ".go",
		"1756400520_" + kind.Treatment.Collection() + ".go",
		"1756400530_care_" + kind.Condition.Collection() + ".go",
		"1756400600_" + kind.FamilyMember.Collection() + ".go",
	}

	require.Equal(t, expected, Files(),
		"the registered set is not the migrations phase 001, phase 002 and 003 declare")

	applied, err := Applied(app)
	require.NoError(t, err)
	assert.Equal(t, Files(), applied)

	pending, err := Pending(app)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// The other half: a set that is equal because both sides are empty proves
// nothing, and neither does one that reports the eight PocketBase system
// migrations sharing the same _migrations table.
func TestTheAppliedSetTracksWhatHasActuallyBeenApplied(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	items := core.AppMigrations.Items()
	require.Len(t, items, len(Files()))

	reverted, err := runnerFor(items)(app).Down(1)
	require.NoError(t, err)
	require.Len(t, reverted, 1)

	last := len(Files()) - 1

	applied, err := Applied(app)
	require.NoError(t, err)
	assert.Equal(t, Files()[:last], applied)

	pending, err := Pending(app)
	require.NoError(t, err)
	assert.Equal(t, Files()[last:], pending,
		"a reverted migration must read as pending, or readyz reports green on a half-migrated instance")

	// PocketBase's own system migrations live in the same table. If they were
	// not filtered out, the applied set would exceed the registered one and
	// equality would never hold on a healthy instance.
	var recorded []string
	require.NoError(t, app.DB().Select("file").From(core.DefaultMigrationsTable).Column(&recorded))
	assert.Greater(t, len(recorded), len(applied),
		"the _migrations table is expected to hold PocketBase's system migrations too")
}
