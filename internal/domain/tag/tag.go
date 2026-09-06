// Package tag is data-model §5.1's account-owned tag vocabulary.
package tag

import (
	"strings"
	"time"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

const (
	NameMin = 1
	NameMax = 40
)

// ColorPattern is data-model §5.1's column pattern, checked again here
// (belt and suspenders): the migration's TextField.Pattern is the storage
// layer's own copy, this is the domain's.
const ColorPattern = `^#[0-9a-fA-F]{6}$`

// Tag is one entry of an account's own vocabulary (data-model §5.1). It
// belongs to the account, not to a patient (FR-062).
type Tag struct {
	ID      string
	OwnerID string

	Name  string
	Color string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

// Validate enforces FR-063's shape: a name of 1..40 characters and a color
// matching ColorPattern when one is supplied. Uniqueness, case-insensitive per
// owner, is enforced by the service against the repository and by the storage
// layer's own index — neither is a property Validate can see on its own.
func (t Tag) Validate() error {
	var invalid domain.ValidationError

	if name := strings.TrimSpace(t.Name); name == "" {
		invalid.Add("name", domain.CodeRequired, "a name is required")
	} else if utf8Len(name) > NameMax {
		invalid.Addf("name", domain.CodeTooLong, "the name accepts at most %d characters", NameMax)
	}

	if t.Color != "" && !colorMatches(t.Color) {
		invalid.Add("color", domain.CodeInvalidValue, "the color must be a 6-digit hex code, such as #aa3311")
	}

	return invalid.OrNil()
}

// MarshalZerologObject emits the two identifiers and nothing else. The name is
// PHI-adjacent (FR-085, data-model §5.1) — a tag may name a condition — and
// never reaches this method.
func (t Tag) MarshalZerologObject(ev *zerolog.Event) {
	ev.Str("tag_id", t.ID).Str("owner_id", t.OwnerID)
}

func colorMatches(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}

	for i := 1; i < 7; i++ {
		c := color[i]

		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}

	return true
}

func utf8Len(s string) int {
	count := 0
	for range s {
		count++
	}

	return count
}
