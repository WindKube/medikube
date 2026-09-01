package records_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
)

func twoKindRegistry(t *testing.T) *records.Registry {
	t.Helper()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))
	require.NoError(t, recordstest.RegisterSynthetic(registry))

	return registry
}

func owner() access.Actor { return access.Actor{UserID: recordstest.OwnerID, RequestID: "req-1"} }

// call runs one operation of the record family against a {kind} segment. Every
// operation has to answer an unregistered segment the same way, so the table
// below drives all six through one shape rather than six near-identical tests.
func call(ctx context.Context, handler *records.Handler, op, segment string) error {
	switch op {
	case "listRecordsOfKind":
		_, err := handler.ListOfKind(ctx, owner(), segment, records.Query{})
		return err
	case "createRecord":
		_, err := handler.Create(ctx, owner(), segment, []byte(`{"name":"n"}`))
		return err
	case "getRecord":
		_, err := handler.Get(ctx, owner(), segment, "r1")
		return err
	case "updateRecord":
		_, err := handler.Update(ctx, owner(), segment, "r1", "v1", []byte(`{"name":"n"}`))
		return err
	case "deleteRecord":
		return handler.Delete(ctx, owner(), segment, "r1", "v1")
	default:
		return errors.New("no such operation: " + op)
	}
}

var kindScopedOperations = []string{
	"listRecordsOfKind", "createRecord", "getRecord", "updateRecord", "deleteRecord",
}

// T108. An unregistered {kind} is 404 and not 400. A 400 would say "that is not
// a kind", which tells an anonymous prober which kinds exist — the same
// disclosure FR-033 closes on record ids, arriving one level up the path.
func TestAnUnregisteredSegmentIsNotFoundAndNotAValidationFailure(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	for _, op := range kindScopedOperations {
		t.Run(op, func(t *testing.T) {
			t.Parallel()

			err := call(context.Background(), handler, op, "no-such-kind")

			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrNotFound)

			var invalid *domain.ValidationError
			assert.False(t, errors.As(err, &invalid),
				"an unregistered kind was reported as a validation failure, which the edge maps to 422 and not 404")
		})
	}
}

// The segment is matched exactly. PocketBase has done no case or trailing-slash
// normalisation since v0.23 (research D-05), so accepting a second spelling
// here would create a second URL for one kind — two cache keys, two audit
// spellings and a canonical form that is nobody's.
func TestADifferentlyCasedSegmentDoesNotMatch(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	segment := kind.Medication.Segment()
	variants := map[string]string{
		"upper":      strings.ToUpper(segment),
		"title":      strings.ToUpper(segment[:1]) + segment[1:],
		"trailing/":  segment + "/",
		"leading /":  "/" + segment,
		"whitespace": " " + segment,
	}

	for name, variant := range variants {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.NotEqual(t, segment, variant, "the variant is the segment itself; the test proves nothing")

			_, err := handler.Dispatch(variant)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrNotFound)
		})
	}

	// The control: the exact spelling does match, so the assertions above are
	// about case and not about the lookup being broken outright.
	entry, err := handler.Dispatch(segment)
	require.NoError(t, err)
	assert.Equal(t, kind.Medication, entry.Kind)
}

// The dispatch table is the whole of the kind-specific knowledge in the record
// family. Every registered kind must be in it, and nothing else may be: a table
// built from anything other than the registry is a table that can go stale.
func TestTheDispatchTableCoversEveryRegisteredKind(t *testing.T) {
	t.Parallel()

	registry := twoKindRegistry(t)
	handler := records.NewHandler(registry)

	assert.Equal(t, registry.Segments(), handler.Segments())

	for _, entry := range registry.Entries() {
		dispatched, err := handler.Dispatch(entry.Segment)
		require.NoErrorf(t, err, "%s is registered and does not dispatch", entry.Kind)
		assert.Equal(t, entry.Kind, dispatched.Kind)
		assert.Same(t, entry.Service, dispatched.Service,
			"the segment dispatched to another kind's service")
	}

	// A kind registered after the handler was built is not in the handler's
	// table, and asking for it is a 404 rather than a nil-map panic.
	third := recordstest.SyntheticRegistration()
	third.Kind = kind.Kind("third_fake_kind")
	require.NoError(t, registry.RegisterSynthetic(third, "third-fake-kinds", "third_fake_kinds"))

	_, err := handler.Dispatch("third-fake-kinds")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// The generic handler decodes into the kind's own type and never into a map, so
// a body carrying `owner` is refused by shape. FR-032 is enforced by the DTO
// having no such field rather than by a runtime check somebody can forget.
func TestAnUnknownFieldIsAValidationFailureNamingTheField(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	_, err := handler.Create(context.Background(), owner(), kind.Medication.Segment(),
		[]byte(`{"name":"n","owner":"someone-else"}`))

	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "owner", invalid.Fields[0].Field)
	assert.Equal(t, domain.CodeUnknownField, invalid.Fields[0].Code)
}

