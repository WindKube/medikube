package api

import (
	"context"
	"net/http"
	"slices"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/httproute"
	"medikube/internal/service/patient"
	"medikube/internal/web"
)

// The operation ids of contracts/patient-photo.md's three operations.
const (
	OpPutPatientPhoto    = "putPatientPhoto"
	OpGetPatientPhoto    = "getPatientPhoto"
	OpDeletePatientPhoto = "deletePatientPhoto"
)

// ParamPhotoSize is `?size=`, defaulting to the first configured thumbnail.
const ParamPhotoSize = "size"

// photoPartName is the one multipart part contracts/patient-photo.md accepts.
const photoPartName = "photo"

// defaultPhotoSize is contracts/patient-photo.md's documented default.
const defaultPhotoSize = "100x100t"

// PhotoServer is the storage seam this file streams bytes through, after the
// service has authorized the request. It never sees an actor: authorization
// already happened, and this is a resolved key and an io.Copy
// (contracts/patient-photo.md — no PocketBase file token, ever).
type PhotoServer interface {
	Serve(ctx context.Context, w http.ResponseWriter, r *http.Request, ownerID, patientID, size string) error
	Sizes() []string
}

// PatientPhotoResolve hands the photo handlers the same lazily-built stack
// PatientResolve hands the CRUD handlers, plus the store that can stream
// bytes directly.
type PatientPhotoResolve func() (*patient.Service, PhotoServer, error)

type patientPhotoHandlers struct {
	resolve PatientPhotoResolve
}

// PatientPhotoHandlers is contracts/patient-photo.md's three operations.
func PatientPhotoHandlers(resolve PatientPhotoResolve) (httproute.Handlers, error) {
	if resolve == nil {
		return nil, ErrNoPatients
	}

	h := &patientPhotoHandlers{resolve: resolve}

	return httproute.Handlers{
		OpPutPatientPhoto:    web.WithActor(h.put),
		OpGetPatientPhoto:    web.WithActor(h.get),
		OpDeletePatientPhoto: web.WithActor(h.remove),
	}, nil
}

// put stores the one photograph a patient may hold (FR-008, FR-009).
func (h *patientPhotoHandlers) put(e *core.RequestEvent, actor access.Actor) error {
	svc, _, err := h.resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathPatientID)

	upload, err := readPhotoUpload(e)
	if err != nil {
		return err
	}
	defer func() { _ = upload.file.Close() }()

	meta, err := svc.SetPhoto(e.Request.Context(), actor, id, patient.Upload{
		Reader: upload.file,
		Size:   upload.size,
		Name:   upload.name,
	})
	if err != nil {
		return err
	}

	e.Response.Header().Set("Cache-Control", patientCacheControl)

	return web.WriteJSON(e, http.StatusOK, struct {
		PhotoURL  string   `json:"photo_url"`
		Sizes     []string `json:"sizes"`
		UpdatedAt string   `json:"updated_at"`
	}{
		PhotoURL:  "/api/v1/patients/" + id + "/photo?size=" + defaultPhotoSize,
		Sizes:     meta.Sizes,
		UpdatedAt: meta.UpdatedAt,
	})
}

// get streams the bytes directly, after authorizing (contracts/patient-photo.md).
func (h *patientPhotoHandlers) get(e *core.RequestEvent, actor access.Actor) error {
	svc, photos, err := h.resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathPatientID)

	size := e.Request.URL.Query().Get(ParamPhotoSize)
	if size == "" {
		size = defaultPhotoSize
	}

	if !slices.Contains(photos.Sizes(), size) {
		var invalid domain.ValidationError
		invalid.Add(ParamPhotoSize, domain.CodeInvalidValue, "not a size this instance serves")

		return invalid.OrNil()
	}

	if err := svc.AuthorizePhotoRead(e.Request.Context(), actor, id); err != nil {
		return err
	}

	return photos.Serve(e.Request.Context(), e.Response, e.Request, actor.UserID, id, size)
}

func (h *patientPhotoHandlers) remove(e *core.RequestEvent, actor access.Actor) error {
	svc, _, err := h.resolve()
	if err != nil {
		return err
	}

	id := e.Request.PathValue(PathPatientID)

	if err := svc.DeletePhoto(e.Request.Context(), actor, id); err != nil {
		return err
	}

	e.Response.Header().Set("Cache-Control", patientCacheControl)

	return e.NoContent(http.StatusNoContent)
}

type photoUpload struct {
	file multipartFile
	size int64
	name string
}

// multipartFile is the subset of multipart.File this package reads.
type multipartFile interface {
	Read(p []byte) (int, error)
	Close() error
}

// readPhotoUpload reads the one required `photo` part. No part, or more than
// one, is 422 — the same taxonomy every other malformed body in this
// application uses.
func readPhotoUpload(e *core.RequestEvent) (photoUpload, error) {
	if err := e.Request.ParseMultipartForm(0); err != nil {
		return photoUpload{}, bodyRefusalPhoto("a multipart/form-data body with one `photo` part is required")
	}

	if e.Request.MultipartForm == nil || len(e.Request.MultipartForm.File[photoPartName]) != 1 {
		return photoUpload{}, bodyRefusalPhoto("exactly one `photo` part is required")
	}

	header := e.Request.MultipartForm.File[photoPartName][0]

	file, err := header.Open()
	if err != nil {
		return photoUpload{}, bodyRefusalPhoto("the `photo` part could not be read")
	}

	return photoUpload{file: file, size: header.Size, name: header.Filename}, nil
}

func bodyRefusalPhoto(message string) error {
	var invalid domain.ValidationError
	invalid.Add(photoPartName, domain.CodeInvalidValue, message)

	return invalid.OrNil()
}
