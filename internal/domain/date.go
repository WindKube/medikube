package domain

import (
	"cmp"
	"database/sql/driver"
	"errors"
	"time"
)

// Date is a calendar date: a year, a month and a day, with no time of day and
// no zone. It is the type every clinical date is held in — a dose taken on the
// third was taken on the third in Auckland and in Los Angeles, and FR-019
// requires it to read that way to every viewer.
//
// A time.Time here would be the bug rather than the implementation. Midnight
// UTC rendered in UTC-5 is the previous day, which silently moves a person's
// medication history by one (research D-27). The type has no member that can
// move, so the failure is not avoided by discipline — it is unrepresentable.
//
// The zero value is the absent date: it is what an unset optional column holds,
// it renders as the empty string and it writes as SQL NULL. Two dates compare
// with ==, so no Equal method is provided.
type Date struct {
	year  int
	month time.Month
	day   int
}

// The wire form, the display form and the stored form are the same string.
// There is no second spelling for a date to drift into.
const dateLayout = time.DateOnly

// The value is never in the message: a start date is medical data, and this
// text reaches the log and Sentry (constitution VII).
var errNotACalendarDate = errors.New("domain: not a calendar date in YYYY-MM-DD form")

// The forms a date column can come back in. PocketBase stores a DateField as
// its own layout, and a driver may hand back an RFC3339 string or a time.Time,
// so all of them are read — and all of them must be midnight UTC.
var instantLayouts = []string{
	"2006-01-02 15:04:05.000Z", // PocketBase's types.DateTime
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05",
}

// NewDate refuses a date the calendar does not have. time.Date would roll
// 30 February forward to 2 March, which is never what the person who typed it
// meant; it is used here only as a calendar, in UTC, and the result is checked
// against what was asked for rather than trusted.
func NewDate(year int, month time.Month, day int) (Date, error) {
	if year < 1 || year > 9999 {
		return Date{}, errNotACalendarDate
	}
	normalised := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if normalised.Year() != year || normalised.Month() != month || normalised.Day() != day {
		return Date{}, errNotACalendarDate
	}
	return Date{year: year, month: month, day: day}, nil
}

// ParseDate reads the one accepted form. The empty string is the absent date,
// which is what an optional field carries; everything else must be exactly
// YYYY-MM-DD, so an instant cannot arrive dressed as a date.
func ParseDate(text string) (Date, error) {
	if text == "" {
		return Date{}, nil
	}
	if len(text) != len(dateLayout) {
		return Date{}, errNotACalendarDate
	}
	parsed, err := time.Parse(dateLayout, text)
	if err != nil {
		return Date{}, errNotACalendarDate
	}
	return NewDate(parsed.Year(), parsed.Month(), parsed.Day())
}

func (d Date) Year() int         { return d.year }
func (d Date) Month() time.Month { return d.month }
func (d Date) Day() int          { return d.day }

func (d Date) IsZero() bool { return d == Date{} }

func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC).Format(dateLayout)
}

// Compare orders by year, then month, then day — the ordering FR-018's
// "ended_on >= started_on" is written against, with equality accepted.
func (d Date) Compare(other Date) int {
	if order := cmp.Compare(d.year, other.year); order != 0 {
		return order
	}
	if order := cmp.Compare(d.month, other.month); order != 0 {
		return order
	}
	return cmp.Compare(d.day, other.day)
}

func (d Date) Before(other Date) bool { return d.Compare(other) < 0 }
func (d Date) After(other Date) bool  { return d.Compare(other) > 0 }

// UTC is the only bridge to an instant, and it lands on midnight UTC. It exists
// for the store, which writes a PocketBase DateField. Converting the result
// into any other zone reintroduces exactly the bug this type prevents.
func (d Date) UTC() time.Time {
	if d.IsZero() {
		return time.Time{}
	}
	return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC)
}

// MarshalText is implemented rather than MarshalJSON so the value carries the
// same spelling through JSON, a URL query, a form and a log field (D-27).
func (d Date) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

func (d *Date) UnmarshalText(text []byte) error {
	parsed, err := ParseDate(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Value writes NULL for the absent date rather than a zeroth day, so an unset
// column stays distinguishable from a set one.
func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

func (d *Date) Scan(src any) error {
	switch value := src.(type) {
	case nil:
		*d = Date{}
		return nil
	case string:
		return d.scanText(value)
	case []byte:
		return d.scanText(string(value))
	case time.Time:
		return d.scanInstant(value)
	default:
		return errNotACalendarDate
	}
}

func (d *Date) scanText(text string) error {
	if len(text) == len(dateLayout) || text == "" {
		return d.UnmarshalText([]byte(text))
	}
	for _, layout := range instantLayouts {
		if parsed, err := time.Parse(layout, text); err == nil {
			return d.scanInstant(parsed)
		}
	}
	return errNotACalendarDate
}

// A date column holding a real time of day is a schema fault, and truncating it
// is how the off-by-one-day comes back: the load succeeds, the record moves,
// and nothing reports it. So it is refused instead.
func (d *Date) scanInstant(instant time.Time) error {
	_, offset := instant.Zone()
	if offset != 0 || instant.Hour() != 0 || instant.Minute() != 0 || instant.Second() != 0 || instant.Nanosecond() != 0 {
		return errNotACalendarDate
	}
	parsed, err := NewDate(instant.Year(), instant.Month(), instant.Day())
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
