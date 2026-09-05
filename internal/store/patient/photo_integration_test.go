package patient_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
	"medikube/internal/service/patient/patienttest"
	"medikube/internal/store"
	pbpatient "medikube/internal/store/patient"

	// See internal/store/medication/repo_integration_test.go's own comment:
	// the migrations register themselves from their own init.
	_ "medikube/internal/store/migrations"
)

// thumbSizes mirrors contracts/patient-photo.md's default configuration
// (MEDIKUBE_FILES_PHOTO_THUMBS).
var thumbSizes = []string{"100x100t", "400x400f"}

// T044/T048. patienttest.PhotoStoreContract's own fixed patient ids
// ("mkpatientone001" and so on) need a real patients row to attach a
// photograph to — Put resolves ownership against the database — so each is
// seeded here before the contract runs, mirroring repo_integration_test.go's
// remapAccounts.
var photoContractPatientIDs = []string{
	"mkpatientone001", "mkpatienttwo002", "mkpatientthre03", "mkpatientfour04",
}

func TestPhotoStoreSatisfiesTheContract(t *testing.T) {
	t.Parallel()

	patienttest.PhotoStoreContract(t, func(t *testing.T) patient.PhotoStore {
		t.Helper()

		app, err := tests.NewTestApp(t.TempDir())
		require.NoError(t, err)
		t.Cleanup(app.Cleanup)

		remapAccounts(t, app)
		seedPhotoContractPatients(t, app)

		store, err := pbpatient.NewPhotoStore(app, thumbSizes)
		require.NoError(t, err)

		return store
	})
}

// TestThumbnailsExistOnDiskBeforeAnyRequestForThem is FR-009: generation is
// eager, on upload, never lazy behind the first read — asserted here by
// checking the filesystem directly, never through the Serve half a lazy
// implementation could still satisfy by generating on the way out.
func TestThumbnailsExistOnDiskBeforeAnyRequestForThem(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	remapAccounts(t, app)
	patientID := seedOnePatient(t, app, "Amara", "Okonkwo")

	photos, err := pbpatient.NewPhotoStore(app, thumbSizes)
	require.NoError(t, err)

	_, err = photos.Put(t.Context(), patienttest.OwnerID, patientID, onePixelUpload(t))
	require.NoError(t, err)

	for _, key := range storedFileKeys(t, app, patientID, thumbSizes) {
		exists, existsErr := existsOnDisk(t, app, key)
		require.NoError(t, existsErr)
		assert.Truef(t, exists, "%s does not exist on disk, so it was never generated eagerly", key)
	}
}

// TestReplacingRemovesTheOldOriginalAndBothOldThumbnails is US1-5: the
// previous photograph is not merely unlinked from the record, its bytes and
// its thumbnails are actually gone.
func TestReplacingRemovesTheOldOriginalAndBothOldThumbnails(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	remapAccounts(t, app)
	patientID := seedOnePatient(t, app, "Amara", "Okonkwo")

	photos, err := pbpatient.NewPhotoStore(app, thumbSizes)
	require.NoError(t, err)

	_, err = photos.Put(t.Context(), patienttest.OwnerID, patientID, onePixelUpload(t))
	require.NoError(t, err)

	previousKeys := storedFileKeys(t, app, patientID, thumbSizes)
	for _, key := range previousKeys {
		exists, existsErr := existsOnDisk(t, app, key)
		require.NoError(t, existsErr)
		require.True(t, exists, "the first upload's own files must exist before the replacement proves anything")
	}

	_, err = photos.Put(t.Context(), patienttest.OwnerID, patientID, onePixelUpload(t))
	require.NoError(t, err)

	for _, key := range previousKeys {
		exists, existsErr := existsOnDisk(t, app, key)
		require.NoError(t, existsErr)
		assert.Falsef(t, exists, "%s survived the replacement (US1-5)", key)
	}
}

// seedOnePatient creates one patients row directly, at a real minted id, owned
// by patienttest.OwnerID.
func seedOnePatient(t *testing.T, app core.App, first, last string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(store.PatientCollection)
	require.NoError(t, err)

	record := core.NewRecord(collection)
	require.NoError(t, store.PatientToRecord(record, person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: first, LastName: last,
	}))
	require.NoError(t, app.Save(record))

	return record.Id
}

func seedPhotoContractPatients(t *testing.T, app core.App) {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(store.PatientCollection)
	require.NoError(t, err)

	for _, id := range photoContractPatientIDs {
		record := core.NewRecord(collection)
		record.Id = id
		require.NoError(t, store.PatientToRecord(record, person.Patient{
			OwnerID: patienttest.OwnerID, FirstName: "Fixture", LastName: id,
		}))
		require.NoError(t, app.Save(record))
	}
}

// onePixelUpload is the same fixture patienttest.PhotoStoreContract accepts —
// the smallest byte sequence PocketBase's own sniffing recognises as
// image/png.
func onePixelUpload(t *testing.T) patient.Upload {
	t.Helper()

	const encoded = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	return patient.Upload{Reader: bytes.NewReader(raw), Size: int64(len(raw)), Name: "photo.png"}
}

// storedFileKeys is the original's and every configured thumbnail's key,
// exactly as internal/store/patient/photo.go builds them, read back from the
// record's own stored filename rather than assumed.
func storedFileKeys(t *testing.T, app core.App, patientID string, sizes []string) []string {
	t.Helper()

	record, err := app.FindRecordById(store.PatientCollection, patientID)
	require.NoError(t, err)

	filename := record.GetString(store.PatientPhoto)
	require.NotEmpty(t, filename, "the record carries no photo filename to build a key from")

	basePath := record.BaseFilesPath()

	keys := make([]string, 0, 1+len(sizes))
	keys = append(keys, basePath+"/"+filename)

	for _, size := range sizes {
		keys = append(keys, basePath+"/thumbs_"+filename+"/"+size+"_"+filename)
	}

	return keys
}

func existsOnDisk(t *testing.T, app core.App, key string) (bool, error) {
	t.Helper()

	fsys, err := app.NewFilesystem()
	require.NoError(t, err)
	defer func() { _ = fsys.Close() }()

	return fsys.Exists(key)
}
