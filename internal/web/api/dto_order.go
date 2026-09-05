package api

import (
	"slices"

	"medikube/internal/domain"
)

// sortFieldOrder is dto_medication.go's orderedRefusal, generalised: it sorts
// a ValidationError's fields into the column order a kind's own Validate
// checks them in (FR-027), so a response naming several problems reads in the
// order the form does.
func sortFieldOrder(invalid *domain.ValidationError, order []string) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(order, left.Field) - slices.Index(order, right.Field)
	})

	return invalid.OrNil()
}
