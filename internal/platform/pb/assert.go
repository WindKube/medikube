package pb

import (
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// The three refusals, each with its own message, because an operator reading a
// container that will not start needs to know which of three unrelated things
// is wrong.
var (
	// ErrAPIRuleSet is the primary lockdown mechanism failing. A non-nil rule
	// means PocketBase's own record API answers for that collection: nil is
	// "superuser only", and types.Pointer("") is its opposite — no constraint
	// at all for anyone the route admits (apis/record_crud.go:52, :165, :231,
	// :420, :557). Nothing warns about getting that backwards; this does.
	ErrAPIRuleSet = errors.New("a non-system collection has a non-nil API rule, so PocketBase's own record API answers for it")

	// ErrBatchEnabled is the door that bypasses the router. Batch calls the
	// record-CRUD handler bodies directly (apis/batch.go:38-88), so no
	// middleware sees those sub-requests.
	ErrBatchEnabled = errors.New("the batch endpoint is enabled, and it re-enters the record-CRUD handlers without passing any middleware")

	// ErrFileFieldUnprotected is the silent one. An unprotected file is not an
	// error anywhere: it is a working download for anyone who has the URL.
	ErrFileFieldUnprotected = errors.New("a file field is not protected, so PocketBase serves it to anyone holding the URL")
)

// AssertLockedDown is the boot gate. The composition root calls it after
// migrations and refuses to start on a non-nil result.
//
// It runs after migrations, not before: MediKube's own collections do not exist
// until RunAllMigrations has applied them, and an assertion that passes because
// there was nothing to check is worse than no assertion.
//
// Every violation is reported together rather than the first one, so an
// operator learns everything that is wrong in one restart.
func AssertLockedDown(app core.App) error {
	collections, err := app.FindAllCollections()
	if err != nil {
		return fmt.Errorf("enumerate the collections: %w", err)
	}

	return errors.Join(
		AssertSettings(app.Settings()),
		AssertCollections(collections),
	)
}

// AssertSettings is the settings half, taking the settings directly so the
// condition can be exercised without a database.
func AssertSettings(settings *core.Settings) error {
	if settings.Batch.Enabled {
		return ErrBatchEnabled
	}

	return nil
}

// AssertCollections is the schema half, taking the collections directly for the
// same reason.
//
// The API-rule check is scoped to non-system collections: PocketBase's own
// _mfas and _otps ship non-nil list rules, MediKube neither owns them nor may
// null them. The file check is not scoped, because "any file field" means any —
// PocketBase ships none, so the wider rule costs nothing today and closes the
// door on an upgrade that adds one.
//
// AuthRule is deliberately not a sixth: it lives on the auth options rather
// than on the base collection, and nulling it disables PocketBase-native
// authentication outright — password, OAuth2, everything.
func AssertCollections(collections []*core.Collection) error {
	var violations []error

	for _, collection := range collections {
		if !collection.System {
			violations = append(violations, apiRuleViolations(collection)...)
		}

		violations = append(violations, fileFieldViolations(collection)...)
	}

	return errors.Join(violations...)
}

func apiRuleViolations(collection *core.Collection) []error {
	rules := []struct {
		name  string
		value *string
	}{
		{"listRule", collection.ListRule},
		{"viewRule", collection.ViewRule},
		{"createRule", collection.CreateRule},
		{"updateRule", collection.UpdateRule},
		{"deleteRule", collection.DeleteRule},
	}

	var violations []error

	for _, rule := range rules {
		if rule.value == nil {
			continue
		}

		violations = append(violations, fmt.Errorf(
			"%w: %s.%s is %q and must be null",
			ErrAPIRuleSet, collection.Name, rule.name, *rule.value,
		))
	}

	return violations
}

func fileFieldViolations(collection *core.Collection) []error {
	var violations []error

	for _, field := range collection.Fields {
		file, ok := field.(*core.FileField)
		if !ok || file.Protected {
			continue
		}

		violations = append(violations, fmt.Errorf(
			"%w: %s.%s must set Protected",
			ErrFileFieldUnprotected, collection.Name, file.Name,
		))
	}

	return violations
}
