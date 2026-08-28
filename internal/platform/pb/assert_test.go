package pb_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/platform/pb"
)

// lockedDownCollection is what every MediKube collection must look like: five
// nil API rules, so PocketBase's HTTP layer answers "superuser only" before any
// record is loaded. nil and "" are opposites here, not degrees — apis/record_crud.go
// checks `rule == nil` for superuser-only and `*rule != ""` for an expression,
// so types.Pointer("") is wide open to anyone the route admits.
func lockedDownCollection(name string) *core.Collection {
	return core.NewBaseCollection(name)
}

// Each of the three conditions is asserted on its own, from a baseline that
// passes, so a test cannot pass because some other violation happened to fire.
func TestBootAssertionsRefuseEachConditionIndependently(t *testing.T) {
	t.Parallel()

	t.Run("a clean instance is accepted", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, pb.AssertCollections([]*core.Collection{lockedDownCollection("clean")}))
		assert.NoError(t, pb.AssertSettings(&core.Settings{}))
	})

	t.Run("a non-nil API rule on a non-system collection", func(t *testing.T) {
		t.Parallel()

		rules := []struct {
			name string
			set  func(c *core.Collection)
		}{
			{"listRule", func(c *core.Collection) { c.ListRule = types.Pointer("") }},
			{"viewRule", func(c *core.Collection) { c.ViewRule = types.Pointer("id = @request.auth.id") }},
			{"createRule", func(c *core.Collection) { c.CreateRule = types.Pointer("") }},
			{"updateRule", func(c *core.Collection) { c.UpdateRule = types.Pointer("") }},
			{"deleteRule", func(c *core.Collection) { c.DeleteRule = types.Pointer("") }},
		}

		for _, rule := range rules {
			t.Run(rule.name, func(t *testing.T) {
				t.Parallel()

				c := lockedDownCollection("probe")
				rule.set(c)

				err := pb.AssertCollections([]*core.Collection{c})

				require.Error(t, err)
				assert.ErrorIs(t, err, pb.ErrAPIRuleSet)
				assert.NotErrorIs(t, err, pb.ErrFileFieldUnprotected)
				assert.Contains(t, err.Error(), rule.name, "the refusal must name the rule an operator has to go and null")
				assert.Contains(t, err.Error(), "probe", "and the collection it is on")
			})
		}
	})

	t.Run("the batch endpoint enabled", func(t *testing.T) {
		t.Parallel()

		settings := &core.Settings{}
		settings.Batch.Enabled = true

		err := pb.AssertSettings(settings)

		require.Error(t, err)
		assert.ErrorIs(t, err, pb.ErrBatchEnabled)
		assert.NotErrorIs(t, err, pb.ErrAPIRuleSet)
	})

	t.Run("a file field that is not protected", func(t *testing.T) {
		t.Parallel()

		c := lockedDownCollection("probe")
		c.Fields.Add(&core.FileField{Name: "scan", MaxSelect: 1})

		err := pb.AssertCollections([]*core.Collection{c})

		require.Error(t, err)
		assert.ErrorIs(t, err, pb.ErrFileFieldUnprotected)
		assert.NotErrorIs(t, err, pb.ErrAPIRuleSet)
		assert.Contains(t, err.Error(), "scan")
	})

	t.Run("the three refusals are distinct", func(t *testing.T) {
		t.Parallel()

		assert.NotEqual(t, pb.ErrAPIRuleSet.Error(), pb.ErrBatchEnabled.Error())
		assert.NotEqual(t, pb.ErrAPIRuleSet.Error(), pb.ErrFileFieldUnprotected.Error())
		assert.NotEqual(t, pb.ErrBatchEnabled.Error(), pb.ErrFileFieldUnprotected.Error())
	})
}

// System collections carry PocketBase's own rules — _mfas and _otps both ship a
// non-nil listRule — and MediKube neither owns them nor may null them. The
// "non-system" qualifier in the requirement is load-bearing, so it gets a test.
func TestBootAssertionsLeavePocketBasesOwnSystemCollectionsAlone(t *testing.T) {
	t.Parallel()

	c := core.NewBaseCollection("_probe")
	c.System = true
	c.ListRule = types.Pointer("")

	assert.NoError(t, pb.AssertCollections([]*core.Collection{c}))
}

// Every violation at once, rather than the first one: an operator restarting a
// container should learn everything that is wrong in one line, not one per
// restart.
func TestBootAssertionsReportEveryViolationTogether(t *testing.T) {
	t.Parallel()

	c := lockedDownCollection("probe")
	c.ListRule = types.Pointer("")
	c.Fields.Add(&core.FileField{Name: "scan", MaxSelect: 1})

	err := pb.AssertCollections([]*core.Collection{c})

	require.Error(t, err)
	assert.ErrorIs(t, err, pb.ErrAPIRuleSet)
	assert.ErrorIs(t, err, pb.ErrFileFieldUnprotected)
}

// And the whole assertion against a real instance. PocketBase's stock `users`
// collection has five non-nil rules and an unprotected `avatar` file field, so
// an untouched instance is exactly the thing that must be refused — which is
// also why the migration that nulls those rules is not optional.
func TestAssertLockedDownRefusesAnUntouchedPocketBase(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp()
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	assertErr := pb.AssertLockedDown(app)

	require.Error(t, assertErr, "stock PocketBase ships users with five non-nil rules; refusing it is the whole point")
	assert.ErrorIs(t, assertErr, pb.ErrAPIRuleSet)
	assert.ErrorIs(t, assertErr, pb.ErrFileFieldUnprotected)
}
