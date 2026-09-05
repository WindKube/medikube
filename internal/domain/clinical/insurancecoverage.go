package clinical

import (
	"strconv"
	"strings"

	"medikube/internal/domain"
)

// Money is a non-negative amount, stored as an integer number of minor units
// (cents) and never as a float64 — data-model §6.2 requires it, because a
// binary float cannot represent a decimal currency amount exactly and a
// deductible that silently drifts by a cent is a defect a person would only
// find by auditing their own insurer.
//
// The wire form is a decimal string ("1250.00"); MarshalMoney and ParseMoney
// are the two edges of that conversion, kept in this file so a caller never
// invents a third rounding rule.
type Money struct {
	Minor int64
}

// ParseMoney reads a decimal string with at most two fractional digits. It
// refuses a negative amount and anything with more precision than a currency
// minor unit has.
func ParseMoney(text string) (Money, error) {
	text = strings.TrimSpace(text)

	whole, fraction, hasFraction := strings.Cut(text, ".")

	if hasFraction && len(fraction) > 2 {
		return Money{}, strconv.ErrSyntax
	}

	for len(fraction) < 2 {
		fraction += "0"
	}

	wholeValue, err := strconv.ParseInt(whole, 10, 63)
	if err != nil || wholeValue < 0 {
		return Money{}, strconv.ErrSyntax
	}

	fractionValue, err := strconv.ParseInt(fraction, 10, 63)
	if err != nil || fractionValue < 0 {
		return Money{}, strconv.ErrSyntax
	}

	return Money{Minor: wholeValue*100 + fractionValue}, nil
}

// String renders the wire form, always with two fractional digits.
func (m Money) String() string {
	return strconv.FormatInt(m.Minor/100, 10) + "." + fmt2(m.Minor%100)
}

func fmt2(n int64) string {
	if n < 0 {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

// Coverage is data-model §6.2's amounts that matter at the point of care.
// Every amount is optional; Currency is required the moment any amount is
// present (FR-044).
type Coverage struct {
	Deductible      *Money
	OOPMax          *Money
	CopayPrimary    *Money
	CopaySpecialist *Money
	CopayER         *Money
	CoinsurancePct  *float64
	Currency        string
}

func (c Coverage) hasAmount() bool {
	return c.Deductible != nil || c.OOPMax != nil || c.CopayPrimary != nil ||
		c.CopaySpecialist != nil || c.CopayER != nil
}

// IsZero reports an entirely absent coverage, so a service can tell "nothing
// recorded" apart from "recorded, sparsely".
func (c Coverage) IsZero() bool {
	return !c.hasAmount() && c.CoinsurancePct == nil && c.Currency == ""
}

// Validate enforces §6.2: a currency is a three-letter uppercase ISO-4217
// code, required whenever any amount is present; oop_max, when both it and
// the deductible are present, must be at least the deductible; the
// coinsurance percentage is bounded 0..100.
func (c Coverage) Validate(field string) *domain.FieldError {
	if c.hasAmount() && !validCurrency(c.Currency) {
		return &domain.FieldError{
			Field: field + ".currency", Code: domain.CodeRequired,
			Message: "a currency is required whenever an amount is recorded",
		}
	}

	if c.Currency != "" && !validCurrency(c.Currency) {
		return &domain.FieldError{
			Field: field + ".currency", Code: domain.CodeInvalidValue,
			Message: "a currency is a three-letter uppercase ISO-4217 code",
		}
	}

	if c.Deductible != nil && c.OOPMax != nil && c.OOPMax.Minor < c.Deductible.Minor {
		return &domain.FieldError{
			Field: field + ".oop_max", Code: CodeEndBeforeStart,
			Message: "the out-of-pocket maximum must be at least the deductible",
		}
	}

	if c.CoinsurancePct != nil && (*c.CoinsurancePct < 0 || *c.CoinsurancePct > 100) {
		return &domain.FieldError{
			Field: field + ".coinsurance_pct", Code: domain.CodeOutOfRange,
			Message: "coinsurance is a percentage between 0 and 100",
		}
	}

	return nil
}

func validCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}

	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}

	return true
}

// Contact is data-model §6.3's insurer contact details.
type Contact struct {
	Phone       string
	ClaimsPhone string
	Website     string
	PortalURL   string
	Address     string
}

func (c Contact) IsZero() bool { return c == Contact{} }
