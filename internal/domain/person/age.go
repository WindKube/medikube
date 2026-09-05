package person

import (
	"fmt"
	"time"

	"medikube/internal/domain"
)

// Age is a person's age at a point in time, carried as years, months and days
// rather than a single integer year count (FR-006, research D-20). A bare
// int cannot render "0 days" for a patient born today or degrade sensibly for
// an infant, and US4-4 requires both.
//
// The zero value renders as "not recorded", which is what an unset birth date
// carries — there is no pointer here because this type already distinguishes
// the two cases.
type Age struct {
	years, months, days int
	recorded            bool
}

// AgeAt computes birth's age as of on. An empty birth (domain.Date's zero
// value, FR-006/D-09) yields the zero Age, which renders as "not recorded"
// rather than a misleading "0".
//
// The calculation walks calendar fields, not a time.Time subtraction: a
// birth date has no time of day or zone (domain.Date), and subtracting
// durations across months of differing length is the wrong arithmetic for
// "how many years, months and days" in the first place. This also means a
// 29 February birth date evaluated in a non-leap year degrades cleanly to
// months and days instead of requiring a calendar day that does not exist.
func AgeAt(birth domain.Date, on time.Time) Age {
	if birth.IsZero() {
		return Age{}
	}

	by, bm, bd := birth.Year(), int(birth.Month()), birth.Day()
	oy, om, od := on.Year(), int(on.Month()), on.Day()

	years := oy - by
	months := om - bm
	days := od - bd

	if days < 0 {
		months--
		// The last day of the month before on's month, in on's year — this is
		// what borrowing a "month" of days actually means on a calendar.
		days += lastDayOfPreviousMonth(oy, om)
	}
	if months < 0 {
		years--
		months += 12
	}

	return Age{years: years, months: months, days: days, recorded: true}
}

func lastDayOfPreviousMonth(year, month int) int {
	// Day 0 of a month is the last day of the month before it.
	return time.Date(year, time.Month(month), 0, 0, 0, 0, 0, time.UTC).Day()
}

func (a Age) Years() int  { return a.years }
func (a Age) Months() int { return a.months }
func (a Age) Days() int   { return a.days }

// Recorded is false only for the zero Age, i.e. an unset birth date.
func (a Age) Recorded() bool { return a.recorded }

// String is the rendering FR-006 and US4-4 require: full years once at least
// one has elapsed, otherwise months and days, so a newborn reads as "0 days"
// and not "0".
func (a Age) String() string {
	if !a.recorded {
		return "not recorded"
	}
	if a.years >= 1 {
		return plural(a.years, "year")
	}
	if a.months >= 1 {
		return plural(a.months, "month") + ", " + plural(a.days, "day")
	}
	return plural(a.days, "day")
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
