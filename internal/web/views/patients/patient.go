package patients

import (
	"strconv"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/identity"
	"medikube/internal/domain/person"
	"medikube/internal/web/views/components"
)

// The names the domain attaches its refusals to and the ones the form offers
// them under (internal/domain/person/validate.go) — one spelling for a form
// control, its aria-describedby pair and the refusal lookup.
const (
	FieldFirstName    = "first_name"
	FieldLastName     = "last_name"
	FieldBirthDate    = "birth_date"
	FieldSex          = "sex"
	FieldBloodType    = "blood_type"
	FieldHeightCM     = "height_cm"
	FieldWeightKG     = "weight_kg"
	FieldAddress      = "address"
	FieldRelationship = "relationship_to_owner"
)

const (
	FormLabelCreate = "Add a person"
	FormLabelEdit   = "Edit person"
)

var patientFields = []string{
	FieldFirstName, FieldLastName, FieldBirthDate, FieldSex, FieldBloodType,
	FieldHeightCM, FieldWeightKG, FieldAddress, FieldRelationship,
}

// PatientFields is what the form offers, cloned so a caller could not reorder
// every form by sorting the result of one.
func PatientFields() []string { return append([]string(nil), patientFields...) }

var fieldLabels = map[string]string{
	FieldFirstName:    "First name",
	FieldLastName:     "Last name",
	FieldBirthDate:    "Date of birth",
	FieldSex:          "Sex",
	FieldBloodType:    "Blood type",
	FieldHeightCM:     "Height",
	FieldWeightKG:     "Weight",
	FieldAddress:      "Address",
	FieldRelationship: "Relationship",
}

func FieldLabel(field string) string {
	if label, known := fieldLabels[field]; known {
		return label
	}

	return field
}

var relationshipLabels = map[person.RelationshipToOwner]string{
	person.RelationshipSelf:    "You",
	person.RelationshipSpouse:  "Spouse",
	person.RelationshipPartner: "Partner",
	person.RelationshipParent:  "Parent",
	person.RelationshipChild:   "Child",
	person.RelationshipSibling: "Sibling",
	person.RelationshipWard:    "Ward",
	person.RelationshipOther:   "Other",
}

// RelationshipLabel renders the plain-language relationship FR-001 asks for,
// answering the empty string for an unrecorded relationship rather than the
// machine spelling.
func RelationshipLabel(value person.RelationshipToOwner) string {
	if value == "" {
		return ""
	}

	if label, known := relationshipLabels[value]; known {
		return label
	}

	return string(value)
}

var sexLabels = map[person.Sex]string{
	person.SexFemale:      "Female",
	person.SexMale:        "Male",
	person.SexIntersex:    "Intersex",
	person.SexUnspecified: "Unspecified",
}

func SexLabel(value person.Sex) string {
	if value == "" {
		return ""
	}

	if label, known := sexLabels[value]; known {
		return label
	}

	return string(value)
}

var bloodTypeLabels = map[person.BloodType]string{
	person.BloodTypeAPos:  "A+",
	person.BloodTypeANeg:  "A-",
	person.BloodTypeBPos:  "B+",
	person.BloodTypeBNeg:  "B-",
	person.BloodTypeABPos: "AB+",
	person.BloodTypeABNeg: "AB-",
	person.BloodTypeOPos:  "O+",
	person.BloodTypeONeg:  "O-",
}

func BloodTypeLabel(value person.BloodType) string {
	if value == "" {
		return ""
	}

	if label, known := bloodTypeLabels[value]; known {
		return label
	}

	return string(value)
}

// Option is one entry of a select.
type Option struct {
	Value    string
	Label    string
	Selected bool
}

func SexOptions(selected person.Sex) []Option {
	published := person.Sexes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: SexLabel(value), Selected: value == selected})
	}

	return options
}

func BloodTypeOptions(selected person.BloodType) []Option {
	published := person.BloodTypes()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: BloodTypeLabel(value), Selected: value == selected})
	}

	return options
}

func RelationshipOptions(selected person.RelationshipToOwner) []Option {
	published := person.RelationshipsToOwner()
	options := make([]Option, 0, len(published))

	for _, value := range published {
		options = append(options, Option{Value: string(value), Label: RelationshipLabel(value), Selected: value == selected})
	}

	return options
}

// PatientView is one patient as its views render it: display strings, never a
// domain.Date or a person.Sex a template would have to format.
type PatientView struct {
	ID string

	FirstName    string
	LastName     string
	BirthDate    string
	Age          string
	Sex          string
	SexValue     string
	BloodType    string
	BloodTypeVal string
	HeightCM     string
	WeightKG     string
	Address      string
	Relationship string
	RelationVal  string
	IsSelfRecord bool
	PhotoURL     string

	// Version is the ETag the edit form and the photo control send back as
	// If-Match (FR-011).
	Version string

	// Links is where the row and the form send the person.
	Links PatientLinks
}

