package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

const usersFieldActivePatient = "active_patient"

func init() {
	register(usersActivePatientUp, usersActivePatientDown)
}

func usersActivePatientUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	// FR-013: a UI convenience, never consulted for authorization (FR-015,
	// research D-08). CascadeDelete is load-bearing and false: true here would
	// mean deleting a patient deletes the account that holds it. Not required,
	// so a patient's deletion can unset it (research D-07) rather than fail the
	// deletion outright.
	users.Fields.Add(&core.RelationField{
		Name:          usersFieldActivePatient,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})

	if err := app.Save(users); err != nil {
		return fmt.Errorf("saving %s: %w", usersCollection, err)
	}

	return nil
}

func usersActivePatientDown(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	users.Fields.RemoveByName(usersFieldActivePatient)

	if err := app.Save(users); err != nil {
		return fmt.Errorf("saving %s: %w", usersCollection, err)
	}

	return nil
}
