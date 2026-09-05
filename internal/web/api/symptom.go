package api

import (
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/service/symptom"
	"medikube/internal/web"
)

// The wire spellings of every symptom member, in data-model §4.6's column
// order — the order Validate checks them in and the order a refusal sorts by.
const (
	MemberSymptomName            = "name"
	MemberSymptomCategory        = "category"
	MemberSymptomSeverity        = "severity"
	MemberSymptomOccurredAt      = "occurred_at"
	MemberSymptomDurationMinutes = "duration_minutes"
	MemberSymptomPainScale       = "pain_scale"
	MemberSymptomBodySite        = "body_site"
	MemberSymptomTriggers        = "triggers"
	MemberSymptomReliefMethods   = "relief_methods"
	MemberSymptomImpact          = "impact"
	MemberSymptomResolvedAt      = "resolved_at"
	MemberSymptomIsChronic       = "is_chronic"
	MemberSymptomStatus          = "status"
)

var symptomMembers = []string{
	MemberSymptomName, MemberSymptomCategory, MemberSymptomSeverity, MemberSymptomOccurredAt,
	MemberSymptomDurationMinutes, MemberSymptomPainScale, MemberSymptomBodySite,
	MemberSymptomTriggers, MemberSymptomReliefMethods, MemberSymptomImpact,
	MemberSymptomResolvedAt, MemberSymptomIsChronic, MemberSymptomStatus,
}

// SymptomSummary is FR-031's list row: every episode plus the derived
// aggregate over its own name.
type SymptomSummary struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`

	Name           string `json:"name"`
	Severity       string `json:"severity"`
	OccurredAt     string `json:"occurred_at"`
	EpisodeCount   int    `json:"episode_count"`
	LastOccurredAt string `json:"last_occurred_at"`
	UpdatedAt      string `json:"updated_at"`
	Status         string `json:"status,omitempty"`
}

type Symptom struct {
	SymptomSummary

	Patient         string   `json:"patient"`
	Category        string   `json:"category,omitempty"`
	DurationMinutes *int     `json:"duration_minutes,omitempty"`
	PainScale       *int     `json:"pain_scale,omitempty"`
	BodySite        string   `json:"body_site,omitempty"`
	Triggers        []string `json:"triggers"`
	ReliefMethods   []string `json:"relief_methods"`
	Impact          string   `json:"impact,omitempty"`
	ResolvedAt      *string  `json:"resolved_at"`
	IsChronic       bool     `json:"is_chronic"`
	Tags            []string `json:"tags,omitempty"`
}

type SymptomCreate struct {
	Patient         string   `json:"patient"`
	Name            string   `json:"name"`
	Category        string   `json:"category,omitempty"`
	Severity        string   `json:"severity"`
	OccurredAt      string   `json:"occurred_at"`
	DurationMinutes *int     `json:"duration_minutes,omitempty"`
	PainScale       *int     `json:"pain_scale,omitempty"`
	BodySite        string   `json:"body_site,omitempty"`
	Triggers        []string `json:"triggers,omitempty"`
	ReliefMethods   []string `json:"relief_methods,omitempty"`
	Impact          string   `json:"impact,omitempty"`
	ResolvedAt      *string  `json:"resolved_at,omitempty"`
	IsChronic       bool     `json:"is_chronic,omitempty"`
	Status          string   `json:"status,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable: a create always supplies its tags,
// even when that is none.
func (c *SymptomCreate) TagIDs() (ids []string, supplied bool) { return c.Tags, true }

