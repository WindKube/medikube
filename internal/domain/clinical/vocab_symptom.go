package clinical

import "slices"

// SymptomCategory groups a symptom episode, with a catch-all.
type SymptomCategory string

const (
	SymptomCategoryPain             SymptomCategory = "pain"
	SymptomCategoryRespiratory      SymptomCategory = "respiratory"
	SymptomCategoryGastrointestinal SymptomCategory = "gastrointestinal"
	SymptomCategoryNeurological     SymptomCategory = "neurological"
	SymptomCategoryCardiovascular   SymptomCategory = "cardiovascular"
	SymptomCategoryMusculoskeletal  SymptomCategory = "musculoskeletal"
	SymptomCategoryDermatological   SymptomCategory = "dermatological"
	SymptomCategoryPsychological    SymptomCategory = "psychological"
	SymptomCategoryConstitutional   SymptomCategory = "constitutional"
	SymptomCategoryOther            SymptomCategory = "other"
)

// SymptomImpact is how much it interfered.
type SymptomImpact string

const (
	SymptomImpactNone     SymptomImpact = "none"
	SymptomImpactMild     SymptomImpact = "mild"
	SymptomImpactModerate SymptomImpact = "moderate"
	SymptomImpactSevere   SymptomImpact = "severe"
)

var (
	symptomCategories = []SymptomCategory{
		SymptomCategoryPain, SymptomCategoryRespiratory, SymptomCategoryGastrointestinal,
		SymptomCategoryNeurological, SymptomCategoryCardiovascular, SymptomCategoryMusculoskeletal,
		SymptomCategoryDermatological, SymptomCategoryPsychological, SymptomCategoryConstitutional,
		SymptomCategoryOther,
	}

	symptomImpacts = []SymptomImpact{SymptomImpactNone, SymptomImpactMild, SymptomImpactModerate, SymptomImpactSevere}
)

func SymptomCategories() []SymptomCategory { return slices.Clone(symptomCategories) }
func SymptomImpacts() []SymptomImpact      { return slices.Clone(symptomImpacts) }

func (v SymptomCategory) Valid() bool { return slices.Contains(symptomCategories, v) }
func (v SymptomImpact) Valid() bool   { return slices.Contains(symptomImpacts, v) }
