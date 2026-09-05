package kinds

import "medikube/internal/records"

// matchOf reads the resolved `?match=` filter value, defaulting to "any" the
// same way records.TagFilters' FilterSpec.Default does — belt and suspenders,
// since resolveQuery has already filled it in by the time an adapter sees it.
func matchOf(values []string) string {
	if len(values) == 0 {
		return records.MatchAny
	}

	return values[0]
}
