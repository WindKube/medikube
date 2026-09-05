package migrations

import (
	"errors"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// The four refusals of data-model §5, as sentinels so a caller can tell which
// one fired without reading the message. Assertions 1 and 2 are refusals to
// start anywhere; 3 and 4 are refusals in production and warnings in
// development, which is a decision for the boot sequence rather than for this
// package — AssertFatal and AssertStrict are split along exactly that line.
var (
	// ErrAPIRuleNotNil is constitution Principle V at the schema layer. A
	// non-nil rule is an open door: nil means superuser-only, and
	// types.Pointer("") means no constraint at all.
	ErrAPIRuleNotNil = errors.New("a non-system collection has a non-nil API rule")

	// ErrFileFieldUnprotected is constitution Principle VII. No file field
	// ships in this phase; the gate does, so phase 002's patients.photo lands
	// into an assertion that already exists rather than one added beside it.
	ErrFileFieldUnprotected = errors.New("a file field is not protected")

	// ErrRelationMismatch is data-model §4's cascade matrix. Both booleans are
	// one character from silently wrong and neither would fail to compile.
	ErrRelationMismatch = errors.New("a relation does not match the declared cascade matrix")

	// ErrSettingsMismatch is the pair of PocketBase settings MediKube owns:
	// batch off, because /api/batch is a second door into the record CRUD
	// handlers the lockdown closes, and log retention at one day rather than
	// zero, because zero means forever (research D-29).
	ErrSettingsMismatch = errors.New("a PocketBase setting does not match MediKube's")
)

// LogsMaxDays is what MediKube keeps of PocketBase's own request log. One day,
// never zero: PocketBase reads zero as "no retention limit", so the setting
// that looks like "keep nothing" is the one that keeps everything.
const LogsMaxDays = 1

// relationRule is one row of data-model §4's matrix. It is data rather than a
// sequence of assertions so that a later phase adding a relation adds a row and
// gets the gate for free.
type relationRule struct {
	collection    string
	field         string
	required      bool
	cascadeDelete bool
}

// Relations is the matrix, declared here and asserted at boot. Reading it: the
// medication cascade is what makes FR-014 and SC-012 true, the audit
// non-cascade is what keeps the account_delete row after its actor is gone, and
// phase 002's four additions are research D-06's: an account's own directory
// and patients are destroyed with it, but a patient losing its practitioner,
// its facility or its primary practitioner — and an account's active-patient
// pointer or a historical audit row losing the patient it concerned — never
// take the referencing row with them.
func Relations() []relationRule {
	return []relationRule{
		{collection: kind.Medication.Collection(), field: medicationFieldPatient, required: true, cascadeDelete: true},
		{collection: kind.Medication.Collection(), field: medicationFieldPractitioner, required: false, cascadeDelete: false},
		{collection: kind.Medication.Collection(), field: medicationFieldPharmacy, required: false, cascadeDelete: false},
		{collection: auditEventsCollection, field: auditFieldActor, required: false, cascadeDelete: false},
		{collection: facilitiesCollection, field: facilityFieldOwner, required: true, cascadeDelete: true},
		{collection: practitionersCollection, field: practitionerFieldOwner, required: true, cascadeDelete: true},
		{collection: practitionersCollection, field: practitionerFieldFacility, required: false, cascadeDelete: false},
		{collection: patientsCollection, field: patientFieldOwner, required: true, cascadeDelete: true},
		{collection: patientsCollection, field: patientFieldPrimaryPractitioner, required: false, cascadeDelete: false},
		// CascadeDelete is load-bearing and false: true here would mean
		// deleting a patient deletes the account that holds it (data-model §4).
		{collection: usersCollection, field: usersFieldActivePatient, required: false, cascadeDelete: false},
		{collection: auditEventsCollection, field: auditFieldPatient, required: false, cascadeDelete: false},
		{collection: kind.Immunization.Collection(), field: immunizationFieldPatient, required: true, cascadeDelete: true},
		{collection: kind.Immunization.Collection(), field: immunizationFieldPractitioner, required: false, cascadeDelete: false},
		{collection: kind.Immunization.Collection(), field: immunizationFieldFacility, required: false, cascadeDelete: false},
		{collection: kind.Injury.Collection(), field: injuryFieldPatient, required: true, cascadeDelete: true},
		{collection: kind.Injury.Collection(), field: injuryFieldPractitioner, required: false, cascadeDelete: false},
		{collection: kind.Equipment.Collection(), field: equipmentFieldPatient, required: true, cascadeDelete: true},
		{collection: kind.Equipment.Collection(), field: equipmentFieldSupplier, required: false, cascadeDelete: false},
		{collection: kind.Equipment.Collection(), field: equipmentFieldPractitioner, required: false, cascadeDelete: false},
		{collection: kind.Insurance.Collection(), field: insuranceFieldPatient, required: true, cascadeDelete: true},
		{collection: kind.Symptom.Collection(), field: symptomFieldPatient, required: true, cascadeDelete: true},
		{collection: kind.Vitals.Collection(), field: vitalsFieldPatient, required: true, cascadeDelete: true},
		{collection: kind.Vitals.Collection(), field: vitalsFieldPractitioner, required: false, cascadeDelete: false},
		{collection: kind.Encounter.Collection(), field: careFieldCondition, required: false, cascadeDelete: false},
		{collection: kind.Procedure.Collection(), field: careFieldCondition, required: false, cascadeDelete: false},
		{collection: kind.Treatment.Collection(), field: careFieldCondition, required: false, cascadeDelete: false},
	}
}

// AssertFatal is data-model §5's assertions 1 and 2: the ones that refuse to
// start whatever the environment. Both walk the whole schema rather than the
// collections this phase created, because the failure they exist to catch is a
// collection somebody added without reading either rule.
//
// Every offence is reported, not the first: an operator who fixes one rule and
// restarts to find a second is being told the truth one line at a time.
func AssertFatal(app core.App) error {
	return errors.Join(AssertAPIRules(app), AssertProtectedFiles(app))
}

// AssertStrict is assertions 3 and 4. They are separated from AssertFatal
// because a developer mid-migration should get a message rather than a process
// that will not boot; in production the boot sequence treats them exactly like
// AssertFatal.
func AssertStrict(app core.App) error {
	return errors.Join(AssertRelations(app), AssertSettings(app))
}

// AssertAPIRules requires all five API rules to be nil on every non-system
// collection. The qualifier is load-bearing: PocketBase's own _mfas, _otps,
// _externalAuths and _authOrigins carry non-nil list and view rules and are
// System, and rewriting those would be rewriting PocketBase.
func AssertAPIRules(app core.App) error {
	collections, err := app.FindAllCollections()
	if err != nil {
		return fmt.Errorf("enumerating collections: %w", err)
	}

	var offences []error

	for _, collection := range collections {
		if collection.System {
			continue
		}

		rules := map[string]*string{
			"listRule":   collection.ListRule,
			"viewRule":   collection.ViewRule,
			"createRule": collection.CreateRule,
			"updateRule": collection.UpdateRule,
			"deleteRule": collection.DeleteRule,
		}

		// Named in the order data-model §1 lists them rather than map order, so
		// the message is stable across runs.
		for _, name := range []string{"listRule", "viewRule", "createRule", "updateRule", "deleteRule"} {
			if rule := rules[name]; rule != nil {
				offences = append(offences, fmt.Errorf(
					"%w: %s.%s is %q, expected nil",
					ErrAPIRuleNotNil, collection.Name, name, *rule,
				))
			}
		}
	}

	return errors.Join(offences...)
}

// AssertProtectedFiles requires every FileField in the schema to be Protected.
// An unprotected file is served by PocketBase's own /api/files route to anyone
// holding the record id, with no rule applied — which is the disclosure
// Principle VII exists to prevent.
func AssertProtectedFiles(app core.App) error {
	collections, err := app.FindAllCollections()
	if err != nil {
		return fmt.Errorf("enumerating collections: %w", err)
	}

	var offences []error

	for _, collection := range collections {
		offences = append(offences, unprotectedFileFields(collection)...)
	}

	return errors.Join(offences...)
}

// unprotectedFileFields is separate so a test can hand it a synthetic
// collection: no file field exists in this phase, and an assertion that has
// never fired is an assertion nobody has tested.
func unprotectedFileFields(collection *core.Collection) []error {
	var offences []error

	for _, field := range collection.Fields {
		file, isFile := field.(*core.FileField)
		if !isFile || file.Protected {
			continue
		}

		offences = append(offences, fmt.Errorf(
			"%w: %s.%s",
			ErrFileFieldUnprotected, collection.Name, file.Name,
		))
	}

	return offences
}

// AssertRelations checks data-model §4's matrix field by field.
func AssertRelations(app core.App) error {
	var offences []error

	for _, rule := range Relations() {
		collection, err := app.FindCollectionByNameOrId(rule.collection)
		if err != nil {
			offences = append(offences, fmt.Errorf("%w: finding %s: %w", ErrRelationMismatch, rule.collection, err))
			continue
		}

		relation, err := relationField(collection, rule.field)
		if err != nil {
			offences = append(offences, fmt.Errorf("%w: %w", ErrRelationMismatch, err))
			continue
		}

		if relation.Required != rule.required {
			offences = append(offences, fmt.Errorf(
				"%w: %s.%s has Required %t, expected %t",
				ErrRelationMismatch, rule.collection, rule.field, relation.Required, rule.required,
			))
		}

		if relation.CascadeDelete != rule.cascadeDelete {
			offences = append(offences, fmt.Errorf(
				"%w: %s.%s has CascadeDelete %t, expected %t",
				ErrRelationMismatch, rule.collection, rule.field, relation.CascadeDelete, rule.cascadeDelete,
			))
		}
	}

	return errors.Join(offences...)
}

// AssertSettings re-reads the two settings MediKube writes at boot rather than
// assuming the write stuck. The test harness itself sets Logs.MaxDays to zero,
// which is precisely the value this refuses.
func AssertSettings(app core.App) error {
	var offences []error

	settings := app.Settings()

	if settings.Batch.Enabled {
		offences = append(offences, fmt.Errorf(
			"%w: Batch.Enabled is true; /api/batch reaches the record CRUD handlers the lockdown closes",
			ErrSettingsMismatch,
		))
	}

	if settings.Logs.MaxDays != LogsMaxDays {
		offences = append(offences, fmt.Errorf(
			"%w: Logs.MaxDays is %d, expected %d (zero means keep forever, not keep nothing)",
			ErrSettingsMismatch, settings.Logs.MaxDays, LogsMaxDays,
		))
	}

	return errors.Join(offences...)
}
