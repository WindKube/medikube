// Package link is FR-055/FR-056/FR-057's shared validation for every
// multi-relation link field: allergies.medications, conditions.medications,
// symptoms.treated_by_medications, symptoms.caused_by_medications, and any
// later phase's own. One kind's PATCH resolves the replace-set semantics
// against clinical.LinkSet and calls ValidateSet with the ids it was handed;
// this package decides nothing about which kind is the subject.
package link

import (
	"context"

	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// Resolver reads a target kind's stored ids, returning each one's own patient
// and whether it exists at all — a missing row and one this actor cannot
// reach both come back Found: false, and ValidateSet reports the identical
// refusal for either (FR-057, research D-08).
type Resolver interface {
	Resolve(ctx context.Context, target kind.Kind, ids []string) ([]clinical.PatientRef, error)
}

// Authorizer is the per-record checkpoint: may this actor edit this one
// target record. data-model §7.4 requires it on every id in a link mutation,
// not just the subject's own record.
type Authorizer interface {
	Record(ctx context.Context, actor access.Actor, k kind.Kind, id string, need access.Permission) (access.Grant, error)
}
