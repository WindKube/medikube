package records_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/web/views/ids"
	"medikube/internal/web/views/records"
	"medikube/internal/web/views/viewstest"
)

func equipmentLinks(id string) records.EquipmentLinks {
	return records.EquipmentLinks{
		Detail: "/" + kind.Equipment.Segment() + "/" + id,
		Record: "/api/v1/records/" + kind.Equipment.Segment() + "/" + id,
	}
}

// TestARowOverdueForServiceRendersItsBasisPill is FR-049: an item the list
// was narrowed by ?service_due_within_days= carries a reason on its own row,
// distinguishing overdue from due-soon.
func TestARowOverdueForServiceRendersItsBasisPill(t *testing.T) {
	t.Parallel()

	item := records.NewEquipmentView(clinical.Equipment{ID: "eqp1", Name: "CPAP machine"},
		[]string{"overdue for service"}, equipmentLinks("eqp1"))

	tree := viewstest.Render(t, records.EquipmentRow(item), "tbody")

	pill := tree.One(t, viewstest.WithID(ids.RecordBasis(kind.Equipment, item.ID)))
	assert.Contains(t, viewstest.Text(pill), "overdue for service")
}

// TestARowNotDueRendersNoBasisPill: no basis, no pill.
func TestARowNotDueRendersNoBasisPill(t *testing.T) {
	t.Parallel()

	item := records.NewEquipmentView(clinical.Equipment{ID: "eqp2", Name: "CPAP machine"},
		nil, equipmentLinks("eqp2"))

	tree := viewstest.Render(t, records.EquipmentRow(item), "tbody")

	require.Empty(t, tree.All(viewstest.WithID(ids.RecordBasis(kind.Equipment, item.ID))))
}
