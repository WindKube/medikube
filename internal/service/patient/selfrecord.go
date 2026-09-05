package patient

import (
	"context"
	"errors"
	"strings"

	"medikube/internal/domain"
	"medikube/internal/domain/person"
)

// CreateSelfRecord provisions the one patient FR-005 guarantees exists for
// every account: at registration, and by the migration for an account that
// predates this phase (research D-10).
//
// It is the one path that may set is_self_record and relationship_to_owner
// without going through Create's ordinary validation: research D-09 makes
// first_name, last_name and birth_date collection-optional precisely so this
// call can leave all three unset, and person.Patient.Validate() already
// exempts a row with IsSelfRecord set.
//
// FR-004's conflict is checked here, ahead of the write, and the partial
// unique index underneath is the second, storage-layer line of defence
// (data-model §3) — never fabricated, never a runtime check nobody else can
// see.
func (s *Service) CreateSelfRecord(ctx context.Context, ownerID, displayName string) (person.Patient, error) {
	if ownerID == "" {
		return person.Patient{}, errors.New("patient: a self-record needs an owner")
	}

	_, err := s.repository.SelfRecord(ctx, ownerID)

	switch {
	case err == nil:
		return person.Patient{}, domain.ErrConflict
	case errors.Is(err, domain.ErrNotFound):
		// The expected case: nothing to provision over.
	default:
		return person.Patient{}, err
	}

	first, last := splitDisplayName(displayName)

	draft := person.Patient{
		OwnerID:             ownerID,
		FirstName:           first,
		LastName:            last,
		RelationshipToOwner: person.RelationshipSelf,
		IsSelfRecord:        true,
	}

	if err := draft.Validate(); err != nil {
		return person.Patient{}, err
	}

	return s.repository.Create(ctx, draft)
}

// splitDisplayName is research D-10's rule: split on the last space, never
// invent data. A name with no space is entirely the first name, and the
// birth date is never fabricated — it stays the zero value, which
// person.Patient renders as "not recorded".
func splitDisplayName(name string) (first, last string) {
	trimmed := strings.TrimSpace(name)

	if i := strings.LastIndex(trimmed, " "); i > 0 {
		return trimmed[:i], trimmed[i+1:]
	}

	return trimmed, ""
}
