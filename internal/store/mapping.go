package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
)

// auditCollection is the audit trail. Like the auth collection above it, it is
// not a kind.Kind: audit_events records that something happened to a record, it
// is not a record kind.
const auditCollection = "audit_events"

// AuditCollection is that name, published. The audit trail is not a record kind
// and therefore has no kind.Collection() to read it from, so the writer that
// appends to it and the reader that inspects it would otherwise each spell it
// for themselves — which is the drift the kind table exists to prevent, one
// collection to the left of where the table can reach.
const AuditCollection = auditCollection

// AuditFieldOccurredAt is the column the retention horizon is measured against,
// published for the same reason AuditCollection is: a test that reads a purged
// trail back has to name the column, and the alternative is spelling it a
// second time somewhere AssertMappedFields cannot see it.
const AuditFieldOccurredAt = auditFieldOccurredAt

// AuditOlderThan is the retention purge's predicate: rows that occurred
// strictly before cutoff.
//
// It lives in this package rather than in internal/store/audit for two reasons
// that turn out to be the same reason. The column name is here, with every
// other column name, because AssertMappedFields is what stops one drifting away
// from the schema. And the predicate carries a `{:param}` placeholder, which is
// dbx's bound-parameter syntax (dbx/db.go:305-312 rewrites it into a driver
// placeholder and binds the value) but is spelled identically to PocketBase's
// filter DSL, where the same token is substituted into the text before parsing.
// The two are indistinguishable to a reader and to the source gate in
// filter_test.go, and this package is the one allowed to write either — so the
// repository packages hold no query-language string at all and the gate over
// them stays absolute.
//
// Strictly before: a row that occurred exactly at the cutoff is exactly the
// configured age and not older than it, which is a row the retention says to
// keep.
func AuditOlderThan(cutoff time.Time) (dbx.Expression, error) {
	// PocketBase persists a date as a string in its own layout, so the
	// comparison is lexicographic and the parameter has to be in that layout to
	// sort against the column at all. Handed a raw time.Time, dbx would bind
	// Go's own rendering of it.
	horizon, err := types.ParseDateTime(cutoff.UTC())
	if err != nil {
		return nil, fmt.Errorf("store: %s is not a date %s can be compared against: %w", cutoff, auditFieldOccurredAt, err)
	}

	return dbx.NewExp("[["+auditFieldOccurredAt+"]] < {:horizon}", dbx.Params{"horizon": horizon}), nil
}

// The columns on every collection. PocketBase supplies id; created and updated
// are AutodateFields the migrations add explicitly, because NewBaseCollection
// does not.
const (
	fieldID      = "id"
	fieldCreated = "created"
	fieldUpdated = "updated"
)

// data-model §1's seven profile columns, plus the three PocketBase auth columns
// the domain reads. Every column this package touches is named once, here, and
// AssertMappedFields is what stops a name from drifting away from the schema —
// core.Record's getters are casts that cannot fail, so a misspelling reads back
// as the zero value and the field silently stops being saved.
const (
	userFieldEmail      = "email"
	userFieldVerified   = "verified"
	userFieldName       = "name"
	userFieldRole       = "role"
	userFieldUnitSystem = "unit_system"
	userFieldLocale     = "locale"
	userFieldDateFormat = "date_format"
	userFieldTheme      = "theme"
	userFieldDisabledAt = "disabled_at"
)

// data-model §2's thirteen columns.
const (
	medicationFieldOwner           = "owner"
	medicationFieldName            = "name"
	medicationFieldAlternativeName = "alternative_name"
	medicationFieldType            = "type"
	medicationFieldDosage          = "dosage"
	medicationFieldFrequency       = "frequency"
	medicationFieldRoute           = "route"
	medicationFieldIndication      = "indication"
	medicationFieldStartedOn       = "started_on"
	medicationFieldEndedOn         = "ended_on"
	medicationFieldStatus          = "status"
	medicationFieldSideEffects     = "side_effects"
	medicationFieldNotes           = "notes"
)

