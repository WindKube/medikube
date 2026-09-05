package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func money(t *testing.T, s string) *Money {
	t.Helper()

	m, err := ParseMoney(s)
	require.NoError(t, err)

	return &m
}

func TestParseMoney(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{name: "whole and cents", in: "12.50", want: 1250},
		{name: "no fraction", in: "10", want: 1000},
		{name: "single fractional digit padded", in: "1.5", want: 150},
		{name: "too much precision", in: "1.505", wantErr: true},
		{name: "negative refused", in: "-1.00", wantErr: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMoney(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Minor)
		})
	}
}

func TestMoneyString(t *testing.T) {
	t.Parallel()

	m := Money{Minor: 1250}
	assert.Equal(t, "12.50", m.String())
}

func TestCoverageValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		coverage Coverage
		wantErr  bool
		field    string
	}{
		{
			name:     "empty coverage is valid",
			coverage: Coverage{},
		},
		{
			name:     "amount without currency is refused",
			coverage: Coverage{Deductible: money(t, "500.00")},
			wantErr:  true,
			field:    "coverage.currency",
		},
		{
			name:     "lowercase currency is refused",
			coverage: Coverage{Deductible: money(t, "500.00"), Currency: "usd"},
			wantErr:  true,
			field:    "coverage.currency",
		},
		{
			name:     "amount with currency is valid",
			coverage: Coverage{Deductible: money(t, "500.00"), Currency: "USD"},
		},
		{
			name: "oop_max below deductible is refused",
			coverage: Coverage{
				Deductible: money(t, "1000.00"), OOPMax: money(t, "500.00"), Currency: "USD",
			},
			wantErr: true,
			field:   "coverage.oop_max",
		},
		{
			name: "oop_max equal to deductible is valid",
			coverage: Coverage{
				Deductible: money(t, "1000.00"), OOPMax: money(t, "1000.00"), Currency: "USD",
			},
		},
		{
			name:     "coinsurance out of range is refused",
			coverage: Coverage{CoinsurancePct: ptr(150.0)},
			wantErr:  true,
			field:    "coverage.coinsurance_pct",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.coverage.Validate("coverage")
			if !tt.wantErr {
				assert.Nil(t, err)
				return
			}

			require.NotNil(t, err)
			assert.Equal(t, tt.field, err.Field)
		})
	}
}

func ptr[T any](v T) *T { return &v }
