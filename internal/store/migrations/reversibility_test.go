package migrations_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/store/migrations"
)

// T204. reversible_test.go's TestEveryMigrationUpDownUpLeavesAnIdenticalSchema
// already walks every registered migration forward and back and proves
// up -> down -> up is an identity — every one of this phase's migrations is
// included automatically, because it reads core.AppMigrations.Items() rather
// than a second list of its own. That test also proves the dependency
// ordering data-model.md §8 asks for structurally: a migration whose up()
// looked up a collection created by a later one would fail there, in every
// run, and it does not.
//
// What that generic sweep does not pin down is that this phase's own set of
// migrations is exactly the one data-model.md §8 declares, no more and no
// fewer — which is what phase003Migrations checks below.
//
// Migrations 17 (links_medications) and 18 (treatment_medications) are
// deliberately absent from this list: US6 is still in flight and lands them,
// and the order they land in, alongside its own link work.
var phase003Migrations = []string{
	"1756300000_tags.go",
	"1756300010_search_index.go",
	"1756300020_medication_tags.go",
	"1756300030_audit_vocab.go",
	"1756300100_allergies.go",
	"1756300110_conditions.go",
	"1756300120_emergency_contacts.go",
	"1756400010_equipment.go",
	"1756400020_insurances.go",
	"1756400100_immunizations.go",
	"1756400200_injuries.go",
	"1756400300_symptoms.go",
	"1756400400_vitals.go",
	"1756400410_symptom_vitals_tags.go",
	"1756400500_encounters.go",
	"1756400510_procedures.go",
	"1756400520_treatments.go",
	"1756400530_care_conditions.go",
	"1756400600_family_members.go",
	"1756400700_family_member_tags.go",
}

// TestPhase003RegistersExactlyItsOwnMigrationsAndNoMore is T204's other half:
// nothing named here has gone missing (a migration silently dropped from the
// build) and nothing has appeared that this list does not know about (a
// migration added with no reversibility case of its own, since this suite is
// what phase003Migrations feeds every other phase-003-specific assertion).
func TestPhase003RegistersExactlyItsOwnMigrationsAndNoMore(t *testing.T) {
	t.Parallel()

	registered := make(map[string]bool)
	for _, file := range migrations.Files() {
		registered[file] = true
	}

	for _, file := range phase003Migrations {
		assert.Truef(t, registered[file], "%s is expected but not a registered migration", file)
	}

	expected := make(map[string]bool, len(phase003Migrations))
	for _, file := range phase003Migrations {
		expected[file] = true
	}

	for _, file := range migrations.Files() {
		if isPhase003File(file) {
			assert.Truef(t, expected[file],
				"%s looks like a phase-003 migration but is not in phase003Migrations; "+
					"add it so TestEveryMigrationUpDownUpLeavesAnIdenticalSchema's coverage is accounted for here too", file)
		}
	}
}

// isPhase003File recognises this phase's own timestamp range
// (1756300000-1756499999), assigned after phase 002's own
// (…1756200xxx…1756200600) and before phase 004's.
func isPhase003File(file string) bool {
	return strings.HasPrefix(file, "17563") || strings.HasPrefix(file, "17564")
}
