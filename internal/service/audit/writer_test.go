package audit

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainaudit "medikube/internal/domain/audit"
)

// writtenByThisPhase is the ten actions phase 001 produces. The vocabulary
// declares twenty — the other ten are declared complete so that no later
// phase's migration has to widen a select field — and this is the subset a row
// can reach the trail through today.
var writtenByThisPhase = []domainaudit.Action{
	domainaudit.ActionCreate,
	domainaudit.ActionUpdate,
	domainaudit.ActionDelete,
	domainaudit.ActionAccessDenied,
	domainaudit.ActionLogin,
	domainaudit.ActionLoginFailed,
	domainaudit.ActionLogout,
	domainaudit.ActionPasswordChange,
	domainaudit.ActionAccountDelete,
	domainaudit.ActionAdminSession,
}

func minimal(action domainaudit.Action) domainaudit.Event {
	return domainaudit.Event{
		OccurredAt: time.Now().UTC(),
		ActorID:    "mkactor00000001",
		ActorKind:  domainaudit.ActorKindUser,
		Action:     action,
		TargetKind: domainaudit.TargetKindMedication,
		TargetID:   "mkrecord0000001",
		RequestID:  "0123456789abcdef0123456789abcdef",
	}
}

func writer(t *testing.T, options ...Option) (*Writer, *trail) {
	t.Helper()

	store := newTrail()

	w, err := New(store, options...)
	require.NoError(t, err)

	return w, store
}

func TestRecordWritesEveryActionThisPhaseProduces(t *testing.T) {
	t.Parallel()

	require.Len(t, writtenByThisPhase, 10,
		"phase 001 writes ten actions; the table has drifted from the ten and would pass while covering fewer")

	for _, action := range writtenByThisPhase {
		require.True(t, action.Valid(), "%q is not a declared action, so no store would accept the row", action)
	}

	for _, action := range writtenByThisPhase {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()

			w, store := writer(t)

			require.NoError(t, w.Record(t.Context(), minimal(action)))

			rows := store.Rows()
			require.Len(t, rows, 1)
			assert.Equal(t, action, rows[0].Action)
			assert.NoError(t, rows[0].Validate(), "the writer appended a row the store would refuse")
		})
	}
}

// eventFields is the complete inventory of domain audit.Event, keyed by field
// name and valued by the reason that field cannot carry a word of what a record
// says. It is an inventory and not an exemption list: a field absent from it is
// a failure, and so is a field here that is no longer on the type.
//
// FR-038 is structural rather than a rule somebody remembers at each call site,
// and this is where that is true. A leak through the trail would have to be a
// new field on this type, which is a spec decision, and this test is what makes
// it one.
var eventFields = map[string]string{
	"OccurredAt": "a server clock reading",
	"ActorID":    "an account identifier, minted by the store",
	"ActorKind":  "one of four declared values",
	"Action":     "one of twenty-one declared values",
	"TargetKind": "one of twenty-five declared values",
	"TargetID":   "an opaque record identifier, bounded at MaxTargetID",
	"RequestID":  "a correlation handle, bounded at MaxRequestID",
	"PatientID":  "the person a patient-scoped action concerned, bounded at MaxPatientID",
}

// contentWords are the field names a value, a name, a note or a diff would
// arrive under. The inventory above already refuses any new field; this names
// why, so the failure reads as the leak it is rather than as a count that moved.
var contentWords = []string{
	"name", "note", "value", "dose", "reason", "description", "detail",
	"payload", "body", "content", "diff", "before", "after", "text",
	"comment", "message", "data", "meta", "extra",
}

func TestTheEventTypeHasNoFieldThatCouldCarryClinicalContent(t *testing.T) {
	t.Parallel()

	eventType := reflect.TypeOf(domainaudit.Event{})

	require.Greater(t, eventType.NumField(), 5,
		"the walk found almost no fields; it is not reading the type it thinks it is")
	require.Equal(t, len(eventFields), eventType.NumField(),
		"a field has been added to or removed from the audit event; every field is an inventoried one or the trail can carry content")

	timeType := reflect.TypeOf(time.Time{})

	for i := range eventType.NumField() {
		field := eventType.Field(i)

		t.Run(field.Name, func(t *testing.T) {
			t.Parallel()

			_, inventoried := eventFields[field.Name]
			require.True(t, inventoried,
				"%s is not in the inventory: adding a field to the audit event is a spec decision, not an edit", field.Name)

			assert.True(t, field.IsExported(), "%s is unreachable by the store's mapper", field.Name)

			switch field.Type.Kind() {
			case reflect.String:
			case reflect.Struct:
				assert.Equal(t, timeType, field.Type,
					"%s is a struct other than time.Time, so it is a place a nested value could be written", field.Name)
			default:
				assert.Failf(t, "unbounded field",
					"%s is a %s: a map, slice, pointer or interface on the audit event is somewhere arbitrary content fits",
					field.Name, field.Type.Kind())
			}

			lowered := strings.ToLower(field.Name)
			for _, word := range contentWords {
				assert.NotContainsf(t, lowered, word,
					"%s reads as a field for what a record said; the trail records that something happened, never what", field.Name)
			}
		})
	}
}

