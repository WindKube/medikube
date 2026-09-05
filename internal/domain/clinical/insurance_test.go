package clinical

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validInsurance(t *testing.T) Insurance {
	t.Helper()

	return Insurance{
		ID:          "ins1",
		PatientID:   "pat1",
		Type:        InsuranceTypeMedical,
		Company:     "Acme Health",
		MemberName:  "Jamie Doe",
		MemberID:    "M12345",
		EffectiveOn: mustDate(t, "2026-01-01"),
	}
}

func TestInsuranceValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(t *testing.T, i *Insurance)
		wantErr bool
		field   string
	}{
		{name: "a minimal valid policy", mutate: func(*testing.T, *Insurance) {}},
		{name: "type is required", mutate: func(_ *testing.T, i *Insurance) { i.Type = "" }, wantErr: true, field: "type"},
		{name: "company is required", mutate: func(_ *testing.T, i *Insurance) { i.Company = "" }, wantErr: true, field: "company"},
		{name: "member_name is required", mutate: func(_ *testing.T, i *Insurance) { i.MemberName = "" }, wantErr: true, field: "member_name"},
		{name: "member_id is required", mutate: func(_ *testing.T, i *Insurance) { i.MemberID = "" }, wantErr: true, field: "member_id"},
		{name: "effective_on is required", mutate: func(_ *testing.T, i *Insurance) { i.EffectiveOn = Date{} }, wantErr: true, field: "effective_on"},
		{
			name:    "expires_on before effective_on is refused",
			mutate:  func(t *testing.T, i *Insurance) { i.ExpiresOn = mustDate(t, "2025-01-01") },
			wantErr: true, field: "expires_on",
		},
		{
			name:   "expires_on on or after effective_on is accepted",
			mutate: func(t *testing.T, i *Insurance) { i.ExpiresOn = mustDate(t, "2026-06-01") },
		},
		{
			name:    "an unpublished type is refused",
			mutate:  func(_ *testing.T, i *Insurance) { i.Type = "dragon" },
			wantErr: true, field: "type",
		},
		{
			name:    "an amount without currency is refused",
			mutate:  func(t *testing.T, i *Insurance) { i.Coverage = Coverage{Deductible: money(t, "10.00")} },
			wantErr: true, field: "coverage.currency",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			insurance := validInsurance(t)
			tt.mutate(t, &insurance)

			err := insurance.Validate()
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.field)
		})
	}
}

func TestInsuranceMarshalZerologObject(t *testing.T) {
	t.Parallel()

	insurance := validInsurance(t)
	insurance.GroupNumber = "G-9"
	insurance.HolderName = "Jamie Holder"

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().Object("insurance", insurance).Send()

	out := buf.String()
	assert.Contains(t, out, insurance.ID)
	assert.Contains(t, out, insurance.PatientID)
	assert.NotContains(t, out, insurance.MemberName)
	assert.NotContains(t, out, insurance.MemberID)
	assert.NotContains(t, out, insurance.GroupNumber)
	assert.NotContains(t, out, insurance.HolderName)
}
