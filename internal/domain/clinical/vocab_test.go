package clinical

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSharedLaddersAcceptExactlyThePublishedValues(t *testing.T) {
	t.Parallel()

	t.Run("Severity", func(t *testing.T) {
		t.Parallel()
		for _, v := range []Severity{"mild", "moderate", "severe", "life_threatening"} {
			assert.True(t, v.Valid())
		}
		assert.False(t, Severity("").Valid())
		assert.False(t, Severity("Mild").Valid())
		assert.False(t, Severity("life-threatening").Valid())
	})

	t.Run("ConditionStatus", func(t *testing.T) {
		t.Parallel()
		for _, v := range []ConditionStatus{"active", "healing", "inactive", "resolved", "chronic"} {
			assert.True(t, v.Valid())
		}
		assert.False(t, ConditionStatus("").Valid())
		assert.False(t, ConditionStatus("Active").Valid())
	})

	t.Run("OrderStatus", func(t *testing.T) {
		t.Parallel()
		for _, v := range []OrderStatus{"ordered", "scheduled", "in_progress", "completed", "cancelled"} {
			assert.True(t, v.Valid())
		}
		assert.False(t, OrderStatus("").Valid())
		assert.False(t, OrderStatus("In-Progress").Valid())
	})

	t.Run("TherapyStatus, phase 001's existing ladder", func(t *testing.T) {
		t.Parallel()
		for _, v := range []TherapyStatus{"active", "on_hold", "completed", "stopped", "cancelled"} {
			assert.True(t, v.Valid())
		}
	})
}

func TestAllIsStableAndDeduplicated(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Severities(), Severities())
	assert.Len(t, Severities(), 4)
	seen := map[Severity]bool{}
	for _, v := range Severities() {
		assert.False(t, seen[v], "duplicate %s", v)
		seen[v] = true
	}
}