// A decoder error message can carry the submitted value, and on this
// application's DTOs the submitted value is medical data. Nothing the decoder
// said reaches the field message.
//
// The overflow row is the one that matters: encoding/json/v2 answers it with
// `cannot unmarshal JSON number 99999999999999999999 into Go int within
// "/doses": value out of range`, submitted value and all. The other two rows
// are the branches beside it, held here so a future edit cannot start
// forwarding the decoder's text down one of them unnoticed.
func TestADecodeFailureNeverCarriesTheSubmittedValue(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	cases := map[string]struct {
		body  string
		leaks string
		field string
		code  string
	}{
		"a value the Go type cannot hold": {
			body:  `{"name":"n","doses":99999999999999999999}`,
			leaks: "99999999999999999999",
			field: "doses",
			code:  domain.CodeInvalidValue,
		},
		"a field this operation does not accept": {
			body:  `{"name":"n","prescriber":"Dr Amara Okafor"}`,
			leaks: "Dr Amara Okafor",
			field: "prescriber",
			code:  domain.CodeUnknownField,
		},
		"a field sent twice": {
			body:  `{"name":"n","name":"Amoxicillin 500mg"}`,
			leaks: "Amoxicillin 500mg",
			field: "name",
			code:  domain.CodeInvalidValue,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := handler.Create(context.Background(), owner(), kind.Medication.Segment(), []byte(testCase.body))
			require.Error(t, err)

			// Error() is what reaches the log and Sentry, so it is asserted as
			// well as the per-field message the person is shown.
			assert.NotContains(t, err.Error(), testCase.leaks)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, testCase.field, invalid.Fields[0].Field)
			assert.Equal(t, testCase.code, invalid.Fields[0].Code)
			assert.NotContains(t, invalid.Fields[0].Message, testCase.leaks)
		})
	}
}

// D-28's other half, which costs nothing and is silently on: a member whose
// case does not match is not the field it looks like, and combined with
// RejectUnknownMembers it is a hard error rather than a value that vanishes.
func TestACaseMismatchedMemberIsNotTheFieldItLooksLike(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	_, err := handler.Create(context.Background(), owner(), kind.Medication.Segment(), []byte(`{"Name":"n"}`))
	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "Name", invalid.Fields[0].Field)
	assert.Equal(t, domain.CodeUnknownField, invalid.Fields[0].Code)
}

// contracts/records.md: If-Match is required on update and on delete, and a
// missing one is 422 with field If-Match and code required. Requiring it in the
// generic handler is what gives every later kind the rule for free — which is
// the whole argument for this package existing.
func TestIfMatchIsRequiredOnEveryWriteThatReplacesState(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))
	segment := kind.Medication.Segment()

	writes := map[string]func() error{
		"updateRecord": func() error {
			_, err := handler.Update(context.Background(), owner(), segment, "r1", "", []byte(`{}`))
			return err
		},
		"deleteRecord": func() error {
			return handler.Delete(context.Background(), owner(), segment, "r1", "")
		},
	}

	for op, write := range writes {
		t.Run(op, func(t *testing.T) {
			t.Parallel()

			err := write()
			require.Error(t, err)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)
			assert.Equal(t, records.IfMatchField, invalid.Fields[0].Field)
			assert.Equal(t, domain.CodeRequired, invalid.Fields[0].Code)
		})
	}
}

