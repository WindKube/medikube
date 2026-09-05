package records_test

import (
	"context"
	"errors"
	"maps"
	"slices"
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

// T104, first half. One Register call has to wire every consumer of a kind, and
// the point of asserting them one at a time is that a partial registration is
// the failure this package exists to make impossible: a kind that reached the
// router but not the audit trail is reachable and untraceable, and a kind that
// reached the authorizer but not the stream is a live view that silently stops
// updating.
//
// The table is keyed by the published consumer name rather than by a list
// written here, so TestEveryPublishedConsumerIsBothWiredAndRefused below can
// prove these are the same seven things the package says it has.
var wired = map[string]func(t *testing.T, registry *records.Registry, entry records.Entry){
	records.ConsumerRoutes: func(t *testing.T, registry *records.Registry, _ records.Entry) {
		// The router needs the path segment and the reverse lookup the generic
		// handler answers 404 from. Neither is spelled here: both come out of
		// the kind table (research D-05).
		assert.Equal(t, []string{kind.Medication.Segment()}, registry.Segments())

		bySegment, found := registry.FromSegment(kind.Medication.Segment())
		require.True(t, found)
		assert.Equal(t, kind.Medication, bySegment.Kind)
		assert.Equal(t, kind.Medication.Collection(), bySegment.Collection)
	},

	records.ConsumerSchema: func(t *testing.T, _ *records.Registry, entry records.Entry) {
		// internal/openapi reflects these four into the kind's oneOf branch and
		// the generic handler decodes request bodies into the same types, so
		// the documented schema and the decoded type cannot be different types.
		require.NotNil(t, entry.Schema.NewSummary)
		require.NotNil(t, entry.Schema.NewDetail)
		require.NotNil(t, entry.Schema.NewCreate)
		require.NotNil(t, entry.Schema.NewPatch)

		assert.IsType(t, &recordstest.Detail{}, entry.Schema.NewDetail())
		assert.IsType(t, &recordstest.Create{}, entry.Schema.NewCreate())

		// The schema name and the discriminator value are two different
		// spellings of one kind — plural segment against singular enum — and
		// mixing them mis-dispatches every generated client (research D-05,
		// D-08).
		assert.Equal(t, "Record_"+kind.Medication.Segment(), entry.SchemaName())
		assert.Equal(t, kind.Medication.Enum(), entry.DiscriminatorValue())
		assert.NotEmpty(t, entry.Schema.Sorts,
			"a kind with no sort allowlist accepts any sort, and a silently ignored sort is a list that looks right and is not")
	},

	records.ConsumerViews: func(t *testing.T, _ *records.Registry, entry records.Entry) {
		require.NotNil(t, entry.Views)
		assert.Equal(t, recordstest.RenderedRow, render(t, entry.Views.Row(records.Record{ID: "r1", Kind: kind.Medication})))
		assert.Equal(t, recordstest.RenderedDetail, render(t, entry.Views.Detail(records.Record{ID: "r1", Kind: kind.Medication})))
		assert.Equal(t, recordstest.RenderedList, render(t, entry.Views.List(domain.NewPage([]records.Record{}, nil))))
		assert.Equal(t, recordstest.RenderedForm, render(t, entry.Views.Form(records.Record{}, nil)))
	},

	records.ConsumerStream: func(t *testing.T, _ *records.Registry, entry records.Entry) {
		require.NotNil(t, entry.Stream)
		assert.True(t, entry.Stream.Streams("r1", recordstest.OwnerID))
	},

	records.ConsumerAudit: func(t *testing.T, _ *records.Registry, entry records.Entry) {
		assert.Equal(t, audit.TargetKindMedication, entry.Target)
		assert.True(t, entry.Target.Valid())
	},

	records.ConsumerAuthorizer: func(t *testing.T, _ *records.Registry, entry records.Entry) {
		require.NotNil(t, entry.Authorizer)

		grant, err := entry.Authorizer.Patient(context.Background(),
			access.Actor{UserID: recordstest.OwnerID}, "r1", access.PermView)
		require.NoError(t, err)
		assert.True(t, grant.Allows(access.PermView))
	},

	records.ConsumerInventory: func(t *testing.T, registry *records.Registry, _ records.Entry) {
		rows := registry.InventoryRows()
		require.Len(t, rows, 1)
		assert.Equal(t, kind.Medication, rows[0].Kind)
		assert.Equal(t, kind.Medication.Segment(), rows[0].Segment)
		assert.NotEmpty(t, rows[0].Title)
		assert.NotEmpty(t, rows[0].Summary)
	},
}

// T104, second half. Each consumer, left unwired on its own, is refused.
var unwired = map[string]func(*records.Registration){
	// An undeclared kind has no path segment and no collection, so there is no
	// route to register and nothing for FromSegment to answer.
	records.ConsumerRoutes:     func(r *records.Registration) { r.Kind = kind.Kind("no_such_kind") },
	records.ConsumerSchema:     func(r *records.Registration) { r.Schema.NewCreate = nil },
	records.ConsumerViews:      func(r *records.Registration) { r.Views = nil },
	records.ConsumerStream:     func(r *records.Registration) { r.Stream = nil },
	records.ConsumerAudit:      func(r *records.Registration) { r.Target = "" },
	records.ConsumerAuthorizer: func(r *records.Registration) { r.Authorizer = nil },
	records.ConsumerInventory:  func(r *records.Registration) { r.Inventory = records.Inventory{} },
}

func TestOneRegistrationWiresAllSevenConsumers(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))

	entry, registered := registry.FromKind(kind.Medication)
	require.True(t, registered, "the kind was accepted and then could not be found")

	for _, consumer := range slices.Sorted(maps.Keys(wired)) {
		t.Run(consumer, func(t *testing.T) {
			t.Parallel()

			wired[consumer](t, registry, entry)
		})
	}
}

