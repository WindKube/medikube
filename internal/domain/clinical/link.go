package clinical

import "medikube/internal/domain"

// PatientRef is one target of a link mutation: the id the caller named, and
// the patient stored on the row it actually resolved to. The comparison is
// always against the stored value and never against a caller-supplied one
// (FR-081, research D-08).
type PatientRef struct {
	ID        string
	PatientID string
	// Found is false for a target that does not exist, or exists but the
	// actor cannot reach — both produce the identical refusal below, because
	// neither may be told apart from the other (FR-057).
	Found bool
}

// SamePatient checks every target against the subject's own patient. A
// differing patient, a non-existent target and an unreachable target all
// produce the identical domain.ErrNotFound, with no other information about
// the target than the id the caller already supplied.
func SamePatient(subjectPatientID string, targets []PatientRef) error {
	for _, target := range targets {
		if !target.Found || target.PatientID != subjectPatientID {
			return domain.ErrNotFound
		}
	}

	return nil
}

// LinkSet is a multi-relation field's replace-set semantics (FR-056): PATCH
// replaces the whole set, and re-adding an existing member is an idempotent
// no-op rather than an error.
type LinkSet struct {
	ids map[string]bool
}

// NewLinkSet builds one from the ids a patch named, deduplicated: the same id
// twice in one submission is one member, not a reason to refuse.
func NewLinkSet(ids ...string) LinkSet {
	set := LinkSet{ids: make(map[string]bool, len(ids))}
	for _, id := range ids {
		set.ids[id] = true
	}

	return set
}

func (s LinkSet) Contains(id string) bool { return s.ids[id] }

func (s LinkSet) Len() int { return len(s.ids) }

// IDs is the set in no particular order; a caller that needs a stable order
// sorts it, which this type does not decide for every caller alike.
func (s LinkSet) IDs() []string {
	ids := make([]string, 0, len(s.ids))
	for id := range s.ids {
		ids = append(ids, id)
	}

	return ids
}

// Equal is whether re-submitting this set would be a no-op against other: two
// sets of the same ids, added in any order or repeated any number of times,
// are one set (FR-056).
func (s LinkSet) Equal(other LinkSet) bool {
	if len(s.ids) != len(other.ids) {
		return false
	}

	for id := range s.ids {
		if !other.ids[id] {
			return false
		}
	}

	return true
}
