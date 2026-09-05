package tag_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain/tag"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		tag     tag.Tag
		wantErr bool
	}{
		"valid": {
			tag: tag.Tag{Name: "cardiology", Color: "#aa3311"},
		},
		"valid without color": {
			tag: tag.Tag{Name: "cardiology"},
		},
		"empty name": {
			tag:     tag.Tag{Name: "  "},
			wantErr: true,
		},
		"name at max": {
			tag: tag.Tag{Name: strings.Repeat("a", tag.NameMax)},
		},
		"name over max": {
			tag:     tag.Tag{Name: strings.Repeat("a", tag.NameMax+1)},
			wantErr: true,
		},
		"bad color no hash": {
			tag:     tag.Tag{Name: "x", Color: "aa3311"},
			wantErr: true,
		},
		"bad color short": {
			tag:     tag.Tag{Name: "x", Color: "#aa33"},
			wantErr: true,
		},
		"bad color non-hex": {
			tag:     tag.Tag{Name: "x", Color: "#zz3311"},
			wantErr: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := tc.tag.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNameComparisonIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	assert.Equal(t, strings.ToLower("Cardiology"), strings.ToLower("cardiology"))
}
