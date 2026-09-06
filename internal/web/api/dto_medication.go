package api

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/medication"
	"medikube/internal/web"
)

// The wire spellings of every medication member, declared once. They are the
// json tags, the field a refusal is attached to and the order the refusals come
// back in, so a member renamed on the wire and not in a refusal is a compile
// error rather than a form that reports an error against a field nobody can
// see.
const (
	MemberID              = "id"
	MemberKind            = "kind"
	MemberPatient         = "patient"
	MemberPractitioner    = "practitioner"
	MemberPharmacy        = "pharmacy"
	MemberName            = "name"
	MemberAlternativeName = "alternative_name"
	MemberType            = "type"
	MemberDosage          = "dosage"
	MemberFrequency       = "frequency"
	MemberRoute           = "route"
	MemberIndication      = "indication"
	MemberStartedOn       = "started_on"
	MemberEndedOn         = "ended_on"
	MemberStatus          = "status"
	MemberSideEffects     = "side_effects"
	MemberNotes           = "notes"
	MemberCreatedAt       = "created_at"
	MemberUpdatedAt       = "updated_at"
)

// medicationMembers is data-model §2's column order, which is the order
// clinical.Validate checks the rules in and the order the form renders. Every
// refusal this file raises is sorted into it before it leaves, so a response
// carrying a date refusal and a rule refusal reads in the order the person
// filled the form in rather than in the order the code happened to look.
var medicationMembers = []string{
	MemberPatient,
	MemberName,
	MemberAlternativeName,
	MemberType,
	MemberDosage,
	MemberFrequency,
	MemberRoute,
	MemberIndication,
	MemberStartedOn,
	MemberEndedOn,
	MemberStatus,
	MemberSideEffects,
	MemberNotes,
	MemberPractitioner,
	MemberPharmacy,
}

// timestampLayout is FR-020's instant on the wire: RFC3339 in UTC, one
// spelling. A local zone here would make two instances of MediKube disagree
// about when a record changed.
const timestampLayout = time.RFC3339

// MedicationSummary is what the list operations return.
//
// The absent optional members are absent rather than empty (FR-024): `omitempty`
// is what makes "never filled in" a property of the wire format instead of a
// rendering convention. The two dates are the exception and carry no
// `omitempty`, so `null` says "not recorded" and a member that is simply
// missing would say "not in this response" — a distinction that costs one word
// and closes a class of client bug.
type MedicationSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name      string  `json:"name"`
	Dosage    string  `json:"dosage,omitempty"`
	Frequency string  `json:"frequency,omitempty"`
	Status    string  `json:"status"`
	StartedOn *string `json:"started_on"`
	UpdatedAt string  `json:"updated_at"`
}

