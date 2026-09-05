package link

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

// treatmentMedicationsCollection and its two relation fields mirror
// internal/store/migrations' own unexported constants: duplicated rather than
// imported, the same way every store package already names its own field
// constants independently.
const (
	treatmentMedicationFieldTreatment  = "treatment"
	treatmentMedicationFieldMedication = "medication"
)

// treatmentMedicationsCollection is derived from the medications collection's
// own name rather than spelled whole (research D-05).
var treatmentMedicationsCollection = "treatment_" + kind.Medication.Collection()

// Ref is one back-related record: enough to render FR-059's "what type it is
// and enough identifying detail to be recognised" once a caller resolves it
// through that kind's own registered Service.Get — the same hydration the
// cross-kind list already does, and never a second copy of the target's
// detail.
type Ref struct {
	Kind kind.Kind
	ID   string
}

// source is one stored relation this package knows how to read backwards:
// collection is the collection carrying the field, field is the relation
// field's own column, and multi says whether it is a MaxSelect:0 array
// (matched with AnyOf) or a single relation (matched with Equal) — the two
// are stored differently and PocketBase's own `~` operator is the only thing
// that used to paper over that, which is exactly the filter-DSL string this
// package now builds a store.Query instead of.
type source struct {
	kind       kind.Kind
	collection string
	field      string
	multi      bool
}

// conditionSources is data-model §7.3's back-relation map for conditions,
// restricted to what this phase actually stores: encounters, procedures and
// treatments each carry a single `condition` relation. symptoms_via_conditions
// and injuries_via_conditions name fields data-model §7.1 does not (yet) add
// to symptoms or injuries, so they are not read here.
func conditionSources() []source {
	return []source{
		{kind: kind.Encounter, collection: kind.Encounter.Collection(), field: "condition"},
		{kind: kind.Procedure, collection: kind.Procedure.Collection(), field: "condition"},
		{kind: kind.Treatment, collection: kind.Treatment.Collection(), field: "condition"},
	}
}

// medicationSources is data-model §7.3's back-relation map for medications:
// every multi-relation field migration 17 and US4's own migration add that
// points at a medication. treatment_medications is read separately
// (TreatmentMedicationRefs) because it is a join row, not a bare relation
// field.
func medicationSources() []source {
	return []source{
		{kind: kind.Allergy, collection: kind.Allergy.Collection(), field: kind.Medication.Collection(), multi: true},
		{kind: kind.Condition, collection: kind.Condition.Collection(), field: kind.Medication.Collection(), multi: true},
		{kind: kind.Injury, collection: kind.Injury.Collection(), field: "medication_ids", multi: true},
		{kind: kind.Symptom, collection: kind.Symptom.Collection(), field: "treated_by_" + kind.Medication.Collection(), multi: true},
		{kind: kind.Symptom, collection: kind.Symptom.Collection(), field: "caused_by_" + kind.Medication.Collection(), multi: true},
	}
}

// Backrelations reads a kind's own back-relations directly off the schema:
// the relationship is recorded once, on the referencing side, and read from
// both ends (FR-055) — this is the read of the end that stores nothing.
type Backrelations struct {
	app core.App
}

func NewBackrelations(app core.App) (*Backrelations, error) {
	if app == nil {
		return nil, fmt.Errorf("store/link: the back-relation reader is wired with no application")
	}

	return &Backrelations{app: app}, nil
}

// Conditions is every encounter, procedure and treatment naming this
// condition.
func (b *Backrelations) Conditions(ctx context.Context, conditionID string) ([]Ref, error) {
	return b.find(ctx, conditionSources(), conditionID)
}

// Medications is every allergy, condition, injury and symptom (in either
// role) naming this medication. It does not include treatment_medications
// rows — TreatmentMedicationRefs reads those.
func (b *Backrelations) Medications(ctx context.Context, medicationID string) ([]Ref, error) {
	return b.find(ctx, medicationSources(), medicationID)
}

// TreatmentMedicationTreatments is every treatment this medication is
// attached to, via treatment_medications.
func (b *Backrelations) TreatmentMedicationTreatments(ctx context.Context, medicationID string) ([]Ref, error) {
	return b.findByFilter(ctx, kind.Treatment, treatmentMedicationsCollection,
		treatmentMedicationFieldTreatment, treatmentMedicationFieldMedication, medicationID)
}

// TreatmentMedicationMedications is every medication attached to this
// treatment, via treatment_medications.
func (b *Backrelations) TreatmentMedicationMedications(ctx context.Context, treatmentID string) ([]Ref, error) {
	return b.findByFilter(ctx, kind.Medication, treatmentMedicationsCollection,
		treatmentMedicationFieldMedication, treatmentMedicationFieldTreatment, treatmentID)
}

func (b *Backrelations) find(ctx context.Context, sources []source, targetID string) ([]Ref, error) {
	var refs []Ref

	for _, s := range sources {
		matches, err := b.matchingRecords(ctx, s.collection, s.field, s.multi, targetID)
		if err != nil {
			return nil, err
		}

		for _, record := range matches {
			refs = append(refs, Ref{Kind: s.kind, ID: record.Id})
		}
	}

	return refs, nil
}

// findByFilter is TreatmentMedicationTreatments/Medications' shared shape:
// read the join's `own` relation column (already the resolved kind's id) off
// every row whose `matchField` names targetID. Both are single-relation
// columns on the join row, so the match is Equal rather than AnyOf.
func (b *Backrelations) findByFilter(
	ctx context.Context, resultKind kind.Kind, collection, ownField, matchField, targetID string,
) ([]Ref, error) {
	records, err := b.matchingRecords(ctx, collection, matchField, false, targetID)
	if err != nil {
		return nil, err
	}

	refs := make([]Ref, 0, len(records))
	for _, record := range records {
		refs = append(refs, Ref{Kind: resultKind, ID: record.GetString(ownField)})
	}

	return refs, nil
}

// matchingRecords is the one query every back-relation read runs: which rows
// of `collection` name targetID in `field`, single-relation (Equal) or multi
// (AnyOf, a MaxSelect:0 JSON array). It builds a store.Query against a
// throwaway single-column Schema rather than PocketBase's filter DSL — the
// column is FilterOnly because nothing here ever orders by it.
func (b *Backrelations) matchingRecords(
	ctx context.Context, collection, field string, multi bool, targetID string,
) ([]*core.Record, error) {
	schema := store.NewSchema(collection, store.Column{Name: field, FilterOnly: true})

	condition := store.Equal(field, targetID)
	if multi {
		condition = store.AnyOf(field, targetID)
	}

	built, err := schema.Build(store.Query{Conditions: []store.Condition{condition}, Limit: store.MaxLimit})
	if err != nil {
		return nil, fmt.Errorf("store/link: building the query against %s.%s: %w", collection, field, err)
	}

	var records []*core.Record
	if err := built.Apply(b.app.RecordQuery(collection)).WithContext(ctx).All(&records); err != nil {
		return nil, fmt.Errorf("store/link: reading %s.%s for %s: %w", collection, field, targetID, err)
	}

	return records, nil
}