// handle is the shape internal/obs mints a request's correlation id in: sixteen
// random bytes, hex. A run's handle is the same shape, so an operator greps for
// one thing.
var handle = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestRecordFillsTheCorrelationHandleFromTheRequestThenTheRun(t *testing.T) {
	t.Parallel()

	const (
		fromCaller  = "caller00000000000000000000000000"
		fromRequest = "request0000000000000000000000000"
	)

	for _, testCase := range []struct {
		name    string
		options []Option
		build   func(t *testing.T) (context.Context, domainaudit.Event)
		want    func(t *testing.T, ctx context.Context, got string)
	}{
		{
			name:    "the caller's own handle wins",
			options: []Option{WithRequestID(func(context.Context) string { return fromRequest })},
			build: func(t *testing.T) (context.Context, domainaudit.Event) {
				event := minimal(domainaudit.ActionCreate)
				event.RequestID = fromCaller

				return t.Context(), event
			},
			want: func(t *testing.T, _ context.Context, got string) {
				assert.Equal(t, fromCaller, got,
					"a caller that resolved the handle itself had it overwritten")
			},
		},
		{
			name:    "the request behind the context",
			options: []Option{WithRequestID(func(context.Context) string { return fromRequest })},
			build: func(t *testing.T) (context.Context, domainaudit.Event) {
				event := minimal(domainaudit.ActionCreate)
				event.RequestID = ""

				return t.Context(), event
			},
			want: func(t *testing.T, _ context.Context, got string) {
				assert.Equal(t, fromRequest, got)
			},
		},
		{
			// The case this test exists for. A cron tick, a job, a migration
			// and a backfill all reach Record on a context that never saw an
			// HTTP request, and the row still has to correlate to the run's own
			// log lines. Without this the retention purge fails Required
			// validation on its first nightly tick, in production.
			name: "a run with no request at all",
			build: func(t *testing.T) (context.Context, domainaudit.Event) {
				ctx, _ := StartRun(context.Background(), "")

				event := minimal(domainaudit.ActionDelete)
				event.RequestID = ""

				return ctx, event
			},
			want: func(t *testing.T, ctx context.Context, got string) {
				assert.Equal(t, RunIDFrom(ctx), got,
					"the row carries a handle that is not the run's, so it joins to none of the run's log lines")
				assert.Regexp(t, handle, got)
			},
		},
		{
			name:    "a run whose request lookup answers nothing",
			options: []Option{WithRequestID(func(context.Context) string { return "" })},
			build: func(t *testing.T) (context.Context, domainaudit.Event) {
				ctx, _ := StartRun(context.Background(), "")

				event := minimal(domainaudit.ActionDelete)
				event.RequestID = ""

				return ctx, event
			},
			want: func(t *testing.T, ctx context.Context, got string) {
				assert.Equal(t, RunIDFrom(ctx), got)
			},
		},
		{
			name: "neither, and still never the empty string",
			build: func(t *testing.T) (context.Context, domainaudit.Event) {
				event := minimal(domainaudit.ActionLogin)
				event.RequestID = ""

				return context.Background(), event
			},
			want: func(t *testing.T, _ context.Context, got string) {
				assert.Regexp(t, handle, got,
					"a row with no handle is refused by the store's Required rule, after the thing it records has happened")
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			w, store := writer(t, testCase.options...)

			ctx, event := testCase.build(t)
			require.NoError(t, w.Record(ctx, event))

			rows := store.Rows()
			require.Len(t, rows, 1, "the row was never read back")
			require.NoError(t, rows[0].Validate(), "the row as stored is one the store would refuse")

			testCase.want(t, ctx, rows[0].RequestID)
		})
	}
}

// One run, one handle. Two rows from the same nightly tick carrying two
// different handles correlate to nothing an operator can join.
func TestEveryRowOfOneRunCarriesTheSameHandle(t *testing.T) {
	t.Parallel()

	w, store := writer(t)

	ctx, id := StartRun(context.Background(), "")
	require.Regexp(t, handle, id)
	require.Equal(t, id, RunIDFrom(ctx))

	for _, action := range []domainaudit.Action{domainaudit.ActionDelete, domainaudit.ActionAccountDelete} {
		event := minimal(action)
		event.RequestID = ""

		require.NoError(t, w.Record(ctx, event))
	}

	rows := store.Rows()
	require.Len(t, rows, 2)
	assert.Equal(t, id, rows[0].RequestID)
	assert.Equal(t, id, rows[1].RequestID)
}

func TestStartRunHonoursAHandleTheCallerAlreadyMinted(t *testing.T) {
	t.Parallel()

	const minted = "abcdef0123456789abcdef0123456789"

	ctx, id := StartRun(context.Background(), minted)

	assert.Equal(t, minted, id, "the caller minted the handle its log lines already carry and got a second one")
	assert.Equal(t, minted, RunIDFrom(ctx))
}

func TestARunHandleFitsTheColumnThatHoldsIt(t *testing.T) {
	t.Parallel()

	_, id := StartRun(context.Background(), "")

	assert.LessOrEqual(t, len(id), domainaudit.MaxRequestID,
		"a handle longer than the column is a row refused after the thing it records has happened")
}

func TestRunIDFromIsEmptyOutsideARun(t *testing.T) {
	t.Parallel()

	assert.Empty(t, RunIDFrom(context.Background()))
}

func TestAnEventTheStoreWouldRefuseNeverReachesIt(t *testing.T) {
	t.Parallel()

	w, store := writer(t)

	event := minimal(domainaudit.ActionCreate)
	event.Action = ""

	err := w.Record(t.Context(), event)

	require.Error(t, err)

	var invalid *domain.ValidationError
	assert.ErrorAs(t, err, &invalid, "the refusal is not the validation error the caller handles")

	assert.Empty(t, store.Rows(), "an invalid row reached the store, which would refuse it after the fact")
}

func TestAnAppendFailureIsReportedAndNotSwallowed(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("the trail is unreachable")

	w, store := writer(t)
	store.FailAppends(sentinel)

	assert.ErrorIs(t, w.Record(t.Context(), minimal(domainaudit.ActionLogin)), sentinel)
}

func TestNewRefusesAWriterThatWouldDiscardEveryRow(t *testing.T) {
	t.Parallel()

	w, err := New(nil)

	require.Error(t, err)
	assert.Nil(t, w)
}