// PatientLinks are the URLs one patient's views address, handed in rather than
// built here (research D-05: a view that spelled the path would be a second
// spelling of it).
type PatientLinks struct {
	Detail string
	Record string
}

// NewPatientView is the whole of the entity-to-page mapping, so a template
// never formats a domain.Date or a person.Sex itself.
func NewPatientView(p person.Patient, photoURL string, unitSystem identity.UnitSystem, links PatientLinks) PatientView {
	age := person.AgeAt(p.BirthDate, time.Now().UTC())

	ageText := ""
	if age.Recorded() {
		ageText = age.String()
	}

	return PatientView{
		ID:           p.ID,
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		BirthDate:    p.BirthDate.String(),
		Age:          ageText,
		Sex:          SexLabel(p.Sex),
		SexValue:     string(p.Sex),
		BloodType:    BloodTypeLabel(p.BloodType),
		BloodTypeVal: string(p.BloodType),
		HeightCM:     person.FormatHeight(p.HeightCM, unitSystem),
		WeightKG:     person.FormatWeight(p.WeightKG, unitSystem),
		Address:      p.Address,
		Relationship: RelationshipLabel(p.RelationshipToOwner),
		RelationVal:  string(p.RelationshipToOwner),
		IsSelfRecord: p.IsSelfRecord,
		PhotoURL:     photoURL,
		Version:      p.Version,
		Links:        links,
	}
}

// FullName is name and date of birth together, wherever people are listed
// (Edge case: two people sharing a name).
func (v PatientView) FullName() string {
	if v.FirstName == "" && v.LastName == "" {
		return ""
	}

	name := v.FirstName
	if v.LastName != "" {
		if name != "" {
			name += " "
		}

		name += v.LastName
	}

	return name
}

// DetailEntry is one labelled value of the chart's own detail card.
type DetailEntry struct {
	Field    string
	Label    string
	Value    string
	Datetime string
}

