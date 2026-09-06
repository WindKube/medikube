package records

import (
	viewtags "medikube/internal/web/views/tags"
	"strconv"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/web/views/components"
)

// The names the domain attaches its refusals to (internal/domain/clinical
// validate.go) and the names the wire DTO publishes. One spelling reaches the
// control's `name`, the aria-describedby pair and the refusal lookup, so a form
// control cannot be named one thing and refused under another.
const (
	FieldName            = "name"
	FieldAlternativeName = "alternative_name"
	FieldType            = "type"
	FieldDosage          = "dosage"
	FieldFrequency       = "frequency"
	FieldRoute           = "route"
	FieldIndication      = "indication"
	FieldStartedOn       = "started_on"
	FieldEndedOn         = "ended_on"
	FieldStatus          = "status"
	FieldSideEffects     = "side_effects"
	FieldNotes           = "notes"
)

// The two the person never fills in. They are shown and never offered, which is
// why they are not in MedicationFields.
const (
	FieldCreated     = "created"
	FieldLastChanged = "last_changed"
)

// The two spellings of the form's accessible name. They are what a Playwright
// getByRole('form', {name}) resolves against, so they live beside the component
// that renders them.
const (
	FormLabelCreate = "Record a medication"
	FormLabelEdit   = "Edit medication"
)

// medicationFields is data-model §2's column order, which is the order
// validate.go checks them in and the order the form offers them — so a person
// reads the same sequence in the form, in the refusals and on the detail.
var medicationFields = []string{
	FieldName,
	FieldAlternativeName,
	FieldType,
	FieldDosage,
	FieldFrequency,
	FieldRoute,
	FieldIndication,
	FieldStartedOn,
	FieldEndedOn,
	FieldStatus,
	FieldSideEffects,
	FieldNotes,
}

// MedicationFields is what the form offers, cloned so a caller that sorted it
// for one display could not reorder every form.
func MedicationFields() []string { return append([]string(nil), medicationFields...) }

// The words on the page. They are deliberately not the column names: FR-016
// publishes the vocabulary in plain language ("currently taking", not
// "active"), and a person recording their own medicine reads "How often" rather
// than "frequency".
var fieldLabels = map[string]string{
	FieldName:            "Name",
	FieldAlternativeName: "Also known as",
	FieldType:            "Kind",
	FieldDosage:          "Dose",
	FieldFrequency:       "How often",
	FieldRoute:           "How it is taken",
	FieldIndication:      "Reason for taking it",
	FieldStartedOn:       "Started",
	FieldEndedOn:         "Ended",
	FieldStatus:          "State",
	FieldSideEffects:     "Side effects",
	FieldNotes:           "Notes",
	FieldCreated:         "Recorded",
	FieldLastChanged:     "Last changed",
}

// FieldLabel answers with the field's own name when there is no label, so a
// field somebody added without one reads wrong on the page and fails the label
// test, rather than rendering as nothing and reading as absent.
func FieldLabel(field string) string {
	if label, known := fieldLabels[field]; known {
		return label
	}
	return field
}

// The display spellings of the three published vocabularies. FR-016 states them
// in these words; the stored values stay the machine spellings that the select
// field, the filter and the OpenAPI enum carry.
var (
	medicationTypeLabels = map[clinical.MedicationType]string{
		clinical.MedicationTypePrescription: "Prescription",
		clinical.MedicationTypeOTC:          "Over-the-counter",
		clinical.MedicationTypeSupplement:   "Supplement",
		clinical.MedicationTypeHerbal:       "Herbal",
	}

	medicationRouteLabels = map[clinical.MedicationRoute]string{
		clinical.MedicationRouteOral:          "By mouth",
		clinical.MedicationRouteSublingual:    "Under the tongue",
		clinical.MedicationRouteTopical:       "On the skin",
		clinical.MedicationRouteTransdermal:   "Through a skin patch",
		clinical.MedicationRouteInhalation:    "Inhaled",
		clinical.MedicationRouteNasal:         "Into the nose",
		clinical.MedicationRouteOphthalmic:    "Into the eye",
		clinical.MedicationRouteOtic:          "Into the ear",
		clinical.MedicationRouteRectal:        "Rectally",
		clinical.MedicationRouteVaginal:       "Vaginally",
		clinical.MedicationRouteIntramuscular: "Injected into a muscle",
		clinical.MedicationRouteSubcutaneous:  "Injected under the skin",
		clinical.MedicationRouteIntravenous:   "Injected into a vein",
		clinical.MedicationRouteOther:         "Some other way",
	}

	therapyStatusLabels = map[clinical.TherapyStatus]string{
		clinical.TherapyStatusActive:    "Currently taking",
		clinical.TherapyStatusOnHold:    "Paused",
		clinical.TherapyStatusCompleted: "Finished",
		clinical.TherapyStatusStopped:   "Stopped taking",
		clinical.TherapyStatusCancelled: "Never started",
	}
)

