package store

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// Owners resolves the account a record belongs to, for any registered kind.
//
// It is the one read the authorization checkpoint makes, and it is deliberately
// the smallest one there is: an id in, an account id out, nothing else loaded
// and nothing else returned. A checkpoint that fetched the record would be a
// second path to a person's data with no authorization in front of it.
//
// The kind supplies its own collection, so a kind added in a later phase is
// resolvable here without this file changing.
type Owners struct {
	app core.App
}

func NewOwners(app core.App) (*Owners, error) {
	if app == nil {
		return nil, fmt.Errorf("store: the owner lookup is wired with no application")
	}

	return &Owners{app: app}, nil
}

// Owner answers domain.ErrNotFound for a record that is not there.
//
// The caller must not turn that into a refusal: a record that does not exist is
// not the checkpoint's refusal to make, and answering "denied" for every
// mistyped identifier would fill the audit trail with attempts nobody made and
// make a genuine miss indistinguishable from a denial in the one place that can
// still tell them apart (research D-20).
func (o *Owners) Owner(ctx context.Context, k kind.Kind, recordID string) (string, error) {
	if !k.Valid() {
		return "", fmt.Errorf("store: %q is not a declared kind: %w", k, domain.ErrNotFound)
	}

	var owned struct {
		Owner string `db:"owner"`
	}

	err := o.app.RecordQuery(k.Collection()).
		Select(medicationFieldOwner).
		AndWhere(dbx.HashExp{fieldID: recordID}).
		Limit(1).
		WithContext(ctx).
		One(&owned)
	if err != nil {
		return "", fmt.Errorf("store: reading the owner of a %s: %w", k, domain.ErrNotFound)
	}

	return owned.Owner, nil
}
