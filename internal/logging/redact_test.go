package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
)

const secret = "Amoxicillin 500mg"

func TestSensitiveCannotBeRenderedByAnyOfTheUsualRoutes(t *testing.T) {
	t.Parallel()

	value := Sensitive(secret)

	tests := []struct {
		name string
		got  func() string
	}{
		{name: "String", got: func() string { return value.String() }},
		{name: "GoString", got: func() string { return value.GoString() }},
		{name: "fmt %s", got: func() string { return fmt.Sprintf("<%s>", value) }},
		{name: "fmt %v", got: func() string { return fmt.Sprintf("%v", value) }},
		{name: "fmt %q", got: func() string { return fmt.Sprintf("%q", value) }},
		{name: "fmt %#v", got: func() string { return fmt.Sprintf("%#v", value) }},
		{name: "fmt inside an error", got: func() string { return fmt.Errorf("saving %v failed", value).Error() }},
		{name: "MarshalText", got: func() string {
			raw, err := value.MarshalText()
			require.NoError(t, err)

			return string(raw)
		}},
		{name: "MarshalJSON", got: func() string {
			raw, err := json.Marshal(value)
			require.NoError(t, err)

			return string(raw)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := tt.got()
			assert.NotContains(t, out, secret, "Principle VII: remembering not to log it is not a control")
			assert.Contains(t, out, Placeholder)
		})
	}
}

func TestSensitiveSurvivesEmbeddingInAStruct(t *testing.T) {
	t.Parallel()

	payload := struct {
		ID   string    `json:"id"`
		Name Sensitive `json:"name"`
	}{ID: "abc123", Name: Sensitive(secret)}

	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	assert.NotContains(t, string(raw), secret)
	assert.Contains(t, string(raw), "abc123", "an opaque id is exactly what is allowed through")
}

func TestSensitiveReachesTheLogStreamRedacted(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := NewTo(&buf, config.LogConfig{Level: "info"}, "test")

	value := Sensitive(secret)
	log.Info().Interface("interface", value).Stringer("stringer", value).Msg("saved")

	entries := lines(t, &buf)
	require.Len(t, entries, 1)

	assert.NotContains(t, buf.String(), secret)
	assert.Equal(t, Placeholder, entries[0]["interface"])
	assert.Equal(t, Placeholder, entries[0]["stringer"])
}

func TestSensitiveRedactsTheEmptyValueToo(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Placeholder, Sensitive("").String(),
		"branching on emptiness would turn the placeholder into a presence signal")
}

func TestRevealIsTheOnlyWayOut(t *testing.T) {
	t.Parallel()

	assert.Equal(t, secret, Sensitive(secret).Reveal(),
		"the value has to be usable; what it must not be is accidental")
}