// MedicationTypeLabel answers with the stored spelling for a value it does not
// know, and with nothing at all for the absent value — which is what an
// optional select carries and what FR-024 omits rather than labels.
// MedicationRouteLabel and TherapyStatusLabel below do the same.
func MedicationTypeLabel(value clinical.MedicationType) string {
	return label(string(value), medicationTypeLabels[value])
}

func MedicationRouteLabel(value clinical.MedicationRoute) string {
	return label(string(value), medicationRouteLabels[value])
}

func TherapyStatusLabel(value clinical.TherapyStatus) string {
	return label(string(value), therapyStatusLabels[value])
}

func label(value, known string) string {
	switch {
	case value == "":
		return ""
	case known == "":
		return value
	default:
		return known
	}
}

// Option is one entry of a select. Value is what is submitted and stored;
// Label is what is read.
type Option struct {
	Value    string
	Label    string
	Selected bool
}

// MedicationTypeOptions and the two below it walk the domain's own published
// slices, so the form cannot offer a value Valid() refuses or withhold one it
// accepts. The empty "not recorded" entry is the template's, not theirs:
// absence is a state the domain distinguishes from an unpublished value.
func MedicationTypeOptions(selected clinical.MedicationType) []Option {
	published := clinical.MedicationTypes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    MedicationTypeLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

func MedicationRouteOptions(selected clinical.MedicationRoute) []Option {
	published := clinical.MedicationRoutes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    MedicationRouteLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

func TherapyStatusOptions(selected clinical.TherapyStatus) []Option {
	published := clinical.TherapyStatuses()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{
			Value:    string(value),
			Label:    TherapyStatusLabel(value),
			Selected: value == selected,
		})
	}

	return options
}

// Timestamp is an instant in both forms a page needs: the one a browser parses
// and the one a person reads. A calendar date is not one of these — FR-019
// keeps those as domain.Date and they render as themselves.
type Timestamp struct {
	// Machine is RFC3339 UTC, for the <time datetime> attribute.
	Machine string
	Human   string
}

const humanInstant = "2 Jan 2006, 15:04 MST"

// NewTimestamp renders in UTC. Phase 002 gives an account a display zone; until
// there is a preference to read, a stated zone is honest and a guessed one is
// the bug domain.Date exists to prevent, one layer up.
func NewTimestamp(at time.Time) Timestamp {
	if at.IsZero() {
		return Timestamp{}
	}

	utc := at.UTC()

	return Timestamp{Machine: utc.Format(time.RFC3339), Human: utc.Format(humanInstant)}
}

func (t Timestamp) IsZero() bool { return t == Timestamp{} }

// MedicationLinks are the URLs one medication's views address. They are handed
// in rather than built here: a view that spelled a kind's path segment would be
// the fourth spelling of it, and the one nothing checks (research D-05).
type MedicationLinks struct {
	// Detail and Edit are pages; Record is the API resource the form patches
	// and the confirmation deletes.
	Detail string
	Edit   string
	Record string
}