// Entries is FR-030's "absent renders as absent" made a property of the
// mapping: a value never recorded produces no entry at all.
func (v PatientView) Entries() []DetailEntry {
	candidates := []DetailEntry{
		{Field: FieldBirthDate, Value: v.BirthDate, Datetime: v.BirthDate},
		{Field: FieldSex, Value: v.Sex},
		{Field: FieldBloodType, Value: v.BloodType},
		{Field: FieldHeightCM, Value: v.HeightCM},
		{Field: FieldWeightKG, Value: v.WeightKG},
		{Field: FieldAddress, Value: v.Address},
		{Field: FieldRelationship, Value: v.Relationship},
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

// FieldErrors is internal/web/views/components' one type for every form in
// the application, aliased here so this package's callers cannot end up with a
// second answer to "which refusals does a control carry".
type FieldErrors = components.FieldErrors

func NewFieldErrors(invalid *domain.ValidationError) FieldErrors {
	return components.NewFieldErrors(invalid)
}

// PatientListProps is one page of the actor's own patients.
type PatientListProps struct {
	Patients     []PatientView
	Total        int
	CreateHref   string
	PreviousHref string
	NextHref     string

	// Notice is FR-017/US3-3's explanation for a stale window: a person the
	// account was just looking at, or just deleted, that this list no longer
	// carries. Empty renders nothing.
	Notice string
}

// ChartTile is one kind's own tile (contracts/patient-chart.md): its label,
// its own empty state when the count is zero, and where "add the first one"
// goes.
type ChartTile struct {
	Label string
	Href  string
	Count int
}

// Empty answers whether the patient has no rows of this kind yet — FR-030's
// own tile-level empty state, distinct from the chart's page-level one.
func (t ChartTile) Empty() bool { return t.Count == 0 }

// NewChartTiles renders one tile per count entry, in the order the chart
// summary sent them — the registry's own order, never re-sorted here.
func NewChartTiles(patientID string, counts []CountTile) []ChartTile {
	tiles := make([]ChartTile, 0, len(counts))

	for _, entry := range counts {
		tiles = append(tiles, ChartTile{
			Label: entry.Label,
			Href:  entry.Path + "?patient=" + patientID,
			Count: entry.Count,
		})
	}

	return tiles
}

// CountTile is the page's own read of one of getPatientChart's `counts`
// entries — a plain struct so this package need not import internal/web/api
// for three strings and an int.
type CountTile struct {
	Label string
	Path  string
	Count int
}

// ActivityItem is one recent-activity entry, already rendered to words: no
// name, value, note or filename ever reaches this type (FR-029), and a
// deleted target links nowhere.
type ActivityItem struct {
	Text string
	When string
	Href string
}

// PatientDetailProps is one patient's own chart: the header, the per-kind
// tiles and the recent-activity list, all inside the one landmark.
type PatientDetailProps struct {
	Patient      PatientView
	Tiles        []ChartTile
	Activity     []ActivityItem
	TotalRecords int
}

// activityKindWords is every target_kind the chart can show this phase
// (data-model §5's own trigger table): a patient-scoped event names only
// "patient" or one of internal/records' registered kinds.
var activityKindWords = map[string]string{
	"patient":    "This person's record",
	"medication": "A medication",
}

func activityKindWord(targetKind string) string {
	if word, known := activityKindWords[targetKind]; known {
		return word
	}

	return "A record"
}

// ActivityText renders one audit action to the words FR-029 asks for: what
// kind of record changed and what happened, never a name, a value or a note.
func ActivityText(action, targetKind string) string {
	subject := activityKindWord(targetKind)

	switch action {
	case "create":
		return subject + " was added"
	case "update":
		return subject + " was updated"
	case "delete":
		return subject + " was deleted"
	case "switch_patient":
		return "This person became the one in view"
	case "read_sensitive":
		return "A photograph was viewed"
	case "access_denied":
		return "An attempt to reach this person was refused"
	default:
		return subject + ": " + action
	}
}

// NewActivityItems renders the chart's recent-activity list, newest first —
// exactly the order it arrived in. targetHref answers the empty string for
// an entry whose target no longer exists, so the item renders as plain text
// and links nowhere (FR-029, US4-5).
func NewActivityItems(events []ActivityEventView, targetHref func(kind, id string) string) []ActivityItem {
	items := make([]ActivityItem, 0, len(events))

	for _, event := range events {
		href := ""
		if event.TargetExists {
			href = targetHref(event.TargetKind, event.TargetID)
		}

		items = append(items, ActivityItem{
			Text: ActivityText(event.Action, event.TargetKind),
			When: event.OccurredAt.Format("2 January"),
			Href: href,
		})
	}

	return items
}

// ActivityEventView is one audit event as this package needs it: the four
// members FR-029 permits and nothing else.
type ActivityEventView struct {
	OccurredAt   time.Time
	Action       string
	TargetKind   string
	TargetID     string
	TargetExists bool
}

// DeleteConfirmProps is FR-048's confirmation: the person's own name and how
// many records will be destroyed, read from the chart summary the page
// already loaded rather than a second, dedicated preview endpoint (research
// D-26).
type DeleteConfirmProps struct {
	PatientID    string
	Name         string
	TotalRecords int
	Version      string
	DeleteHref   string
}

// Consequence is FR-048's own sentence: how many records go with the person.
func (p DeleteConfirmProps) Consequence() string {
	switch p.TotalRecords {
	case 0:
		return "This is permanent. There is no recycle bin and no undo."
	case 1:
		return "This is permanent and destroys the one record kept for them. There is no recycle bin and no undo."
	default:
		return "This is permanent and destroys all " + strconv.Itoa(p.TotalRecords) +
			" records kept for them. There is no recycle bin and no undo."
	}
}

// Empty is FR-030/US4-2's page-level empty state: nothing recorded and
// nothing to show in the activity list either, which is when the whole
// tile-and-activity section renders as one @EmptyState rather than a grid of
// zero-count tiles.
func (p PatientDetailProps) Empty() bool {
	return p.TotalRecords == 0 && len(p.Activity) == 0
}

// PatientFormProps is the create form and the edit form: the same nine
// fields either way.
type PatientFormProps struct {
	FormID string
	New    bool

	OnSubmit   string
	CancelHref string

	Patient PatientView
	Errors  FieldErrors
}

func (p PatientFormProps) Label() string {
	if p.New {
		return FormLabelCreate
	}

	return FormLabelEdit
}

func (p PatientFormProps) SubmitLabel() string {
	if p.New {
		return "Add them"
	}

	return "Save changes"
}

// Value is what a form control holds for one field.
func (v PatientView) Value(field string) string {
	switch field {
	case FieldFirstName:
		return v.FirstName
	case FieldLastName:
		return v.LastName
	case FieldBirthDate:
		return v.BirthDate
	case FieldSex:
		return v.SexValue
	case FieldBloodType:
		return v.BloodTypeVal
	case FieldAddress:
		return v.Address
	case FieldRelationship:
		return v.RelationVal
	default:
		return ""
	}
}

// PatientPhotoProps is the photo control the detail and the form both embed.
type PatientPhotoProps struct {
	PatientID string
	PhotoURL  string
	Name      string
}
