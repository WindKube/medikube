package audit

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
)

func minimalEvent() Event {
	return Event{
		OccurredAt: time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC),
		ActorID:    "usr0000000001",
		ActorKind:  ActorKindUser,
		Action:     ActionLogin,
		TargetKind: TargetKindUser,
		TargetID:   "usr0000000001",
		RequestID:  "01K3Q8Z0000000000000000000",
	}
}

func refusals(t *testing.T, err error) []domain.FieldError {
	t.Helper()

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid, "Validate must return a *domain.ValidationError")

	return invalid.Fields
}

func refusedFields(t *testing.T, err error) []string {
	t.Helper()

	fields := refusals(t, err)
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.Field)
	}

	return names
}

// This is the whole point of the type, so it is the first test in the file.
// FR-038 is structural rather than procedural only for as long as the struct
// below has nowhere to put a name, a value, a note or a diff — and the only
// thing standing between "nowhere to put it" and "somebody added a column" is
// this assertion. A field added here is not a merge conflict to resolve; it is
// a privacy decision that has to be argued in the spec first.
func TestEventHasNoFieldThatContentCouldBeWrittenInto(t *testing.T) {
	t.Parallel()

	type column struct {
		name string
		typ  string
	}

	// data-model §3, in the order the table declares it.
	want := []column{
		{name: "OccurredAt", typ: "time.Time"},
		{name: "ActorID", typ: "string"},
		{name: "ActorKind", typ: "audit.ActorKind"},
		{name: "Action", typ: "audit.Action"},
		{name: "TargetKind", typ: "audit.TargetKind"},
		{name: "TargetID", typ: "string"},
		{name: "RequestID", typ: "string"},
	}

	fields := reflect.VisibleFields(reflect.TypeFor[Event]())

	got := make([]column, 0, len(fields))
	for _, field := range fields {
		got = append(got, column{name: field.Name, typ: field.Type.String()})
	}

	t.Run("the fields are exactly data-model §3's columns", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, want, got,
			"the audit event grew a field: FR-036 enumerates the columns exhaustively and FR-038 forbids content in any of them")
	})

	t.Run("no field is named for something content-shaped", func(t *testing.T) {
		t.Parallel()

		// Not the guard — the list above is. This is the second line of it, so
		// that renaming a field past the exact-match assertion still trips.
		forbidden := []string{
			"content", "body", "value", "note", "message", "detail", "description",
			"text", "payload", "diff", "before", "after", "comment", "data", "email",
			"name", "file", "path", "dose", "reason",
		}

		for _, field := range got {
			for _, token := range forbidden {
				assert.NotContains(t, strings.ToLower(field.name), token,
					"%q reads like a field a value could be written into", field.name)
			}
		}
	})

	t.Run("every free string is a bounded identifier", func(t *testing.T) {
		t.Parallel()

		// The typed fields are closed vocabularies and the timestamp is a clock
		// reading. Everything else is a plain string, and a plain string is
		// where content would arrive, so the set of them is enumerated and each
		// one is length-bounded by Validate.
		var free []string
		for _, field := range got {
			if field.typ == "string" {
				free = append(free, field.name)
			}
		}

		assert.Equal(t, []string{"ActorID", "TargetID", "RequestID"}, free)
	})

	t.Run("no field carries a wire name", func(t *testing.T) {
		t.Parallel()

		for _, field := range fields {
			assert.Empty(t, string(field.Tag),
				"%s carries a struct tag; a domain type has no wire form and an audit row is not serialised by the domain", field.Name)
		}
	})
}

func TestAValidEventIsAccepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() Event
	}{
		{
			name:  "a person acting on a record",
			build: minimalEvent,
		},
		{
			name: "a system action has no actor",
			build: func() Event {
				event := minimalEvent()
				event.ActorID = ""
				event.ActorKind = ActorKindSystem
				event.Action = ActionDelete
				event.TargetKind = TargetKindSystem
				event.TargetID = "medikube_purge_artifacts"

				return event
			},
		},
		{
			name: "a superuser session has no target id",
			build: func() Event {
				event := minimalEvent()
				event.ActorKind = ActorKindSuperuser
				event.Action = ActionAdminSession
				event.TargetID = ""

				return event
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.NoError(t, test.build().Validate())
		})
	}
}

