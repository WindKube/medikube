package insurance

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

const (
	fieldPatient       = "patient"
	fieldType          = "type"
	fieldCompany       = "company"
	fieldPlanName      = "plan_name"
	fieldEmployerGroup = "employer_group"
	fieldMemberName    = "member_name"
	fieldMemberID      = "member_id"
	fieldGroupNumber   = "group_number"
	fieldHolderName    = "holder_name"
	fieldRelationship  = "relationship_to_holder"
	fieldEffectiveOn   = "effective_on"
	fieldExpiresOn     = "expires_on"
	fieldStatus        = "status"
	fieldIsPrimary     = "is_primary"
	fieldCoverage      = "coverage"
	fieldContact       = "contact"
	fieldNotes         = "notes"
	fieldTags          = "tags"
	fieldCreated       = "created"
	fieldUpdated       = "updated"
)

const (
	ColumnID          = "id"
	ColumnPatient     = fieldPatient
	ColumnCompany     = fieldCompany
	ColumnType        = fieldType
	ColumnStatus      = fieldStatus
	ColumnIsPrimary   = fieldIsPrimary
	ColumnEffectiveOn = fieldEffectiveOn
	ColumnExpiresOn   = fieldExpiresOn
)

var ErrUnexpectedCollection = errors.New("insurance: the record is not from the insurances collection")

func Schema() store.Schema {
	return store.NewSchema(kind.Insurance.Collection(),
		store.Column{Name: fieldPatient},
		store.Column{
			Name:       fieldCompany,
			Expr:       "LOWER(" + quote(fieldCompany) + ")",
			Searchable: true,
			Value:      func(record *core.Record) string { return asciiLower(record.GetString(fieldCompany)) },
		},
		store.Column{Name: fieldType},
		store.Column{Name: fieldStatus},
		store.Column{Name: fieldIsPrimary, FilterOnly: true},
		store.Column{Name: fieldEffectiveOn, AbsentLast: true},
		store.Column{Name: fieldExpiresOn},
		// FilterOnly: `?tags=` narrows, but a multi-select relation's JSON
		// column is never an ordering (research D-05's cursor-disclosure
		// rule).
		store.Column{Name: fieldTags, FilterOnly: true},
		store.Column{Name: fieldCreated},
		store.Column{Name: fieldUpdated},
	)
}

func quote(name string) string { return "[[" + name + "]]" }

func asciiLower(value string) string {
	folded := []byte(value)
	for i, b := range folded {
		if b >= 'A' && b <= 'Z' {
			folded[i] = b + ('a' - 'A')
		}
	}

	return string(folded)
}

func expectCollection(record *core.Record) error {
	if record.Collection().Name != kind.Insurance.Collection() {
		return ErrUnexpectedCollection
	}

	return nil
}

// wireCoverage and wireContact are the JSON columns' on-the-wire shape:
// exactly data-model §6.2/§6.3, spelled with json tags because this is the
// mapper and not the API's DTO layer — the two are allowed to agree on a
// shape without being the same type (research D-11's principle applied to
// storage rather than to the wire).
type wireCoverage struct {
	Deductible      string   `json:"deductible,omitempty"`
	OOPMax          string   `json:"oop_max,omitempty"`
	CopayPrimary    string   `json:"copay_primary,omitempty"`
	CopaySpecialist string   `json:"copay_specialist,omitempty"`
	CopayER         string   `json:"copay_er,omitempty"`
	CoinsurancePct  *float64 `json:"coinsurance_pct,omitempty"`
	Currency        string   `json:"currency,omitempty"`
}

type wireContact struct {
	Phone       string `json:"phone,omitempty"`
	ClaimsPhone string `json:"claims_phone,omitempty"`
	Website     string `json:"website,omitempty"`
	PortalURL   string `json:"portal_url,omitempty"`
	Address     string `json:"address,omitempty"`
}

func FromRecord(record *core.Record) (clinical.Insurance, error) {
	if err := expectCollection(record); err != nil {
		return clinical.Insurance{}, err
	}

	effectiveOn, err := recordDate(record, fieldEffectiveOn)
	if err != nil {
		return clinical.Insurance{}, err
	}

	expiresOn, err := recordDate(record, fieldExpiresOn)
	if err != nil {
		return clinical.Insurance{}, err
	}

	coverage, err := readCoverage(record)
	if err != nil {
		return clinical.Insurance{}, err
	}

	return clinical.Insurance{
		ID:            record.Id,
		PatientID:     record.GetString(fieldPatient),
		Type:          clinical.InsuranceType(record.GetString(fieldType)),
		Company:       record.GetString(fieldCompany),
		PlanName:      record.GetString(fieldPlanName),
		EmployerGroup: record.GetString(fieldEmployerGroup),
		MemberName:    record.GetString(fieldMemberName),
		MemberID:      record.GetString(fieldMemberID),
		GroupNumber:   record.GetString(fieldGroupNumber),
		HolderName:    record.GetString(fieldHolderName),
		Relationship:  clinical.HolderRelationship(record.GetString(fieldRelationship)),
		EffectiveOn:   effectiveOn,
		ExpiresOn:     expiresOn,
		Status:        clinical.InsuranceStatus(record.GetString(fieldStatus)),
		IsPrimary:     record.GetBool(fieldIsPrimary),
		Coverage:      coverage,
		Contact:       readContact(record),
		Notes:         record.GetString(fieldNotes),
		Tags:          record.GetStringSlice(fieldTags),
		CreatedAt:     recordInstant(record, fieldCreated),
		UpdatedAt:     recordInstant(record, fieldUpdated),
		Version:       store.Version(record),
	}, nil
}

