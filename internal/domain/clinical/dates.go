package clinical

import (
	"time"

	"medikube/internal/domain"
)

// Date is a calendar date — a `*_on` field (data-model §0.10). It is
// domain.Date exactly, not a copy of it: two distinct Go types for one idea
// is the drift research D-03 exists to prevent, and every date-only column in
// this phase already has a type built for the job.
type Date = domain.Date

// Instant is a point in time — a `*_at` field. It is stored as RFC3339 UTC and
// presented in the viewer's local terms at the edge; the type itself carries no
// notion of a viewer, so nothing here can leak one.
//
// A bare time.Time is not used directly because a location travelling with a
// clinical instant is exactly the bug domain.Date's own doc comment warns
// about for dates: research D-03 draws the same line for instants, one type
// forcing UTC in and UTC out.
type Instant struct {
	t time.Time
}

// NewInstant normalises to UTC, so two calls handed the same wall-clock moment
// in two different zones produce the same value.
func NewInstant(t time.Time) Instant { return Instant{t: t.UTC()} }

// Now is the one place this package reads the clock, so a test can freeze
// "the present" by constructing an Instant directly instead.
func Now() Instant { return NewInstant(time.Now()) }

func (i Instant) IsZero() bool    { return i.t.IsZero() }
func (i Instant) Time() time.Time { return i.t }

func (i Instant) Before(other Instant) bool { return i.t.Before(other.t) }
func (i Instant) After(other Instant) bool  { return i.t.After(other.t) }

// String is RFC3339 UTC, the one wire spelling.
func (i Instant) String() string {
	if i.IsZero() {
		return ""
	}
	return i.t.Format(time.RFC3339)
}

func (i Instant) MarshalText() ([]byte, error) { return []byte(i.String()), nil }

func (i *Instant) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*i = Instant{}
		return nil
	}

	parsed, err := time.Parse(time.RFC3339, string(text))
	if err != nil {
		parsed, err = time.Parse(InputLayout, string(text))
	}
	if err != nil {
		return err
	}

	*i = NewInstant(parsed)

	return nil
}

// InputLayout is what a datetime-local control reads and writes: no seconds,
// no zone. Both sides of the form treat it as UTC.
const InputLayout = "2006-01-02T15:04"

// Input is the instant in InputLayout, empty for the zero value.
func (i Instant) Input() string {
	if i.IsZero() {
		return ""
	}
	return i.t.Format(InputLayout)
}
