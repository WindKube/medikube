package i18n

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collapsedFormsAllowed is every plural id where Polish grammar legitimately
// renders two of {one, few, many} identically, so the blanket distinctness
// check below does not treat it as a translation bug:
//
//   - kind.equipment: "sprzęt" is a mass noun; its few form ("2 sprzęt") is
//     identical to its one form ("1 sprzęt") the same way English "one
//     equipment"/"two equipment" would be, if English pluralised it at all.
//   - tag.delete_consequence: the counted noun sits after "do" (a
//     preposition that always governs the genitive, regardless of how many
//     of the thing there are), so its few and many forms both take the
//     genitive plural "wpisów" — the few/many distinction only surfaces in
//     the nominative case, which this sentence never uses.
var collapsedFormsAllowed = map[string]bool{
	"kind.equipment":         true,
	"tag.delete_consequence": true,
}

// pluralIDs lists every plural message id the shipped catalogue carries: any
// message with two or more CLDR forms set (catalogue_test.go's own
// formsPresent, which fixed-form messages fail since they set only "other").
func pluralIDs(t *testing.T) []string {
	t.Helper()

	files, err := parseCatalogue(localeFS, localesDir)
	require.NoError(t, err)

	ref, ok := files["en"]
	require.True(t, ok, "no active.en.toml")

	ids := make([]string, 0, len(ref.messages))

	for id, m := range ref.messages {
		if len(formsPresent(m)) >= 2 {
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)

	return ids
}

// TestEveryPluralIDRendersInPolish proves every plural id in the catalogue
// actually resolves to text (never falls back to the bare id) at every count
// CLDR's Polish rule treats differently: 1 (one), 2 (few), 5 (many), and 22
// (few again — 22 % 10 == 2 and 22 isn't in the 12-14 exception), and that
// the id's one/few/many forms differ from one another except where Polish
// grammar itself collapses two of them (collapsedFormsAllowed).
func TestEveryPluralIDRendersInPolish(t *testing.T) {
	t.Parallel()

	ctx := With(context.Background(), Resolve("pl", ""))

	for _, id := range pluralIDs(t) {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			counts := map[string]int{"one": 1, "few-2": 2, "many": 5, "few-22": 22}
			rendered := make(map[string]string, len(counts))

			for label, count := range counts {
				text := N(ctx, id, count)
				assert.NotEqual(t, id, text, "count %d rendered the bare id, not text", count)
				// The rendered count itself always differs (2 vs 22); what
				// this test cares about is the wording around it, so the
				// count's own digits are trimmed back off before comparing.
				rendered[label] = strings.Replace(text, strconv.Itoa(count), "N", 1)
			}

			assert.Equal(t, rendered["few-2"], rendered["few-22"],
				"2 and 22 both fall in Polish's few category and must render identically")

			if collapsedFormsAllowed[id] {
				return
			}

			assert.NotEqual(t, rendered["one"], rendered["few-2"],
				"one and few render identically; is that intentional (add to collapsedFormsAllowed)?")
			assert.NotEqual(t, rendered["few-2"], rendered["many"],
				"few and many render identically; is that intentional (add to collapsedFormsAllowed)?")
			assert.NotEqual(t, rendered["one"], rendered["many"],
				"one and many render identically; is that intentional (add to collapsedFormsAllowed)?")
		})
	}
}
