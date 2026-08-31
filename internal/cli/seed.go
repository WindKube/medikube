package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/seed"
)

// usersCollection is the account collection's name. The migrations own the
// spelling and hold it unexported, so it is re-typed here as it is in every
// other package that needs it; a lookup against a renamed collection fails
// loudly rather than reporting an empty result.
const usersCollection = "users"

// The environments `medikube seed` will write to.
//
// Everything else — including the empty string and a misspelling — is treated
// as production and refused. Failing closed is the whole point: seed data in a
// medical production database is indistinguishable from real data once it is
// in there (FR-060), and the one mistake worth guarding against is a variable
// that was not set at all.
var seedableEnvironments = []string{"staging", "development"}

// ErrProduction is FR-060's refusal. There is deliberately no --force: a flag
// that lets an operator overrule this is a flag somebody will use at 3am.
var ErrProduction = errors.New("cli: medikube seed refuses to write demo data outside a staging or development instance")

// ErrNotMigrated is a database the migrations have not been applied to. It is
// a separate sentinel from a write failure because the fix is different: run
// `medikube migrate`, not "look at the error".
var ErrNotMigrated = errors.New("cli: the database has not been migrated, so there is nothing to seed into")

// SeedAccount is what one account's line of output is built from.
type SeedAccount struct {
	ID    string
	Email string
	// Created distinguishes the first run from every one after it. The seed is
	// idempotent (FR-060), so the second run reports every account as skipped
	// and changes nothing but the updated timestamps.
	Created bool
	// Medications is how many rows the fixture gives this account. It is on
	// the line because the mixed set is the point of the fixture — one account
	// populated, one holding fewer, one empty — and an operator who cannot see
	// the shape of what was written cannot tell a half-applied seed from a
	// complete one (data-model §6, research D-39).
	Medications int
}

// Seed writes the demo fixture of data-model §6 and reports one line per
// account.
//
// The rows themselves are not declared here. internal/testsupport/seed is the
// one place they are written, because the committed fixture every test clones
// and this command must produce the same database — a second table of demo
// medications is how the fixture and the demo instance drift into two
// different applications. This function owns the policy around that write:
// which environments may run it, whether there is a schema to write into, and
// what the operator is told.
func Seed(app core.App, env string, out io.Writer) error {
	if !slices.Contains(seedableEnvironments, env) {
		return fmt.Errorf("%w (MEDIKUBE_ENV is %q)", ErrProduction, env)
	}

	if err := requireSchema(app); err != nil {
		return err
	}

	accounts, err := seedAccounts(app)
	if err != nil {
		return err
	}

	if err := seed.Apply(app); err != nil {
		return fmt.Errorf("cli: writing the demo fixture: %w", err)
	}

	return report(out, accounts)
}

// requireSchema refuses a database the migrations have not reached.
//
// The medication collection is what it asks for rather than the account
// collection: `users` is PocketBase's own and exists on a bare instance, so
// finding it proves nothing about MediKube's migrations having run.
func requireSchema(app core.App) error {
	for _, collection := range []string{usersCollection, kind.Medication.Collection()} {
		if _, err := app.FindCollectionByNameOrId(collection); err != nil {
			return fmt.Errorf("%w: no %s collection: %w", ErrNotMigrated, collection, err)
		}
	}

	return nil
}

// seedAccounts reads the state of each account BEFORE the write, which is the
// only moment "created" and "skipped" are distinguishable: seed.Apply updates
// an existing record in place, so afterwards the two cases are identical.
func seedAccounts(app core.App) ([]SeedAccount, error) {
	counts := medicationCounts()
	accounts := make([]SeedAccount, 0, len(seed.Accounts()))

	for _, account := range seed.Accounts() {
		existing, err := found(app, account.ID)
		if err != nil {
			return nil, err
		}

		accounts = append(accounts, SeedAccount{
			ID:          account.ID,
			Email:       account.Email,
			Created:     !existing,
			Medications: counts[account.ID],
		})
	}

	return accounts, nil
}

// medicationCounts is the fixture's own shape, counted from the table rather
// than declared a second time. An account the fixture gives no rows to is
// absent from the map and reports zero, which is account C and is deliberate.
func medicationCounts() map[string]int {
	counts := make(map[string]int)
	for _, medication := range seed.Medications() {
		counts[medication.OwnerID]++
	}

	return counts
}

func found(app core.App, id string) (bool, error) {
	if _, err := app.FindRecordById(usersCollection, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("cli: reading account %s: %w", id, err)
	}

	return true, nil
}

// report is the command's human output. It goes to the writer it is handed and
// never to the log stream: contracts/cli.md keeps the two apart, so that a
// human reading a terminal and a collector reading JSON each get one thing.
func report(out io.Writer, accounts []SeedAccount) error {
	if out == nil {
		return nil
	}

	for _, account := range accounts {
		state := "skipped"
		if account.Created {
			state = "created"
		}

		if _, err := fmt.Fprintf(out, "%s %s (%s), %d %s\n",
			state, account.Email, account.ID, account.Medications, kind.Medication.Segment()); err != nil {
			return fmt.Errorf("cli: reporting the seed: %w", err)
		}
	}

	return nil
}
