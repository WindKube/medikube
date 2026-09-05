package patient

import (
	"context"

	"medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/domain/person"
)

// Chart is the whole of getPatientChart's derived data
// (contracts/patient-chart.md). Nothing here is stored separately from the
// data it summarises (research D-22): it is assembled fresh on every call.
type Chart struct {
	Patient        person.Patient
	Counts         []CountEntry
	TotalRecords   int
	RecentActivity []domainaudit.Event
}

// recentActivityLimit is contracts/patient-chart.md's "last ten".
const recentActivityLimit = 10

// Summary answers one patient's chart, authorized against the person rather
// than a kind (research D-05), exactly as Get is.
func (s *Service) Summary(ctx context.Context, actor access.Actor, id string) (Chart, error) {
	if _, err := s.authorizer.Patient(ctx, actor, id, access.PermView); err != nil {
		return Chart{}, err
	}

	found, err := s.repository.Get(ctx, actor.UserID, id)
	if err != nil {
		return Chart{}, err
	}

	counts, err := s.counter.CountsByKind(ctx, id)
	if err != nil {
		return Chart{}, err
	}

	total := 0
	for _, entry := range counts {
		total += entry.Count
	}

	recent, err := s.activity.RecentForPatient(ctx, id, recentActivityLimit)
	if err != nil {
		return Chart{}, err
	}

	return Chart{Patient: found, Counts: counts, TotalRecords: total, RecentActivity: recent}, nil
}