// The precondition is checked before the kind is resolved is the wrong order:
// an unregistered kind must stay a 404 whatever else is wrong with the request,
// or the 404 becomes a 422 and tells the prober the path was real.
func TestAnUnregisteredKindOutranksAMissingPrecondition(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	err := handler.Delete(context.Background(), owner(), "no-such-kind", "r1", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	var invalid *domain.ValidationError
	assert.False(t, errors.As(err, &invalid))
}

// contracts/records.md: a sort outside the kind's allowlist is 422
// invalid_value and never silently ignored, because a silently ignored sort
// produces a list that looks right and is not.
func TestASortOutsideTheKindsAllowlistIsRefused(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))
	segment := kind.Medication.Segment()

	_, err := handler.ListOfKind(context.Background(), owner(), segment,
		records.Query{Sort: []domain.SortKey{{Field: "owner"}}})

	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "sort", invalid.Fields[0].Field)
	assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)

	// The control: the kind's own allowlist is accepted.
	entry, err := handler.Dispatch(segment)
	require.NoError(t, err)
	_, err = handler.ListOfKind(context.Background(), owner(), segment,
		records.Query{Sort: entry.Schema.Sorts})
	assert.NoError(t, err)
}

// The same rule for the kind's named query parameters. PocketBase's filter DSL
// never reaches the wire, so a parameter outside the declared vocabulary is a
// caller guessing at one.
func TestAFilterOutsideTheKindsVocabularyIsRefused(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	_, err := handler.ListOfKind(context.Background(), owner(), kind.Medication.Segment(),
		records.Query{Filters: map[string][]string{"owner": {"someone-else"}}})

	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "owner", invalid.Fields[0].Field)
	assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
}

// The cross-kind list. With one selected kind it is the kind's own list, cursor
// and all. With more than one it is refused loudly rather than served with a
// cursor that can only continue one of its sources — a page that repeats or
// skips rows is what FR-023 forbids, and it would do so invisibly.
func TestTheCrossKindListRefusesToPageAcrossMoreThanOneKind(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	t.Run("one kind is the kind's own list", func(t *testing.T) {
		t.Parallel()

		page, err := handler.List(context.Background(), owner(), records.Query{Kinds: []kind.Kind{kind.Medication}})
		require.NoError(t, err)
		assert.NotNil(t, page.Items)
	})

	t.Run("two kinds is refused", func(t *testing.T) {
		t.Parallel()

		_, err := handler.List(context.Background(), owner(),
			records.Query{Kinds: []kind.Kind{kind.Medication, recordstest.Kind}})

		require.Error(t, err)
		assert.ErrorIs(t, err, records.ErrCrossKindPaging)
	})

	t.Run("no selection means every registered kind", func(t *testing.T) {
		t.Parallel()

		// Two kinds are registered here, so the default selection is also the
		// refused one. With the production registry's single kind it is the
		// delegating case, which is why this is a blocker for phase 003 and not
		// for this one.
		_, err := handler.List(context.Background(), owner(), records.Query{})
		assert.ErrorIs(t, err, records.ErrCrossKindPaging)
	})

	t.Run("an unregistered kind in the selection is a validation failure", func(t *testing.T) {
		t.Parallel()

		// A query parameter, unlike a path segment, is 422: the caller already
		// knows the path exists, so naming the offending value discloses
		// nothing it did not have.
		_, err := handler.List(context.Background(), owner(),
			records.Query{Kinds: []kind.Kind{kind.Kind("no_such_kind")}})

		require.Error(t, err)

		var invalid *domain.ValidationError
		require.ErrorAs(t, err, &invalid)
		require.Len(t, invalid.Fields, 1)
		assert.Equal(t, "kind", invalid.Fields[0].Field)
		assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
	})
}

// The round trip through the generic handler: what goes in as the kind's own
// DTO comes back as the kind's own DTO, and nothing in between knew which type
// it was.
func TestTheGenericHandlerServesAKindItWasNeverWrittenFor(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))
	ctx := context.Background()

	created, err := handler.Create(ctx, owner(), recordstest.Segment, []byte(`{"name":"a synthetic record"}`))
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)
	require.NotEmpty(t, created.Version)
	assert.Equal(t, recordstest.Kind, created.Kind)

	detail, isDetail := created.Body.(*recordstest.Detail)
	require.True(t, isDetail, "the body is not the kind's own type: %T", created.Body)
	assert.Equal(t, "a synthetic record", detail.Name)

	fetched, err := handler.Get(ctx, owner(), recordstest.Segment, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Version, fetched.Version)

	updated, err := handler.Update(ctx, owner(), recordstest.Segment, created.ID, created.Version,
		[]byte(`{"note":"changed"}`))
	require.NoError(t, err)
	assert.NotEqual(t, created.Version, updated.Version, "the ETag source did not move on a write")

	_, stale := handler.Update(ctx, owner(), recordstest.Segment, created.ID, created.Version, []byte(`{"note":"again"}`))
	assert.ErrorIs(t, stale, domain.ErrVersionMismatch)

	require.NoError(t, handler.Delete(ctx, owner(), recordstest.Segment, created.ID, updated.Version))

	_, gone := handler.Get(ctx, owner(), recordstest.Segment, created.ID)
	assert.ErrorIs(t, gone, domain.ErrNotFound)
}