func TestARegistrationMissingOneConsumerIsRefused(t *testing.T) {
	t.Parallel()

	for _, consumer := range slices.Sorted(maps.Keys(unwired)) {
		t.Run(consumer, func(t *testing.T) {
			t.Parallel()

			registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
			unwired[consumer](&registration)

			registry := records.NewRegistry()
			err := registry.Register(registration)

			require.Errorf(t, err, "the registration was accepted with %s unwired", consumer)
			assert.Containsf(t, err.Error(), consumer,
				"the refusal does not name the consumer that is missing, so nobody reading it knows what to add")
			assert.Empty(t, registry.Kinds(), "a refused registration left the kind half-registered")
			assert.Empty(t, registry.Segments())
		})
	}
}

// The bite that keeps the two tables honest. Consumers() is what internal/cli,
// internal/openapi and the review read; a consumer added there with no wiring
// assertion and no refusal case beside it is a consumer nobody checks, and this
// is the test that says so.
func TestEveryPublishedConsumerIsBothWiredAndRefused(t *testing.T) {
	t.Parallel()

	published := records.Consumers()
	require.Len(t, published, 7, "T104 names seven consumers")

	assert.ElementsMatch(t, published, slices.Collect(maps.Keys(wired)))
	assert.ElementsMatch(t, published, slices.Collect(maps.Keys(unwired)))
}

// The service is not one of the seven consumers — it is what the generic
// handler dispatches to — and it is just as mandatory. A kind whose five
// operations nobody implements is a route that panics on its first request.
func TestARegistrationWithNoServiceIsRefused(t *testing.T) {
	t.Parallel()

	registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	registration.Service = nil

	registry := records.NewRegistry()
	err := registry.Register(registration)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service")
	assert.Empty(t, registry.Kinds())
}

// Every missing consumer at once, in one refusal. A registration reported one
// consumer at a time costs seven boots to fix, which is FR-027's argument
// applied to the operator instead of to the person filling in a form.
func TestAnIncompleteRegistrationReportsEveryMissingConsumerAtOnce(t *testing.T) {
	t.Parallel()

	registration := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	registration.Views = nil
	registration.Stream = nil
	registration.Authorizer = nil

	err := records.NewRegistry().Register(registration)
	require.Error(t, err)

	var incomplete *records.IncompleteError
	require.ErrorAs(t, err, &incomplete)
	assert.ElementsMatch(t,
		[]string{records.ConsumerViews, records.ConsumerStream, records.ConsumerAuthorizer},
		incomplete.Missing)
}

// The audit target is derivable from the kind and is declared anyway, so the
// two vocabularies — kind.Kinds() and audit.TargetKinds(), each declared
// complete in its own file — are cross-checked at the one place both are known.
func TestAnAuditTargetThatDisagreesWithTheKindIsRefused(t *testing.T) {
	t.Parallel()

	err := records.NewRegistry().Register(recordstest.Registration(kind.Medication, audit.TargetKindAllergy))

	require.Error(t, err)
	assert.Contains(t, err.Error(), records.ConsumerAudit)
	assert.ErrorIs(t, err, records.ErrAuditTargetMismatch)
}

