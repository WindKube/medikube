package symptom

import "medikube/internal/domain"

// resolve applies the published vocabulary: an unpublished sort, severity or
// status is refused rather than dropped.
func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	for _, severity := range q.Severities {
		if !severity.Valid() {
			invalid.Add(FilterSeverity, domain.CodeInvalidValue, "not one of the severities MediKube accepts")

			break
		}
	}

	for _, status := range q.Statuses {
		if !status.Valid() {
			invalid.Add(FilterStatus, domain.CodeInvalidValue, "not one of the states MediKube accepts")

			break
		}
	}

	published := Sorts()

	if len(q.Sort) == 0 {
		q.Sort = published[:1]
	}

	for _, term := range q.Sort {
		if !containsSort(published, term) {
			invalid.Add(ParamSort, domain.CodeInvalidValue, "not one of the orderings MediKube publishes")

			break
		}
	}

	if err := invalid.OrNil(); err != nil {
		return Query{}, err
	}

	return q, nil
}

func containsSort(published []domain.SortKey, term domain.SortKey) bool {
	for _, key := range published {
		if key == term {
			return true
		}
	}

	return false
}
