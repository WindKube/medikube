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
// What that generic sweep does not pin down is a count: that this phase's own
// timestamp range carries exactly the number of migrations data-model.md §8
// declares, no more silently dropped and no more silently added with no
// reversibility case of its own. The count is data-model.md §8's nineteen
// plus the two migrations 17 and 18 that add the cross-kind link fields and
// the join collection carrying a payload, now that US6 has landed them.
//
// This does not spell any migration's filename: internal/architecture's own
// kind-literal gate refuses a collection or path segment spelled anywhere
// outside internal/domain/kind, and most of this phase's migration files are
// named after the collection they create.
const expectedPhase003MigrationCount = 22

// isPhase003File recognises this phase's own timestamp range
// (1756300000-1756499999), assigned after phase 002's own
// (…1756200xxx…1756200600) and before phase 004's.
func isPhase003File(file string) bool {
	return strings.HasPrefix(file, "17563") || strings.HasPrefix(file, "17564")
}

func TestPhase003RegistersExactlyTheMigrationsDataModelDeclares(t *testing.T) {
	t.Parallel()

	var count int

	for _, file := range migrations.Files() {
		if isPhase003File(file) {
			count++
		}
	}

	assert.Equalf(t, expectedPhase003MigrationCount, count,
		"this phase's timestamp range carries %d registered migrations, expected %d "+
			"(data-model.md §8's nineteen, minus the two link migrations US6 has not landed yet)",
		count, expectedPhase003MigrationCount)
}
