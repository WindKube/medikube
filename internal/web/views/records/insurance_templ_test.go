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

func insuranceLinks(id string) records.InsuranceLinks {
	return records.InsuranceLinks{
		Detail: "/" + kind.Insurance.Segment() + "/" + id,
		Record: "/api/v1/records/" + kind.Insurance.Segment() + "/" + id,
	}
}

// TestARowExpiringSoonRendersItsBasisPill is FR-046: a policy the list was
// narrowed by ?expiring_within_days= carries a reason on its own row.
func TestARowExpiringSoonRendersItsBasisPill(t *testing.T) {
	t.Parallel()

	policy := records.NewInsuranceView(clinical.Insurance{ID: "ins1", Company: "Acme Health"},
		[]string{"expiring within 45 days"}, insuranceLinks("ins1"))

	tree := viewstest.Render(t, records.InsuranceRow(policy), "tbody")

	pill := tree.One(t, viewstest.WithID(ids.RecordBasis(kind.Insurance, policy.ID)))
	assert.Contains(t, viewstest.Text(pill), "expiring within 45 days")
}

// TestARowNotExpiringRendersNoBasisPill: a row with no basis carries no pill
// to begin with, so a later change to the narrowing logic that starts
// stamping every row would be caught here.
func TestARowNotExpiringRendersNoBasisPill(t *testing.T) {
	t.Parallel()

	policy := records.NewInsuranceView(clinical.Insurance{ID: "ins2", Company: "Acme Health"},
		nil, insuranceLinks("ins2"))

	tree := viewstest.Render(t, records.InsuranceRow(policy), "tbody")

	require.Empty(t, tree.All(viewstest.WithID(ids.RecordBasis(kind.Insurance, policy.ID))))
}