// Medication is what the detail operations return: every recorded field of
// FR-015 plus the created and last-changed instants of FR-020.
type Medication struct {
	MedicationSummary

	Patient      string `json:"patient"`
	Practitioner string `json:"practitioner,omitempty"`
	Pharmacy     string `json:"pharmacy,omitempty"`

	AlternativeName string   `json:"alternative_name,omitempty"`
	Type            string   `json:"type,omitempty"`
	Route           string   `json:"route,omitempty"`
	Indication      string   `json:"indication,omitempty"`
	EndedOn         *string  `json:"ended_on"`
	SideEffects     string   `json:"side_effects,omitempty"`
	Notes           string   `json:"notes,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

// GetTags implements records.Tagged so the search index stays in step with
// this record's tags on write (T164-T177 follow-up).
func (m *Medication) GetTags() []string { return m.Tags }

// MedicationCreate is the create body. It has no `owner`, no `id` and no
// timestamps, and that absence is the enforcement of FR-032 rather than a
// runtime check: unknown members are rejected by the decoder, so a body that
// nominates an owner is refused before any handler decides anything.
//
// FR-015: a name alone is sufficient beyond the patient. `Patient` is required
// (FR-021, US2-3): an absent or empty one is a 422 naming the field, not a
// fallback to anybody's active patient. `Practitioner` and `Pharmacy` are
// phase 002's optional attributions (US5).
type MedicationCreate struct {
	Patient         string   `json:"patient"`
	Name            string   `json:"name"`
	AlternativeName string   `json:"alternative_name,omitempty"`
	Type            string   `json:"type,omitempty"`
	Dosage          string   `json:"dosage,omitempty"`
	Frequency       string   `json:"frequency,omitempty"`
	Route           string   `json:"route,omitempty"`
	Indication      string   `json:"indication,omitempty"`
	StartedOn       *string  `json:"started_on,omitempty"`
	EndedOn         *string  `json:"ended_on,omitempty"`
	Status          string   `json:"status,omitempty"`
	SideEffects     string   `json:"side_effects,omitempty"`
	Notes           string   `json:"notes,omitempty"`
	Practitioner    *string  `json:"practitioner,omitempty"`
	Pharmacy        *string  `json:"pharmacy,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *MedicationCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

// MedicationPatch is the partial update. Only supplied members change.
//
// contracts/records.md spells the two dates `**string` and says the mechanism
// carries absent-versus-explicit-null. It does not — encoding/json zeroes the
// whole pointer chain when it reads a null, so both states arrive as a nil
// outer pointer, which internal/web/json_semantics_test.go pins. web.Optional
// is the mechanism that does work and is what internal/web/dto.go directs every
// PATCH DTO to use; `omitzero` is required beside it or an absent member
// marshals back as null and the clear is indistinguishable again on the way
// out.
//
// The other ten members stay plain pointers, because for them "clear it" and
// "set it to the empty string" are the same instruction and a third state would
// be one nobody could reach.
type MedicationPatch struct {
	Name            *string `json:"name,omitempty"`
	AlternativeName *string `json:"alternative_name,omitempty"`
	Type            *string `json:"type,omitempty"`
	Dosage          *string `json:"dosage,omitempty"`
	Frequency       *string `json:"frequency,omitempty"`
	Route           *string `json:"route,omitempty"`
	Indication      *string `json:"indication,omitempty"`

	StartedOn web.Optional[string] `json:"started_on,omitzero"`
	EndedOn   web.Optional[string] `json:"ended_on,omitzero"`

	Status      *string `json:"status,omitempty"`
	SideEffects *string `json:"side_effects,omitempty"`
	Notes       *string `json:"notes,omitempty"`

	// Practitioner and Pharmacy, phase 002's additions. There is deliberately
	// no Patient field here (contracts/medications-rescope.md): re-attribution
	// is refused by DTO shape rather than a runtime check.
	Practitioner *string `json:"practitioner,omitempty"`
	Pharmacy     *string `json:"pharmacy,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *MedicationPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// ErrWrongBodyType is a decoded body that is not the type this kind minted.
//
// It is a wiring failure and never a caller's: records.Schema.NewCreate and
// NewPatch are what produced the value, so a mismatch means the registration
// pairs one kind's schema with another kind's codec. It must reach the operator
// as a 500 and not the person as a rejected form.
var ErrWrongBodyType = errors.New("api: the decoded body is not the type this kind's schema mints")

// MedicationCodec is the DTO boundary for medications: the only place a
// clinical.Medication becomes a wire shape and the only place a wire shape
// becomes one.
//
// It satisfies medication.Codec, which internal/service/medication declares and
// deliberately does not implement — a service that named a wire type would put
// the HTTP edge in the dependency graph of every use case.
type MedicationCodec struct{}

var _ medication.Codec = MedicationCodec{}

// MedicationSchema is the four constructors the registry publishes and the
// generic handler decodes into. Each returns a new pointer: a shared value
// would let one request's decode be visible in the next.
func MedicationSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(MedicationSummary) },
		NewDetail:  func() any { return new(Medication) },
		NewCreate:  func() any { return new(MedicationCreate) },
		NewPatch:   func() any { return new(MedicationPatch) },
	}
}

// MedicationSearchFields reads the two search_index columns off the wire DTO
// Record.Body carries after a create or an update (research D-11): the name,
// and the side effects and notes FR-069 calls "the details of each type".
func MedicationSearchFields(body any) (title, text string) {
	medication, ok := body.(*Medication)
	if !ok {
		return "", ""
	}

	return medication.Name, medication.SideEffects + " " + medication.Notes
}

// MedicationBasis narrows nothing yet: medication's one narrowing beyond
// status is `?active=true` (contracts/records-clinical.md §1), which is a
// query and not a per-row distinction the way procedure's scheduled/ordered
// is. It is declared anyway, so the registry's completeness check has
// something to find.
func MedicationBasis(any, records.Criteria) []string { return nil }

// Summary renders the list shape.
func (MedicationCodec) Summary(m clinical.Medication) any {
	return &MedicationSummary{
		ID:        m.ID,
		Kind:      kind.Medication.Enum(),
		Name:      m.Name,
		Dosage:    m.Dosage,
		Frequency: m.Frequency,
		Status:    string(m.Status),
		StartedOn: wireDate(m.StartedOn),
		UpdatedAt: wireInstant(m.UpdatedAt),
	}
}

// Detail renders the full shape.
func (c MedicationCodec) Detail(m clinical.Medication) any {
	summary, ok := c.Summary(m).(*MedicationSummary)
	if !ok {
		// Unreachable while Summary returns what it says it does, and a
		// panic here would be a 500 with no line naming the cause.
		return &Medication{}
	}

	return &Medication{
		MedicationSummary: *summary,
		Patient:           m.PatientID,
		Practitioner:      m.PractitionerID,
		Pharmacy:          m.PharmacyID,
		AlternativeName:   m.AlternativeName,
		Type:              string(m.Type),
		Route:             string(m.Route),
		Indication:        m.Indication,
		EndedOn:           wireDate(m.EndedOn),
		SideEffects:       m.SideEffects,
		Notes:             m.Notes,
		Tags:              m.Tags,
		CreatedAt:         wireInstant(m.CreatedAt),
	}
}

// Draft reads a create body.
//
// It fills no server-owned member: the DTO has none, and the service overwrites
// the four it could not have carried anyway. What is left is the calendar, and
// both dates are read together so that two malformed dates are two refusals and
// not one round trip each (FR-027).
func (MedicationCodec) Draft(body any) (clinical.Medication, error) {
	create, ok := body.(*MedicationCreate)
	if !ok {
		return clinical.Medication{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	started := readDate(&invalid, MemberStartedOn, create.StartedOn)
	ended := readDate(&invalid, MemberEndedOn, create.EndedOn)

	if err := orderedRefusal(&invalid); err != nil {
		return clinical.Medication{}, err
	}

	return clinical.Medication{
		PatientID:       create.Patient,
		Name:            create.Name,
		AlternativeName: create.AlternativeName,
		Type:            clinical.MedicationType(create.Type),
		Dosage:          create.Dosage,
		Frequency:       create.Frequency,
		Route:           clinical.MedicationRoute(create.Route),
		Indication:      create.Indication,
		StartedOn:       started,
		EndedOn:         ended,
		Status:          clinical.TherapyStatus(create.Status),
		SideEffects:     create.SideEffects,
		Notes:           create.Notes,
		PractitionerID:  deref(create.Practitioner),
		PharmacyID:      deref(create.Pharmacy),
		Tags:            create.Tags,
	}, nil
}

// deref reads an optional string member, or the empty string when it was not
// supplied. Create carries no notion of "clear this" — there is nothing yet to
// clear — so an absent pointer and an empty string mean the same thing here.
func deref(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

// Patch reads an update body.
//
// A supplied member becomes a non-nil pointer whatever its value, so setting a
// dose to the empty string clears the dose and saying nothing leaves it alone.
// The two dates come through web.Optional: present-with-a-value is the new day,
// present-and-null is the clear, and absent is silence.
func (MedicationCodec) Patch(body any) (medication.Patch, error) {
	incoming, ok := body.(*MedicationPatch)
	if !ok {
		return medication.Patch{}, fmt.Errorf("%w: %T", ErrWrongBodyType, body)
	}

	var invalid domain.ValidationError

	patch := medication.Patch{
		Name:            incoming.Name,
		AlternativeName: incoming.AlternativeName,
		Type:            convert[clinical.MedicationType](incoming.Type),
		Dosage:          incoming.Dosage,
		Frequency:       incoming.Frequency,
		Route:           convert[clinical.MedicationRoute](incoming.Route),
		Indication:      incoming.Indication,
		StartedOn:       readOptionalDate(&invalid, MemberStartedOn, incoming.StartedOn),
		EndedOn:         readOptionalDate(&invalid, MemberEndedOn, incoming.EndedOn),
		Status:          convert[clinical.TherapyStatus](incoming.Status),
		SideEffects:     incoming.SideEffects,
		Notes:           incoming.Notes,
		Practitioner:    incoming.Practitioner,
		Pharmacy:        incoming.Pharmacy,
		Tags:            incoming.Tags,
	}

	if err := orderedRefusal(&invalid); err != nil {
		return medication.Patch{}, err
	}

	return patch, nil
}

// readDate parses an optional wire date. An absent member and an explicit null
// are both the absent date, which is what an unset optional column holds.
//
// The submitted text is never in the message. A start date is medical data and
// this text reaches the response, the log and Sentry (constitution VII).
func readDate(invalid *domain.ValidationError, member string, raw *string) domain.Date {
	if raw == nil {
		return domain.Date{}
	}

	parsed, err := domain.ParseDate(*raw)
	if err != nil {
		invalid.Add(member, domain.CodeInvalidDate, "a date is a real calendar day written YYYY-MM-DD")

		return domain.Date{}
	}

	return parsed
}

// readOptionalDate is readDate over the three PATCH states. Absent is nil —
// leave the stored day alone; an explicit null is a pointer to the zero date —
// clear it; a value is that day.
func readOptionalDate(invalid *domain.ValidationError, member string, supplied web.Optional[string]) *domain.Date {
	if !supplied.Present() {
		return nil
	}

	raw, given := supplied.Get()
	if !given {
		return &domain.Date{}
	}

	parsed := readDate(invalid, member, &raw)

	return &parsed
}

// orderedRefusal sorts the refusals into data-model §2's column order and
// returns them, or nil when there are none.
//
// The order is contract rather than cosmetics: FR-027 requires every problem in
// one response, and a response that lists them in the order the code looked
// rather than the order the form reads is one a person has to search.
func orderedRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(medicationMembers, left.Field) - slices.Index(medicationMembers, right.Field)
	})

	return invalid.OrNil()
}

// convert re-types a supplied string member without judging it. An unpublished
// value is refused by the domain against its own vocabulary, so that the
// refusal is raised once, in the layer that publishes the list.
func convert[T ~string](supplied *string) *T {
	if supplied == nil {
		return nil
	}

	converted := T(*supplied)

	return &converted
}

// wireDate renders an optional calendar date. The absent date is an explicit
// null rather than an empty string: "" is a value somebody typed and null is a
// field nobody filled in.
func wireDate(date domain.Date) *string {
	if date.IsZero() {
		return nil
	}

	rendered := date.String()

	return &rendered
}

// wireInstant renders a stored instant. A record that has never been saved has
// no instant, and the empty string is what says so — a zero time rendered as
// 0001-01-01 is a date somebody would try to display.
func wireInstant(instant time.Time) string {
	if instant.IsZero() {
		return ""
	}

	return instant.UTC().Format(timestampLayout)
}
