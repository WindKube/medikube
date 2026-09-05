package encounter

import "medikube/internal/domain"

func (q Query) resolve() (Query, error) {
	var invalid domain.ValidationError

	for _, v := range q.VisitTypes {
		if !v.Valid() {
			invalid.Add(FilterVisitType, domain.CodeInvalidValue, "not one of the visit types MediKube accepts")

			break
		}
	}

	for _, p := range q.Priorities {
		if !p.Valid() {
			invalid.Add(FilterPriority, domain.CodeInvalidValue, "not one of the priorities MediKube accepts")

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
