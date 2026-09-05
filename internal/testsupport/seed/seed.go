package seed

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
)

// The patients medications are attributed to. AccountAPatientSelfID and
// AccountBPatientSelfID are Patients' own constants, redeclared here rather
// than imported so this file has one dependency graph and not two — the seed
// package's two files agree on ids by convention (fixtures_test.go is the
// gate), the same way the account and password constants above do.
const (
	accountAPatientSelfID   = "mkpatamara00001"
	accountAPatientParentID = "mkpatamara00003"
	accountBPatientSelfID   = "mkpatboris00001"
)

// Password is the one credential every demo account shares. It is published in
// quickstart.md, it clears FR-004's eight-character floor, and it is kept out of
// production by the command that writes it rather than by being a secret.
//
//nolint:gosec // a published demo credential is the point of this constant
const Password = "medikube-dev-password"

// The account identifiers. Fifteen lowercase alphanumerics, because that is
// what PocketBase's id column accepts, and readable rather than random, because
// a failing test that prints one should say which account it means.
const (
	AccountAID  = "mkacctamara0001"
	AccountBID  = "mkacctboris0001"
	AccountCID  = "mkacctchidi0001"
	SuperuserID = "mksuperadmin001"
)

const (
	AccountAEmail  = "amara@example.test"
	AccountBEmail  = "boris@example.test"
	AccountCEmail  = "chidi@example.test"
	SuperuserEmail = "admin@example.test"
)

// The four rows data-model §6 names individually, because a test that wants the
// partial-data case or the right-to-left case has to be able to ask for it.
const (
	// NameOnlyID carries a name and a state and nothing else: every optional
	// column is empty. It is the "partial data" edge case, and it is what a
	// renderer that assumes a dose or a date is present breaks on.
	NameOnlyID = "mkmedamara00003"
	// ScriptedNameID mixes right-to-left text with characters that look like
	// markup. A template that escapes nothing renders it as an element.
	ScriptedNameID = "mkmedamara00004"
	// SingleDayID starts and ends on the same day. FR-018 accepts it and a
	// range renderer that assumes start < end does not.
	SingleDayID = "mkmedamara00005"
	// FutureStartID starts in 2099. The entity has no clock, so this is
	// accepted; a validator that grew one would refuse the fixture.
	FutureStartID = "mkmedamara00006"
)

// Account is one demo account. Password is not a member: every account shares
// the published one, and an account with its own would be a credential to keep
// track of for no requirement's sake.
type Account struct {
	ID    string
	Email string
	Name  string
	Role  identity.Role
	// Verified is PocketBase's own column, and account C leaves it false so the
	// settings page's "not confirmed, send it again" state is a seeded case
	// rather than an untested branch (FR-075).
	Verified bool
}

// Accounts is the cast of data-model §6, in the order it lists them.
func Accounts() []Account {
	return []Account{
		{ID: AccountAID, Email: AccountAEmail, Name: "Amara Okonkwo", Role: identity.RoleUser, Verified: true},
		{ID: AccountBID, Email: AccountBEmail, Name: "Boris Novak", Role: identity.RoleUser, Verified: true},
		{ID: AccountCID, Email: AccountCEmail, Name: "Chidi Eze", Role: identity.RoleUser, Verified: false},
	}
}

