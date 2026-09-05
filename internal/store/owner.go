package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// ownerColumn is the account-relation column a kind carries in phase 001's
// shape. It is spelled here, once, rather than borrowed from a kind's own
// mapper constant: medication stopped having an "owner" column in phase 002
// (research D-13), and this lookup is generic across every kind that still
// has one.
const ownerColumn = "owner"

// Owners resolves the account a record belongs to, for any registered kind.
//
// It is the one read the authorization checkpoint makes, and it is deliberately
// the smallest one there is: an id in, an account id out, nothing else loaded
// and nothing else returned. A checkpoint that fetched the record would be a
// second path to a person's data with no authorization in front of it.
//
// The kind supplies its own collection, so a kind added in a later phase is
// resolvable here without this file changing.
type Owners struct {
	app core.App
}

func NewOwners(app core.App) (*Owners, error) {
	if app == nil {
		return nil, fmt.Errorf("store: the owner lookup is wired with no application")
	}

	return &Owners{app: app}, nil
}

// Owner answers domain.ErrNotFound for a record that is not there, and the
// failure itself for a read that could not be made.
//
// The caller must not turn a miss into a refusal: a record that does not exist
// is not the checkpoint's refusal to make, and answering "denied" for every
// mistyped identifier would fill the audit trail with attempts nobody made and
// make a genuine miss indistinguishable from a denial in the one place that can
// still tell them apart (research D-20).
//
// The distinction between the two is what keeps that grant safe. The checkpoint
// answers ErrNotFound with a full grant, deliberately, so an owner lookup that
// reported every failure as a miss would turn a cancelled query, a locked
// database or a dropped connection into an unconditional grant on somebody
// else's record — and would leave the checkpoint's own "could not answer"
// branch permanently unreachable. Only sql.ErrNoRows, which is what dbx returns
// for an empty result set (dbx/rows.go:244), is a miss.
func (o *Owners) Owner(ctx context.Context, k kind.Kind, recordID string) (string, error) {
	if !k.Valid() {
		return "", fmt.Errorf("store: %q is not a declared kind: %w", k, domain.ErrNotFound)
	}

	var owned struct {
		Owner string `db:"owner"`
	}

	err := o.app.RecordQuery(k.Collection()).
		Select(ownerColumn).
		AndWhere(dbx.HashExp{fieldID: recordID}).
		Limit(1).
		WithContext(ctx).
		One(&owned)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("store: reading the owner of a %s: %w", k, domain.ErrNotFound)
		}

		return "", fmt.Errorf("store: reading the owner of a %s: %w", k, err)
	}

	return owned.Owner, nil
}

// patientsCollectionName is the collection PatientOwner resolves against.
// Phase 002 owns "patients" (internal/store/migrations declares its own copy
// for the same reason kind.Kind literals live in one table): this package has
// no kind.Kind for a patient — a patient is the anchor kinds are authorized
// against, not one of them (research D-05) — so the name is spelled once here.
const patientsCollectionName = "patients"

// PatientOwners resolves the account a patient belongs to.
//
// It is access.PatientOwners' one implementation, and it is the same shape as
// Owners for the same reason: an id in, an account id out, nothing else loaded.
type PatientOwners struct {
	app core.App
}

func NewPatientOwners(app core.App) (*PatientOwners, error) {
	if app == nil {
		return nil, fmt.Errorf("store: the patient-owner lookup is wired with no application")
	}

	return &PatientOwners{app: app}, nil
}

// PatientsOfOwner lists the ids of every patient one account owns.
//
// It exists for the one caller that must enumerate an account's patients
// without importing the patient package itself (internal/web/api/me.go's
// per-kind counts, research D-13): once a clinical kind is patient-anchored,
// "the account's records" is the union over its patients, and this is the
// smallest read that makes that union possible.
func (o *PatientOwners) PatientsOfOwner(ctx context.Context, ownerID string) ([]string, error) {
	var rows []struct {
		ID string `db:"id"`
	}

	err := o.app.RecordQuery(patientsCollectionName).
		Select(fieldID).
		AndWhere(dbx.HashExp{ownerColumn: ownerID}).
		WithContext(ctx).
		All(&rows)
	if err != nil {
		return nil, fmt.Errorf("store: listing %s's patients: %w", ownerID, err)
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	return ids, nil
}

// PatientOwner answers domain.ErrNotFound for a patient that is not there, on
// the same terms Owner does: only sql.ErrNoRows is a miss, and everything else
// is a failure the checkpoint must not read as a refusal (research D-20).
func (o *PatientOwners) PatientOwner(ctx context.Context, patientID string) (string, error) {
	var owned struct {
		Owner string `db:"owner"`
	}

	err := o.app.RecordQuery(patientsCollectionName).
		Select(ownerColumn).
		AndWhere(dbx.HashExp{fieldID: patientID}).
		Limit(1).
		WithContext(ctx).
		One(&owned)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("store: reading the owner of a patient: %w", domain.ErrNotFound)
		}

		return "", fmt.Errorf("store: reading the owner of a patient: %w", err)
	}

	return owned.Owner, nil
}
