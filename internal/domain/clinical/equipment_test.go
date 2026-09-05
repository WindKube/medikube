package clinical

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validEquipment() Equipment {
	return Equipment{
		ID:        "eq1",
		PatientID: "pat1",
		Name:      "CPAP machine",
		Type:      EquipmentTypeCPAP,
	}
}

func TestEquipmentValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(t *testing.T, e *Equipment)
		wantErr bool
		field   string
	}{
		{name: "a minimal valid record", mutate: func(*testing.T, *Equipment) {}},
		{name: "name is required", mutate: func(_ *testing.T, e *Equipment) { e.Name = "" }, wantErr: true, field: "name"},
		{name: "name too short is refused", mutate: func(_ *testing.T, e *Equipment) { e.Name = "x" }, wantErr: true, field: "name"},
		{name: "type is required", mutate: func(_ *testing.T, e *Equipment) { e.Type = "" }, wantErr: true, field: "type"},
		{
			name:    "an unpublished type is refused",
			mutate:  func(_ *testing.T, e *Equipment) { e.Type = "spaceship" },
			wantErr: true, field: "type",
		},
		{
			name: "service_due_on before serviced_on is refused",
			mutate: func(t *testing.T, e *Equipment) {
				e.ServicedOn = mustDate(t, "2026-06-01")
				e.ServiceDueOn = mustDate(t, "2026-01-01")
			},
			wantErr: true, field: "service_due_on",
		},
		{
			name: "service_due_on on or after serviced_on is accepted",
			mutate: func(t *testing.T, e *Equipment) {
				e.ServicedOn = mustDate(t, "2026-01-01")
				e.ServiceDueOn = mustDate(t, "2026-06-01")
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			equipment := validEquipment()
			tt.mutate(t, &equipment)

			err := equipment.Validate()
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.field)
		})
	}
}

func TestEquipmentMarshalZerologObject(t *testing.T) {
	t.Parallel()

	equipment := validEquipment()
	equipment.Serial = "SN-123456"

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().Object("equipment", equipment).Send()

	out := buf.String()
	assert.Contains(t, out, equipment.ID)
	assert.Contains(t, out, equipment.PatientID)
	assert.NotContains(t, out, equipment.Serial)
}
