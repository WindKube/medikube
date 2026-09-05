package patient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"medikube/internal/domain"
	service "medikube/internal/service/patient"
	"medikube/internal/store"
)

// SizeOriginal is the full-resolution photograph, always servable alongside
// whatever thumbnail sizes are configured.
const SizeOriginal = "original"

// PhotoStore is the PocketBase adapter for service.PhotoStore.
//
// Thumbnails are generated eagerly (FR-009): PocketBase's own lazy generation
// lives in apis/file.go, behind the /api/files route this application never
// serves (FR-044, contracts/patient-photo.md), so nothing would ever create
// them without this package doing it itself, once, on upload.
type PhotoStore struct {
	app    core.App
	thumbs []string
}

var _ service.PhotoStore = (*PhotoStore)(nil)

// NewPhotoStore wires the photo store to the configured thumbnail sizes
// (MEDIKUBE_FILES_PHOTO_THUMBS).
func NewPhotoStore(app core.App, thumbs []string) (*PhotoStore, error) {
	if app == nil {
		return nil, fmt.Errorf("patient photo store: wired with no application")
	}

	if len(thumbs) == 0 {
		return nil, fmt.Errorf("patient photo store: wired with no thumbnail sizes")
	}

	return &PhotoStore{app: app, thumbs: thumbs}, nil
}

// Put stores the upload as the patient's one photograph (FR-008), replacing
// whatever was there before it (US1-5) — PocketBase's own file-field cleanup
// removes the previous original and, because the eager thumbnails below use
// its `thumbs_<filename>/` key layout, the previous thumbnails too
// (core/field_file.go:612) — and generates both configured thumbnails before
// returning (FR-009).
func (p *PhotoStore) Put(ctx context.Context, ownerID, patientID string, upload service.Upload) (service.PhotoMeta, error) {
	var meta service.PhotoMeta

	// Read whole: the field's own MaxSize bounds this at 15 MiB by default
	// (config.FilesConfig), and filesystem.File has no reader-based
	// constructor other than a multipart header's, which this package's
	// caller — the HTTP edge — does not hold once it has decided to read the
	// part into service.Upload.
	raw, err := io.ReadAll(upload.Reader)
	if err != nil {
		return meta, fmt.Errorf("reading the uploaded photo: %w", err)
	}

	file, err := filesystem.NewFileFromBytes(raw, upload.Name)
	if err != nil {
		return meta, fmt.Errorf("reading the uploaded photo: %w", err)
	}

	write := func(txApp core.App) error {
		record, findErr := p.owned(ctx, txApp, ownerID, patientID)
		if findErr != nil {
			return findErr
		}

		record.Set(store.PatientPhoto, file)

		if saveErr := txApp.SaveWithContext(ctx, record); saveErr != nil {
			return saveErr
		}

		filename := record.GetString(store.PatientPhoto)
		if filename == "" {
			return fmt.Errorf("patient photo store: the record carries no filename after saving the upload")
		}

		basePath := record.BaseFilesPath()
		sizes := append([]string{"original"}, p.thumbs...)
		meta = service.PhotoMeta{Sizes: sizes, UpdatedAt: record.GetDateTime("updated").String()}

		// Eager thumbnail generation, inside the transaction's own
		// OnComplete callback (contracts/patient-photo.md): the original is
		// only durably on disk once this transaction commits, and generating
		// before that risks a thumbnail for a photograph the write then rolls
		// back.
		if txApp.TxInfo() != nil {
			txApp.TxInfo().OnComplete(func(txErr error) error {
				if txErr != nil {
					return nil
				}

				return p.createThumbs(basePath, filename)
			})
		} else if genErr := p.createThumbs(basePath, filename); genErr != nil {
			return genErr
		}

		return nil
	}

	if txErr := store.RunInTransaction(p.app, write); txErr != nil {
		return service.PhotoMeta{}, mapPhotoError(txErr)
	}

	return meta, nil
}

// Remove deletes the photograph and its thumbnails. Idempotent: a patient
// with no photograph answers nil, because clearing an already-empty file
// field is a no-op PocketBase's own save leaves alone.
func (p *PhotoStore) Remove(ctx context.Context, ownerID, patientID string) error {
	write := func(txApp core.App) error {
		record, err := p.owned(ctx, txApp, ownerID, patientID)
		if err != nil {
			return err
		}

		record.Set(store.PatientPhoto, "")

		return txApp.SaveWithContext(ctx, record)
	}

	return store.RunInTransaction(p.app, write)
}

// Sizes is every size this store can serve: the original plus the
// configured thumbnails, in the order contracts/patient-photo.md publishes
// them.
func (p *PhotoStore) Sizes() []string {
	return append([]string{SizeOriginal}, p.thumbs...)
}

