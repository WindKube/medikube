package migrations_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/store/migrations"
	"medikube/internal/testsupport"
	"medikube/internal/web/apitest"
)

// T203. This phase's own collections, on top of the ones phase 001 and 002
// already proved locked down: the thirteen new record kinds plus tags and
// search_index. treatment_medications is left out on purpose — US6 is still
// in flight and lands its own lockdown case alongside the link work.
var phase003Collections = []string{
	migrations.TagsCollection,
	migrations.SearchIndexCollection,
	kind.Allergy.Collection(),
	kind.Condition.Collection(),
	kind.EmergencyContact.Collection(),
	kind.Encounter.Collection(),
	kind.Procedure.Collection(),
	kind.Equipment.Collection(),
	kind.Treatment.Collection(),
	kind.Symptom.Collection(),
	kind.Vitals.Collection(),
	kind.Immunization.Collection(),
	kind.Injury.Collection(),
	kind.Insurance.Collection(),
	kind.FamilyMember.Collection(),
}

// probeID is a syntactically valid record id that exists nowhere, mirroring
// internal/platform/pb/lockdown_test.go's own probe: under the lockdown it
// never reaches a handler, so it does not matter that it is not real.
const probeID = "abc123def456ghi"

// requestsAgainst is every method PocketBase binds under
// /api/collections/{collection}/records.
func requestsAgainst(collection string) []struct{ method, url, body string } {
	base := "/api/collections/" + collection + "/records"

	return []struct{ method, url, body string }{
		{http.MethodGet, base, ""},
		{http.MethodGet, base + "/" + probeID, ""},
		{http.MethodPost, base, `{}`},
		{http.MethodPatch, base + "/" + probeID, `{}`},
		{http.MethodDelete, base + "/" + probeID, ""},
	}
}

// TestPhase003CollectionsAre404ToAnOrdinaryUserWithAllFiveRulesNil is T203:
// one tests.ApiScenario, and one TestApp, per request against each collection
// this phase added, proving the whole record-CRUD subtree is 404 to a normal
// authenticated user (Constitution V) and that the collection's five API
// rules are the nil the migration wrote (data-model §8's "every migration's
// up sets all five API rules to nil explicitly").
func TestPhase003CollectionsAre404ToAnOrdinaryUserWithAllFiveRulesNil(t *testing.T) {
	t.Parallel()

	for _, collection := range phase003Collections {
		t.Run(collection, func(t *testing.T) {
			t.Parallel()

			for _, req := range requestsAgainst(collection) {
				t.Run(req.method, func(t *testing.T) {
					t.Parallel()

					headers := map[string]string{}

					scenario := tests.ApiScenario{
						Method:          req.method,
						URL:             req.url,
						Body:            strings.NewReader(req.body),
						Headers:         headers,
						ExpectedStatus:  http.StatusNotFound,
						ExpectedContent: []string{`"request_id"`},
						TestAppFactory:  apitest.Factory(),
						BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
							t.Helper()

							headers["Authorization"] = testsupport.UserToken(t, app, testsupport.AccountAEmail)
						},
						AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
							t.Helper()

							assertAllFiveRulesNil(t, app, collection)
						},
					}
					scenario.Test(t)
				})
			}
		})
	}
}

func assertAllFiveRulesNil(t testing.TB, app *tests.TestApp, collection string) {
	t.Helper()

	found, err := app.FindCollectionByNameOrId(collection)
	require.NoError(t, err)

	assert.Nilf(t, found.ListRule, "%s.listRule", collection)
	assert.Nilf(t, found.ViewRule, "%s.viewRule", collection)
	assert.Nilf(t, found.CreateRule, "%s.createRule", collection)
	assert.Nilf(t, found.UpdateRule, "%s.updateRule", collection)
	assert.Nilf(t, found.DeleteRule, "%s.deleteRule", collection)
}