// Medications is account A's twelve rows and account B's three, in id order.
// Account C has none and is absent from this list on purpose: an empty list is
// the state the smoke gate navigates to, and a fixture that gave every account
// a record would never exercise it (research D-39).
//
// The set spans all five states and all four kinds. It is built as
// clinical.Medication and not as a map of columns so that Apply can run the
// domain's own Validate over it: a fixture the application would refuse to
// accept is not a fixture, it is a trap for whoever writes the next test.
func Medications() []clinical.Medication {
	return []clinical.Medication{
		{
			ID: "mkmedamara00001", PatientID: accountAPatientSelfID,
			Name: "Lisinopril", Type: clinical.MedicationTypePrescription,
			Dosage: "10 mg", Frequency: "once daily", Route: clinical.MedicationRouteOral,
			Indication: "blood pressure", StartedOn: date(2024, 3, 1),
			Status: clinical.TherapyStatusActive,
		},
		{
			ID: "mkmedamara00002", PatientID: accountAPatientSelfID,
			Name: "Metformin", AlternativeName: "Glucophage",
			Type:   clinical.MedicationTypePrescription,
			Dosage: "500 mg", Frequency: "twice daily", Route: clinical.MedicationRouteOral,
			Indication: "type 2 diabetes", StartedOn: date(2023, 11, 14),
			Status: clinical.TherapyStatusActive,
			Notes:  "taken with food",
		},
		{
			ID: NameOnlyID, PatientID: accountAPatientSelfID,
			Name:   "Paracetamol",
			Status: clinical.TherapyStatusActive,
		},
		{
			ID: ScriptedNameID, PatientID: accountAPatientSelfID,
			// Arabic for "painkiller", then a tag, an ampersand and a quote.
			Name:   "مسكن <b>alpha</b> & \"strong\"",
			Type:   clinical.MedicationTypeOTC,
			Route:  clinical.MedicationRouteOral,
			Status: clinical.TherapyStatusActive,
			Notes:  "<script>alert(1)</script>",
		},
		{
			ID: SingleDayID, PatientID: accountAPatientSelfID,
			Name: "Dexamethasone", Type: clinical.MedicationTypePrescription,
			Dosage: "8 mg", Route: clinical.MedicationRouteOral,
			StartedOn: date(2025, 6, 2), EndedOn: date(2025, 6, 2),
			Status: clinical.TherapyStatusCompleted,
		},
		{
			ID: FutureStartID, PatientID: accountAPatientSelfID,
			Name: "Denosumab", Type: clinical.MedicationTypePrescription,
			Dosage: "60 mg", Frequency: "every six months",
			Route: clinical.MedicationRouteSubcutaneous, StartedOn: date(2099, 1, 1),
			Status: clinical.TherapyStatusActive,
		},
		{
			ID: "mkmedamara00007", PatientID: accountAPatientSelfID,
			Name: "Ibuprofen", Type: clinical.MedicationTypeOTC,
			Dosage: "400 mg", Frequency: "as needed", Route: clinical.MedicationRouteOral,
			Indication: "back pain", StartedOn: date(2025, 1, 9),
			Status: clinical.TherapyStatusOnHold,
		},
		{
			ID: "mkmedamara00008", PatientID: accountAPatientSelfID,
			Name: "Cholecalciferol", AlternativeName: "vitamin D3",
			Type:   clinical.MedicationTypeSupplement,
			Dosage: "2000 IU", Frequency: "once daily", Route: clinical.MedicationRouteOral,
			StartedOn: date(2024, 10, 1), EndedOn: date(2025, 4, 30),
			Status: clinical.TherapyStatusCompleted,
		},
		{
			ID: "mkmedamara00009", PatientID: accountAPatientSelfID,
			Name: "Valerian root", Type: clinical.MedicationTypeHerbal,
			Dosage: "300 mg", Frequency: "at night", Route: clinical.MedicationRouteOral,
			StartedOn: date(2024, 5, 20), EndedOn: date(2024, 8, 2),
			Status:      clinical.TherapyStatusStopped,
			SideEffects: "drowsiness the following morning",
		},
		{
			ID: "mkmedamara00010", PatientID: accountAPatientSelfID,
			Name: "Iron sucrose", Type: clinical.MedicationTypePrescription,
			Dosage: "200 mg", Route: clinical.MedicationRouteIntravenous,
			Indication: "anaemia", StartedOn: date(2025, 2, 3),
			Status: clinical.TherapyStatusCancelled,
			Notes:  "cancelled before the first infusion",
		},
		{
			ID: "mkmedamara00011", PatientID: accountAPatientSelfID,
			Name: "Hydrocortisone cream", Type: clinical.MedicationTypeOTC,
			Dosage: "1%", Frequency: "twice daily", Route: clinical.MedicationRouteTopical,
			Indication: "eczema", StartedOn: date(2025, 7, 18),
			Status: clinical.TherapyStatusActive,
		},
		{
			ID: "mkmedamara00012", PatientID: accountAPatientSelfID,
			Name: "Magnesium citrate", Type: clinical.MedicationTypeSupplement,
			Dosage: "150 mg", Frequency: "once daily", Route: clinical.MedicationRouteOral,
			StartedOn: date(2025, 5, 5),
			Status:    clinical.TherapyStatusActive,
		},
		{
			ID: "mkmedboris00001", PatientID: accountBPatientSelfID,
			Name: "Atorvastatin", Type: clinical.MedicationTypePrescription,
			Dosage: "20 mg", Frequency: "once daily", Route: clinical.MedicationRouteOral,
			Indication: "cholesterol", StartedOn: date(2024, 2, 12),
			Status: clinical.TherapyStatusActive,
		},
		{
			ID: "mkmedboris00002", PatientID: accountBPatientSelfID,
			Name: "Loratadine", Type: clinical.MedicationTypeOTC,
			Dosage: "10 mg", Frequency: "once daily", Route: clinical.MedicationRouteOral,
			StartedOn: date(2025, 3, 1), EndedOn: date(2025, 5, 31),
			Status: clinical.TherapyStatusCompleted,
		},
		{
			ID: "mkmedboris00003", PatientID: accountBPatientSelfID,
			Name: "Omega-3", Type: clinical.MedicationTypeSupplement,
			Dosage: "1 g", Frequency: "once daily", Route: clinical.MedicationRouteOral,
			StartedOn: date(2024, 9, 1), EndedOn: date(2025, 1, 15),
			Status: clinical.TherapyStatusStopped,
		},
	}
}