// FR-033 through the generic layer: another account's id is answered exactly as
// an id that never existed. The handler does not decide this — the kind's
// service does — and this asserts the generic layer does not weaken it.
func TestAnotherAccountsRecordIsAnsweredAsNotFound(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))
	ctx := context.Background()

	created, err := handler.Create(ctx, owner(), recordstest.Segment, []byte(`{"name":"n"}`))
	require.NoError(t, err)

	stranger := access.Actor{UserID: "somebody-else", RequestID: "req-2"}

	_, mine := handler.Get(ctx, stranger, recordstest.Segment, created.ID)
	_, neverExisted := handler.Get(ctx, stranger, recordstest.Segment, "mknosuchrecord1")

	require.Error(t, mine)
	require.Error(t, neverExisted)
	assert.ErrorIs(t, mine, domain.ErrNotFound)
	assert.Equal(t, neverExisted.Error(), mine.Error(),
		"the two refusals read differently, so the error text is an existence oracle")
}

// A handler over an empty registry answers 404 for everything rather than
// panicking on a nil map. It is what a mid-phase build looks like before the
// first kind is wired.
func TestAHandlerOverAnEmptyRegistryIsAllNotFound(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(records.NewRegistry())

	assert.Empty(t, handler.Segments())

	for _, op := range kindScopedOperations {
		t.Run(op, func(t *testing.T) {
			t.Parallel()

			assert.ErrorIs(t, call(context.Background(), handler, op, kind.Medication.Segment()), domain.ErrNotFound)
		})
	}
}

// The generic handler has its own copy of the decode-failure translation, so it
// has its own copy of this hole and needs its own proof that it is closed.
//
// For json.ErrUnknownName the member name is BY DEFINITION one MediKube does
// not publish: it is whatever the client sent, and it travels into the response
// body and into the one log stream. A typo still has to name the field the
// client meant; nothing else survives at whatever length or in whatever
// alphabet it arrived.
func TestAnUnknownFieldIsNamedBackBoundedAndRestricted(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	cases := []struct {
		name  string
		body  string
		field string
	}{
		{
			name:  "a developer's typo is still useful",
			body:  `{"name":"n","dosge":"1"}`,
			field: "dosge",
		},
		{
			name:  "a newline cannot inject a second log line",
			body:  `{"name":"n","a\nlevel=fatal":1}`,
			field: "a_level_fatal",
		},
		{
			name:  "a quote cannot close a JSON string in the stream",
			body:  `{"name":"n","a\",\"injected\":\"":1}`,
			field: "a___injected___",
		},
		{
			name:  "an escape sequence cannot repaint a terminal",
			body:  `{"name":"n","a\u001b[31mb":1}`,
			field: "a__31mb",
		},
		{
			name:  "a name of a hundred thousand bytes is bounded",
			body:  `{"name":"n","` + strings.Repeat("z", 100_000) + `":1}`,
			field: strings.Repeat("z", domain.MaxFieldNameLen),
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			_, err := handler.Create(context.Background(), owner(), kind.Medication.Segment(), []byte(one.body))
			require.Error(t, err)

			var invalid *domain.ValidationError
			require.ErrorAs(t, err, &invalid)
			require.Len(t, invalid.Fields, 1)

			assert.Equal(t, domain.CodeUnknownField, invalid.Fields[0].Code)
			assert.Equal(t, one.field, invalid.Fields[0].Field)

			// Error() is the string that reaches the one log stream and the
			// error-reporting destination.
			assert.LessOrEqual(t, len(err.Error()), 128,
				"an unbounded member name reached the one log stream")
			assert.NotContains(t, err.Error(), "\n")
			assert.NotContains(t, err.Error(), `"`)
		})
	}
}
