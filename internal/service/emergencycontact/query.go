package emergencycontact

import "medikube/internal/domain"

// resolve applies the published vocabulary. Unlike a kind that offers several
// alternative orderings, this kind publishes exactly one — FR-051's compound
// default — so an unstated sort resolves to all of it rather than its first
// term alone.
func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	published := Sorts()

	if len(q.Sort) == 0 {
		q.Sort = published
	} else if !equalSort(q.Sort, published) {
		invalid.Add(ParamSort, domain.CodeInvalidValue, "not the ordering MediKube publishes")
	}

	if err := invalid.OrNil(); err != nil {
		return Query{}, err
	}

	return q, nil
}

func equalSort(a, b []domain.SortKey) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