// data-model §3's seven columns. The list is the point: there is nothing here a
// value, a name, a note or a diff could be written into.
const (
	auditFieldOccurredAt = "occurred_at"
	auditFieldActor      = "actor"
	auditFieldActorKind  = "actor_kind"
	auditFieldAction     = "action"
	auditFieldTargetKind = "target_kind"
	auditFieldTargetID   = "target_id"
	auditFieldRequestID  = "request_id"
	// auditFieldPatient is phase 002's addition (data-model §5): the person a
	// patient-scoped action concerned, null for a non-patient action.
	auditFieldPatient = "patient"
)

var (
	// ErrUnexpectedCollection is a record handed to the wrong mapper. It is an
	// error rather than a best-effort read because every getter would answer
	// with a zero value and the result would look like an empty entity.
	ErrUnexpectedCollection = errors.New("store: the record is not from the collection this mapper reads")

	// ErrMalformedRecord is a column holding something its type does not allow
	// — in practice a date column carrying a time of day, which truncation
	// would turn into a record that moved by a day and reported nothing.
	ErrMalformedRecord = errors.New("store: a column holds a value the domain cannot represent")

	// ErrSchemaDrift is a column this package names that the schema does not
	// have.
	ErrSchemaDrift = errors.New("store: a column the mapper reads is not in the schema")
)

// MedicationFromRecord reads a stored row into the entity.
//
// It does not validate. The domain is the one authority on whether a value is
// acceptable (research D-26), and a row that is already stored is a fact
// whatever the current rules say about it; re-judging it here would make a
// tightened rule silently unreadable rather than visibly invalid.
func MedicationFromRecord(record *core.Record) (clinical.Medication, error) {
	if err := expectCollection(record, kind.Medication.Collection()); err != nil {
		return clinical.Medication{}, err
	}

	startedOn, err := recordDate(record, medicationFieldStartedOn)
	if err != nil {
		return clinical.Medication{}, err
	}

	endedOn, err := recordDate(record, medicationFieldEndedOn)
	if err != nil {
		return clinical.Medication{}, err
	}

	return clinical.Medication{
		ID:              record.Id,
		OwnerID:         record.GetString(medicationFieldOwner),
		Name:            record.GetString(medicationFieldName),
		AlternativeName: record.GetString(medicationFieldAlternativeName),
		Type:            clinical.MedicationType(record.GetString(medicationFieldType)),
		Dosage:          record.GetString(medicationFieldDosage),
		Frequency:       record.GetString(medicationFieldFrequency),
		Route:           clinical.MedicationRoute(record.GetString(medicationFieldRoute)),
		Indication:      record.GetString(medicationFieldIndication),
		StartedOn:       startedOn,
		EndedOn:         endedOn,
		Status:          clinical.TherapyStatus(record.GetString(medicationFieldStatus)),
		SideEffects:     record.GetString(medicationFieldSideEffects),
		Notes:           record.GetString(medicationFieldNotes),
		CreatedAt:       recordInstant(record, fieldCreated),
		UpdatedAt:       recordInstant(record, fieldUpdated),
		Version:         Version(record),
	}, nil
}

// MedicationToRecord writes the entity's columns onto the record.
//
// It writes the owner, and it is the only place that does: OwnerID is
// server-set and absent from every request DTO, so a create path that forgot it
// hits the column's Required rather than storing a medication nobody can reach.
// It does not write id, created, updated or version — PocketBase owns all four.
func MedicationToRecord(record *core.Record, medication clinical.Medication) error {
	if err := expectCollection(record, kind.Medication.Collection()); err != nil {
		return err
	}

	record.Set(medicationFieldOwner, medication.OwnerID)
	record.Set(medicationFieldName, medication.Name)
	record.Set(medicationFieldAlternativeName, medication.AlternativeName)
	record.Set(medicationFieldType, string(medication.Type))
	record.Set(medicationFieldDosage, medication.Dosage)
	record.Set(medicationFieldFrequency, medication.Frequency)
	record.Set(medicationFieldRoute, string(medication.Route))
	record.Set(medicationFieldIndication, medication.Indication)
	setDate(record, medicationFieldStartedOn, medication.StartedOn)
	setDate(record, medicationFieldEndedOn, medication.EndedOn)
	record.Set(medicationFieldStatus, string(medication.Status))
	record.Set(medicationFieldSideEffects, medication.SideEffects)
	record.Set(medicationFieldNotes, medication.Notes)

	return nil
}

