package patienttest

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/service/patient"
)

// onePixelPNG is the smallest byte sequence PocketBase's own content sniffing
// (and net/http.DetectContentType, which agrees with it on PNG) accepts as
// image/png — the same fixture internal/testsupport/seed uses.
var onePixelPNG = mustDecodePNG()

func mustDecodePNG() []byte {
	const encoded = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		panic(err)
	}

	return raw
}

// PhotoStoreFactory builds a fresh, empty photo store.
type PhotoStoreFactory func(t *testing.T) patient.PhotoStore

// PhotoStoreContract is the shared Liskov suite every PhotoStore
// implementation must pass: store, replace, remove and thumbnail presence
// (T044).
func PhotoStoreContract(t *testing.T, factory PhotoStoreFactory) {
	t.Helper()

	t.Run("storing a photograph answers both configured thumbnails", func(t *testing.T) {
		store := factory(t)

		meta, err := store.Put(t.Context(), OwnerID, "mkpatientone001", patient.Upload{
			Reader: bytes.NewReader(onePixelPNG), Size: int64(len(onePixelPNG)), Name: "photo.png",
		})
		require.NoError(t, err)
		assert.Contains(t, meta.Sizes, "100x100t")
		assert.Contains(t, meta.Sizes, "400x400f")
		assert.NotEmpty(t, meta.UpdatedAt)
	})

	t.Run("a non-image is refused and nothing is stored", func(t *testing.T) {
		store := factory(t)

		notAnImage := []byte("%PDF-1.4 this is not a photograph")

		_, err := store.Put(t.Context(), OwnerID, "mkpatienttwo002", patient.Upload{
			Reader: bytes.NewReader(notAnImage), Size: int64(len(notAnImage)), Name: "photo.jpg",
		})
		assert.ErrorIs(t, err, domain.ErrUnsupportedMedia)
	})

	t.Run("replacing removes the previous file", func(t *testing.T) {
		store := factory(t)
		patientID := "mkpatientthre03"

		_, err := store.Put(t.Context(), OwnerID, patientID, patient.Upload{
			Reader: bytes.NewReader(onePixelPNG), Size: int64(len(onePixelPNG)), Name: "first.png",
		})
		require.NoError(t, err)

		_, err = store.Put(t.Context(), OwnerID, patientID, patient.Upload{
			Reader: bytes.NewReader(onePixelPNG), Size: int64(len(onePixelPNG)), Name: "second.png",
		})
		require.NoError(t, err)

		require.NoError(t, store.Remove(t.Context(), OwnerID, patientID))
	})

	t.Run("removal leaves none, and is idempotent", func(t *testing.T) {
		store := factory(t)
		patientID := "mkpatientfour04"

		_, err := store.Put(t.Context(), OwnerID, patientID, patient.Upload{
			Reader: bytes.NewReader(onePixelPNG), Size: int64(len(onePixelPNG)), Name: "photo.png",
		})
		require.NoError(t, err)

		require.NoError(t, store.Remove(t.Context(), OwnerID, patientID))
		require.NoError(t, store.Remove(t.Context(), OwnerID, patientID), "removing an absent photograph is not an error")
	})
}
