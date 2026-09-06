package clinical

import (
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Immunization is one vaccination as the person recorded it (FR-038). Like
// Medication, it never crosses the wire unrendered — a DTO always mediates —
// and MarshalZerologObject below is an allowlist of two identifiers, because
// the vaccine name and the practitioner are patient data.
//
// There is deliberately no CatalogVaccineID: a standardised vaccine library is
// explicitly deferred (plan.md Deviations), and adding one later is one
// reversible migration rather than a field this phase has to carry unused.
type Immunization struct {
	ID string

	PatientID string

	PractitionerID string
	FacilityID     string

	VaccineName    string
	TradeName      string
	AdministeredOn domain.Date
	// DoseNumber is nil when not recorded. A pointer and not a bare int
	// because a recorded dose of exactly zero is refused rather than treated
	// as absent (FR-039, spec US4 scenario 2): the two states are
	// distinguishable only if the zero value does not also mean "absent".
	DoseNumber   *int
	LotNumber    string
	Manufacturer string
	Site         ImmunizationSite
	Route        ImmunizationRoute
	ExpiresOn    domain.Date

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

func (i Immunization) MarshalZerologObject(e *zerolog.Event) {
	e.Str("immunization_id", i.ID).Str("patient_id", i.PatientID)
}
