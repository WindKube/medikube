package emergencycontact

import "medikube/internal/domain/clinical"

// Patch is a change to an emergency contact: every field optional.
type Patch struct {
	Name         *string
	Relationship *clinical.ContactRelationship
	Phone        *string
	PhoneAlt     *string
	Email        *string
	Address      *string
	IsPrimary    *bool
	IsActive     *bool
	Notes        *string
}

func (p Patch) applyTo(entity clinical.EmergencyContact) clinical.EmergencyContact {
	assign(&entity.Name, p.Name)
	assign(&entity.Relationship, p.Relationship)
	assign(&entity.Phone, p.Phone)
	assign(&entity.PhoneAlt, p.PhoneAlt)
	assign(&entity.Email, p.Email)
	assign(&entity.Address, p.Address)
	assign(&entity.IsPrimary, p.IsPrimary)
	assign(&entity.IsActive, p.IsActive)
	assign(&entity.Notes, p.Notes)

	return entity
}

func assign[T any](field *T, supplied *T) {
	if supplied != nil {
		*field = *supplied
	}
}
