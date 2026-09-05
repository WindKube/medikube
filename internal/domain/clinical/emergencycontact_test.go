package clinical

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func validContact() EmergencyContact {
	return EmergencyContact{Name: "Chidi Eze", Relationship: ContactRelationshipSibling, Phone: "+1 555 0100"}
}

func TestEmergencyContactValidate(t *testing.T) {
	t.Parallel()

	t.Run("a minimal valid contact passes", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, validContact().Validate())
	})

	t.Run("name is required", func(t *testing.T) {
		t.Parallel()
		c := validContact()
		c.Name = ""
		err := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "name", invalid.Fields[0].Field)
	})

	t.Run("relationship is required", func(t *testing.T) {
		t.Parallel()
		c := validContact()
		c.Relationship = ""
		err := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "relationship", invalid.Fields[0].Field)
	})

	t.Run("phone is required", func(t *testing.T) {
		t.Parallel()
		c := validContact()
		c.Phone = ""
		err := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "phone", invalid.Fields[0].Field)
	})

	t.Run("the second number, email, address, primary and active flags are optional", func(t *testing.T) {
		t.Parallel()
		c := validContact()
		c.PhoneAlt = "+1 555 0101"
		c.Email = "chidi@example.test"
		c.Address = "1 Main St"
		c.IsPrimary = true
		c.IsActive = true
		assert.NoError(t, c.Validate())
	})

	t.Run("a malformed email is refused", func(t *testing.T) {
		t.Parallel()
		c := validContact()
		c.Email = "not-an-email"
		err := c.Validate()
		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "email", invalid.Fields[0].Field)
	})
}

func TestEmergencyContactMarshalZerologObjectRedactsPatientData(t *testing.T) {
	t.Parallel()

	c := EmergencyContact{
		ID: "contact-1", PatientID: "patient-1",
		Name: "SECRET-NAME", Phone: "SECRET-PHONE", Address: "SECRET-ADDRESS",
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(c).Msg("")

	line := buf.String()
	assert.Contains(t, line, "contact-1")
	assert.NotContains(t, line, "SECRET-NAME")
	assert.NotContains(t, line, "SECRET-PHONE")
	assert.NotContains(t, line, "SECRET-ADDRESS")
}
