package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/domain/person"
	"medikube/internal/obs"
)

// The three columns contracts/medications-rescope.md and data-model §8 add,
// replacing medicationFieldOwner (research D-13).
const (
	medicationFieldPatient      = "patient"
	medicationFieldPractitioner = "practitioner"
	medicationFieldPharmacy     = "pharmacy"
)

// ErrUnattributedMedication is research D-13 step 4's post-condition: a
// medication the backfill did not attribute to a patient. Returning it rolls
// back the whole migration batch (core/migrations_runner.go:129-131), because
// there is no partially repointed state worth keeping.
var ErrUnattributedMedication = errors.New("migrations: a medication carries no patient after the backfill")

func init() {
	register(medicationsRepointUp, medicationsRepointDown)
}

// medicationsRepointUp is research D-13's six steps, in order.
func medicationsRepointUp(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Medication.Collection(), err)
	}

	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	facilities, err := app.FindCollectionByNameOrId(facilitiesCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", facilitiesCollection, err)
	}

	// Step 1: add patient, practitioner and pharmacy, all non-required for now
	// — patient cannot be Required until every row has one (step 5), and
	// practitioner/pharmacy never are (US5, FR-039).
	collection.Fields.Add(&core.RelationField{
		Name:          medicationFieldPatient,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name:          medicationFieldPractitioner,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  practitioners.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name:          medicationFieldPharmacy,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  facilities.Id,
	})

	if err = app.Save(collection); err != nil {
		return fmt.Errorf("adding %s's patient columns: %w", collection.Name, err)
	}

	// Step 2: every account without a self-record gets one, via app.Save so
	// validation and the patient audit hook both run (research D-14). One run
	// id threads through every insert, so every audit row this backfill writes
	// — and this function's own log lines, once it has any — share one handle.
	runCtx, edge := obs.NewEdge(context.Background(), "")
	runCtx = obs.WithCorrelationID(runCtx, edge.CorrelationID())

	if err = provisionSelfRecords(runCtx, app, patients); err != nil {
		return err
	}

	// Step 3: one statement, not one Save per medication (Complexity Tracking
	// CT-1) — a Save per row would fire OnRecordAfterUpdateSuccess once per
	// medication and write a "system updated medication" audit row nobody made.
	repoint := "UPDATE " + kind.Medication.Collection() + " SET " + medicationFieldPatient +
		" = (SELECT p.id FROM " + patientsCollection + " p WHERE p." + patientFieldOwner +
		" = " + kind.Medication.Collection() + "." + medicationFieldOwner +
		" AND p." + patientFieldIsSelfRecord + " = 1)"

	if _, execErr := app.DB().NewQuery(repoint).Execute(); execErr != nil {
		return fmt.Errorf("repointing %s to its patients: %w", collection.Name, execErr)
	}

	// Step 4: the post-condition. A failure here rolls back steps 1-3 and
	// every other pending migration in the batch, because
	// core/migrations_runner.go wraps the whole run in one transaction — there
	// is no partially repointed database to design a recovery for.
	unattributed := struct {
		Count int `db:"count"`
	}{}

	countQuery := "SELECT COUNT(*) AS count FROM " + kind.Medication.Collection() +
		" WHERE " + medicationFieldPatient + " = '' OR " + medicationFieldPatient + " IS NULL"

	if err = app.DB().NewQuery(countQuery).One(&unattributed); err != nil {
		return fmt.Errorf("counting unattributed %s rows: %w", collection.Name, err)
	}

	if unattributed.Count != 0 {
		return fmt.Errorf("%w: %d row(s)", ErrUnattributedMedication, unattributed.Count)
	}

	// Step 5: now that every row has one, patient becomes required and
	// cascading — the same anchor owner was (FR-026, SC-010).
	patient, err := relationField(collection, medicationFieldPatient)
	if err != nil {
		return err
	}

	patient.Required = true
	patient.CascadeDelete = true

	// Step 6: owner is retired; patient (through patients.owner) is the
	// authorization anchor now.
	collection.Fields.RemoveByName(medicationFieldOwner)

	name := kind.Medication.Collection()
	collection.RemoveIndex("idx_" + name + "_owner")
	collection.RemoveIndex("idx_" + name + "_owner_start")
	collection.RemoveIndex("idx_" + name + "_owner_name")
	collection.RemoveIndex("idx_" + name + "_owner_upd")
	collection.RemoveIndex("idx_" + name + "_owner_status")

	// The three indexes data-model §8 replaces the retired owner indexes with:
	// FR-022's default ordering, SC-004's per-patient counts, and FR-040's two
	// usage counts.
	collection.AddIndex("idx_"+name+"_patient", false, medicationFieldPatient, "")
	collection.AddIndex("idx_"+name+"_patient_start", false,
		medicationFieldPatient+", "+medicationFieldStartedOn+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_practitioner", false, medicationFieldPractitioner, "")
	collection.AddIndex("idx_"+name+"_pharmacy", false, medicationFieldPharmacy, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("repointing %s: %w", collection.Name, err)
	}

	return nil
}