func TestAnEventMissingOrMisspellingAColumnIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() Event
		field string
		code  string
	}{
		{
			name:  "no timestamp",
			build: func() Event { e := minimalEvent(); e.OccurredAt = time.Time{}; return e },
			field: "occurred_at",
			code:  domain.CodeRequired,
		},
		{
			name:  "no actor kind",
			build: func() Event { e := minimalEvent(); e.ActorKind = ""; return e },
			field: "actor_kind",
			code:  domain.CodeRequired,
		},
		{
			name:  "an undeclared actor kind",
			build: func() Event { e := minimalEvent(); e.ActorKind = "anonymous"; return e },
			field: "actor_kind",
			code:  domain.CodeInvalidValue,
		},
		{
			name:  "no action",
			build: func() Event { e := minimalEvent(); e.Action = ""; return e },
			field: "action",
			code:  domain.CodeRequired,
		},
		{
			name:  "an undeclared action",
			build: func() Event { e := minimalEvent(); e.Action = "read"; return e },
			field: "action",
			code:  domain.CodeInvalidValue,
		},
		{
			name:  "no target kind",
			build: func() Event { e := minimalEvent(); e.TargetKind = ""; return e },
			field: "target_kind",
			code:  domain.CodeRequired,
		},
		{
			name:  "an undeclared target kind",
			build: func() Event { e := minimalEvent(); e.TargetKind = "practitioner"; return e },
			field: "target_kind",
			code:  domain.CodeInvalidValue,
		},
		{
			// The column is required precisely so a row that correlates to
			// nothing cannot be written: a background run mints a run id from
			// the same helper an HTTP request uses (data-model §3).
			name:  "no request id",
			build: func() Event { e := minimalEvent(); e.RequestID = ""; return e },
			field: "request_id",
			code:  domain.CodeRequired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fields := refusals(t, test.build().Validate())
			require.Len(t, fields, 1, "expected exactly one refusal, got %v", fields)

			assert.Equal(t, test.field, fields[0].Field)
			assert.Equal(t, test.code, fields[0].Code)
		})
	}
}

// FR-027's rule holds here too: every offending column at once, so a caller
// fixing a hand-built event is not led through them one round trip at a time.
func TestEveryOffendingColumnIsReportedAtOnce(t *testing.T) {
	t.Parallel()

	err := Event{TargetID: strings.Repeat("t", MaxTargetID+1)}.Validate()

	assert.ElementsMatch(t,
		[]string{"occurred_at", "actor_kind", "action", "target_kind", "target_id", "request_id"},
		refusedFields(t, err))
}

// data-model §3 sizes both text columns at 64 and gives the arithmetic. The
// arithmetic is asserted rather than restated: the longest name the suite
// composes is phase 006's restore safety copy over a manual archive, and it has
// to fit.
func TestTheIdentifierColumnsAreBoundedAt64(t *testing.T) {
	t.Parallel()

	t.Run("the longest name any phase composes fits", func(t *testing.T) {
		t.Parallel()

		longest := "medikube_safety_20260827120000_medikube_20260827120000.zip"

		require.LessOrEqual(t, len(longest), MaxTargetID,
			"the bound no longer holds the name data-model §3 sized it for")

		event := minimalEvent()
		event.TargetKind = TargetKindBackup
		event.TargetID = longest

		assert.NoError(t, event.Validate())
	})

	bounds := []struct {
		name    string
		build   func(string) Event
		field   string
		maximum int
	}{
		{
			name:    "target_id",
			build:   func(value string) Event { e := minimalEvent(); e.TargetID = value; return e },
			field:   "target_id",
			maximum: MaxTargetID,
		},
		{
			name:    "request_id",
			build:   func(value string) Event { e := minimalEvent(); e.RequestID = value; return e },
			field:   "request_id",
			maximum: MaxRequestID,
		},
	}

	for _, bound := range bounds {
		t.Run(bound.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, 64, bound.maximum, "data-model §3 sizes the column at 64")

			t.Run("at the bound", func(t *testing.T) {
				t.Parallel()

				assert.NoError(t, bound.build(strings.Repeat("x", bound.maximum)).Validate())
			})

			t.Run("one past the bound", func(t *testing.T) {
				t.Parallel()

				fields := refusals(t, bound.build(strings.Repeat("x", bound.maximum+1)).Validate())
				require.Len(t, fields, 1)

				assert.Equal(t, bound.field, fields[0].Field)
				assert.Equal(t, domain.CodeTooLong, fields[0].Code)
				assert.NotContains(t, fields[0].Message, "xxx",
					"a refusal quotes the limit, never the value it refused")
			})

			t.Run("counted in runes, as the storage layer counts it", func(t *testing.T) {
				t.Parallel()

				assert.NoError(t, bound.build(strings.Repeat("é", bound.maximum)).Validate())
			})
		})
	}
}