// MedicationView is one medication as its views render it: display strings and
// nothing that still needs a decision made about it.
//
// It is not the wire DTO and not the entity. The entity carries domain.Date and
// clinical.TherapyStatus, which a template would have to format — and a
// template that formats is a template with a branch nothing tests.
type MedicationView struct {
	ID string

	// PatientID is FR-025's fixed target: rendered into the create form's
	// hidden field at page-render time and never re-read after (US3-6).
	PatientID string

	Name            string
	AlternativeName string
	Type            string
	TypeValue       string
	Dosage          string
	Frequency       string
	Route           string
	RouteValue      string
	Indication      string
	StartedOn       string
	EndedOn         string
	Status          string
	StatusValue     string
	SideEffects     string
	Notes           string

	Created     Timestamp
	LastChanged Timestamp

	// Version is the ETag the edit form and the delete confirmation send back
	// as If-Match (FR-026).
	Version string

	Links MedicationLinks
}

// NewMedicationView is the whole of the entity-to-page mapping. Every branch a
// template would otherwise carry lives here, where a test can reach it without
// rendering anything.
func NewMedicationView(medication clinical.Medication, links MedicationLinks) MedicationView {
	return MedicationView{
		ID:              medication.ID,
		PatientID:       medication.PatientID,
		Name:            medication.Name,
		AlternativeName: medication.AlternativeName,
		Type:            MedicationTypeLabel(medication.Type),
		TypeValue:       string(medication.Type),
		Dosage:          medication.Dosage,
		Frequency:       medication.Frequency,
		Route:           MedicationRouteLabel(medication.Route),
		RouteValue:      string(medication.Route),
		Indication:      medication.Indication,
		StartedOn:       medication.StartedOn.String(),
		EndedOn:         medication.EndedOn.String(),
		Status:          TherapyStatusLabel(medication.Status),
		StatusValue:     string(medication.Status),
		SideEffects:     medication.SideEffects,
		Notes:           medication.Notes,
		Created:         NewTimestamp(medication.CreatedAt),
		LastChanged:     NewTimestamp(medication.UpdatedAt),
		Version:         medication.Version,
		Links:           links,
	}
}

// DetailEntry is one labelled value of the detail view.
type DetailEntry struct {
	Field string
	Label string
	Value string

	// Datetime is the machine form, for a <time> element. Empty for the values
	// that are not instants or dates.
	Datetime string

	// Multiline says the value was typed as prose and keeps its line breaks.
	Multiline bool
}

// Entries is FR-024 made a property of the mapping rather than of the template:
// a value that was never recorded produces no entry, so there is nothing for a
// template to render an empty placeholder beside.
//
// The name is absent on purpose — it is the detail's heading, and repeating it
// as a labelled row says the same thing twice.
func (m MedicationView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldAlternativeName, Value: m.AlternativeName},
		{Field: FieldType, Value: m.Type},
		{Field: FieldDosage, Value: m.Dosage},
		{Field: FieldFrequency, Value: m.Frequency},
		{Field: FieldRoute, Value: m.Route},
		{Field: FieldIndication, Value: m.Indication},
		{Field: FieldStartedOn, Value: m.StartedOn, Datetime: m.StartedOn},
		{Field: FieldEndedOn, Value: m.EndedOn, Datetime: m.EndedOn},
		{Field: FieldStatus, Value: m.Status},
		{Field: FieldSideEffects, Value: m.SideEffects, Multiline: true},
		{Field: FieldNotes, Value: m.Notes, Multiline: true},
		{Field: FieldCreated, Value: m.Created.Human, Datetime: m.Created.Machine},
		{Field: FieldLastChanged, Value: m.LastChanged.Human, Datetime: m.LastChanged.Machine},
	}

	entries := make([]DetailEntry, 0, len(candidates))

	for _, entry := range candidates {
		if entry.Value == "" {
			continue
		}
		entry.Label = FieldLabel(entry.Field)
		entries = append(entries, entry)
	}

	return entries
}

// FieldErrors and NewFieldErrors are internal/web/views/components' — one type
// for every form in the application, aliased here so this package's callers and
// the two form packages beside it cannot end up with two answers to "which
// refusals does this control carry".
type FieldErrors = components.FieldErrors