type SymptomPatch struct {
	Name       *string `json:"name,omitempty"`
	Category   *string `json:"category,omitempty"`
	Severity   *string `json:"severity,omitempty"`
	OccurredAt *string `json:"occurred_at,omitempty"`

	DurationMinutes web.Optional[int] `json:"duration_minutes,omitzero"`
	PainScale       web.Optional[int] `json:"pain_scale,omitzero"`

	BodySite      *string   `json:"body_site,omitempty"`
	Triggers      *[]string `json:"triggers,omitempty"`
	ReliefMethods *[]string `json:"relief_methods,omitempty"`
	Impact        *string   `json:"impact,omitempty"`

	ResolvedAt web.Optional[string] `json:"resolved_at,omitzero"`

	IsChronic *bool   `json:"is_chronic,omitempty"`
	Status    *string `json:"status,omitempty"`

	// Tags is replace-set (FR-064, FR-065): a nil pointer leaves the
	// applied tags alone, a non-nil one — including an empty array —
	// replaces the whole set.
	Tags *[]string `json:"tags,omitempty"`
}

// TagIDs implements records.Taggable.
func (p *SymptomPatch) TagIDs() (ids []string, supplied bool) {
	if p.Tags == nil {
		return nil, false
	}

	return *p.Tags, true
}

// SymptomCodec is the DTO boundary for symptom episodes.
type SymptomCodec struct{}

var _ symptom.Codec = SymptomCodec{}

func SymptomSchema() records.Schema {
	return records.Schema{
		NewSummary: func() any { return new(SymptomSummary) },
		NewDetail:  func() any { return new(Symptom) },
		NewCreate:  func() any { return new(SymptomCreate) },
		NewPatch:   func() any { return new(SymptomPatch) },
	}
}

// SymptomSearchFields reads the name and the body site — the free text a
// person wrote about the episode.
func SymptomSearchFields(body any) (title, text string) {
	s, ok := body.(*Symptom)
	if !ok {
		return "", ""
	}

	return s.Name, s.BodySite
}

// SymptomBasis narrows nothing beyond the published query parameters
// (contracts/records-clinical.md §1: symptom's narrowings are all plain
// filters, none needing a per-row explanation).
func SymptomBasis(any, records.Criteria) []string { return nil }

func (SymptomCodec) Summary(s clinical.Symptom) any {
	return &SymptomSummary{
		ID:             s.ID,
		Kind:           kind.Symptom.Enum(),
		Name:           s.Name,
		Severity:       string(s.Severity),
		OccurredAt:     wireClinicalInstant(s.OccurredAt),
		EpisodeCount:   s.EpisodeCount,
		LastOccurredAt: wireClinicalInstant(s.LastOccurredAt),
		UpdatedAt:      wireInstant(s.UpdatedAt),
		Status:         string(s.Status),
	}
}

func (c SymptomCodec) Detail(s clinical.Symptom) any {
	summary, ok := c.Summary(s).(*SymptomSummary)
	if !ok {
		return &Symptom{}
	}

	return &Symptom{
		SymptomSummary:  *summary,
		Patient:         s.PatientID,
		Category:        string(s.Category),
		DurationMinutes: s.DurationMinutes,
		PainScale:       s.PainScale,
		BodySite:        s.BodySite,
		Triggers:        orEmptySlice(s.Triggers),
		ReliefMethods:   orEmptySlice(s.ReliefMethods),
		Impact:          string(s.Impact),
		ResolvedAt:      wireClinicalInstantPtr(s.ResolvedAt),
		IsChronic:       s.IsChronic,
		Tags:            s.Tags,
	}
}

func (SymptomCodec) Draft(body any) (clinical.Symptom, error) {
	create, ok := body.(*SymptomCreate)
	if !ok {
		return clinical.Symptom{}, ErrWrongBodyType
	}

	var invalid domain.ValidationError

	occurredAt := readClinicalInstant(&invalid, MemberSymptomOccurredAt, &create.OccurredAt)
	resolvedAt := readClinicalInstant(&invalid, MemberSymptomResolvedAt, create.ResolvedAt)

	if err := orderedSymptomRefusal(&invalid); err != nil {
		return clinical.Symptom{}, err
	}

	return clinical.Symptom{
		PatientID:       create.Patient,
		Name:            create.Name,
		Category:        clinical.SymptomCategory(create.Category),
		Severity:        clinical.Severity(create.Severity),
		OccurredAt:      occurredAt,
		DurationMinutes: create.DurationMinutes,
		PainScale:       create.PainScale,
		BodySite:        create.BodySite,
		Triggers:        create.Triggers,
		ReliefMethods:   create.ReliefMethods,
		Impact:          clinical.SymptomImpact(create.Impact),
		ResolvedAt:      resolvedAt,
		IsChronic:       create.IsChronic,
		Status:          clinical.ConditionStatus(create.Status),
		Tags:            create.Tags,
	}, nil
}

