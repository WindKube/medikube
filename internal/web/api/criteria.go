package api

import (
	"slices"

	"medikube/internal/records"
)

// basisCarrier is what every kind's Summary DTO implements: a pointer-
// receiver setter for the `basis` json member the contract requires on
// every row (contracts/records-clinical.md §1). A Detail type satisfies it
// too, by embedding its kind's Summary — the same reason its own SearchFields
// reads the embedded fields.
type basisCarrier interface {
	SetBasis(basis []string)
}

// criteriaEnvelope is the JSON shape `criteria` echoes: the server's resolved
// narrowing, so a page can render removable chips and a status view can
// state its basis (FR-026, FR-046, FR-049, FR-078). patient is always
// present; every other member is one of the kind's own resolved filters.
type criteriaEnvelope map[string]any

// buildCriteria turns a resolved records.Criteria into the wire shape.
// Filters are always present (even an empty slice), because a caller renders
// a chip from a member's presence and an absent one would be indistinguishable
// from "not narrowed by this at all" — which is exactly the difference
// research D-05 requires stated.
func buildCriteria(patientID string, criteria records.Criteria) criteriaEnvelope {
	envelope := criteriaEnvelope{"patient": patientID}

	for name, values := range criteria.Filters {
		envelope[name] = slices.Clone(values)
	}

	if criteria.Search != "" {
		envelope["q"] = criteria.Search
	}

	return envelope
}

// applyBasis populates each row's `basis` member from the kind's own
// registered Basis function, against the resolved criteria the same list
// request produced. A row whose Body carries no basisCarrier is left alone —
// every registered kind's Summary carries one, so this is defensive rather
// than expected.
func applyBasis(entry records.Entry, criteria records.Criteria, items []records.Record) {
	for _, item := range items {
		basis := entry.Basis(item.Body, criteria)

		if carrier, ok := item.Body.(basisCarrier); ok {
			carrier.SetBasis(basis)
		}
	}
}
