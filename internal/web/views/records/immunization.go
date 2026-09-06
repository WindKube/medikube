package records

import (
	viewtags "medikube/internal/web/views/tags"
	"strconv"

	"medikube/internal/domain/clinical"
)

// The names the domain attaches its refusals to and the wire DTO publishes,
// mirroring medication.go's FieldX constants.
const (
	ImmunizationFieldVaccineName    = "vaccine_name"
	ImmunizationFieldTradeName      = "trade_name"
	ImmunizationFieldAdministeredOn = "administered_on"
	ImmunizationFieldDoseNumber     = "dose_number"
	ImmunizationFieldLotNumber      = "lot_number"
	ImmunizationFieldManufacturer   = "manufacturer"
	ImmunizationFieldSite           = "site"
	ImmunizationFieldRoute          = "route"
	ImmunizationFieldExpiresOn      = "expires_on"
)

const (
	ImmunizationFieldCreated     = "created"
	ImmunizationFieldLastChanged = "last_changed"
)

const (
	ImmunizationFormLabelCreate = "Record a vaccination"
	ImmunizationFormLabelEdit   = "Edit vaccination"
)

// immunizationFields is data-model §4.8's column order.
var immunizationFields = []string{
	ImmunizationFieldVaccineName,
	ImmunizationFieldTradeName,
	ImmunizationFieldAdministeredOn,
	ImmunizationFieldDoseNumber,
	ImmunizationFieldLotNumber,
	ImmunizationFieldManufacturer,
	ImmunizationFieldSite,
	ImmunizationFieldRoute,
	ImmunizationFieldExpiresOn,
}

// ImmunizationFields is what the form offers, cloned so a caller that sorted
// it for one display could not reorder every form.
func ImmunizationFields() []string { return append([]string(nil), immunizationFields...) }

var immunizationFieldLabels = map[string]string{
	ImmunizationFieldVaccineName:    "Vaccine",
	ImmunizationFieldTradeName:      "Brand name",
	ImmunizationFieldAdministeredOn: "Given on",
	ImmunizationFieldDoseNumber:     "Dose number",
	ImmunizationFieldLotNumber:      "Batch number",
	ImmunizationFieldManufacturer:   "Manufacturer",
	ImmunizationFieldSite:           "Site",
	ImmunizationFieldRoute:          "How it was given",
	ImmunizationFieldExpiresOn:      "Expires",
	ImmunizationFieldCreated:        "Recorded",
	ImmunizationFieldLastChanged:    "Last changed",
}

// ImmunizationFieldLabel answers with the field's own name when there is no
// label, the same fallback FieldLabel uses.
func ImmunizationFieldLabel(field string) string {
	if label, known := immunizationFieldLabels[field]; known {
		return label
	}
	return field
}

var (
	immunizationSiteLabels = map[clinical.ImmunizationSite]string{
		clinical.ImmunizationSiteLeftArm:    "Left arm",
		clinical.ImmunizationSiteRightArm:   "Right arm",
		clinical.ImmunizationSiteLeftThigh:  "Left thigh",
		clinical.ImmunizationSiteRightThigh: "Right thigh",
		clinical.ImmunizationSiteOral:       "Oral",
		clinical.ImmunizationSiteNasal:      "Nasal",
		clinical.ImmunizationSiteOther:      "Other",
	}

	immunizationRouteLabels = map[clinical.ImmunizationRoute]string{
		clinical.ImmunizationRouteIntramuscular: "Injected into a muscle",
		clinical.ImmunizationRouteSubcutaneous:  "Injected under the skin",
		clinical.ImmunizationRouteIntradermal:   "Injected into the skin",
		clinical.ImmunizationRouteOral:          "By mouth",
		clinical.ImmunizationRouteIntranasal:    "Into the nose",
	}
)

func ImmunizationSiteLabel(value clinical.ImmunizationSite) string {
	return label(string(value), immunizationSiteLabels[value])
}

func ImmunizationRouteLabel(value clinical.ImmunizationRoute) string {
	return label(string(value), immunizationRouteLabels[value])
}