// T109's second implementation. A kind the production vocabulary does not
// declare has no segment and no collection to derive, so it is registered
// through the door that takes them explicitly — and that door is the only one
// that accepts one.
func TestASyntheticKindNeedsTheExplicitDoor(t *testing.T) {
	t.Parallel()

	t.Run("Register refuses it", func(t *testing.T) {
		t.Parallel()

		err := records.NewRegistry().Register(recordstest.SyntheticRegistration())
		require.Error(t, err)
		assert.ErrorIs(t, err, records.ErrKindUndeclared)
	})

	t.Run("RegisterSynthetic accepts it and marks it", func(t *testing.T) {
		t.Parallel()

		registry := records.NewRegistry()
		require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))
		require.NoError(t, recordstest.RegisterSynthetic(registry))

		assert.Equal(t, []kind.Kind{recordstest.Kind}, registry.SyntheticKinds())
		assert.Len(t, registry.Kinds(), 2, "the two-kind registry the oneOf gate needs (research D-08)")

		entry, found := registry.FromSegment(recordstest.Segment)
		require.True(t, found)
		assert.True(t, entry.Synthetic)
		assert.Equal(t, "Record_"+recordstest.Segment, entry.SchemaName())
		assert.Equal(t, string(recordstest.Kind), entry.DiscriminatorValue())
	})

	// Every consumer check applies on the synthetic path too. It is not a
	// formality: the audit-target check is the only one a declared kind does
	// not exercise, because a declared kind with no target is caught first by
	// the mismatch check that a synthetic kind has no row to be matched
	// against. A door that skipped a check would only prove the check can be
	// skipped.
	t.Run("a synthetic registration is checked for the same consumers", func(t *testing.T) {
		t.Parallel()

		for _, consumer := range slices.Sorted(maps.Keys(unwired)) {
			if consumer == records.ConsumerRoutes {
				// Its mutation is "declare a kind the table does not have",
				// which is what the synthetic door exists to accept.
				continue
			}

			t.Run(consumer, func(t *testing.T) {
				t.Parallel()

				registration := recordstest.SyntheticRegistration()
				unwired[consumer](&registration)

				registry := records.NewRegistry()
				err := registry.RegisterSynthetic(registration, recordstest.Segment, recordstest.Collection)

				require.Errorf(t, err, "the synthetic registration was accepted with %s unwired", consumer)
				assert.Contains(t, err.Error(), consumer)
				assert.Empty(t, registry.Kinds())
			})
		}
	})

	t.Run("a synthetic registration with no spelling is refused", func(t *testing.T) {
		t.Parallel()

		spellings := map[string][2]string{
			"no segment":    {"", recordstest.Collection},
			"no collection": {recordstest.Segment, ""},
		}

		for _, name := range slices.Sorted(maps.Keys(spellings)) {
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				err := records.NewRegistry().RegisterSynthetic(
					recordstest.SyntheticRegistration(), spellings[name][0], spellings[name][1])
				assert.Error(t, err)
			})
		}
	})

	t.Run("a declared kind may not be smuggled through it", func(t *testing.T) {
		t.Parallel()

		// Otherwise the synthetic door is a way to give a real kind a second
		// path segment, which is exactly the drift D-05 exists to stop.
		err := records.NewRegistry().RegisterSynthetic(
			recordstest.Registration(kind.Medication, audit.TargetKindMedication),
			recordstest.Segment, recordstest.Collection)

		require.Error(t, err)
		assert.ErrorIs(t, err, records.ErrKindDeclared)
	})
}

// Registration order is the order the {kind} enum, the discriminator mapping
// and `medikube routes` all print. A map iteration anywhere in the registry
// would make the generated OpenAPI document differ between two runs of the
// generator, and the committed-document diff gate would then fail at random.
func TestTheRegistryKeepsRegistrationOrder(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, recordstest.RegisterSynthetic(registry))
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))

	assert.Equal(t, []kind.Kind{recordstest.Kind, kind.Medication}, registry.Kinds())

	for range 20 {
		assert.Equal(t, []string{recordstest.Segment, kind.Medication.Segment()}, registry.Segments())
	}
}

// Entries() hands the inventory to four consumers at once; one of them sorting
// or rewriting it would reorder it for the other three.
func TestEntriesIsACopy(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))

	entries := registry.Entries()
	require.Len(t, entries, 1)
	entries[0].Segment = "clobbered"

	assert.Equal(t, kind.Medication.Segment(), registry.Entries()[0].Segment)
}

// T031. The chart summary's extension point: one count per registered kind's
// collection, with nothing here deciding which collection belongs to which
// kind by switching on it — that answer comes out of the registry itself.
func TestCountByKindDispatchesOverEveryRegisteredCollectionWithoutSwitchingOnKind(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))

	seen := map[string]int{}
	counts, err := registry.CountByKind(context.Background(), func(_ context.Context, collection string) (int, error) {
		seen[collection]++

		return 7, nil
	})

	require.NoError(t, err)
	assert.Equal(t, map[kind.Kind]int{kind.Medication: 7}, counts)
	assert.Equal(t, map[string]int{kind.Medication.Collection(): 1}, seen)
}

func TestCountByKindReportsTheCollectionAFailedCountCameFrom(t *testing.T) {
	t.Parallel()

	registry := records.NewRegistry()
	require.NoError(t, registry.Register(recordstest.Registration(kind.Medication, audit.TargetKindMedication)))

	broken := errors.New("the count could not be made")
	_, err := registry.CountByKind(context.Background(), func(_ context.Context, _ string) (int, error) {
		return 0, broken
	})

	require.ErrorIs(t, err, broken)
	assert.Contains(t, err.Error(), kind.Medication.Collection())
}

func render(t *testing.T, renderer records.Renderer) string {
	t.Helper()

	require.NotNil(t, renderer)

	var out strings.Builder
	require.NoError(t, renderer.Render(context.Background(), &out))

	return out.String()
}
