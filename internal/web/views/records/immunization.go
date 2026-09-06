package records

import (
	"strconv"

	"medikube/internal/domain/clinical"
	viewtags "medikube/internal/web/views/tags"
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

// The values below are message ids (D-06), resolved at render time.
const (
	ImmunizationFormLabelCreate = "page.immunization.record"
	ImmunizationFormLabelEdit   = "page.immunization.edit"
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

// The values below are message ids (D-06), resolved at render time.
var immunizationFieldLabels = map[string]string{
	ImmunizationFieldVaccineName:    "field.immunization.vaccine_name",
	ImmunizationFieldTradeName:      "field.immunization.trade_name",
	ImmunizationFieldAdministeredOn: "field.immunization.administered_on",
	ImmunizationFieldDoseNumber:     "field.immunization.dose_number",
	ImmunizationFieldLotNumber:      "field.immunization.lot_number",
	ImmunizationFieldManufacturer:   "field.manufacturer",
	ImmunizationFieldSite:           "field.immunization.site",
	ImmunizationFieldRoute:          "field.immunization.route",
	ImmunizationFieldExpiresOn:      "field.immunization.expires_on",
	ImmunizationFieldCreated:        "field.recorded",
	ImmunizationFieldLastChanged:    "field.last_changed",
}

// ImmunizationFieldLabel answers with the field's own name when there is no
// label, the same fallback FieldLabel uses.
func ImmunizationFieldLabel(field string) string {
	if label, known := immunizationFieldLabels[field]; known {
		return label
	}
	return field
}

// The values below are message ids (D-06), resolved at render time.
var (
	immunizationSiteLabels = map[clinical.ImmunizationSite]string{
		clinical.ImmunizationSiteLeftArm:    "enum.immunization_site.left_arm",
		clinical.ImmunizationSiteRightArm:   "enum.immunization_site.right_arm",
		clinical.ImmunizationSiteLeftThigh:  "enum.immunization_site.left_thigh",
		clinical.ImmunizationSiteRightThigh: "enum.immunization_site.right_thigh",
		clinical.ImmunizationSiteOral:       "enum.immunization_site.oral",
		clinical.ImmunizationSiteNasal:      "enum.immunization_site.nasal",
		clinical.ImmunizationSiteOther:      "enum.immunization_site.other",
	}

	immunizationRouteLabels = map[clinical.ImmunizationRoute]string{
		clinical.ImmunizationRouteIntramuscular: "enum.immunization_route.intramuscular",
		clinical.ImmunizationRouteSubcutaneous:  "enum.immunization_route.subcutaneous",
		clinical.ImmunizationRouteIntradermal:   "enum.immunization_route.intradermal",
		clinical.ImmunizationRouteOral:          "enum.immunization_route.oral",
		clinical.ImmunizationRouteIntranasal:    "enum.immunization_route.intranasal",
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
		{Field: ImmunizationFieldSite, Value: v.Site, Translate: true},
		{Field: ImmunizationFieldRoute, Value: v.Route, Translate: true},
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
		return "action.record_it"
	}
	return "action.save_changes"
}

func immunizationDeleteExpression(immunization ImmunizationView) string {
	return "@delete(" + jsLiteral(immunization.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