// date is the fixture's only way to write a calendar date. It panics rather
// than returning an error because every argument is a literal in this file: a
// failure is a typo in the table above, found the first time anything imports
// the package, and not a condition a caller can do anything about.
func date(year int, month time.Month, day int) domain.Date {
	value, err := domain.NewDate(year, month, day)
	if err != nil {
		panic(fmt.Sprintf("seed: %04d-%02d-%02d is not a calendar date", year, month, day))
	}

	return value
}

// The columns this package writes. The migrations own the spelling and hold it
// in unexported constants, so it is re-typed here — and every write is guarded
// by requireColumns below, because core.Record.Set stores an unknown key raw
// and DBExport then drops it. A renamed column would otherwise seed a row of
// empty strings and report success.
const usersCollection = "users"

const (
	columnName       = "name"
	columnRole       = "role"
	columnUnitSystem = "unit_system"
	columnLocale     = "locale"
	columnDateFormat = "date_format"
	columnTheme      = "theme"
)

const (
	columnOwner           = "owner"
	columnPatient         = "patient"
	columnPractitioner    = "practitioner"
	columnPharmacy        = "pharmacy"
	columnAlternativeName = "alternative_name"
	columnType            = "type"
	columnDosage          = "dosage"
	columnFrequency       = "frequency"
	columnRoute           = "route"
	columnIndication      = "indication"
	columnStartedOn       = "started_on"
	columnEndedOn         = "ended_on"
	columnStatus          = "status"
	columnSideEffects     = "side_effects"
	columnNotes           = "notes"
)

// Apply writes the whole fixture and is safe to run twice: every record is
// addressed by its own identifier and updated in place if it is already there,
// so a second run changes the updated timestamps and nothing else (FR-060).
//
// It runs in one transaction. A seed that half-applied would leave an account
// without its records, which is the one shape every ownership test would then
// pass against for the wrong reason.
func Apply(app core.App) error {
	return app.RunInTransaction(func(tx core.App) error {
		if err := applySuperuser(tx); err != nil {
			return err
		}

		if err := applyAccounts(tx); err != nil {
			return err
		}

		if err := applyFacilities(tx); err != nil {
			return err
		}

		if err := applyPractitioners(tx); err != nil {
			return err
		}

		if err := applyPatients(tx); err != nil {
			return err
		}

		// Medications are patient-owned (research D-13): the patients they name
		// have to exist first, or the relation this collection now requires
		// would refuse every row.
		if err := applyMedications(tx); err != nil {
			return err
		}

		return applyActivePatients(tx)
	})
}

