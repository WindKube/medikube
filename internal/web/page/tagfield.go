package page

import (
	"context"

	"medikube/internal/domain/access"
	recordfamily "medikube/internal/records"
	"medikube/internal/web/api"
	viewtags "medikube/internal/web/views/tags"
)

// attachTagOptions is the one place every kind's page handler reaches the tag
// service from, before rendering that kind's Form: recordfamily.Views.Form
// has no ctx or actor of its own to fetch the actor's tag catalog with, so
// whichever caller does have them attaches it to the record first
// (recordfamily.AttachTagOptions). A nil resolve — most test harnesses — is a
// deliberate no-op, the same as it is there.
func attachTagOptions(ctx context.Context, actor access.Actor, resolve api.TagResolve, record *recordfamily.Record) error {
	return recordfamily.AttachTagOptions(ctx, actor, recordfamily.TagCatalogResolve(resolve), record)
}

// tagField turns a record's attached tag catalog and its own applied tags
// into tags.Field's props (FR-064, US7): the one bridge every kind's
// Views.Form calls, rather than each kind converting the catalog itself.
func tagField(formID string, record recordfamily.Record) viewtags.FieldProps {
	options := make([]viewtags.TagView, 0, len(record.Tags))
	for _, option := range record.Tags {
		options = append(options, viewtags.TagView{
			ID: option.ID, Name: option.Name, Color: option.Color, UsageCount: option.UsageCount,
		})
	}

	return viewtags.FieldProps{
		FormID:   formID,
		Options:  options,
		Selected: recordfamily.SelectedTagIDs(record.Body),
	}
}
