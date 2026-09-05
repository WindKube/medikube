package clinical

import "medikube/internal/domain"

// CodeNotPositive is FR-039's refusal: a dose number that is not a positive
// whole number. It lives beside the rule that raises it, the same reasoning
// CodeEndBeforeStart in validate.go gives.
const CodeNotPositive = "not_positive"

const (
	maxVaccineName  = 200
	maxTradeName    = 200
	maxLotNumber    = 50
	maxManufacturer = 200
)

// Validate reports every offending field at once (FR-027), in data-model
// §4.8's column order.
func (i Immunization) Validate() error {
	var invalid domain.ValidationError

	if name := i.VaccineName; len(name) < 2 {
		invalid.Add("vaccine_name", domain.CodeRequired, "a vaccine name of at least two characters is required")
	} else {
		checkLength(&invalid, "vaccine_name", "the vaccine name", name, maxVaccineName)
	}

	checkLength(&invalid, "trade_name", "the trade name", i.TradeName, maxTradeName)

	if i.AdministeredOn.IsZero() {
		invalid.Add("administered_on", domain.CodeRequired, "the date it was given is required")
	}

	if fieldErr := NotFuture(Ref{Field: "administered_on", Value: i.AdministeredOn}, Today()); fieldErr != nil {
		invalid.Fields = append(invalid.Fields, *fieldErr)
	}

	// FR-039: a recorded dose must be a positive whole number. Absence (a
	// nil pointer) passes; zero and negative are both refused.
	if i.DoseNumber != nil && *i.DoseNumber < 1 {
		invalid.Add("dose_number", CodeNotPositive, "a dose number must be a positive whole number")
	}

	checkLength(&invalid, "lot_number", "the batch number", i.LotNumber, maxLotNumber)
	checkLength(&invalid, "manufacturer", "the manufacturer", i.Manufacturer, maxManufacturer)

	if i.Site != "" && !i.Site.Valid() {
		invalid.Add("site", domain.CodeInvalidValue, "not one of the sites MediKube accepts")
	}

	if i.Route != "" && !i.Route.Valid() {
		invalid.Add("route", domain.CodeInvalidValue, "not one of the routes MediKube accepts")
	}

	if fieldErr := Order(Ref{Field: "administered_on", Value: i.AdministeredOn}, Ref{Field: "expires_on", Value: i.ExpiresOn}); fieldErr != nil {
		invalid.Fields = append(invalid.Fields, *fieldErr)
	}

	return invalid.OrNil()
}
