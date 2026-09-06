// Package link is the PocketBase side of FR-055's shared link machinery:
// resolving a multi-relation's target ids to their stored patient
// (Resolver, for internal/service/link) and reading a kind's own
// back-relations (Backrelations, for the detail view and the pre-delete
// reference count).
package link

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// fieldPatient is every clinical collection's own anchor column (data-model
// §0). Duplicated here rather than imported, the same way every store
// package already names its own field constants independently.
const fieldPatient = "patient"

// Resolver implements internal/service/link.Resolver against real
// collections: a target's stored patient and whether it exists at all. A
// missing row and one this actor could not otherwise reach are the same
// Found: false, because FR-057's refusal never distinguishes them.
type Resolver struct {
	app core.App
}

func NewResolver(app core.App) (*Resolver, error) {
	if app == nil {
		return nil, fmt.Errorf("store/link: the resolver is wired with no application")
	}

	return &Resolver{app: app}, nil
}

func (r *Resolver) Resolve(_ context.Context, target kind.Kind, ids []string) ([]clinical.PatientRef, error) {
	refs := make([]clinical.PatientRef, 0, len(ids))

	for _, id := range ids {
		record, err := r.app.FindRecordById(target.Collection(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				refs = append(refs, clinical.PatientRef{ID: id, Found: false})

				continue
			}

			return nil, fmt.Errorf("store/link: finding %s %s: %w", target.Collection(), id, err)
		}

		refs = append(refs, clinical.PatientRef{
			ID: id, PatientID: record.GetString(fieldPatient), Found: true,
		})
	}

	return refs, nil
}
