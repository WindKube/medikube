package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	domainaudit "medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/service/patient"
	"medikube/internal/web"
)

// OpGetPatientChart is contracts/patient-chart.md's one operation.
const OpGetPatientChart = "getPatientChart"

// CountTile is one of `counts`' entries.
type CountTile struct {
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// ActivityEntry is one of `recent_activity`'s entries. There is nowhere here
// a name, a value, a note or a filename could be written (FR-029) — the same
// property audit.Event itself has, carried through unchanged.
type ActivityEntry struct {
	OccurredAt   string `json:"occurred_at"`
	Action       string `json:"action"`
	TargetKind   string `json:"target_kind"`
	TargetID     string `json:"target_id"`
	TargetExists bool   `json:"target_exists"`
	ActorKind    string `json:"actor_kind"`
}

// PatientChart is contracts/patient-chart.md's response shape.
type PatientChart struct {
	Patient        Patient         `json:"patient"`
	Counts         []CountTile     `json:"counts"`
	TotalRecords   int             `json:"total_records"`
	RecentActivity []ActivityEntry `json:"recent_activity"`
}

// getPatientChart is the phase's only derived read: nothing here is stored
// separately from the data it summarises (research D-22), and it never
// carries an ETag — the response aggregates several collections, so one
// `updated` timestamp would be a lie.
func (h *patientHandlers) chart(e *core.RequestEvent, actor access.Actor) error {
	svc, err := h.resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathPatientID)

	chart, err := svc.Summary(e.Request.Context(), actor, id)
	if err != nil {
		return err
	}

	rendered, err := h.renderChart(e.Request.Context(), actor, chart)
	if err != nil {
		return err
	}

	e.Response.Header().Set("Cache-Control", patientCacheControl)

	return web.WriteJSON(e, http.StatusOK, rendered)
}

func (h *patientHandlers) renderChart(ctx context.Context, actor access.Actor, chart patient.Chart) (PatientChart, error) {
	detail, err := h.detail(ctx, actor, chart.Patient)
	if err != nil {
		return PatientChart{}, err
	}

	counts := make([]CountTile, 0, len(chart.Counts))
	for _, entry := range chart.Counts {
		counts = append(counts, CountTile{
			Kind: entry.Kind.Enum(), Path: entry.Path, Label: entry.Label, Count: entry.Count,
		})
	}

	activity := make([]ActivityEntry, 0, len(chart.RecentActivity))
	for _, event := range chart.RecentActivity {
		activity = append(activity, ActivityEntry{
			OccurredAt:   wireInstant(event.OccurredAt),
			Action:       string(event.Action),
			TargetKind:   string(event.TargetKind),
			TargetID:     event.TargetID,
			TargetExists: h.targetExists(ctx, actor, event),
			ActorKind:    string(event.ActorKind),
		})
	}

	return PatientChart{
		Patient: detail, Counts: counts, TotalRecords: chart.TotalRecords, RecentActivity: activity,
	}, nil
}

// TargetExists is targetExists, exported so internal/web/page can resolve the
// same answer for the same reason the JSON handler does: the chart carries
// no separate existence store (research D-22), and there is exactly one way
// to check it.
func TargetExists(ctx context.Context, records Resolve, actor access.Actor, event domainaudit.Event) bool {
	h := &patientHandlers{records: records}

	return h.targetExists(ctx, actor, event)
}

// targetExists resolves whether an audited target row still exists, with no
// query of its own for a target_kind this build cannot address: the chart
// carries no separate existence store (research D-22), so this is answered
// through the same authorized read every other client of a record kind uses.
//
// target_kind "patient" is always true: the chart could not have loaded for a
// patient that does not exist, and a delete row's own PatientID is unset by
// the schema the moment it is written (data-model §5), so it never reaches
// this patient's own activity list again.
func (h *patientHandlers) targetExists(ctx context.Context, actor access.Actor, event domainaudit.Event) bool {
	if event.TargetKind == domainaudit.TargetKindPatient {
		return true
	}

	if h.records == nil {
		return true
	}

	k, known := kind.FromEnum(string(event.TargetKind))
	if !known {
		return true
	}

	handler, err := h.records()
	if err != nil {
		return true
	}

	_, err = handler.Get(ctx, actor, k.Segment(), event.TargetID)
	if err == nil {
		return true
	}

	return !errors.Is(err, domain.ErrNotFound)
}
