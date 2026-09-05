package patient_test

import (
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
	pbpatient "medikube/internal/store/patient"

	_ "medikube/internal/store/migrations"
)

// T145, FR-049, US6-2, US6-3, SC-010. Deleting a patient destroys every
// medication attributed to it and the photo (and thumbnails) with it — the
// cascade this method relies on rather than reimplements.
func TestDeletingAPatientDestroysItsMedicationsAndItsPhoto(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	const owner = "mkdelownerpat01"
	seedAccountWithID(t, app, owner, "delete-owner@example.test")
	subject := seedRawPatient(t, app, owner, "Amara")
	seedMedication(t, app, subject, "Amoxicillin")
	seedMedication(t, app, subject, "Ibuprofen")

	photos, err := pbpatient.NewPhotoStore(app, thumbSizes)
	require.NoError(t, err)

	_, err = photos.Put(t.Context(), owner, subject, onePixelUpload(t))
	require.NoError(t, err)

	keys := storedFileKeys(t, app, subject, thumbSizes)
	for _, key := range keys {
		exists, existsErr := existsOnDisk(t, app, key)
		require.NoError(t, existsErr)
		require.True(t, exists, "the fixture photo is not actually on disk, so this proves nothing")
	}

	record, err := app.FindRecordById(store.PatientCollection, subject)
	require.NoError(t, err)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbpatient.New(app, codec)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(t.Context(), owner, subject, store.Version(record)))

	count, err := store.CountByPatient(t.Context(), app, kind.Medication.Collection(), subject)
	require.NoError(t, err)
	assert.Zero(t, count, "a medication survived the patient it belonged to")

	// PocketBase's own file cleanup runs as a background, "optimistic"
	// delete (core/base.go's registerBaseHooks) so it does not hold the
	// delete transaction open on S3 latency — it is not yet gone the instant
	// Delete returns, only eventually.
	for _, key := range keys {
		assert.Eventuallyf(t, func() bool {
			exists, existsErr := existsOnDisk(t, app, key)
			return existsErr == nil && !exists
		}, 2*time.Second, 10*time.Millisecond, "%s survived the patient's own deletion", key)
	}

	_, err = app.FindRecordById(store.PatientCollection, subject)
	assert.Error(t, err, "the patient row itself is still there")
}

// T146. Every collection that can name a patient is walked, and none of them
// still does once the patient behind that id is gone (FR-049).
func TestDeletingAPatientLeavesNoRowAnywhereReferencingIt(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	const owner = "mkdelownerpat02"
	seedAccountWithID(t, app, owner, "delete-owner-2@example.test")
	subject := seedRawPatient(t, app, owner, "Bo")
	seedMedication(t, app, subject, "Paracetamol")

	// The account's own active_patient, pointed at the row this test is
	// about to delete, so its auto-unset is one of the things asserted below
	// (data-model §6, US3's own repointing).
	account, err := app.FindRecordById(store.AccountCollection, owner)
	require.NoError(t, err)
	account.Set("active_patient", subject)
	require.NoError(t, app.Save(account))

	// An audit row naming the patient, exactly as the create hook would have
	// written one; asserted to have its reference auto-unset rather than
	// surviving as a ghost (data-model §7's "auto-unset when the patient is
	// deleted").
	auditCollection, err := app.FindCollectionByNameOrId("audit_events")
	require.NoError(t, err)
	auditRecord := core.NewRecord(auditCollection)
	auditRecord.Set("occurred_at", "2026-01-01 00:00:00.000Z")
	auditRecord.Set("actor_kind", "user")
	auditRecord.Set("action", "update")
	auditRecord.Set("target_kind", "patient")
	auditRecord.Set("target_id", subject)
	auditRecord.Set("patient", subject)
	auditRecord.Set("request_id", "req-delete-test-0001")
	require.NoError(t, app.Save(auditRecord))

	record, err := app.FindRecordById(store.PatientCollection, subject)
	require.NoError(t, err)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbpatient.New(app, codec)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(t.Context(), owner, subject, store.Version(record)))

	var medicationCount int
	require.NoError(t, app.RecordQuery(kind.Medication.Collection()).
		Select("count(*)").AndWhere(dbx.HashExp{"patient": subject}).Row(&medicationCount))
	assert.Zero(t, medicationCount)

	reloadedAccount, err := app.FindRecordById(store.AccountCollection, owner)
	require.NoError(t, err)
	assert.Empty(t, reloadedAccount.GetString("active_patient"), "active_patient still names the deleted patient")

	reloadedAudit, err := app.FindRecordById("audit_events", auditRecord.Id)
	require.NoError(t, err)
	assert.Empty(t, reloadedAudit.GetString("patient"), "the historical audit row still names the deleted patient")
}
