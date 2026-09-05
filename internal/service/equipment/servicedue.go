package equipment

import (
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

// The two service-due bases FR-049 requires every row to distinguish.
const (
	BasisOverdue = "overdue"
	BasisDueSoon = "due_soon"
)

// ServiceDueBasis reports why a piece of equipment qualifies for
// `?service_due_within_days=`: overdue when service_due_on is before today,
// due_soon when it falls within the window, and nothing when it does not
// qualify at all or has no service_due_on recorded.
func ServiceDueBasis(entity clinical.Equipment, withinDays int) []string {
	if entity.ServiceDueOn.IsZero() {
		return nil
	}

	today := clinical.Today()

	if entity.ServiceDueOn.Before(today) {
		return []string{BasisOverdue}
	}

	horizon := addDays(today, withinDays)
	if !entity.ServiceDueOn.After(horizon) {
		return []string{BasisDueSoon}
	}

	return nil
}

// addDays is domain.Date's missing calendar arithmetic, kept local to this
// package: nothing else in this phase needs to add days to a date, and a
// method on the shared type would invite a caller elsewhere to add days to an
// instant instead, which is what research D-03 warns against.
func addDays(d domain.Date, days int) domain.Date {
	shifted := d.UTC().AddDate(0, 0, days)

	result, err := domain.NewDate(shifted.Year(), shifted.Month(), shifted.Day())
	if err != nil {
		// Unreachable: AddDate on a valid calendar date always produces
		// another valid calendar date.
		return domain.Date{}
	}

	return result
}
