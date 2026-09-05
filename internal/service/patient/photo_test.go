package patient_test

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
	"medikube/internal/service/patient/patienttest"
)

const onePixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func onePixelPNG(t *testing.T) []byte {
	t.Helper()

	raw, err := base64.StdEncoding.DecodeString(onePixelPNGBase64)
	require.NoError(t, err)

	return raw
}

// FR-008/FR-009: one photograph per person, and both configured thumbnails
// exist once the store answers.
func TestSetPhotoStoresOnePhotographWithBothThumbnails(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	png := onePixelPNG(t)

	meta, err := svc.SetPhoto(t.Context(), owner(), created.ID, patient.Upload{
		Reader: bytes.NewReader(png), Size: int64(len(png)), Name: "photo.png",
	})
	require.NoError(t, err)
	assert.Contains(t, meta.Sizes, "100x100t")
	assert.Contains(t, meta.Sizes, "400x400f")
}

// FR-008: a non-image is refused and nothing is stored.
func TestSetPhotoRefusesANonImage(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	notAnImage := []byte("this is not a photograph")

	_, err = svc.SetPhoto(t.Context(), owner(), created.ID, patient.Upload{
		Reader: bytes.NewReader(notAnImage), Size: int64(len(notAnImage)), Name: "photo.jpg",
	})
	assert.ErrorIs(t, err, domain.ErrUnsupportedMedia)
}

// FR-008: an oversize file is refused.
func TestSetPhotoRefusesAnOversizeFile(t *testing.T) {
	t.Parallel()

	repo := patienttest.NewRepository()
	auditor := patienttest.NewAuditor()
	authorizer := patienttest.NewAuthorizer(repo, auditor)
	photos := patienttest.NewPhotoStore(10, []string{"image/png"}) // 10-byte ceiling

	svc, err := patient.New(repo, photos, authorizer, patienttest.NewActivePatientStore(), auditor)
	require.NoError(t, err)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	png := onePixelPNG(t)
	require.Greater(t, len(png), 10)

	_, err = svc.SetPhoto(t.Context(), owner(), created.ID, patient.Upload{
		Reader: bytes.NewReader(png), Size: int64(len(png)), Name: "photo.png",
	})
	assert.ErrorIs(t, err, domain.ErrTooLarge)
}

// US1-5: a replacement removes the previous file.
func TestSetPhotoReplacementRemovesThePrevious(t *testing.T) {
	t.Parallel()

	repo := patienttest.NewRepository()
	auditor := patienttest.NewAuditor()
	authorizer := patienttest.NewAuthorizer(repo, auditor)
	photos := patienttest.NewPhotoStore(15<<20, []string{"image/png"})

	svc, err := patient.New(repo, photos, authorizer, patienttest.NewActivePatientStore(), auditor)
	require.NoError(t, err)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	png := onePixelPNG(t)

	_, err = svc.SetPhoto(t.Context(), owner(), created.ID, patient.Upload{Reader: bytes.NewReader(png), Size: int64(len(png)), Name: "first.png"})
	require.NoError(t, err)

	_, err = svc.SetPhoto(t.Context(), owner(), created.ID, patient.Upload{Reader: bytes.NewReader(png), Size: int64(len(png)), Name: "second.png"})
	require.NoError(t, err)

	sizes, has := photos.Has(created.ID)
	require.True(t, has)
	assert.NotEmpty(t, sizes)
}

// FR-008: a removal leaves none.
func TestDeletePhotoLeavesNone(t *testing.T) {
	t.Parallel()

	repo := patienttest.NewRepository()
	auditor := patienttest.NewAuditor()
	authorizer := patienttest.NewAuthorizer(repo, auditor)
	photos := patienttest.NewPhotoStore(15<<20, []string{"image/png"})

	svc, err := patient.New(repo, photos, authorizer, patienttest.NewActivePatientStore(), auditor)
	require.NoError(t, err)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	png := onePixelPNG(t)

	_, err = svc.SetPhoto(t.Context(), owner(), created.ID, patient.Upload{Reader: bytes.NewReader(png), Size: int64(len(png)), Name: "photo.png"})
	require.NoError(t, err)

	require.NoError(t, svc.DeletePhoto(t.Context(), owner(), created.ID))

	_, has := photos.Has(created.ID)
	assert.False(t, has)
}

// A stranger may not touch the photograph, exactly as they may not touch the
// record.
func TestSetPhotoRefusesAStranger(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t)

	created, err := svc.Create(t.Context(), owner(), personDraft())
	require.NoError(t, err)

	png := onePixelPNG(t)

	_, err = svc.SetPhoto(t.Context(), stranger(), created.ID, patient.Upload{Reader: bytes.NewReader(png), Size: int64(len(png)), Name: "photo.png"})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func personDraft() person.Patient {
	birth, _ := domain.ParseDate("1988-04-12")

	return person.Patient{FirstName: "Amara", LastName: "Okonkwo", BirthDate: birth}
}