func applySuperuser(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return fmt.Errorf("finding %s: %w", core.CollectionNameSuperusers, err)
	}

	record, err := findOrNew(app, collection, SuperuserID)
	if err != nil {
		return err
	}

	record.SetEmail(SuperuserEmail)
	record.SetPassword(Password)

	if err := app.Save(record); err != nil {
		return fmt.Errorf("seeding the superuser: %w", err)
	}

	return nil
}

func applyAccounts(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	if err := requireColumns(collection,
		columnName, columnRole, columnUnitSystem, columnLocale, columnDateFormat, columnTheme,
	); err != nil {
		return err
	}

	for _, account := range Accounts() {
		record, err := findOrNew(app, collection, account.ID)
		if err != nil {
			return err
		}

		record.SetEmail(account.Email)
		record.SetPassword(Password)
		record.SetVerified(account.Verified)

		record.Set(columnName, account.Name)
		record.Set(columnRole, string(account.Role))

		// The four preference columns take the domain's declared defaults
		// rather than a spread of values. They are presentation settings and a
		// fixture that varied them would be asserting the renderer's behaviour
		// through the seed instead of through a test that says so.
		record.Set(columnUnitSystem, string(identity.DefaultUnitSystem))
		record.Set(columnLocale, identity.DefaultLocale)
		record.Set(columnDateFormat, string(identity.DefaultDateFormat))
		record.Set(columnTheme, string(identity.DefaultTheme))

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", account.ID, err)
		}
	}

	return nil
}

func applyMedications(app core.App) error {
	name := kind.Medication.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnName, columnAlternativeName, columnType, columnDosage,
		columnFrequency, columnRoute, columnIndication, columnStartedOn, columnEndedOn,
		columnStatus, columnSideEffects, columnNotes,
	); err != nil {
		return err
	}

	for _, medication := range Medications() {
		// The fixture is held to the same rules a person's input is. A row the
		// application would refuse to accept is not demo data, it is a trap for
		// whoever writes the next test against it.
		if err := medication.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", medication.ID, err)
		}

		record, err := findOrNew(app, collection, medication.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, medication.PatientID)
		record.Set(columnName, medication.Name)
		record.Set(columnAlternativeName, medication.AlternativeName)
		record.Set(columnType, string(medication.Type))
		record.Set(columnDosage, medication.Dosage)
		record.Set(columnFrequency, medication.Frequency)
		record.Set(columnRoute, string(medication.Route))
		record.Set(columnIndication, medication.Indication)

		// UTC() and not the Date itself: it lands on midnight UTC, which is the
		// one instant a calendar date may become. Handing over the Date works
		// too, through two layers of PocketBase's own fallbacks, and works by
		// accident.
		record.Set(columnStartedOn, medication.StartedOn.UTC())
		record.Set(columnEndedOn, medication.EndedOn.UTC())

		record.Set(columnStatus, string(medication.Status))
		record.Set(columnSideEffects, medication.SideEffects)
		record.Set(columnNotes, medication.Notes)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", medication.ID, err)
		}

		if err := IndexRecord(app, kind.Medication, medication.ID, medication.PatientID,
			medication.Name, medication.Indication, medication.StartedOn); err != nil {
			return err
		}
	}

	return nil
}

// findOrNew is what makes a second run a no-op. A missing record is the normal
// case on a fresh instance, so only that one error is swallowed.
func findOrNew(app core.App, collection *core.Collection, id string) (*core.Record, error) {
	record, err := app.FindRecordById(collection, id)
	if err == nil {
		return record, nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("reading %s %s: %w", collection.Name, id, err)
	}

	record = core.NewRecord(collection)
	record.Id = id

	return record, nil
}

// requireColumns refuses to seed a collection whose schema has moved.
//
// core.Record.Set on an unknown key stores the value raw and DBExport then
// drops it, so a renamed column produces a saved record with an empty value and
// no error anywhere. This turns that into a failure at the seed rather than
// into a test that quietly asserts nothing.
func requireColumns(collection *core.Collection, names ...string) error {
	for _, name := range names {
		if collection.Fields.GetByName(name) == nil {
			return fmt.Errorf("%s has no %s column: the schema has moved and the seed would write nothing into it", collection.Name, name)
		}
	}

	return nil
}
