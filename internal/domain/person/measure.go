package person

import (
	"math"
	"strconv"

	"medikube/internal/domain/identity"
)

// Height and weight live here rather than a shared internal/domain/units
// package: a patient's are the only consumers this phase has, and extracting
// a shared package before a second one exists is speculative (Principle I,
// research D-21).
//
// FormatHeight and FormatWeight are pure: heightCM and weightKG are canonical
// SI, passed by value, and are never written back to — changing the display
// preference never alters what was recorded (FR-007).

const (
	cmPerInch = 2.54
	ozPerKg   = 35.27396195
)

// FormatHeight renders heightCM in the given unit system. Zero (below the
// 30..272 valid range) is "not set" and renders as the empty string.
func FormatHeight(heightCM float64, system identity.UnitSystem) string {
	if heightCM <= 0 {
		return ""
	}
	if system == identity.UnitSystemImperial {
		totalInches := int(math.Round(heightCM / cmPerInch))
		feet := totalInches / 12
		inches := totalInches % 12
		return strconv.Itoa(feet) + " ft " + strconv.Itoa(inches) + " in"
	}
	return strconv.FormatFloat(heightCM, 'f', -1, 64) + " cm"
}

// FormatWeight renders weightKG in the given unit system. Zero (below the
// 0.5..450 valid range) is "not set" and renders as the empty string.
func FormatWeight(weightKG float64, system identity.UnitSystem) string {
	if weightKG <= 0 {
		return ""
	}
	if system == identity.UnitSystemImperial {
		totalOunces := int(math.Round(weightKG * ozPerKg))
		pounds := totalOunces / 16
		ounces := totalOunces % 16
		return strconv.Itoa(pounds) + " lb " + strconv.Itoa(ounces) + " oz"
	}
	return strconv.FormatFloat(weightKG, 'f', -1, 64) + " kg"
}
