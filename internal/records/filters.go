package records

import (
	"fmt"
	"maps"
	"slices"

	"medikube/internal/domain"
)

// FilterKind is the shape a filter's value takes, which is what its parser
// checks a supplied value against.
type FilterKind int

const (
	// FilterEnum is a closed vocabulary: every supplied value must be one of
	// Allowed, checked before the kind's own service ever sees it
	// (contracts/records-clinical.md §1).
	FilterEnum FilterKind = iota
	// FilterFreeform accepts any non-empty string; there is no vocabulary to
	// check it against. A kind whose filter is a free string still declares
	// it, so an unrecognised parameter name is still refused.
	FilterFreeform
)

// FilterSpec is one named query parameter a kind publishes: what it is
// called, what shape its value takes, the vocabulary it is checked against
// when that shape is FilterEnum, and the value assumed when the caller
// supplies none.
type FilterSpec struct {
	Name    string
	Kind    FilterKind
	Allowed []string
	Default string
}

// checkFilters validates supplied filter values against the kind's declared
// vocabulary and fills in each unset filter's Default.
//
// An unknown parameter name, or a value outside a FilterEnum spec's Allowed
// set, is refused with domain.ErrBadRequest and no field name: naming the
// parameter discloses nothing a caller who already knows the kind's own
// vocabulary did not have, which is why this is a flat 400 and not a
// per-field 422 (contracts/records-clinical.md §1).
func checkFilters(declared map[string]FilterSpec, supplied map[string][]string) (map[string][]string, error) {
	for _, name := range slices.Sorted(maps.Keys(supplied)) {
		spec, known := declared[name]
		if !known {
			return nil, fmt.Errorf("%w: %q is not a parameter this kind publishes", domain.ErrBadRequest, name)
		}

		if spec.Kind != FilterEnum || len(spec.Allowed) == 0 {
			continue
		}

		for _, value := range supplied[name] {
			if !slices.Contains(spec.Allowed, value) {
				return nil, fmt.Errorf("%w: %q was supplied a value outside its published vocabulary", domain.ErrBadRequest, name)
			}
		}
	}

	resolved := maps.Clone(supplied)

	for name, spec := range declared {
		if spec.Default == "" {
			continue
		}

		if _, has := resolved[name]; has {
			continue
		}

		if resolved == nil {
			resolved = make(map[string][]string, len(declared))
		}

		resolved[name] = []string{spec.Default}
	}

	return resolved, nil
}
