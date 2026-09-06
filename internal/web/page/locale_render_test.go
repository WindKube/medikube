package page_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
	"medikube/internal/testsupport"
)

// T030 (US1-2, US1-3, SC-001): every signed-in page route's smoke URL (and
// every SmokeVariants entry), rendered for a Polish account, carries no
// English page title and no English nav label, and the seeded user data
// (a diagnosis, an allergen, a patient's name) is shown exactly as typed.
// The same URLs are also checked as an English account, which is what proves
// the English strings this test looks for are real ones the gate would
// otherwise find — not typos that never occur anywhere.

// englishValues reads active.en.toml directly (rather than through
// medikube/internal/i18n, which exposes no way to list every id) and returns
// the "other" text of every message whose id passes match — every page.*.title
// and every nav.* value, at the time of writing.
func englishValues(t *testing.T, match func(id string) bool) []string {
	t.Helper()

	path := filepath.Join("..", "..", "i18n", "locales", "active.en.toml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	idPattern := regexp.MustCompile(`^\[([a-zA-Z0-9_.]+)\]`)
	otherPattern := regexp.MustCompile(`(?m)^other = "([^"]*)"`)

	var values []string
	for _, block := range strings.Split(string(data), "\n\n") {
		idMatch := idPattern.FindStringSubmatch(block)
		if idMatch == nil || !match(idMatch[1]) {
			continue
		}

		otherMatch := otherPattern.FindStringSubmatch(block)
		if otherMatch == nil {
			continue
		}

		values = append(values, otherMatch[1])
	}

	require.NotEmpty(t, values, "the id filter matched nothing in active.en.toml")

	return values
}

func isPageTitleID(id string) bool {
	return strings.HasPrefix(id, "page.") && strings.HasSuffix(id, ".title")
}

func isNavID(id string) bool {
	return strings.HasPrefix(id, "nav.")
}

// signedInPageSmokeURLs is every signed-in-only page route's own SmokeURL
// plus every SmokeVariants entry — the anonymous-only auth pages (login,
// register, recovery) carry no account locale to render in, and are US2's
// scope, not this one's.
func signedInPageSmokeURLs(t *testing.T) map[string][]string {
	t.Helper()

	urls := map[string][]string{}

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage || route.Auth != httproute.AuthUser {
			continue
		}

		require.NotEmpty(t, route.SmokeURL, "%s declares no smoke URL", route.OpID)

		all := append([]string{route.SmokeURL}, route.SmokeVariants...)
		urls[route.OpID] = all
	}

	require.NotEmpty(t, urls, "the route table declares no signed-in page")

	return urls
}

func TestEverySignedInPageRendersInPolishForAPolishAccount(t *testing.T) {
	t.Parallel()

	englishTitles := englishValues(t, isPageTitleID)
	englishNav := englishValues(t, isNavID)

	for opID, urls := range signedInPageSmokeURLs(t) {
		for _, url := range urls {
			t.Run(opID+" "+url, func(t *testing.T) {
				t.Parallel()

				rig := newBrowser(t)
				setAccountLocale(t, rig, testsupport.AccountAEmail, "pl")

				status, _, body := rig.get(url)
				require.Equal(t, 200, status, "%s: %s", opID, body)

				assert.Contains(t, body, `lang="pl"`, "%s did not render in Polish", opID)

				for _, title := range englishTitles {
					assert.NotContains(t, body, title, "%s leaked the English page title %q", opID, title)
				}

				for _, label := range englishNav {
					assert.NotContains(t, body, label, "%s leaked the English nav label %q", opID, label)
				}
			})
		}
	}
}

// TestEverySignedInPageStillRendersInEnglish is this test's own guard: an
// English account opening the same URLs still renders in English and still
// carries a <title>, which is what proves the harness itself works (a page
// that answered a redirect or an error view would fail here rather than
// silently passing the Polish assertion above by rendering nothing).
func TestEverySignedInPageStillRendersInEnglish(t *testing.T) {
	t.Parallel()

	for opID, urls := range signedInPageSmokeURLs(t) {
		url := urls[0]

		t.Run(opID, func(t *testing.T) {
			t.Parallel()

			rig := newBrowser(t)

			status, _, body := rig.get(url)
			require.Equal(t, 200, status, "%s: %s", opID, body)

			assert.Contains(t, body, `lang="en"`, "%s did not render in English", opID)
			assert.Contains(t, body, "<title>", "%s rendered no <title>", opID)
		})
	}
}

// TestSomeKnownEnglishPageTitleActuallyAppears is englishValues' own guard:
// at least one page in the inventory titles itself from a page.*.title id, so
// TestEverySignedInPageRendersInPolishForAPolishAccount's "no English title
// leaked" assertion is checking real strings and not ones nothing renders.
func TestSomeKnownEnglishPageTitleActuallyAppears(t *testing.T) {
	t.Parallel()

	englishTitles := englishValues(t, isPageTitleID)

	rig := newBrowser(t)

	found := false
	for opID, urls := range signedInPageSmokeURLs(t) {
		status, _, body := rig.get(urls[0])
		require.Equal(t, 200, status, "%s", opID)

		for _, title := range englishTitles {
			if strings.Contains(body, title) {
				found = true
				break
			}
		}

		if found {
			break
		}
	}

	assert.True(t, found, "no page in the inventory renders any of the collected page.*.title values; englishValues may be stale")
}

// TestPolishPatientDetailShowsSeededDataVerbatim is US1-3: the record fields
// people typed — a diagnosis, an allergen, a patient's own name — are shown
// exactly as entered, never translated, even though every label around them
// is Polish.
func TestPolishPatientDetailShowsSeededDataVerbatim(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t)
	setAccountLocale(t, rig, testsupport.AccountAEmail, "pl")

	t.Run("the patient's own name", func(t *testing.T) {
		t.Parallel()

		status, _, body := rig.get("/patients/" + testsupport.AccountAPatientSelfID)
		require.Equal(t, 200, status, body)
		assert.Contains(t, body, testsupport.AccountAName)
	})

	t.Run("a seeded diagnosis", func(t *testing.T) {
		t.Parallel()

		status, _, body := rig.get("/" + kind.Condition.Segment() + "/" + testsupport.ResolvedConditionID)
		require.Equal(t, 200, status, body)
		assert.Contains(t, body, "Bacterial pneumonia")
	})

	t.Run("a seeded allergen", func(t *testing.T) {
		t.Parallel()

		status, _, body := rig.get("/" + kind.Allergy.Segment() + "/" + testsupport.CriticalAllergyID)
		require.Equal(t, 200, status, body)
		assert.Contains(t, body, "Penicillin")
	})
}
