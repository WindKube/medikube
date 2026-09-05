package records_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
)

// T020. FilterSpec is the typed shape a kind's own query parameter takes, and
// the generic handler validates a request's filters against it before the
// kind's own service ever sees them.
func TestAnUnknownFilterParameterIsBadRequest(t *testing.T) {
	t.Parallel()

	handler := records.NewHandler(twoKindRegistry(t))

	_, err := handler.ListOfKind(context.Background(), owner(), kind.Medication.Segment(),
		records.Query{Filters: map[string][]string{"not-published": {"x"}}})

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrBadRequest)
}

// A value outside a FilterEnum spec's Allowed set is the same refusal as an
// unknown name: contracts/records-clinical.md §1 does not distinguish them,
// because both are a caller guessing at a vocabulary it does not publish.
func TestAFilterValueOutsideTheAllowedSetIsBadRequest(t *testing.T) {
	t.Parallel()

	registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	registration.Schema.Filters = map[string]records.FilterSpec{
		recordstest.FilterName: {Name: recordstest.FilterName, Kind: records.FilterEnum, Allowed: []string{"alice", "bob"}},
	}

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(registration))

	handler := records.NewHandler(registry)

	_, err := handler.ListOfKind(context.Background(), owner(), kind.Medication.Segment(),
		records.Query{Filters: map[string][]string{recordstest.FilterName: {"carol"}}})

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrBadRequest)

	// The control: an allowed value passes straight through.
	_, err = handler.ListOfKind(context.Background(), owner(), kind.Medication.Segment(),
		records.Query{Filters: map[string][]string{recordstest.FilterName: {"alice"}}})
	assert.NoError(t, err)
}

// A FilterFreeform spec, or one with no Allowed set at all, checks the name
// and nothing about the value — there is no vocabulary to check it against.
func TestAFreeformFilterAcceptsAnyValue(t *testing.T) {
	t.Parallel()

	registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	registration.Schema.Filters = map[string]records.FilterSpec{
		recordstest.FilterName: {Name: recordstest.FilterName, Kind: records.FilterFreeform},
	}

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(registration))

	handler := records.NewHandler(registry)

	_, err := handler.ListOfKind(context.Background(), owner(), kind.Medication.Segment(),
		records.Query{Filters: map[string][]string{recordstest.FilterName: {"anything at all"}}})
	assert.NoError(t, err)
}

// A FilterSpec's Default is filled in when the caller supplies nothing for
// that name, and left alone when the caller supplies its own value — proven
// through the fake's own narrowing, since that is what a filled-in default
// actually changes about the answer.
func TestAFilterDefaultFillsInWhenTheCallerSuppliesNothing(t *testing.T) {
	t.Parallel()

	registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	registration.Schema.Filters = map[string]records.FilterSpec{
		recordstest.FilterName: {Name: recordstest.FilterName, Kind: records.FilterFreeform, Default: "alice"},
	}

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(registration))

	handler := records.NewHandler(registry)
	ctx := context.Background()

	_, err := handler.Create(ctx, owner(), kind.Medication.Segment(), []byte(`{"name":"alice"}`))
	require.NoError(t, err)
	_, err = handler.Create(ctx, owner(), kind.Medication.Segment(), []byte(`{"name":"bob"}`))
	require.NoError(t, err)

	// No `name` supplied: the default narrows to alice alone.
	defaulted, err := handler.ListOfKind(ctx, owner(), kind.Medication.Segment(), records.Query{})
	require.NoError(t, err)
	require.Len(t, defaulted.Items, 1)

	// An explicit value overrides the default.
	explicit, err := handler.ListOfKind(ctx, owner(), kind.Medication.Segment(),
		records.Query{Filters: map[string][]string{recordstest.FilterName: {"bob"}}})
	require.NoError(t, err)
	require.Len(t, explicit.Items, 1)
}
