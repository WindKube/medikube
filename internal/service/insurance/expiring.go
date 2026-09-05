package insurance

import (
	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
)

// BasisExpiring is FR-046's row basis: every row `?expiring_within_days=`
// selects carries it, so a status view and the equivalent hand narrowing
// cannot disagree about why a row is there.
const BasisExpiring = "expiring"

// ExpiringBasis reports whether the policy's cover ends within the window.
func ExpiringBasis(entity clinical.Insurance, withinDays int) []string {
	if entity.ExpiresOn.IsZero() {
		return nil
	}

	today := clinical.Today()
	horizon := addDays(today, withinDays)

	if !entity.ExpiresOn.Before(today) && !entity.ExpiresOn.After(horizon) {
		return []string{BasisExpiring}
	}

	return nil
}

// addDays is domain.Date's missing calendar arithmetic, kept local to this
// package for the same reason internal/service/equipment keeps its own copy.
func addDays(d domain.Date, days int) domain.Date {
	shifted := d.UTC().AddDate(0, 0, days)

	result, err := domain.NewDate(shifted.Year(), shifted.Month(), shifted.Day())
	if err != nil {
		return domain.Date{}
	}

	return result
}