// Serve streams the stored photograph directly to the response, after the
// caller has authorized the request — this package never authorizes, it only
// resolves a key and reads bytes (contracts/patient-photo.md's "never a
// PocketBase file token").
//
// domain.ErrNotFound covers three indistinguishable cases: the patient does
// not exist, is not ownerID's, or has no photograph — none of which may be
// told apart by the response (FR-042, FR-044).
func (p *PhotoStore) Serve(ctx context.Context, w http.ResponseWriter, r *http.Request, ownerID, patientID, size string) error {
	if !slices.Contains(p.Sizes(), size) {
		return fmt.Errorf("patient photo store: %q is not a size this instance serves", size)
	}

	record, err := p.owned(ctx, p.app, ownerID, patientID)
	if err != nil {
		return err
	}

	filename := record.GetString(store.PatientPhoto)
	if filename == "" {
		return fmt.Errorf("patient %s carries no photograph: %w", patientID, domain.ErrNotFound)
	}

	basePath := record.BaseFilesPath()

	key := basePath + "/" + filename
	if size != SizeOriginal {
		key = basePath + "/thumbs_" + filename + "/" + size + "_" + filename
	}

	fsys, err := p.app.NewFilesystem()
	if err != nil {
		return fmt.Errorf("opening the filesystem to serve a photograph: %w", err)
	}
	defer fsys.Close()

	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Cookie, Authorization")
	w.Header().Set("Content-Disposition", `inline; filename="photo.jpg"`)
	w.Header().Set("ETag", `"`+fileETag(key)+`"`)

	if serveErr := fsys.Serve(w, r, key, "photo.jpg"); serveErr != nil {
		// A record naming a blob the filesystem does not have is the same
		// answer as no photograph at all (FR-042, FR-044): the reference is
		// dangling either way, and reporting an internal error here would
		// disclose that a storage/record mismatch exists rather than just
		// that there is nothing to serve.
		if errors.Is(serveErr, filesystem.ErrNotFound) {
			return fmt.Errorf("patient %s: %w", patientID, domain.ErrNotFound)
		}

		return serveErr
	}

	return nil
}

// fileETag derives an entity-tag from the stored key, which carries
// PocketBase's random filename suffix and therefore changes on every
// replacement — a cheap, leak-free validator (contracts/patient-photo.md).
func fileETag(key string) string {
	sum := sha256.Sum256([]byte(key))

	return hex.EncodeToString(sum[:8])
}

func (p *PhotoStore) createThumbs(basePath, filename string) error {
	fsys, err := p.app.NewFilesystem()
	if err != nil {
		return fmt.Errorf("opening the filesystem for thumbnails: %w", err)
	}
	defer fsys.Close()

	original := basePath + "/" + filename

	for _, size := range p.thumbs {
		thumbKey := basePath + "/thumbs_" + filename + "/" + size + "_" + filename

		if err := fsys.CreateThumb(original, thumbKey, size); err != nil {
			return fmt.Errorf("creating the %s thumbnail: %w", size, err)
		}
	}

	return nil
}

func (p *PhotoStore) owned(ctx context.Context, app core.App, ownerID, patientID string) (*core.Record, error) {
	built, err := store.PatientsSchema().Build(store.Query{
		Conditions: []store.Condition{
			store.Equal(store.PatientOwner, ownerID),
			store.Equal(store.ColumnID, patientID),
		},
		Limit: 1,
	})
	if err != nil {
		return nil, err
	}

	var records []*core.Record

	if queryErr := built.Apply(app.RecordQuery(store.PatientCollection)).
		WithContext(ctx).All(&records); queryErr != nil {
		return nil, fmt.Errorf("finding patient %s: %w", patientID, queryErr)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("patient %s: %w", patientID, domain.ErrNotFound)
	}

	return records[0], nil
}

// mapPhotoError maps a PocketBase file-field validation failure onto
// MediKube's own sentinels (research D-17): the message embeds the uploaded
// filename, which constitution VII names as PHI, so it must never reach the
// caller or the log stream — only the sentinel does.
func mapPhotoError(err error) error {
	if err == nil {
		return nil
	}

	if mapped := mapFileValidationSubstring(err.Error()); mapped != nil {
		return mapped
	}

	return err
}

func mapFileValidationSubstring(message string) error {
	switch {
	case strings.Contains(message, "unsupported file type"):
		return domain.ErrUnsupportedMedia
	case strings.Contains(message, "the maximum allowed file size is"):
		return domain.ErrTooLarge
	default:
		return nil
	}
}
