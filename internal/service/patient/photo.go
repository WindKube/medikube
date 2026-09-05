package patient

import (
	"context"

	"medikube/internal/domain/access"
)

// SetPhoto stores the one photograph a patient may hold, replacing whatever
// was there before it (FR-008, US1-5). The sniffing, the size limit and the
// eager thumbnails are the store's (contracts/patient-photo.md); this method's
// whole job is the authorization checkpoint every write in this package opens
// with.
func (s *Service) SetPhoto(ctx context.Context, actor access.Actor, patientID string, upload Upload) (PhotoMeta, error) {
	if _, err := s.authorizer.Patient(ctx, actor, patientID, access.PermOwn); err != nil {
		return PhotoMeta{}, err
	}

	return s.photos.Put(ctx, actor.UserID, patientID, upload)
}

// DeletePhoto removes the photograph and its thumbnails. Idempotent: a
// patient with no photograph answers nil, exactly as PhotoStore.Remove does.
func (s *Service) DeletePhoto(ctx context.Context, actor access.Actor, patientID string) error {
	if _, err := s.authorizer.Patient(ctx, actor, patientID, access.PermOwn); err != nil {
		return err
	}

	return s.photos.Remove(ctx, actor.UserID, patientID)
}

// AuthorizePhotoRead is getPatientPhoto's checkpoint: it answers whether the
// actor may reach this patient's photograph at all, and leaves the bytes to
// internal/web/api, which serves them directly against
// internal/store/patient's PocketBase-specific type (contracts/patient-photo.md
// — the bytes never pass through this package, which may not import
// net/http).
//
// In this phase the ladder has one rung that reaches here at all: an
// authenticated owner (research D-05). Authorizer.Patient answers
// domain.ErrNotFound for everyone else, so read_sensitive — written only when
// the resolved grant is not the actor's own ownership (005
// widened-authorization.md) — has nothing to write for in phase 002; a
// superuser and a share recipient arrive with phase 005.
func (s *Service) AuthorizePhotoRead(ctx context.Context, actor access.Actor, patientID string) error {
	_, err := s.authorizer.Patient(ctx, actor, patientID, access.PermView)

	return err
}
