package link

import (
	"context"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// ValidateSet is data-model §7.4's four invariants applied to one
// multi-relation field's incoming ids: deduplicated (FR-056), same patient as
// the subject (FR-057), and editable by the actor (FR-057, both ends). It
// returns the deduplicated set a kind's PATCH may write as-is.
//
// A cross-patient member, a non-existent member and an unreachable member all
// produce the byte-identical domain.ErrNotFound (FR-057, US6-3, SC-004):
// clinical.SamePatient never distinguishes them, and neither does the
// Authorizer.Patient refusal below it.
func ValidateSet(
	ctx context.Context,
	resolver Resolver,
	authorizer Authorizer,
	actor access.Actor,
	subjectPatientID string,
	target kind.Kind,
	ids []string,
) ([]string, error) {
	set := clinical.NewLinkSet(ids...)
	unique := set.IDs()

	if len(unique) == 0 {
		return unique, nil
	}

	refs, err := resolver.Resolve(ctx, target, unique)
	if err != nil {
		return nil, err
	}

	if err := clinical.SamePatient(subjectPatientID, refs); err != nil {
		return nil, err
	}

	for _, ref := range refs {
		grant, err := authorizer.Patient(ctx, actor, ref.PatientID, access.PermEdit)
		if err != nil {
			return nil, err
		}

		if !grant.Allows(access.PermEdit) {
			return nil, domain.ErrNotFound
		}
	}

	return unique, nil
}
