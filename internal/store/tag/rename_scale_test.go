//go:build scale

package tag_test

import (
	"context"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/domain/tag"
	tagsvc "medikube/internal/service/tag"
	"medikube/internal/store"
	tagstore "medikube/internal/store/tag"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T154, SC-007: a tag carried by 500 records is renamed in one action; 100%
// show the new name and 0 lose it; deletion removes it from 100% and
// destroys 0 records.
//
// Phase 003 registers seven kinds on this branch, one short of the "≥8
// kinds" the task names — there is no eighth to spread the 500 rows across.
// apitest.Populate's bulk path is medication's own, so all 500 carriers here
// are medications; internal/store/filter_tags_test.go and
// internal/service/tag/service_test.go's own contract cover the derivation
// working identically across every kind that does exist.
func TestRenamingATagCarriedBy500RecordsTouchesOnlyTheTagRow(t *testing.T) {
	const carriers = 500

	instance := apitest.NewPopulated(t, testsupport.AccountAPatientSelfID, carriers)
	app := instance.App

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	cursors, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := tagstore.New(app, cursors, func() []string { return []string{kind.Medication.Collection()} })
	require.NoError(t, err)

	service, err := tagsvc.New(repo, repo, repo, tagsvc.DefaultAuthorizer, noopAuditor{})
	require.NoError(t, err)

	created, err := service.Create(t.Context(), actorOf(t), tag.Tag{Name: "before-rename", Color: "#aa3311"})
	require.NoError(t, err)

	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	bulkOnly := dbx.NewExp("name LIKE {:prefix}", dbx.Params{"prefix": "Bulk %"})

	var records []*core.Record
	require.NoError(t, app.RecordQuery(medications.Name).
		AndWhere(dbx.HashExp{store.RecordPatientField: testsupport.AccountAPatientSelfID}).
		AndWhere(bulkOnly).
		All(&records))
	require.Len(t, records, carriers)

	versionsBefore := make(map[string]string, carriers)

	require.NoError(t, store.RunInTransaction(app, func(txApp core.App) error {
		for _, record := range records {
			record.Set("tags", []string{created.ID})
			if err := txApp.SaveNoValidate(record); err != nil {
				return err
			}

			versionsBefore[record.Id] = store.Version(record)
		}

		return nil
	}))

	// The rename: one row update, timed so a regression that started
	// touching every carrier shows up as a duration outlier and not only as
	// a row-count assertion.
	start := time.Now()

	renamed, err := service.Update(t.Context(), actorOf(t), created.ID, tagsvc.Patch{Name: ptrTo("after-rename")})
	require.NoError(t, err)
	assert.Equal(t, "after-rename", renamed.Name)

	elapsed := time.Since(start)
	assert.Lessf(t, elapsed, 2*time.Second, "renaming one tag took %s across %d carriers — it should be O(1)", elapsed, carriers)

	var afterRename []*core.Record
	require.NoError(t, app.RecordQuery(medications.Name).
		AndWhere(dbx.HashExp{store.RecordPatientField: testsupport.AccountAPatientSelfID}).
		AndWhere(bulkOnly).
		All(&afterRename))
	require.Len(t, afterRename, carriers, "the rename destroyed or created a medication")

	carrying := 0

	for _, record := range afterRename {
		if contains(record.GetStringSlice("tags"), created.ID) {
			carrying++
		}

		// 0 lose it, and the row itself was never rewritten: same version.
		assert.Equal(t, versionsBefore[record.Id], store.Version(record), "medication %s was rewritten by the rename", record.Id)
	}

	assert.Equal(t, carriers, carrying, "not every carrier still carries the renamed tag")

	// The delete: every carrier survives, none carries the tag any more.
	require.NoError(t, service.Delete(t.Context(), actorOf(t), created.ID))

	var afterDelete []*core.Record
	require.NoError(t, app.RecordQuery(medications.Name).
		AndWhere(dbx.HashExp{store.RecordPatientField: testsupport.AccountAPatientSelfID}).
		AndWhere(bulkOnly).
		All(&afterDelete))
	require.Len(t, afterDelete, carriers, "deleting the tag destroyed a medication")

	for _, record := range afterDelete {
		assert.False(t, contains(record.GetStringSlice("tags"), created.ID), "medication %s still carries the deleted tag", record.Id)
	}
}

func actorOf(t *testing.T) access.Actor {
	t.Helper()

	return access.Actor{UserID: testsupport.AccountAID, Role: identity.RoleUser}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func ptrTo(s string) *string { return &s }

type noopAuditor struct{}

func (noopAuditor) Record(_ context.Context, _ audit.Event) error { return nil }