// ImmunizationSiteOptions and ImmunizationRouteOptions walk the domain's own
// published slices, mirroring MedicationTypeOptions.
func ImmunizationSiteOptions(selected clinical.ImmunizationSite) []Option {
	published := clinical.ImmunizationSites()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    ImmunizationSiteLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

func ImmunizationRouteOptions(selected clinical.ImmunizationRoute) []Option {
	published := clinical.ImmunizationRoutes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    ImmunizationRouteLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

// ImmunizationLinks are the URLs one immunization's views address, mirroring
// MedicationLinks.
type ImmunizationLinks struct {
	Detail string
	Edit   string
	Record string
}

// ImmunizationView is one immunization as its views render it.
type ImmunizationView struct {
	ID string

	PatientID string

	VaccineName    string
	TradeName      string
	AdministeredOn string
	DoseNumber     string
	LotNumber      string
	Manufacturer   string
	Site           string
	SiteValue      string
	Route          string
	RouteValue     string
	ExpiresOn      string

	Created     Timestamp
	LastChanged Timestamp

	Version string

	Links ImmunizationLinks
}

// NewImmunizationView is the whole of the entity-to-page mapping.
func NewImmunizationView(immunization clinical.Immunization, links ImmunizationLinks) ImmunizationView {
	doseNumber := ""
	if immunization.DoseNumber != nil {
		doseNumber = strconv.Itoa(*immunization.DoseNumber)
	}

	return ImmunizationView{
		ID:             immunization.ID,
		PatientID:      immunization.PatientID,
		VaccineName:    immunization.VaccineName,
		TradeName:      immunization.TradeName,
		AdministeredOn: immunization.AdministeredOn.String(),
		DoseNumber:     doseNumber,
		LotNumber:      immunization.LotNumber,
		Manufacturer:   immunization.Manufacturer,
		Site:           ImmunizationSiteLabel(immunization.Site),
		SiteValue:      string(immunization.Site),
		Route:          ImmunizationRouteLabel(immunization.Route),
		RouteValue:     string(immunization.Route),
		ExpiresOn:      immunization.ExpiresOn.String(),
		Created:        NewTimestamp(immunization.CreatedAt),
		LastChanged:    NewTimestamp(immunization.UpdatedAt),
		Version:        immunization.Version,
		Links:          links,
	}
}

// Entries is FR-024 made a property of the mapping, mirroring MedicationView.
func (v ImmunizationView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: ImmunizationFieldTradeName, Value: v.TradeName},
		{Field: ImmunizationFieldAdministeredOn, Value: v.AdministeredOn, Datetime: v.AdministeredOn},
		{Field: ImmunizationFieldDoseNumber, Value: v.DoseNumber},
		{Field: ImmunizationFieldLotNumber, Value: v.LotNumber},
		{Field: ImmunizationFieldManufacturer, Value: v.Manufacturer},
		{Field: ImmunizationFieldSite, Value: v.Site},
		{Field: ImmunizationFieldRoute, Value: v.Route},
		{Field: ImmunizationFieldExpiresOn, Value: v.ExpiresOn, Datetime: v.ExpiresOn},
		{Field: ImmunizationFieldCreated, Value: v.Created.Human, Datetime: v.Created.Machine},
		{Field: ImmunizationFieldLastChanged, Value: v.LastChanged.Human, Datetime: v.LastChanged.Machine},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}
		entry.Label = ImmunizationFieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

// ImmunizationListProps is one page of the list.
type ImmunizationListProps struct {
	Immunizations []ImmunizationView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

// ImmunizationDetailProps is one immunization and its delete confirmation.
type ImmunizationDetailProps struct {
	Immunization ImmunizationView
}

// ImmunizationFormProps is the create form and the edit form.
type ImmunizationFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	Immunization ImmunizationView
	Errors       FieldErrors

	Notice string

	Tags viewtags.FieldProps
}

func (p ImmunizationFormProps) Label() string {
	if p.New {
		return ImmunizationFormLabelCreate
	}
	return ImmunizationFormLabelEdit
}

func (v ImmunizationView) SiteOptions() []Option {
	return ImmunizationSiteOptions(clinical.ImmunizationSite(v.SiteValue))
}

func (v ImmunizationView) RouteOptions() []Option {
	return ImmunizationRouteOptions(clinical.ImmunizationRoute(v.RouteValue))
}

// Value is what a form control holds for one field.
func (v ImmunizationView) Value(field string) string {
	switch field {
	case ImmunizationFieldVaccineName:
		return v.VaccineName
	case ImmunizationFieldTradeName:
		return v.TradeName
	case ImmunizationFieldAdministeredOn:
		return v.AdministeredOn
	case ImmunizationFieldDoseNumber:
		return v.DoseNumber
	case ImmunizationFieldLotNumber:
		return v.LotNumber
	case ImmunizationFieldManufacturer:
		return v.Manufacturer
	case ImmunizationFieldSite:
		return v.SiteValue
	case ImmunizationFieldRoute:
		return v.RouteValue
	case ImmunizationFieldExpiresOn:
		return v.ExpiresOn
	default:
		return ""
	}
}

func (p ImmunizationFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}
	return "Save changes"
}

func immunizationDeleteExpression(immunization ImmunizationView) string {
	return "@delete(" + jsLiteral(immunization.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
