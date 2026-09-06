//go:build scale

// scale_test.go is SC-003/FR-089: at 50,000 indexed rows, a grouped search's
// first page returns within 3 seconds and every kind holding a match is
// represented. Build-tagged and invisible to `task test`, run through
// `task test:scale` (Taskfile.yaml), the same reason
// internal/testsupport/phileak is tagged rather than always-on: this is
// minutes of fixture-building wall clock, not seconds.
package search_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// scaleKinds spreads the 50,000 rows over several kinds, rather than piling
// all of them into one, so "100% of the kinds containing a match" (SC-003)
// is a claim this test can actually fail.
var scaleKinds = []kind.Kind{kind.Medication, kind.Allergy, kind.Condition, kind.Encounter}

const scaleRowCount = 50_000

func TestSearchKindAt50000Rows(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	on := date(t, "2026-01-01")

	// Every row carries the term somewhere, so a first page is never starved
	// by rows that do not match — SC-003 is about the read path at scale, not
	// about how selective the term happens to be.
	for i := range scaleRowCount {
		k := scaleKinds[i%len(scaleKinds)]
		h.seedTerm(t, k, recordID(i+1), fmt.Sprintf("scale-term record %d", i), "", &on)
	}

	for _, k := range scaleKinds {
		t.Run(string(k), func(t *testing.T) {
			started := time.Now()

			page, err := h.repo.SearchKind(ctx, h.patient, k, "scale-term", nil, "", 25, "")
			elapsed := time.Since(started)
			require.NoError(t, err)

			t.Logf("first page of %s at %d indexed rows: %s", k, scaleRowCount, elapsed)

			assert.Less(t, elapsed, 3*time.Second,
				"the first page of a grouped search must return within 3s at 50,000 indexed rows (SC-003)")
			assert.NotEmpty(t, page.Items, "every kind holding a match must be represented (SC-003, FR-089)")
			assert.LessOrEqual(t, len(page.Items), 25)
		})
	}
}
