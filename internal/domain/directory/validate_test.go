package directory

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func renderLine(t *testing.T, v zerolog.LogObjectMarshaler) string {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Log().EmbedObject(v).Send()
	return buf.String()
}

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	require.Error(t, err)
	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	got := make([]string, 0, len(invalid.Fields))
	for _, f := range invalid.Fields {
		got = append(got, f.Field)
	}
	return got
}

func TestPractitionerValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(p *Practitioner)
		wantFields []string
	}{
		{name: "valid", mutate: func(p *Practitioner) {}},
		{
			name:       "empty name",
			mutate:     func(p *Practitioner) { p.Name = "  " },
			wantFields: []string{"name"},
		},
		{
			name:       "name too long",
			mutate:     func(p *Practitioner) { p.Name = strings.Repeat("a", 201) },
			wantFields: []string{"name"},
		},
		{
			name:       "invalid specialty",
			mutate:     func(p *Practitioner) { p.Specialty = Specialty("witchcraft") },
			wantFields: []string{"specialty"},
		},
		{
			name:   "unset specialty is accepted",
			mutate: func(p *Practitioner) { p.Specialty = "" },
		},
		{
			name:       "phone too long",
			mutate:     func(p *Practitioner) { p.Phone = strings.Repeat("1", 41) },
			wantFields: []string{"phone"},
		},
		{
			name:       "notes too long",
			mutate:     func(p *Practitioner) { p.Notes = strings.Repeat("n", 5001) },
			wantFields: []string{"notes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := Practitioner{Name: "Dr Chen", Specialty: SpecialtyCardiology}
			tt.mutate(&p)

			err := p.Validate()
			if len(tt.wantFields) == 0 {
				assert.NoError(t, err)
				return
			}
			assert.ElementsMatch(t, tt.wantFields, fieldsOf(t, err))
		})
	}
}

func TestPractitionerMarshalZerologObjectEmitsOnlyIDAndOwnerID(t *testing.T) {
	t.Parallel()

	line := renderLine(t, Practitioner{
		ID:      "prac000000001",
		OwnerID: "usr0000000001",
		Name:    "Dr Amara Chen",
		Phone:   "+1 555 0100",
		Notes:   "recommended by a friend",
	})

	assert.JSONEq(t, `{"id":"prac000000001","owner_id":"usr0000000001"}`, line)
	assert.NotContains(t, line, "Chen")
	assert.NotContains(t, line, "555")
	assert.NotContains(t, line, "friend")
}

func TestFacilityValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(f *Facility)
		wantFields []string
	}{
		{name: "valid", mutate: func(f *Facility) {}},
		{
			name:       "missing kind",
			mutate:     func(f *Facility) { f.Kind = "" },
			wantFields: []string{"kind"},
		},
		{
			name:       "invalid kind",
			mutate:     func(f *Facility) { f.Kind = FacilityKind("clinic") },
			wantFields: []string{"kind"},
		},
		{
			name:       "empty name",
			mutate:     func(f *Facility) { f.Name = "" },
			wantFields: []string{"name"},
		},
		{
			name:       "name too long",
			mutate:     func(f *Facility) { f.Name = strings.Repeat("a", 201) },
			wantFields: []string{"name"},
		},
		{
			name:       "brand too long",
			mutate:     func(f *Facility) { f.Brand = strings.Repeat("b", 121) },
			wantFields: []string{"brand"},
		},
		{
			name:       "invalid email",
			mutate:     func(f *Facility) { f.Email = "not-an-address" },
			wantFields: []string{"email"},
		},
		{
			name:       "email with a display name is refused",
			mutate:     func(f *Facility) { f.Email = "Boots <pharmacy@example.test>" },
			wantFields: []string{"email"},
		},
		{
			name:       "invalid website",
			mutate:     func(f *Facility) { f.Website = "not a url" },
			wantFields: []string{"website"},
		},
		{
			name:       "relative website is refused",
			mutate:     func(f *Facility) { f.Website = "/path/only" },
			wantFields: []string{"website"},
		},
		{
			name:       "invalid portal url",
			mutate:     func(f *Facility) { f.PortalURL = "ftp://example.test" },
			wantFields: []string{"portal_url"},
		},
		{
			name:   "valid website and portal url",
			mutate: func(f *Facility) { f.Website = "https://example.test"; f.PortalURL = "http://portal.example.test" },
		},
		{
			name:       "notes too long",
			mutate:     func(f *Facility) { f.Notes = strings.Repeat("n", 5001) },
			wantFields: []string{"notes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := Facility{Kind: FacilityKindPractice, Name: "Riverside Practice"}
			tt.mutate(&f)

			err := f.Validate()
			if len(tt.wantFields) == 0 {
				assert.NoError(t, err)
				return
			}
			assert.ElementsMatch(t, tt.wantFields, fieldsOf(t, err))
		})
	}
}

func TestFacilityMarshalZerologObjectEmitsOnlyIDAndOwnerID(t *testing.T) {
	t.Parallel()

	line := renderLine(t, Facility{
		ID:      "fac0000000001",
		OwnerID: "usr0000000001",
		Name:    "Riverside Practice",
		Street:  "1 River Lane",
		Notes:   "wheelchair accessible entrance around the back",
	})

	assert.JSONEq(t, `{"id":"fac0000000001","owner_id":"usr0000000001"}`, line)
	assert.NotContains(t, line, "Riverside")
	assert.NotContains(t, line, "River Lane")
	assert.NotContains(t, line, "wheelchair")
}
