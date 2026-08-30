package records_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
)

// T105. Two registrations of one kind is a wiring mistake with two silent
// outcomes and no good one: last-wins hands every request to whichever package
// was constructed second, and first-wins leaves a fully built service that
// nothing will ever call. Both look like a working boot. The registry refuses
// instead, and the refusal is the whole test.
func TestRegisteringTheSameKindTwiceIsRefused(t *testing.T) {
	t.Parallel()

	first := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	first.Inventory.Title = "first"

	second := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	second.Inventory.Title = "second"

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(first))

	err := registry.Register(second)
	require.Error(t, err)
	assert.ErrorIs(t, err, records.ErrAlreadyRegistered)
	assert.Contains(t, err.Error(), kind.Medication.Enum())
}

// The half of T105 that a bare error check would miss: the refusal must leave
// the registry exactly as it was. A registry that reports the duplicate and
// then keeps the second service has done the damage anyway and told you it
// did not.
func TestTheRefusedDuplicateChangesNothing(t *testing.T) {
	t.Parallel()

	first := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	first.Inventory.Title = "first"

	second := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	second.Inventory.Title = "second"
	second.Service = recordstest.NewFakeKindService()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(first))
	require.Error(t, registry.Register(second))

	assert.Equal(t, []kind.Kind{kind.Medication}, registry.Kinds())
	assert.Equal(t, []string{kind.Medication.Segment()}, registry.Segments())
	require.Len(t, registry.InventoryRows(), 1)
	assert.Equal(t, "first", registry.InventoryRows()[0].Title)

	entry, found := registry.FromKind(kind.Medication)
	require.True(t, found)
	assert.Same(t, first.Service, entry.Service, "the refused registration's service won anyway")
}

// The same refusal reached from the other side. Two kinds sharing one path
// segment would make one of them unreachable through /api/v1/records/{kind}
// and give the other two spellings — the drift research D-05 declares the kind
// table to prevent, arriving through the door that bypasses the kind table.
func TestTwoKindsMayNotShareOnePathSegment(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, recordstest.RegisterSynthetic(registry))

	clash := recordstest.SyntheticRegistration()
	clash.Kind = kind.Kind("other_fake_kind")

	err := registry.RegisterSynthetic(clash, recordstest.Segment, "other_fake_kinds")
	require.Error(t, err)
	assert.ErrorIs(t, err, records.ErrSegmentTaken)
	assert.Equal(t, []kind.Kind{recordstest.Kind}, registry.Kinds())
}

// And from the third side. Two kinds pointed at one collection is the mistake
// that makes a kind's list return the other kind's rows — the failure the kind
// table's own init() panic exists to stop, repeated here because the synthetic
// door does not go through that table.
func TestTwoKindsMayNotShareOneCollection(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, recordstest.RegisterSynthetic(registry))

	clash := recordstest.SyntheticRegistration()
	clash.Kind = kind.Kind("other_fake_kind")

	err := registry.RegisterSynthetic(clash, "other-fake-kinds", recordstest.Collection)
	require.Error(t, err)
	assert.ErrorIs(t, err, records.ErrCollectionTaken)
	assert.Equal(t, []kind.Kind{recordstest.Kind}, registry.Kinds())
}

// A duplicate that got through would be invisible at registration and obvious
// only in production, so this asserts the consequence directly: the kind's
// service is the one that was registered first, and the handler dispatches to
// it.
func TestDispatchAfterARefusedDuplicateStillReachesTheFirstService(t *testing.T) {
	t.Parallel()

	firstService := recordstest.NewFakeKindService()
	secondService := recordstest.NewFakeKindService()

	first := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	first.Service = firstService

	second := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	second.Service = secondService

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(first))
	require.Error(t, registry.Register(second))

	owner := access.Actor{UserID: recordstest.OwnerID}
	created, err := firstService.Create(context.Background(), owner, &recordstest.Create{Name: "n"})
	require.NoError(t, err)

	got, err := records.NewHandler(registry).Get(context.Background(), owner, kind.Medication.Segment(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)

	_, missing := secondService.Get(context.Background(), owner, created.ID)
	assert.Error(t, missing, "the second service was never reached, so it holds nothing")
}
