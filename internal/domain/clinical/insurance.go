package clinical

import (
	"strings"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

const (
	maxCompany    = 200
	maxPlanName   = 200
	maxEmployer   = 200
	maxMemberName = 200
	maxMemberID   = 80
	maxGroupNum   = 80
	maxHolderName = 200
)

// Insurance is one policy a person holds (data-model §4.10). MemberName,
// MemberID, GroupNumber and HolderName are identifying content (FR-047), so
// MarshalZerologObject below emits none of them — the whole enforcement of
// US5-5 is that this method has no line that could.
type Insurance struct {
	ID        string
	PatientID string

	Type          InsuranceType
	Company       string
	PlanName      string
	EmployerGroup string
	MemberName    string
	MemberID      string
	GroupNumber   string
	HolderName    string
	Relationship  HolderRelationship

	EffectiveOn Date
	ExpiresOn   Date
	Status      InsuranceStatus
	IsPrimary   bool

	Coverage Coverage
	Contact  Contact

	Notes string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

// Validate enforces FR-043/044: type, company, member name, member id and the
// effective date are required; expires_on, when present, is not before
// effective_on; the coverage struct validates itself.
func (i Insurance) Validate() error {
	var invalid domain.ValidationError

	if i.Type == "" {
		invalid.Add("type", domain.CodeRequired, "a kind of cover is required")
	} else if !i.Type.Valid() {
		invalid.Add("type", domain.CodeInvalidValue, "not one of the kinds MediKube accepts")
	}

	if company := strings.TrimSpace(i.Company); company == "" {
		invalid.Add("company", domain.CodeRequired, "the insurer is required")
	} else {
		checkLength(&invalid, "company", "the insurer", company, maxCompany)
	}

	checkLength(&invalid, "plan_name", "the plan name", i.PlanName, maxPlanName)
	checkLength(&invalid, "employer_group", "the employer group", i.EmployerGroup, maxEmployer)

	if name := strings.TrimSpace(i.MemberName); name == "" {
		invalid.Add("member_name", domain.CodeRequired, "the member's name is required")
	} else {
		checkLength(&invalid, "member_name", "the member's name", name, maxMemberName)
	}

	if id := strings.TrimSpace(i.MemberID); id == "" {
		invalid.Add("member_id", domain.CodeRequired, "the member number is required")
	} else {
		checkLength(&invalid, "member_id", "the member number", id, maxMemberID)
	}

	checkLength(&invalid, "group_number", "the group number", i.GroupNumber, maxGroupNum)
	checkLength(&invalid, "holder_name", "the policy holder's name", i.HolderName, maxHolderName)

	if i.Relationship != "" && !i.Relationship.Valid() {
		invalid.Add("relationship_to_holder", domain.CodeInvalidValue, "not one of the relationships MediKube accepts")
	}

	if i.EffectiveOn.IsZero() {
		invalid.Add("effective_on", domain.CodeRequired, "the date cover began is required")
	}

	if err := Order(Ref{Field: "effective_on", Value: i.EffectiveOn}, Ref{Field: "expires_on", Value: i.ExpiresOn}); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}

	if i.Status != "" && !i.Status.Valid() {
		invalid.Add("status", domain.CodeInvalidValue, "not one of the states MediKube accepts")
	}

	if err := i.Coverage.Validate("coverage"); err != nil {
		invalid.Add(err.Field, err.Code, err.Message)
	}

	checkLength(&invalid, "notes", "the notes", i.Notes, maxNotes)

	return invalid.OrNil()
}

// MarshalZerologObject emits the two identifiers and nothing else — never the
// member name, the member number, the group number or the holder name
// (FR-047, US5-5, SC-012).
func (i Insurance) MarshalZerologObject(e *zerolog.Event) {
	e.Str("insurance_id", i.ID).Str("patient_id", i.PatientID)
}