func (SymptomCodec) Patch(body any) (symptom.Patch, error) {
	incoming, ok := body.(*SymptomPatch)
	if !ok {
		return symptom.Patch{}, ErrWrongBodyType
	}

	var invalid domain.ValidationError

	patch := symptom.Patch{
		Name:            incoming.Name,
		Category:        convert[clinical.SymptomCategory](incoming.Category),
		Severity:        convert[clinical.Severity](incoming.Severity),
		OccurredAt:      readOptionalClinicalInstant(&invalid, MemberSymptomOccurredAt, incoming.OccurredAt),
		DurationMinutes: readOptionalIntPtr(incoming.DurationMinutes),
		PainScale:       readOptionalIntPtr(incoming.PainScale),
		BodySite:        incoming.BodySite,
		Triggers:        incoming.Triggers,
		ReliefMethods:   incoming.ReliefMethods,
		Impact:          convert[clinical.SymptomImpact](incoming.Impact),
		ResolvedAt:      readOptionalClinicalInstantPtr(&invalid, MemberSymptomResolvedAt, incoming.ResolvedAt),
		IsChronic:       incoming.IsChronic,
		Status:          convert[clinical.ConditionStatus](incoming.Status),
		Tags:            incoming.Tags,
	}

	if err := orderedSymptomRefusal(&invalid); err != nil {
		return symptom.Patch{}, err
	}

	return patch, nil
}

func orderedSymptomRefusal(invalid *domain.ValidationError) error {
	if invalid.Empty() {
		return nil
	}

	slices.SortStableFunc(invalid.Fields, func(left, right domain.FieldError) int {
		return slices.Index(symptomMembers, left.Field) - slices.Index(symptomMembers, right.Field)
	})

	return invalid.OrNil()
}

func orEmptySlice(values []string) []string {
	if values == nil {
		return []string{}
	}

	return values
}

// wireClinicalInstant renders a required instant.
func wireClinicalInstant(i clinical.Instant) string { return i.String() }

// wireClinicalInstantPtr renders an optional one as an explicit null.
func wireClinicalInstantPtr(i clinical.Instant) *string {
	if i.IsZero() {
		return nil
	}

	s := i.String()

	return &s
}

// readClinicalInstant parses a wire instant. The submitted text never reaches
// the message — it is medical data (constitution VII).
func readClinicalInstant(invalid *domain.ValidationError, member string, raw *string) clinical.Instant {
	if raw == nil || *raw == "" {
		return clinical.Instant{}
	}

	var parsed clinical.Instant
	if err := parsed.UnmarshalText([]byte(*raw)); err != nil {
		invalid.Add(member, domain.CodeInvalidDate, "an instant is RFC3339, e.g. 2026-01-02T15:04:05Z")

		return clinical.Instant{}
	}

	return parsed
}

func readOptionalClinicalInstant(invalid *domain.ValidationError, member string, supplied *string) *clinical.Instant {
	if supplied == nil {
		return nil
	}

	parsed := readClinicalInstant(invalid, member, supplied)

	return &parsed
}

func readOptionalClinicalInstantPtr(invalid *domain.ValidationError, member string, supplied web.Optional[string]) *clinical.Instant {
	if !supplied.Present() {
		return nil
	}

	raw, given := supplied.Get()
	if !given {
		return &clinical.Instant{}
	}

	parsed := readClinicalInstant(invalid, member, &raw)

	return &parsed
}

func readOptionalIntPtr(supplied web.Optional[int]) **int {
	if !supplied.Present() {
		return nil
	}

	value, given := supplied.Get()
	if !given {
		var cleared *int

		return &cleared
	}

	v := value
	pv := &v

	return &pv
}
