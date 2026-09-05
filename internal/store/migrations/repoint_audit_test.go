package migrations

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditservice "medikube/internal/service/audit"
	auditstore "medikube/internal/store/audit"

	"medikube/internal/obs"
	"medikube/internal/platform/pb"
)

type auditRow struct {
	ActorKind string `db:"actor_kind"`
	Actor     string `db:"actor"`
	RequestID string `db:"request_id"`
}

// T074. The self-record backfill (step 2 of research D-13) writes one
// create/patient audit row per provisioned self-record, with actor_kind
// system, and every row of one backfill carries the same non-empty
// request_id — the migration's own run_id — so a system row written with no
// HTTP request still correlates (001 data-model §3, 001 T240).
//
// The patient audit hook is bound BEFORE the migration runs, unlike
// cmd/medikube's own boot order: tests.NewTestApp (via preRepointApp)
// already ran every migration once before this test ever touches the app,
// so binding has to happen here, ahead of runRepoint, for this specific
// re-run to be observed at all.
func TestRepointBackfillWritesOneAuditRowPerProvisionedSelfRecord(t *testing.T) {
	t.Parallel()

	app, items, idx := preRepointApp(t)

	trail, err := auditstore.New(app)
	require.NoError(t, err)

	auditor, err := auditservice.New(trail)
	require.NoError(t, err)

	// No Actor callback: a migration runs with no request's actor to read
	// (research D-14), and BindPatientAudit tolerates that being nil — it
	// must not panic, which is itself part of this test. Request mirrors
	// production wiring (cmd/medikube/handlers.go, apitest.go): it reads the
	// correlation id the migration threaded through runCtx, rather than
	// minting a fresh one per row.
	require.NoError(t, pb.BindPatientAudit(app, pb.PatientAudit{Trail: auditor, Request: obs.CorrelationID}))

	amara := seedLegacyUser(t, app, "amara@example.com", "Amara Okonkwo")
	chidi := seedLegacyUser(t, app, "chidi@example.com", "Chidi Eze")

	applied, err := runRepoint(app, items, idx)
	require.NoError(t, err)
	require.Equal(t, []string{items[idx].File}, applied)

	var provisioned []auditRow
	require.NoError(t, app.DB().NewQuery(
		"SELECT "+auditFieldActorKind+" AS actor_kind, "+auditFieldActor+" AS actor, "+auditFieldRequestID+" AS request_id "+
			"FROM "+auditEventsCollection+" WHERE "+auditFieldAction+" = {:action} AND "+auditFieldTargetKind+" = {:targetKind}",
	).Bind(dbx.Params{"action": "create", "targetKind": "patient"}).All(&provisioned))

	require.Len(t, provisioned, 2, "one create/patient row per provisioned self-record — Amara's and Chidi's, and no others")

	requestIDs := make(map[string]bool, 1)

	for _, row := range provisioned {
		assert.Equal(t, "system", row.ActorKind)
		assert.Empty(t, row.Actor, "a system row names no actor")
		assert.NotEmpty(t, row.RequestID, "a system row still needs a correlation handle")
		requestIDs[row.RequestID] = true
	}

	assert.Len(t, requestIDs, 1, "every row of one backfill shares the same run_id")

	_ = amara.Id
	_ = chidi.Id
}