func ToRecord(record *core.Record, entity clinical.Insurance) error {
	if err := expectCollection(record); err != nil {
		return err
	}

	record.Set(fieldPatient, entity.PatientID)
	record.Set(fieldType, string(entity.Type))
	record.Set(fieldCompany, entity.Company)
	record.Set(fieldPlanName, entity.PlanName)
	record.Set(fieldEmployerGroup, entity.EmployerGroup)
	record.Set(fieldMemberName, entity.MemberName)
	record.Set(fieldMemberID, entity.MemberID)
	record.Set(fieldGroupNumber, entity.GroupNumber)
	record.Set(fieldHolderName, entity.HolderName)
	record.Set(fieldRelationship, string(entity.Relationship))
	setDate(record, fieldEffectiveOn, entity.EffectiveOn)
	setDate(record, fieldExpiresOn, entity.ExpiresOn)
	record.Set(fieldStatus, string(entity.Status))
	record.Set(fieldIsPrimary, entity.IsPrimary)

	if err := writeCoverage(record, entity.Coverage); err != nil {
		return err
	}

	writeContact(record, entity.Contact)
	record.Set(fieldNotes, entity.Notes)
	record.Set(fieldTags, entity.Tags)

	return nil
}

func readCoverage(record *core.Record) (clinical.Coverage, error) {
	raw := record.GetString(fieldCoverage)
	if isEmptyJSON(raw) {
		return clinical.Coverage{}, nil
	}

	var wire wireCoverage
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return clinical.Coverage{}, fmt.Errorf("insurance: %s holds a value that is not the coverage shape: %w", fieldCoverage, err)
	}

	coverage := clinical.Coverage{CoinsurancePct: wire.CoinsurancePct, Currency: wire.Currency}

	var parseErr error

	coverage.Deductible, parseErr = parseMoneyPtr(wire.Deductible, parseErr)
	coverage.OOPMax, parseErr = parseMoneyPtr(wire.OOPMax, parseErr)
	coverage.CopayPrimary, parseErr = parseMoneyPtr(wire.CopayPrimary, parseErr)
	coverage.CopaySpecialist, parseErr = parseMoneyPtr(wire.CopaySpecialist, parseErr)
	coverage.CopayER, parseErr = parseMoneyPtr(wire.CopayER, parseErr)

	if parseErr != nil {
		return clinical.Coverage{}, fmt.Errorf("insurance: %s holds an amount that is not a decimal: %w", fieldCoverage, parseErr)
	}

	return coverage, nil
}

func parseMoneyPtr(raw string, prior error) (*clinical.Money, error) {
	if prior != nil || raw == "" {
		return nil, prior
	}

	money, err := clinical.ParseMoney(raw)
	if err != nil {
		return nil, err
	}

	return &money, nil
}

func writeCoverage(record *core.Record, coverage clinical.Coverage) error {
	if coverage.IsZero() {
		record.Set(fieldCoverage, "null")
		return nil
	}

	wire := wireCoverage{
		CoinsurancePct:  coverage.CoinsurancePct,
		Currency:        coverage.Currency,
		Deductible:      moneyString(coverage.Deductible),
		OOPMax:          moneyString(coverage.OOPMax),
		CopayPrimary:    moneyString(coverage.CopayPrimary),
		CopaySpecialist: moneyString(coverage.CopaySpecialist),
		CopayER:         moneyString(coverage.CopayER),
	}

	encoded, err := json.Marshal(wire)
	if err != nil {
		return fmt.Errorf("insurance: encoding coverage: %w", err)
	}

	record.Set(fieldCoverage, string(encoded))

	return nil
}

func moneyString(m *clinical.Money) string {
	if m == nil {
		return ""
	}

	return m.String()
}

func readContact(record *core.Record) clinical.Contact {
	raw := record.GetString(fieldContact)
	if isEmptyJSON(raw) {
		return clinical.Contact{}
	}

	var wire wireContact
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return clinical.Contact{}
	}

	return clinical.Contact(wire)
}

func writeContact(record *core.Record, contact clinical.Contact) {
	if contact.IsZero() {
		record.Set(fieldContact, "null")
		return
	}

	encoded, err := json.Marshal(wireContact(contact))
	if err != nil {
		record.Set(fieldContact, "")
		return
	}

	record.Set(fieldContact, string(encoded))
}

// isEmptyJSON matches every rendering PocketBase's JSONField gives an
// absent value: the raw column untouched, and every shape
// core.JSONField.PrepareValue normalises an empty Go string or an explicit
// "null" into.
func isEmptyJSON(raw string) bool {
	switch raw {
	case "", "null", `""`, "[]", "{}":
		return true
	default:
		return false
	}
}

func recordDate(record *core.Record, field string) (domain.Date, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return domain.Date{}, nil
	}

	var date domain.Date
	if err := date.Scan(stored.Time().UTC()); err != nil {
		return domain.Date{}, fmt.Errorf("insurance: %s is not a calendar date: %w", field, err)
	}

	return date, nil
}

func setDate(record *core.Record, field string, date domain.Date) {
	record.Set(field, date.UTC())
}

func recordInstant(record *core.Record, field string) time.Time {
	return record.GetDateTime(field).Time().UTC().Truncate(time.Millisecond)
}
