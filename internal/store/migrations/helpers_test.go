package migrations

import (
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/require"
)

// newTestApp builds one instance per test against an empty directory rather
// than PocketBase's own tests/data fixture, which carries demo collections this
// package would then have to filter out of every schema assertion.
//
// tests.NewTestApp has already run every migration by the time it returns.
func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	return app
}

// masterRow is one row of sqlite_master: what the database physically has, as
// opposed to what the collection metadata says it should.
type masterRow struct {
	Type    string `db:"type"`
	Name    string `db:"name"`
	TblName string `db:"tbl_name"`
	SQL     string `db:"sql"`
}

// schemaSnapshot is the equality oracle T068 rests on, and it compares two
// independent things because either alone is too weak.
//
// The first is every collection's own deterministic JSON, sorted by id. That is
// PocketBase's serialization of everything a migration can change: the name,
// the type, the system flag, all five API rules, the complete ordered field
// list with every per-field setting (Required, Min, Max, Values, MaxSelect,
// CollectionId, CascadeDelete, OnCreate/OnUpdate, Pattern), the index SQL, and
// for an auth collection every auth option down to each token duration. It is
// emitted with json.Deterministic(true), so key order is stable, and
// MarshalJSON blanks the five token secrets — which is what makes the
// comparison survive a delete-and-recreate, since a recreated auth collection
// gets fresh random ones.
//
// The second is sqlite_master. The metadata can agree while the database does
// not: nearly every field type compiles to `TEXT DEFAULT ” NOT NULL`, so a
// changed Max is invisible in the DDL — but a column the table sync failed to
// add, or an index that never reached SQLite, is invisible in the metadata.
// Neither half catches the other's blind spot; together they cover both.
//
// Two things are deliberately excluded. `created` and `updated` on the
// collection record differ by construction on any recreate and are zeroed.
// Row data is not compared at all: _migrations.applied is a fresh microsecond
// timestamp on every apply.
func schemaSnapshot(t *testing.T, app core.App) string {
	t.Helper()

	collections, err := app.FindAllCollections()
	require.NoError(t, err)
	require.NotEmpty(t, collections, "the snapshot found no collections; it is not looking at a bootstrapped app")

	slices.SortFunc(collections, func(a, b *core.Collection) int {
		return strings.Compare(a.Id, b.Id)
	})

	var out strings.Builder

	for _, collection := range collections {
		collection.Created = types.DateTime{}
		collection.Updated = types.DateTime{}

		out.WriteString(collection.String())
		out.WriteByte('\n')
	}

	var rows []masterRow

	err = app.DB().
		Select("type", "name", "tbl_name", "sql").
		From("sqlite_master").
		Where(dbx.NewExp("sql IS NOT NULL AND name NOT LIKE 'sqlite_autoindex_%'")).
		OrderBy("type ASC", "tbl_name ASC", "name ASC").
		All(&rows)
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	for _, row := range rows {
		out.WriteString(row.Type)
		out.WriteByte('\t')
		out.WriteString(row.TblName)
		out.WriteByte('\t')
		out.WriteString(row.Name)
		out.WriteByte('\t')
		out.WriteString(normalizeDDL(row.SQL))
		out.WriteByte('\n')
	}

	return out.String()
}

// normalizeDDL removes the two ways SQLite spells the same table differently
// without meaning anything different by it.
//
// Identifier quoting: PocketBase writes backticks, SQLite's own ALTER TABLE
// rebuild writes double quotes.
//
// Column order: dropping a column rebuilds the table and appends the survivors
// in a new order. Nothing in PocketBase addresses a column by position — every
// read and write is by name — so a reversal that restores every column with
// every constraint but leaves them in another order is still a reversal. The
// column *set* is what this compares, and a column that failed to be added or
// to be dropped still shows up.
func normalizeDDL(sql string) string {
	flat := strings.Join(strings.Fields(strings.NewReplacer("`", "", `"`, "").Replace(sql)), " ")

	open := strings.Index(flat, "(")
	if !strings.HasPrefix(flat, "CREATE TABLE ") || open < 0 || !strings.HasSuffix(flat, ")") {
		return flat
	}

	columns := splitTopLevel(flat[open+1 : len(flat)-1])
	slices.Sort(columns)

	return flat[:open+1] + strings.Join(columns, ", ") + ")"
}

// splitTopLevel splits on commas that are not inside parentheses, so a column
// default such as ('r'||lower(hex(randomblob(7)))) survives intact.
func splitTopLevel(list string) []string {
	var (
		parts []string
		depth int
		start int
	)

	for i, r := range list {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(list[start:i]))
				start = i + 1
			}
		}
	}

	return append(parts, strings.TrimSpace(list[start:]))
}

// runnerFor drives an arbitrary prefix of the registered migrations. Up applies
// everything pending in its own list, so stepping forward one migration at a
// time means handing it a shorter list rather than asking it for one step.
func runnerFor(migrations []*core.Migration) func(core.App) *core.MigrationsRunner {
	var list core.MigrationsList
	for _, migration := range migrations {
		list.Add(&core.Migration{Up: migration.Up, Down: migration.Down, File: migration.File})
	}

	return func(app core.App) *core.MigrationsRunner {
		return core.NewMigrationsRunner(app, list)
	}
}
