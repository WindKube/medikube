// Package tags is the account's tag manager (contracts/pages.md's /tags,
// US7): the templ views manager.templ and picker.templ render, plus the
// wire-shape helpers the page handler and the API's form-patch responses
// share.
package tags

import (
	"context"

	"medikube/internal/domain/tag"
	"medikube/internal/i18n"
	"medikube/internal/web/views/components"
)

// Segment is /tags' own path segment. Tags are not a kind.Kind (they are
// what a record's own tags relation points at), so this is a plain constant
// rather than a kind.Kind.Segment().
const Segment = "tags"

const (
	FieldName  = "name"
	FieldColor = "color"
)

// Links is what a tag's own row and forms need: the one URL every write
// targets.
type Links struct {
	Record string
}

// TagView is one tag as the manager and the picker render it: the wire shape
// plus the derived usage_count neither owns.
type TagView struct {
	ID         string
	Name       string
	Color      string
	UsageCount int
	Links      Links
}

func NewTagView(t tag.Tag, usage int, links Links) TagView {
	return TagView{ID: t.ID, Name: t.Name, Color: t.Color, UsageCount: usage, Links: links}
}

// Value reads a form field's current value off the view, the same shape
// FacilityView.Value follows for its own form.
func (v TagView) Value(field string) string {
	switch field {
	case FieldName:
		return v.Name
	case FieldColor:
		return v.Color
	default:
		return ""
	}
}

// ManagerProps is manager.templ's whole input.
type ManagerProps struct {
	Tags       []TagView
	CreateHref string
	NextHref   string
}

// FormProps is the one form manager.templ renders for both a new tag and a
// rename — FacilityForm's own reasoning, at tag scale.
type FormProps struct {
	FormID   string
	New      bool
	OnSubmit string
	// CancelOn is a Datastar expression that hides the form again; unlike
	// FacilityForm there is no separate page to cancel back to; a tag has no
	// detail page.
	CancelOn string
	Tag      TagView
	Errors   components.FieldErrors
}

func (p FormProps) Label(ctx context.Context) string {
	if p.New {
		return i18n.T(ctx, "action.add_a_tag")
	}

	return i18n.T(ctx, "tag.rename_form", map[string]any{"Name": p.Tag.Name})
}

func (p FormProps) SubmitLabel(ctx context.Context) string {
	if p.New {
		return i18n.T(ctx, "action.add_tag")
	}

	return i18n.T(ctx, "action.save_changes")
}

// PickerProps is picker.templ's input: the candidate tags a text field
// autocompletes against, each with its own usage count (FR-068).
type PickerProps struct {
	ID      string
	Label   string
	Options []TagView
	// Signal is the Datastar signal the typed query is bound to; each
	// option is shown or hidden by comparing itself against it client-side,
	// so the list narrows as the person types with no round trip.
	Signal string
}