func UserFromRecord(record *core.Record) (identity.User, error) {
	if err := expectCollection(record, authCollection); err != nil {
		return identity.User{}, err
	}

	return identity.User{
		ID:             record.Id,
		Email:          record.Email(),
		EmailConfirmed: record.GetBool(userFieldVerified),
		Name:           record.GetString(userFieldName),
		Role:           identity.Role(record.GetString(userFieldRole)),
		UnitSystem:     identity.UnitSystem(record.GetString(userFieldUnitSystem)),
		Locale:         record.GetString(userFieldLocale),
		DateFormat:     identity.DateFormat(record.GetString(userFieldDateFormat)),
		Theme:          identity.Theme(record.GetString(userFieldTheme)),
		DisabledAt:     recordInstant(record, userFieldDisabledAt),
		CreatedAt:      recordInstant(record, fieldCreated),
		UpdatedAt:      recordInstant(record, fieldUpdated),
	}, nil
}

// UserToRecord writes the profile columns and *only* the profile columns.
//
// Email, verified, password and tokenKey are deliberately absent. PocketBase
// owns the credential and the token key a password change rotates (research
// D-13, D-16); the address is the one thing FR-011 does not let a person change
// about themselves and it moves only through the confirmed email-change flow;
// and verified is set by confirming a message, never by a request (FR-075).
// A mapper that wrote them would be a second, unaudited path to all four.
func UserToRecord(record *core.Record, user identity.User) error {
	if err := expectCollection(record, authCollection); err != nil {
		return err
	}

	record.Set(userFieldName, user.Name)
	record.Set(userFieldRole, string(user.Role))
	record.Set(userFieldUnitSystem, string(user.UnitSystem))
	record.Set(userFieldLocale, user.Locale)
	record.Set(userFieldDateFormat, string(user.DateFormat))
	record.Set(userFieldTheme, string(user.Theme))
	record.Set(userFieldDisabledAt, user.DisabledAt.UTC())

	return nil
}

func AuditEventFromRecord(record *core.Record) (audit.Event, error) {
	if err := expectCollection(record, auditCollection); err != nil {
		return audit.Event{}, err
	}

	return audit.Event{
		OccurredAt: recordInstant(record, auditFieldOccurredAt),
		ActorID:    record.GetString(auditFieldActor),
		ActorKind:  audit.ActorKind(record.GetString(auditFieldActorKind)),
		Action:     audit.Action(record.GetString(auditFieldAction)),
		TargetKind: audit.TargetKind(record.GetString(auditFieldTargetKind)),
		TargetID:   record.GetString(auditFieldTargetID),
		RequestID:  record.GetString(auditFieldRequestID),
		PatientID:  record.GetString(auditFieldPatient),
	}, nil
}

func AuditEventToRecord(record *core.Record, event audit.Event) error {
	if err := expectCollection(record, auditCollection); err != nil {
		return err
	}

	record.Set(auditFieldOccurredAt, event.OccurredAt.UTC())
	record.Set(auditFieldActor, event.ActorID)
	record.Set(auditFieldActorKind, string(event.ActorKind))
	record.Set(auditFieldAction, string(event.Action))
	record.Set(auditFieldTargetKind, string(event.TargetKind))
	record.Set(auditFieldTargetID, event.TargetID)
	record.Set(auditFieldRequestID, event.RequestID)
	record.Set(auditFieldPatient, event.PatientID)

	return nil
}

// Version is the ETag source of research D-24, derived from `updated` and never
// a column of its own.
//
// It is a hash rather than the instant itself for two reasons. PocketBase's own
// date layout carries a space, and RFC 9110's etagc excludes it, so the raw
// value is not a legal ETag at all. And a version that reads as a timestamp
// invites somebody to compare two of them for order, which is not what a
// version is for.
//
// A record with no `updated` yet — one that has never been saved — has no
// version, rather than the one version every unsaved record would share.
func Version(record *core.Record) string {
	updated := record.GetDateTime(fieldUpdated)
	if updated.IsZero() {
		return ""
	}

	sum := sha256.Sum256([]byte(updated.String()))

	return hex.EncodeToString(sum[:8])
}

