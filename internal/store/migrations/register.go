package migrations

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	pbmigrations "github.com/pocketbase/pocketbase/migrations"
)

// usersCollection is the collection PocketBase's own initial migration creates
// and this phase amends. It is not a kind.Kind — an account is not a clinical
// record — so the spelling lives here rather than in the kind table.
const usersCollection = "users"

// auditEventsCollection is the audit trail. Also not a kind: audit_events is
// what records that something happened to a record, not a record kind itself.
const auditEventsCollection = "audit_events"

// register is what every migration file in this package calls from its init.
//
// It exists for two reasons that a direct call to the PocketBase alias does not
// give. First, both directions are non-nil or the process does not start:
// Principle IX's reversibility rule is only enforced by the signature if
// nothing may pass nil through it. Second, the file name is derived here from
// the *caller's* frame rather than passed as a literal — the collection name
// appears in one of these file names, and internal/architecture's kind-literal
// walk refuses a kind's collection spelled as a string anywhere outside the
// kind table.
func register(up, down func(app core.App) error) {
	_, path, _, ok := runtime.Caller(1)
	if !ok {
		panic("migrations: cannot determine the calling migration's file name")
	}

	file := filepath.Base(path)

	if up == nil || down == nil {
		panic("migrations: " + file + " must supply both an up and a down")
	}

	pbmigrations.Register(up, down, file)
}

// Files is MediKube's registered migrations, in the order the runner applies
// them, which is file-name order. It reads core.AppMigrations rather than a
// second list of its own, so a migration that is registered but forgotten here
// is not a thing that can happen.
func Files() []string {
	items := core.AppMigrations.Items()

	files := make([]string, 0, len(items))
	for _, item := range items {
		files = append(files, item.File)
	}

	return files
}

// Applied is the subset of Files already recorded in PocketBase's _migrations
// table, sorted the same way.
//
// The table is shared with PocketBase's own system migrations, so the rows are
// intersected with the registered set rather than returned raw: without that,
// "applied equals registered" is false by eight on every healthy instance.
func Applied(app core.App) ([]string, error) {
	var recorded []string

	err := app.DB().
		Select("file").
		From(core.DefaultMigrationsTable).
		OrderBy("file ASC").
		Column(&recorded)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", core.DefaultMigrationsTable, err)
	}

	registered := Files()

	applied := make([]string, 0, len(registered))
	for _, file := range registered {
		if slices.Contains(recorded, file) {
			applied = append(applied, file)
		}
	}

	return applied, nil
}

// Pending is what readyz reports on: a non-empty result means the schema this
// build expects is not the schema the database has.
func Pending(app core.App) ([]string, error) {
	applied, err := Applied(app)
	if err != nil {
		return nil, err
	}

	pending := make([]string, 0)
	for _, file := range Files() {
		if !slices.Contains(applied, file) {
			pending = append(pending, file)
		}
	}

	return pending, nil
}

// lockRules is the lockdown at the schema layer (constitution Principle V):
// nil on all five is superuser-only, and it is the opposite of
// types.Pointer(""), which is no constraint at all. The two are one character
// apart, nothing at save time distinguishes them, and AssertAPIRules is the
// only control — so the migrations set them through one named call rather than
// five assignments each.
func lockRules(collection *core.Collection) {
	collection.ListRule = nil
	collection.ViewRule = nil
	collection.CreateRule = nil
	collection.UpdateRule = nil
	collection.DeleteRule = nil
}

// enumValues renders a domain vocabulary for a core.SelectField. The migration
// never writes the values out by hand: the Go enum and the select field are the
// same list or a value the domain accepts fails validation at the column.
func enumValues[T ~string](vocabulary []T) []string {
	values := make([]string, 0, len(vocabulary))
	for _, value := range vocabulary {
		values = append(values, string(value))
	}

	return values
}

// deleteCollection is the down of every migration that creates one.
func deleteCollection(app core.App, name string) error {
	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := app.Delete(collection); err != nil {
		return fmt.Errorf("deleting %s: %w", name, err)
	}

	return nil
}

// relationField is how the assertions and the migrations both reach a relation's
// two booleans: neither Required nor CascadeDelete is on the core.Field
// interface, so every reader needs the concrete type.
func relationField(collection *core.Collection, name string) (*core.RelationField, error) {
	field := collection.Fields.GetByName(name)
	if field == nil {
		return nil, fmt.Errorf("%s has no %s field", collection.Name, name)
	}

	relation, ok := field.(*core.RelationField)
	if !ok {
		return nil, fmt.Errorf("%s.%s is a %s field, not a relation", collection.Name, name, field.Type())
	}

	return relation, nil
}
