package api

import (
	"context"
	"errors"

	"medikube/internal/domain/kind"
	"medikube/internal/store/link"
)

// ReferencesSummary is FR-006's pre-delete count: every record that names
// this one, so a delete confirmation can say what else is affected before it
// happens rather than after. It is read-only and never itself a write
// target — nothing decodes into it.
type ReferencesSummary struct {
	Total  int            `json:"total"`
	ByKind map[string]int `json:"by_kind,omitempty"`
}

// ReferencesResolve resolves the back-relation reader once, the same lazy
// shape CourseMedicationResolve and Resolve already use (cmd/medikube's own
// doc comment on Resolve explains why: the reader needs a migrated database
// that does not exist at route-table construction time).
type ReferencesResolve func() (*link.Backrelations, error)

// ErrNoReferences is a build whose back-relation reader was never resolved.
var ErrNoReferences = errors.New("api: the references summary was wired without a way to resolve the back-relation reader")

// summarizeReferences groups a back-relation read by the kind that made it,
// which is `by_kind`'s whole definition.
func summarizeReferences(refs []link.Ref) *ReferencesSummary {
	byKind := make(map[string]int, len(refs))
	for _, ref := range refs {
		byKind[string(ref.Kind)]++
	}

	return &ReferencesSummary{Total: len(refs), ByKind: byKind}
}

// referenceReaders is the only place that knows which kinds have real inbound
// edges in this data model, and it is a map rather than a switch on purpose
// (internal/architecture's kind_switch_test.go forbids dispatching on a
// kind.Kind-typed expression anywhere outside internal/records). Every kind
// with no entry here is a leaf with no multi-relation field or join row
// pointing at it — its references are {total: 0} by construction, which the
// map's zero value (a nil lookup) answers directly rather than running a
// query that can only ever find nothing.
var referenceReaders = map[kind.Kind]func(ctx context.Context, backrel *link.Backrelations, id string) ([]link.Ref, error){
	kind.Medication: func(ctx context.Context, backrel *link.Backrelations, id string) ([]link.Ref, error) {
		refs, err := backrel.Medications(ctx, id)
		if err != nil {
			return nil, err
		}

		joins, err := backrel.TreatmentMedicationTreatments(ctx, id)
		if err != nil {
			return nil, err
		}

		return append(refs, joins...), nil
	},
	kind.Condition: func(ctx context.Context, backrel *link.Backrelations, id string) ([]link.Ref, error) {
		return backrel.Conditions(ctx, id)
	},
	kind.Treatment: func(ctx context.Context, backrel *link.Backrelations, id string) ([]link.Ref, error) {
		return backrel.TreatmentMedicationMedications(ctx, id)
	},
}

func referencesFor(ctx context.Context, backrel *link.Backrelations, k kind.Kind, id string) (*ReferencesSummary, error) {
	read, registered := referenceReaders[k]
	if !registered {
		return summarizeReferences(nil), nil
	}

	refs, err := read(ctx, backrel, id)
	if err != nil {
		return nil, err
	}

	return summarizeReferences(refs), nil
}

// attachReferences populates the one DTO field references.go owns, on
// whichever of the three kinds with real inbound edges the body happens to
// be. Every other kind's Detail body is left exactly as its own codec built
// it — Codec.Detail stays the pure, ctx-free function it is documented to be
// (medication.Adapter's own doc comment), and this is the one seam that
// reaches past it after the fact rather than threading a database dependency
// through every kind's codec for the sake of three of them.
//
// A free function rather than a *recordHandlers method: courseMedicationHandlers'
// own stale-If-Match response (coursemedications.go's failure) re-reads a
// treatment the same way records.go's own update/delete failure paths do,
// and must attach the same field the same way rather than reimplementing it.
func attachReferences(ctx context.Context, references ReferencesResolve, k kind.Kind, id string, body any) error {
	if references == nil {
		return nil
	}

	switch v := body.(type) {
	case *Medication:
		summary, err := resolveReferences(ctx, references, k, id)
		if err != nil {
			return err
		}

		v.References = summary

	case *Condition:
		summary, err := resolveReferences(ctx, references, k, id)
		if err != nil {
			return err
		}

		v.References = summary

	case *Treatment:
		summary, err := resolveReferences(ctx, references, k, id)
		if err != nil {
			return err
		}

		v.References = summary
	}

	return nil
}

func resolveReferences(ctx context.Context, references ReferencesResolve, k kind.Kind, id string) (*ReferencesSummary, error) {
	backrel, err := references()
	if err != nil {
		return nil, err
	}

	return referencesFor(ctx, backrel, k, id)
}