// AssertMappedFields refuses to let the instance run against a schema this
// package cannot read.
//
// Every core.Record getter is a cast with nowhere to report a failure: reading
// a column that does not exist returns the zero value and the mapper carries
// on. So a renamed column does not break a test, it quietly stops saving a
// field. This is the only thing between that and a boot that looks fine.
func AssertMappedFields(app core.App) error {
	expected := map[string][]string{
		authCollection: {
			fieldID, fieldCreated, fieldUpdated,
			userFieldEmail, userFieldVerified, userFieldName, userFieldRole,
			userFieldUnitSystem, userFieldLocale, userFieldDateFormat,
			userFieldTheme, userFieldDisabledAt,
		},
		kind.Medication.Collection(): {
			fieldID, fieldCreated, fieldUpdated,
			medicationFieldOwner, medicationFieldName, medicationFieldAlternativeName,
			medicationFieldType, medicationFieldDosage, medicationFieldFrequency,
			medicationFieldRoute, medicationFieldIndication, medicationFieldStartedOn,
			medicationFieldEndedOn, medicationFieldStatus, medicationFieldSideEffects,
			medicationFieldNotes,
		},
		auditCollection: {
			fieldID, fieldCreated, fieldUpdated,
			auditFieldOccurredAt, auditFieldActor, auditFieldActorKind,
			auditFieldAction, auditFieldTargetKind, auditFieldTargetID,
			auditFieldRequestID, auditFieldPatient,
		},
	}

	var problems []error

	for _, name := range []string{authCollection, kind.Medication.Collection(), auditCollection} {
		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			problems = append(problems, fmt.Errorf("%w: %s is missing entirely: %w", ErrSchemaDrift, name, err))
			continue
		}

		for _, field := range expected[name] {
			if collection.Fields.GetByName(field) == nil {
				problems = append(problems, fmt.Errorf("%w: %s.%s", ErrSchemaDrift, name, field))
			}
		}
	}

	return errors.Join(problems...)
}

func expectCollection(record *core.Record, name string) error {
	collection := record.Collection()
	if collection == nil || collection.Name != name {
		return fmt.Errorf("%w: expected %s", ErrUnexpectedCollection, name)
	}

	return nil
}

// recordDate reads a calendar date, and the branch on the zero value is the
// whole function.
//
// An unset date column holds the empty string, which comes back as the zero
// types.DateTime, whose Time() is time.Time{} — year 1, January, day 1, every
// clock component zero and offset zero. That is a date the domain would accept,
// so "no start date recorded" would become 1 January of the year 1 and would
// render as one. The zero value has to be recognised before it is converted.
func recordDate(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("%w: %s is not a calendar date: %w", ErrMalformedRecord, field, err)
	}

	return date, nil
}

// setDate writes the calendar date as midnight UTC — or, for the absent date,
// as the zero time, which types.DateTime stores as the empty string.
//
// domain.Date.UTC() is the only bridge to an instant this may use. Building the
// instant any other way reintroduces the exact bug the type exists to prevent:
// time.Date(y, m, d, 0, 0, 0, 0, time.Local) in UTC+14 is the previous day.
func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

// recordInstant reads a stored instant in UTC. Instants are RFC3339 UTC
// everywhere above this line (research D-27), and the conversion belongs here
// rather than at each of the dozen places that would otherwise do it.
//
// It is truncated to the precision the column actually holds. PocketBase's
// date layout is "2006-01-02 15:04:05.000Z" — milliseconds — but a record that
// has just been saved still carries the full-precision time.Time it was
// stamped with in memory. Without the truncation the entity returned by a
// create carries a `created` the database does not have, and re-reading the
// same row a moment later answers with a different instant: the same value
// written and read back would compare unequal, which is a difference nothing
// downstream would explain.
func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
