package audit

import (
	"context"
	"errors"
	"fmt"

	domainaudit "medikube/internal/domain/audit"
)

// DefaultRecentLimit is the chart summary's own count (data-model §5, "the
// last 10 activity entries"), applied whenever a caller asks for zero or
// fewer.
const DefaultRecentLimit = 10

// Reader is the read side of the trail: the one query the chart summary needs
// (research D-22). It is a second seam from Repository for the same reason
// Purger is one — the trail is append-only, and a reader that could also
// write would be the shape somebody reaches for when a query needs a row
// fixed up in place rather than a new one appended.
type Reader interface {
	// RecentForPatient returns up to limit events concerning patientID, most
	// recent first. There is no content to omit — audit.Event carries none —
	// so this returns the domain type itself rather than a projection of it.
	RecentForPatient(ctx context.Context, patientID string, limit int) ([]domainaudit.Event, error)
}

// RecentActivity is audit's implementation of the RecentActivityReader port
// patient.Service.Summary consumes (research D-22): a one-method adapter over
// Reader that applies the chart's own default and refuses a request with no
// patient to answer for.
type RecentActivity struct {
	reader Reader
}

func NewRecentActivity(reader Reader) (*RecentActivity, error) {
	if reader == nil {
		return nil, errors.New("audit: recent activity is wired with no reader, so every request would answer with nothing")
	}

	return &RecentActivity{reader: reader}, nil
}

// RecentForPatient is what the chart summary calls. A non-positive limit is
// not "no rows" — that is what a request for zero would mean if it reached
// the reader unchanged — so it is read as "unspecified" and replaced with
// DefaultRecentLimit.
func (r *RecentActivity) RecentForPatient(ctx context.Context, patientID string, limit int) ([]domainaudit.Event, error) {
	if patientID == "" {
		return nil, errors.New("audit: recent activity requires a patient id")
	}

	if limit <= 0 {
		limit = DefaultRecentLimit
	}

	events, err := r.reader.RecentForPatient(ctx, patientID, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: reading recent activity for a patient: %w", err)
	}

	return events, nil
}
