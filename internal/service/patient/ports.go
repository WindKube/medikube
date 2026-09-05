package patient

import (
	"context"
	"io"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/person"
)

// Query is one list request, resolved by the edge before it reaches this
// package: the ordering (research D-29, web.PatientsSort) and the limit are
// validated against the published allowlist by internal/web, which is where
// every kind's list parameters are validated in this phase — patients are not
// a kind.Kind (research D-05), so there is no per-kind service package to
// publish a second copy of that allowlist ahead of this story.
type Query struct {
	// Search is the case-insensitive substring match over first and last
	// name (contracts/patients.md `?q=`).
	Search string

	Sort   []domain.SortKey
	Limit  int
	Cursor string
	Count  bool
}

// Repository is the storage seam, declared by the consumer (Principle II).
//
// Every method is owner-scoped, mirroring internal/service/medication's own
// repository: the service authorizes above this and the repository refuses a
// row that is not the owner's anyway, deliberately, as two independent
// refusals guarding the one failure that matters.
//
// Five methods, plan.md's interface-segregation cap. SelfRecord exists beside
// Get because FR-004's conflict must be checked before an insert is attempted,
// and a List call filtered down to one flag would be a second, implicit
// meaning bolted onto a query built for something else.
type Repository interface {
	List(ctx context.Context, ownerID string, query Query) (domain.Page[person.Patient], error)

	// Get answers domain.ErrNotFound for a row that does not exist and for
	// one that is not this owner's (FR-042).
	Get(ctx context.Context, ownerID, id string) (person.Patient, error)

	// Create mints the identity, the timestamps and the version, and returns
	// the row as stored.
	Create(ctx context.Context, draft person.Patient) (person.Patient, error)

	// Update writes the entity over the row it identifies, only while the
	// stored version is still expectedVersion. A mismatch is
	// domain.ErrVersionMismatch; a row that is not this owner's is
	// domain.ErrNotFound.
	Update(ctx context.Context, patient person.Patient, expectedVersion string) (person.Patient, error)

	// SelfRecord answers the owner's is_self_record row, or
	// domain.ErrNotFound when there is none yet — which is what lets
	// CreateSelfRecord's 409 (FR-004) be checked before the write is even
	// attempted, ahead of the partial unique index that guards it underneath.
	SelfRecord(ctx context.Context, ownerID string) (person.Patient, error)
}

// Upload is one photograph as the edge read it: PocketBase's own content
// sniffing decides the type from Reader, never from Name or a declared
// content type (FR-008), so Name is carried only so a store can build a
// filesystem.File and is never trusted as a type.
type Upload struct {
	Reader io.Reader
	Size   int64
	Name   string
}

// PhotoMeta is what a stored photograph answers with: the sizes actually on
// disk (FR-009, eager) and when it was last written.
type PhotoMeta struct {
	Sizes     []string
	UpdatedAt string
}

// PhotoStore is the photo policy's storage seam, declared by the consumer.
//
// Two methods: a whole-resource replace and a removal. There is no Get here —
// the bytes are served directly by internal/web/api against
// internal/store/patient's own PocketBase-specific type, never through a
// service-layer abstraction that would have to carry an http.ResponseWriter
// two layers deeper than this package may see (contracts/patient-photo.md).
type PhotoStore interface {
	// Put stores the upload as the patient's one photograph, replacing and
	// removing whatever was there before it (US1-5), and generates every
	// configured thumbnail eagerly (FR-009) before it returns.
	Put(ctx context.Context, ownerID, patientID string, upload Upload) (PhotoMeta, error)

	// Remove deletes the photograph and its thumbnails. It is idempotent: a
	// patient with no photograph answers nil.
	Remove(ctx context.Context, ownerID, patientID string) error
}

// Authorizer is the patient anchor (research D-05), exactly as
// internal/service/access.Authorizer implements it: every refusal already
// carries its own audit row and its own sentinel
// (domain.ErrUnauthenticated, domain.ErrNotFound), so this package writes no
// audit row of its own for a denial — unlike internal/service/medication,
// which authorizes against a kind and audits the refusal itself.
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}

// ActivePatientStore is the users.active_patient column (contracts/active-
// patient.md), a seam of its own rather than a Repository method: it reaches
// past the patient row into the account that points at it, which Repository's
// five owner-scoped methods never do.
type ActivePatientStore interface {
	// ActivePatient answers the pointer, or "" when it is unset.
	ActivePatient(ctx context.Context, userID string) (string, error)

	// SetActivePatient writes the pointer. patientID "" clears it.
	SetActivePatient(ctx context.Context, userID, patientID string) error
}

// Auditor writes the trail. This package reaches it for exactly one thing:
// FR-045's switch_patient row, mirroring
// internal/service/practitioner.Auditor.
type Auditor interface {
	Record(ctx context.Context, event audit.Event) error
}
