package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// data-model §2. Each vocabulary's published set, pinned as a literal so this
// test proves the code matches the spec rather than the spec matches the code.
func TestPerKindVocabulariesMatchDataModelSection2(t *testing.T) {
	t.Parallel()

	t.Run("Laterality", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, Lateralities(), "left", "right", "bilateral", "not_applicable")
	})
	t.Run("ImmunizationRoute", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, ImmunizationRoutes(), "intramuscular", "subcutaneous", "intradermal", "oral", "intranasal")
	})
	t.Run("ImmunizationSite", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, ImmunizationSites(),
			"left_arm", "right_arm", "left_thigh", "right_thigh", "oral", "nasal", "other")
	})
	t.Run("ProcedureSetting", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, ProcedureSettings(), "outpatient", "inpatient", "office")
	})
	t.Run("ProcedureType", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, ProcedureTypes(), "surgical", "diagnostic", "therapeutic", "preventive", "other")
	})
	t.Run("ProcedureOutcome", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, ProcedureOutcomes(), "successful", "partial", "unsuccessful", "complications")
	})
	t.Run("Anesthesia", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, Anesthesias(), "none", "local", "regional", "sedation", "general")
	})
	t.Run("VisitType", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, VisitTypes(),
			"office", "telehealth", "urgent_care", "emergency", "inpatient", "follow_up", "annual", "other")
	})
	t.Run("VisitPriority", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, VisitPriorities(), "routine", "urgent", "emergency")
	})
	t.Run("TreatmentSetting", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, TreatmentSettings(), "inpatient", "outpatient", "home")
	})
	t.Run("SymptomCategory", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, SymptomCategories(),
			"pain", "respiratory", "gastrointestinal", "neurological", "cardiovascular",
			"musculoskeletal", "dermatological", "psychological", "constitutional", "other")
	})
	t.Run("SymptomImpact", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, SymptomImpacts(), "none", "mild", "moderate", "severe")
	})
	t.Run("GlucoseContext", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, GlucoseContexts(), "fasting", "before_meal", "after_meal", "random")
	})
	t.Run("InsuranceType", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, InsuranceTypes(), "medical", "dental", "vision", "prescription", "other")
	})
	t.Run("InsuranceStatus", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, InsuranceStatuses(), "active", "inactive", "expired", "pending")
	})
	t.Run("HolderRelationship", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, HolderRelationships(), "self", "spouse", "child", "dependent", "other")
	})
	t.Run("ContactRelationship", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, ContactRelationships(),
			"spouse", "partner", "parent", "child", "sibling", "friend", "guardian", "caregiver", "other")
	})
	t.Run("EquipmentType", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, EquipmentTypes(),
			"cpap", "nebulizer", "wheelchair", "walker", "glucose_meter", "bp_monitor",
			"oximeter", "oxygen", "hearing_aid", "prosthetic", "orthotic", "other")
	})
	t.Run("InjuryType", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, InjuryTypes(),
			"sprain", "strain", "fracture", "dislocation", "laceration", "contusion",
			"burn", "concussion", "puncture", "abrasion", "other")
	})
	t.Run("FamilyRelationship", func(t *testing.T) {
		t.Parallel()
		assertVocab(t, FamilyRelationships(),
			"mother", "father", "sister", "brother", "daughter", "son", "grandmother",
			"grandfather", "aunt", "uncle", "cousin", "niece", "nephew", "half_sibling", "other")
	})
}

// Every vocabulary marked with a catch-all in data-model §2 contains "other".
func TestEveryCatchAllVocabularyContainsOther(t *testing.T) {
	t.Parallel()

	assert.Contains(t, ImmunizationSites(), ImmunizationSite("other"))
	assert.Contains(t, ProcedureTypes(), ProcedureType("other"))
	assert.Contains(t, VisitTypes(), VisitType("other"))
	assert.Contains(t, SymptomCategories(), SymptomCategory("other"))
	assert.Contains(t, InsuranceTypes(), InsuranceType("other"))
	assert.Contains(t, HolderRelationships(), HolderRelationship("other"))
	assert.Contains(t, ContactRelationships(), ContactRelationship("other"))
	assert.Contains(t, EquipmentTypes(), EquipmentType("other"))
	assert.Contains(t, InjuryTypes(), InjuryType("other"))
	assert.Contains(t, FamilyRelationships(), FamilyRelationship("other"))
}

type stringVocab interface {
	~string
	Valid() bool
}

func assertVocab[T stringVocab](t *testing.T, got []T, want ...string) {
	t.Helper()

	strs := make([]string, 0, len(got))
	for _, v := range got {
		strs = append(strs, string(v))
	}
	assert.Equal(t, want, strs)

	for _, v := range got {
		assert.Truef(t, v.Valid(), "%v does not validate itself", v)
	}
}
