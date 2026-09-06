package page

import (
	"context"

	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	recordfamily "medikube/internal/records"
	"medikube/internal/web/api"
	views "medikube/internal/web/views/records"
)

// patientMedicationOptions is FR-055's editor picker, shared by every kind
// that links to a medication: the patient's own medications, offered by id
// and name off the same generic list the /medications page itself reads.
func patientMedicationOptions(
	ctx context.Context, handler *recordfamily.Handler, actor access.Actor, patientID string,
) ([]views.MedicationLinkOption, error) {
	if patientID == "" {
		return nil, nil
	}

	listing, err := handler.ListOfKind(ctx, actor, kind.Medication.Segment(), recordfamily.Query{PatientID: patientID})
	if err != nil {
		return nil, err
	}

	options := make([]views.MedicationLinkOption, 0, len(listing.Items))

	for _, item := range listing.Items {
		summary, ok := item.Body.(*api.MedicationSummary)
		if !ok {
			continue
		}

		options = append(options, views.MedicationLinkOption{ID: summary.ID, Name: summary.Name})
	}

	return options, nil
}

// medicationLinkRole resolves one multi-relation field's currently-linked
// medications into the editor's own shape, naming each by the picker's own
// options rather than a second read per id.
func medicationLinkRole(
	field, label string, ids []string, options []views.MedicationLinkOption, href func(string) string,
) views.MedicationLinkRole {
	names := make(map[string]string, len(options))
	for _, option := range options {
		names[option.ID] = option.Name
	}

	items := make([]views.LinkedRecordItem, 0, len(ids))

	for _, id := range ids {
		name := names[id]
		if name == "" {
			name = id
		}

		items = append(items, views.LinkedRecordItem{Kind: string(kind.Medication), Summary: name, Href: href(id)})
	}

	return views.MedicationLinkRole{Field: field, Label: label, Items: items, IDs: ids}
}
