package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/audit"
)

// The seven columns of data-model §3, and the list is the point: there is no
// column here that a value, a name, a note or a diff could be written into.
// That is what makes FR-036's content rule structural rather than a review
// item. No ip, no reason, no affected, no content — each is named in
// data-model §7 with the phase that adds it, and none of them is this one.
const (
	auditFieldOccurredAt = "occurred_at"
	auditFieldActor      = "actor"
	auditFieldActorKind  = "actor_kind"
	auditFieldAction     = "action"
	auditFieldTargetKind = "target_kind"
	auditFieldTargetID   = "target_id"
	auditFieldRequestID  = "request_id"
)

const (
	auditOccurredIndex  = "idx_audit_occurred"
	auditActorTimeIndex = "idx_audit_actor_time"
	auditTargetIndex    = "idx_audit_target"
)

func init() {
	register(auditEventsUp, auditEventsDown)
}

func auditEventsUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	collection := core.NewBaseCollection(auditEventsCollection)
	lockRules(collection)

	collection.Fields.Add(&core.DateField{
		Name:     auditFieldOccurredAt,
		Required: true,
	})

	// The deliberate non-cascade. Deleting an account unsets this reference and
	// keeps the row, so the account_delete entry outlives its actor
	// (research D-22); actor_kind below is what still says a person did it.
	// CascadeDelete true here would destroy the record that the account was
	// deleted, and Required true would make deleting an account with history
	// fail outright.
	collection.Fields.Add(&core.RelationField{
		Name:          auditFieldActor,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  users.Id,
	})

	// The three vocabularies are declared complete, not grown phase by phase: a
	// select field refuses an undeclared value, so a vocabulary assembled from
	// six deltas fails in production on the first share, the first non-owner
	// photo fetch and the first backup rather than in a test (data-model §3).
	collection.Fields.Add(&core.SelectField{
		Name:      auditFieldActorKind,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(audit.ActorKinds()),
	})
	collection.Fields.Add(&core.SelectField{
		Name:      auditFieldAction,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(audit.Actions()),
	})
	collection.Fields.Add(&core.SelectField{
		Name:      auditFieldTargetKind,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(audit.TargetKinds()),
	})

	// An opaque id, never a name, never a path, never a filename — with the one
	// bounded exception data-model §3 works out, where a system, backup or
	// export target has no record to point at and this carries the job or
	// archive name instead. 64 is that arithmetic, not a guess.
	collection.Fields.Add(&core.TextField{
		Name: auditFieldTargetID,
		Max:  audit.MaxTargetID,
	})

	// Required, so a row that correlates to nothing cannot be written. A
	// background run has no HTTP request and still fills it: the cron, job and
	// migration contexts mint a run id from the same helper that mints request
	// ids, and the run's zerolog lines carry the same value.
	collection.Fields.Add(&core.TextField{
		Name:     auditFieldRequestID,
		Required: true,
		Max:      audit.MaxRequestID,
	})

	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	// Wide enough for phase 006's reader on day one, deliberately: each carries
	// the tiebreaker 006's keyset paging needs to stay index-only, so 006
	// creates no audit index at all. The alternative puts six b-trees on the
	// highest-write-volume collection in the instance and makes 006's
	// idx_audit_target collide by name with this one.
	collection.AddIndex(auditOccurredIndex, false,
		auditFieldOccurredAt+" DESC, id DESC", "")
	collection.AddIndex(auditActorTimeIndex, false,
		auditFieldActor+", "+auditFieldOccurredAt+" DESC, id DESC", "")
	collection.AddIndex(auditTargetIndex, false,
		auditFieldTargetKind+", "+auditFieldTargetID+", "+auditFieldOccurredAt+" DESC", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", auditEventsCollection, err)
	}

	return nil
}

func auditEventsDown(app core.App) error {
	return deleteCollection(app, auditEventsCollection)
}
