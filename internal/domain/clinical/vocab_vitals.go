package clinical

import "slices"

// GlucoseContext is the circumstances a blood-glucose reading was taken under.
type GlucoseContext string

const (
	GlucoseContextFasting    GlucoseContext = "fasting"
	GlucoseContextBeforeMeal GlucoseContext = "before_meal"
	GlucoseContextAfterMeal  GlucoseContext = "after_meal"
	GlucoseContextRandom     GlucoseContext = "random"
)

var glucoseContexts = []GlucoseContext{
	GlucoseContextFasting, GlucoseContextBeforeMeal, GlucoseContextAfterMeal, GlucoseContextRandom,
}

func GlucoseContexts() []GlucoseContext { return slices.Clone(glucoseContexts) }

func (v GlucoseContext) Valid() bool { return slices.Contains(glucoseContexts, v) }
