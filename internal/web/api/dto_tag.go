package api

import (
	dtag "medikube/internal/domain/tag"
)

// Tag is what every tag operation returns. usage_count is derived
// (FR-090, contracts/tags.md §1) and never a stored column.
type Tag struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	UsageCount int    `json:"usage_count"`
}

// TagCreate is createTag's body. There is no owner and no id: the actor is
// the owner and unknown members are rejected by the decoder.
type TagCreate struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// TagPatch is updateTag's body. Both fields optional; there is no If-Match
// (contracts/tags.md §3).
type TagPatch struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// tagDTO renders one tag with its usage count.
func tagDTO(t dtag.Tag, usage int) *Tag {
	return &Tag{ID: t.ID, Name: t.Name, Color: t.Color, UsageCount: usage}
}

// tagDraft reads a create body into the domain entity. Owner, id and the
// timestamps are the service's to set.
func tagDraft(create TagCreate) dtag.Tag {
	return dtag.Tag{Name: create.Name, Color: create.Color}
}