// provisionSelfRecords is step 2: every users row without an is_self_record
// patient gets one, split per research D-10.
func provisionSelfRecords(ctx context.Context, app core.App, patients *core.Collection) error {
	var owned []string

	err := app.DB().NewQuery(
		"SELECT " + patientFieldOwner + " FROM " + patientsCollection + " WHERE " + patientFieldIsSelfRecord + " = 1",
	).Column(&owned)
	if err != nil {
		return fmt.Errorf("reading existing self-records: %w", err)
	}

	already := make(map[string]bool, len(owned))
	for _, ownerID := range owned {
		already[ownerID] = true
	}

	var users []struct {
		ID   string `db:"id"`
		Name string `db:"name"`
	}

	if err := app.DB().NewQuery(
		"SELECT id, " + usersFieldName + " FROM " + usersCollection,
	).All(&users); err != nil {
		return fmt.Errorf("reading %s: %w", usersCollection, err)
	}

	for _, user := range users {
		if already[user.ID] {
			continue
		}

		first, last := splitDisplayName(user.Name)

		record := core.NewRecord(patients)
		record.Set(patientFieldOwner, user.ID)
		record.Set(patientFieldFirstName, first)
		record.Set(patientFieldLastName, last)
		record.Set(patientFieldRelationshipToOwner, string(person.RelationshipSelf))
		record.Set(patientFieldIsSelfRecord, true)

		if err := app.SaveWithContext(ctx, record); err != nil {
			return fmt.Errorf("provisioning %s's self-record: %w", user.ID, err)
		}
	}

	return nil
}

// splitDisplayName is research D-10's rule: split on the last space, never
// invent a character. "Amara Okonkwo" -> ("Amara", "Okonkwo"). A single token
// is entirely the first name; there is no lawful last name to put in its
// place.
func splitDisplayName(name string) (first, last string) {
	trimmed := strings.TrimSpace(name)

	if i := strings.LastIndex(trimmed, " "); i > 0 {
		return trimmed[:i], trimmed[i+1:]
	}

	return trimmed, ""
}

// medicationsRepointDown is reversible in shape and lossy in substance
// (Principle IX, research D-13): owner is restored and backfilled from the
// patient it repoints through, but the self-records step 2 provisioned are
// not un-provisioned — the profile detail an account since entered on one
// would be destroyed by deleting it, and this migration does not run the
// patients collection's own down to get there.
func medicationsRepointDown(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Medication.Collection(), err)
	}

	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	// AddAt(1, ...) and not Add: owner sat right after id in medicationsUp's
	// own field order, and the up-down-up identity this migration's down must
	// preserve (Principle IX) compares that order along with everything else.
	collection.Fields.AddAt(1, &core.RelationField{
		Name:          medicationFieldOwner,
		Required:      false,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  users.Id,
	})

	if err = app.Save(collection); err != nil {
		return fmt.Errorf("re-adding %s.%s: %w", collection.Name, medicationFieldOwner, err)
	}

	backfill := "UPDATE " + kind.Medication.Collection() + " SET " + medicationFieldOwner +
		" = (SELECT p." + patientFieldOwner + " FROM " + patientsCollection + " p WHERE p.id = " +
		kind.Medication.Collection() + "." + medicationFieldPatient + ")"

	if _, execErr := app.DB().NewQuery(backfill).Execute(); execErr != nil {
		return fmt.Errorf("restoring %s.%s: %w", collection.Name, medicationFieldOwner, execErr)
	}

	owner, err := relationField(collection, medicationFieldOwner)
	if err != nil {
		return err
	}

	owner.Required = true

	name := kind.Medication.Collection()
	collection.RemoveIndex("idx_" + name + "_patient")
	collection.RemoveIndex("idx_" + name + "_patient_start")
	collection.RemoveIndex("idx_" + name + "_practitioner")
	collection.RemoveIndex("idx_" + name + "_pharmacy")

	collection.Fields.RemoveByName(medicationFieldPatient)
	collection.Fields.RemoveByName(medicationFieldPractitioner)
	collection.Fields.RemoveByName(medicationFieldPharmacy)

	collection.AddIndex("idx_"+name+"_owner", false, medicationFieldOwner, "")
	collection.AddIndex("idx_"+name+"_owner_start", false,
		medicationFieldOwner+", "+medicationFieldStartedOn+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_owner_name", false,
		medicationFieldOwner+", LOWER("+medicationFieldName+"), id DESC", "")
	collection.AddIndex("idx_"+name+"_owner_upd", false,
		medicationFieldOwner+", "+fieldUpdated+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_owner_status", false,
		medicationFieldOwner+", "+medicationFieldStatus, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("restoring %s: %w", collection.Name, err)
	}

	return nil
}