func NewFieldErrors(invalid *domain.ValidationError) FieldErrors {
	return components.NewFieldErrors(invalid)
}

// MedicationListProps is one page of the list. The ids are the component's, not
// the caller's: the element the stream patches and the selector it patches with
// have to come from the same call.
type MedicationListProps struct {
	Medications []MedicationView

	CreateHref   string
	PreviousHref string
	NextHref     string
}

// MedicationDetailProps is one medication and the confirmation that guards its
// deletion.
type MedicationDetailProps struct {
	Medication     MedicationView
	Links          RemovableLinksProps
	ReferenceCount int
}

// MedicationFormProps is the create form and the edit form, which are the same
// component: FR-015 and FR-025 offer the same twelve fields either way, and two
// components would be two places for one of them to go missing.
type MedicationFormProps struct {
	// FormID is what ids.Field and ids.FieldError hang off, so two forms on one
	// page cannot give their controls the same id.
	FormID string
	New    bool

	// OnSubmit is the Datastar expression the submission runs. The caller
	// builds it because the URL is the caller's.
	OnSubmit   string
	CancelHref string

	Medication MedicationView
	Errors     FieldErrors

	// Notice is set when the form was re-rendered from the server's current
	// values after a stale If-Match.
	Notice string

	Tags viewtags.FieldProps
}

// Label is the form's accessible name, and it is the component's rather than
// the caller's because it is a Playwright selector.
func (p MedicationFormProps) Label() string {
	if p.New {
		return FormLabelCreate
	}
	return FormLabelEdit
}

// TypeOptions, RouteOptions and StatusOptions are the view's own selects, so
// the form does not have to convert a display string back into a domain type
// to ask what to offer.

func (m MedicationView) TypeOptions() []Option {
	return MedicationTypeOptions(clinical.MedicationType(m.TypeValue))
}

func (m MedicationView) RouteOptions() []Option {
	return MedicationRouteOptions(clinical.MedicationRoute(m.RouteValue))
}

func (m MedicationView) StatusOptions() []Option {
	return TherapyStatusOptions(clinical.TherapyStatus(m.StatusValue))
}

// Value is what a form control holds for one field, so the twelve controls are
// one component called twelve times rather than twelve components.
func (m MedicationView) Value(field string) string {
	switch field {
	case FieldName:
		return m.Name
	case FieldAlternativeName:
		return m.AlternativeName
	case FieldType:
		return m.TypeValue
	case FieldDosage:
		return m.Dosage
	case FieldFrequency:
		return m.Frequency
	case FieldRoute:
		return m.RouteValue
	case FieldIndication:
		return m.Indication
	case FieldStartedOn:
		return m.StartedOn
	case FieldEndedOn:
		return m.EndedOn
	case FieldStatus:
		return m.StatusValue
	case FieldSideEffects:
		return m.SideEffects
	case FieldNotes:
		return m.Notes
	default:
		return ""
	}
}

// SubmitLabel names the act rather than the mechanism: "Save" tells a person
// nothing about whether they are creating or changing something.
func (p MedicationFormProps) SubmitLabel() string {
	if p.New {
		return "Record it"
	}
	return "Save changes"
}

// jsLiteral quotes a server-side value for a Datastar expression.
//
// Nothing that reaches here is caller-supplied today — a URL is built from the
// kind table and a version is derived from a timestamp — and that is exactly
// the sort of fact that stops being true quietly. strconv.Quote escapes the
// quote, the backslash and every control character, which is the whole of what
// could otherwise end the string and start an expression; templ escapes the
// result again on its way into the attribute.
func jsLiteral(value string) string { return strconv.Quote(value) }

// deleteExpression is the confirmation's action. The If-Match comes from the
// $_etag signal the detail declares rather than from a second read, because a
// version fetched again is a version that can already be stale (FR-026).
func deleteExpression(medication MedicationView) string {
	return "@delete(" + jsLiteral(medication.Links.Record) + ", {headers: {'If-Match': $_etag}})"
}
