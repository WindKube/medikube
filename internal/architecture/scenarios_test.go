package architecture

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T300, FR-068, SC-004. spec.md publishes 54 acceptance scenarios across its
// six user stories (tasks.md's own count of 50 predates a later edit to the
// spec; spec.md is the corpus this test reads, so 54 is what it is held to).
// Every one of them needs a named automated test, or a recorded, honest reason
// why it does not have one yet — a scenario silently proven by nothing is
// worse than one this table admits is unproven.
//
// scenarioTests is the map SC-004 asks for: a scenario id to the fully
// qualified test that proves it. missingScenarios is the other half — every
// scenario this test could not find a test for, with the one-line reason,
// each of which was verified by hand rather than invented, so the map can be
// trusted more than a comment claiming coverage.

// scenarioTests names, for each proven scenario, the package import path and
// the Test function inside it. A test named twice is a build error (Go does
// not allow two functions of the same name in one package), so uniqueness of
// the VALUE is enforced by the compiler; scenario_ids_test.go asserts
// uniqueness of the KEY.
var scenarioTests = map[string]string{
	"US1-S1": "medikube/internal/web/page.TestAnAccountWithNothingRecordedStillGetsTheLandmark",
	"US1-S2": "medikube/internal/web/api.TestANameAloneIsSufficientAndEverythingElseIsOptional",
	"US1-S3": "medikube/internal/web/api.TestTheCreatedRepresentationIsWhatWasStored",
	"US1-S4": "medikube/internal/web/api.TestABlankNameAndAnEndBeforeTheStartAreTwoEntriesInOneResponse",
	"US1-S7": "medikube/internal/web/page.TestTheDetailPageIsTitledWithTheRecordsOwnName",
	"US1-S8": "medikube/internal/web/stream.TestAWriteReachesASecondOpenViewWithinFiveSeconds",
	"US1-S9": "medikube/internal/web/api.TestAStaleChangeAnswers412CarryingTheCurrentRepresentation",

	"US2-S1":  "medikube/internal/web/api.TestRegisteringCreatesAnAccountAndSignsThePersonIn",
	"US2-S2":  "medikube/internal/web/api.TestAClosedInstanceRefusesEveryRegistration",
	"US2-S3":  "medikube/internal/web/api.TestRegisteringAnAddressThatAlreadyHasAnAccountIsRefused",
	"US2-S4":  "medikube/internal/web/api.TestTheTwoSignInRefusalsAreIndistinguishableInEveryPartOfTheResponse",
	"US2-S5":  "medikube/internal/web/api.TestSigningOutEndsEverySessionTheAccountHadOpen",
	"US2-S6":  "medikube/internal/web/api.TestChangingThePasswordKeepsThisSessionAndEndsTheOthers",
	"US2-S7":  "medikube/internal/web/api.TestNoSpellingOfARoleReachesTheAccount",
	"US2-S8":  "medikube/internal/web/api.TestADeletedAccountIsGoneAndNobodyElseIs",
	"US2-S9":  "medikube/internal/web/api.TestEveryRecoveryRequestIsAnsweredIdentically",
	"US2-S10": "medikube/internal/web/api.TestARecoveryLinkSetsThePasswordAndEndsEverySessionBeforeIt",
	"US2-S11": "medikube/internal/web/api.TestEveryUnusableRecoveryLinkIsAnsweredTheSameWay",

	"US3-S1": "medikube/internal/web/api.TestEveryNotFoundOnTheRecordSurfaceIsTheSameResponse",
	"US3-S2": "medikube/internal/service/access.TestPatientRefusesAStrangerAsANotFoundAndAuditsIt",
	"US3-S3": "medikube/internal/web/api.TestEveryRouteTheTableDoesNotPublishRefusesAnAnonymousCaller",
	"US3-S4": "medikube/internal/web/api.TestNoPocketBaseBrowsingSurfaceAnswersAnOrdinaryAccount",
	"US3-S5": "medikube/internal/testsupport/phileak.TestNothingAPersonRecordedReachesADiagnosticSink",
	"US3-S6": "medikube/internal/platform/pb.TestAnUntouchedInstanceWarns",
	"US3-S7": "medikube/internal/platform/pb.TestASuperuserSessionWritesExactlyOneAdminSessionRow",

	"US4-S1": "medikube/internal/web/views.TestTheShellCarriesAllFourLandmarksInOrderOnEveryPage",
	"US4-S3": "medikube/internal/web/views/shell.TestTheClassIsWrittenOnHTMLWithNoInlineScript",
	"US4-S4": "medikube/internal/web/page.TestNoErrorPageRendersAStackTraceADriverMessageOrAQuery",
	"US4-S5": "medikube/internal/web/page.TestAnErrorPageCarriesTheRequestIdAndNothingElseFromTheFailure",
	"US4-S7": "medikube/internal/web/views/shell.TestEveryPageCarriesANoscriptBlockInsideMain",
	"US4-S8": "medikube/internal/web/views/shell.TestTheTwoLiveRegionsCarryTheirRolesOnEveryPage",

	"US5-S2": "medikube/internal/config.TestValidateRejectsEachSetting",
	"US5-S3": "medikube/internal/web/api.TestHealthzAnswersOkAndTouchesNoDatabase",
	"US5-S4": "medikube/internal/web/api.TestReadyzReportsAFailingDatabaseAndLeaksNothingOfIt",
	"US5-S5": "medikube/internal/obs.TestOneRequestProducesExactlyOneLineCarryingTheCorrelationId",
	"US5-S6": "medikube/internal/obs.TestSentryIsEntirelyOffUntilADSNIsConfigured",
	"US5-S7": "medikube/internal/cli.TestSeedReportsEachAccountOnceAndTheSecondRunSkipsThemAll",
	"US5-S8": "medikube/internal/cli.TestRoutesJSONListsExactlyTheRegistry",
	"US5-S9": "medikube/internal/web/api.TestAnInFlightRequestStillCompletesWhileDraining",

	"US6-S4": "medikube/internal/openapi.TestRegeneratingTheDocumentProducesNoDiff",
	"US6-S5": "medikube/internal/architecture.TestEveryAcceptanceScenarioHasANamedTest",
	"US6-S6": "medikube/internal/architecture.TestEveryClinicalUserRouteIsInTheOwnershipMatrix",
	"US6-S7": "medikube/internal/architecture.TestCIWorkflowRunsEveryGate",
}

// missingScenarios is the honest ledger: every scenario spec.md states that
// no automated test proves today, and why. Each reason was checked by reading
// the candidate tests, not guessed from an FR number in a comment.
var missingScenarios = map[string]string{
	"US1-S5": "no test combines marking a medication stopped-with-a-reason and then asserting BOTH the list shows it stopped AND the detail view shows the reason and the last-changed date as one scenario; the mechanics are proven separately (patch handling, notes round-trip, updated_at) but not together",
	"US1-S6": "no test combines a name filter, most-recently-started sort and a ~1000-row scale with a no-repeat-no-skip page walk in one case; records_bench_test.go proves scale and no-repeat/no-skip without a filter, and records_list_test.go proves filter and sort separately",

	"US2-S12": "TestAnInstanceThatCannotSendMailRefusesTheRequest proves the plain refusal; no test proves the operator is warned about the same condition at every start-up (the admin-access warning has one, mail configuration does not)",

	"US3-S8": "no test observes zero outbound network dials at the transport level; coverage is only per-subsystem (Sentry, tracing) staying inactive until configured, which is necessary but not sufficient for this scenario's claim",

	"US4-S2": "phone-width, no-horizontal-scroll is an e2e/smoke.spec.ts assertion at the 390x844 viewport; a Go-only source walk over *_test.go cannot see a Playwright spec",
	"US4-S6": "keyboard reachability and focus visibility are e2e/smoke.spec.ts assertions; not visible to a Go-only source walk",

	"US5-S1": "starting the real binary against an empty data directory with only required settings is exercised by hand per quickstart.md (T308), not by an automated Go test",

	"US6-S1": "every page loading with zero browser errors at two viewports is e2e/smoke.spec.ts's whole job; not visible to a Go-only source walk",
	"US6-S2": "the red-gate demonstration for a broken page (T295/T296) is e2e-owned and tracked in e2e/README.md, not proven by a Go test",
	"US6-S3": "internal/httproute/gate_test.go (T294 — registry/OpenAPI/Playwright-list three-way agreement) is assigned to another agent working this same phase and is not present in this worktree yet",
	"US6-S8": "a clean-checkout image build is proven by the `image` job in .github/workflows/go.yaml actually running, and by T305's manual verification — not by a Go test",
}

func TestEveryAcceptanceScenarioHasANamedTest(t *testing.T) {
	t.Parallel()

	scenarios := acceptanceScenarios(t)
	require.Len(t, scenarios, 54,
		"spec.md's acceptance scenarios no longer number 54 — recount and update this test's expectations")

	var unclaimed, doubleClaimed []string

	for _, id := range scenarios {
		_, proven := scenarioTests[id]
		_, missing := missingScenarios[id]

		switch {
		case proven && missing:
			doubleClaimed = append(doubleClaimed, id)
		case !proven && !missing:
			unclaimed = append(unclaimed, id)
		}
	}

	assert.Emptyf(t, unclaimed,
		"these scenarios have neither a test in scenarioTests nor a recorded reason in missingScenarios: %v", unclaimed)
	assert.Emptyf(t, doubleClaimed,
		"these scenarios are claimed as both proven and missing: %v", doubleClaimed)

	for id, qualified := range scenarioTests {
		assert.Truef(t, testFunctionExists(t, qualified), "%s names %s, which does not exist", id, qualified)
	}

	// The gate: a NEW gap is a regression and fails the build. A gap already
	// on this ledger stays failing-safe (fixing it is a welcome edit to both
	// maps, never required by this test), but the set may not grow silently.
	for id := range missingScenarios {
		assert.Containsf(t, scenarios, id, "missingScenarios records %s, which spec.md no longer lists", id)
	}

	if t.Failed() {
		return
	}

	t.Logf("%d scenarios proven, %d recorded as unproven: %v", len(scenarioTests), len(missingScenarios), sortedKeys(missingScenarios))
}

// acceptanceScenarios parses spec.md's six "### User Story N" sections and
// numbers each section's "**Acceptance Scenarios**:" list as USn-Sk, k being
// the scenario's 1-based position in that story's own list — exactly the
// numbering already visible in the document.
func acceptanceScenarios(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "specs/001-walking-skeleton/spec.md"))
	require.NoError(t, err)

	storyHeading := regexp.MustCompile(`(?m)^### User Story (\d+) - `)
	locs := storyHeading.FindAllStringSubmatchIndex(string(raw), -1)
	require.NotEmpty(t, locs, "spec.md has no '### User Story N' headings — the parser or the document changed shape")

	text := string(raw)

	var ids []string

	for i, loc := range locs {
		storyNum := text[loc[2]:loc[3]]

		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}

		section := text[loc[1]:end]

		scenariosAt := strings.Index(section, "**Acceptance Scenarios**:")
		require.GreaterOrEqualf(t, scenariosAt, 0, "User Story %s has no Acceptance Scenarios list", storyNum)

		list := section[scenariosAt:]
		if next := strings.Index(list, "\n---"); next >= 0 {
			list = list[:next]
		}

		scenarioLine := regexp.MustCompile(`(?m)^\d+\. \*\*Given\*\*`)
		matches := scenarioLine.FindAllStringIndex(list, -1)
		require.NotEmptyf(t, matches, "User Story %s's Acceptance Scenarios list has no numbered scenarios", storyNum)

		for k := range matches {
			ids = append(ids, fmt.Sprintf("US%s-S%d", storyNum, k+1))
		}
	}

	return ids
}

// testFunctionExists resolves "medikube/internal/web/api.TestFoo" to a
// directory and a function name, and confirms some *_test.go file in that
// directory declares it. It does not run the test — existence, not passage,
// is what a coverage map asserts; the suite itself is what proves passage.
func testFunctionExists(t *testing.T, qualified string) bool {
	t.Helper()

	dot := strings.LastIndex(qualified, ".")
	require.Greaterf(t, dot, 0, "%q is not package.Test — no separating dot", qualified)

	importPath, funcName := qualified[:dot], qualified[dot+1:]

	const module = "medikube"
	require.Truef(t, strings.HasPrefix(importPath, module), "%q is not under the medikube module", importPath)

	dir := filepath.Join(repoRoot(t), filepath.FromSlash(strings.TrimPrefix(importPath, module+"/")))

	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	declPattern := regexp.MustCompile(`(?m)^func ` + regexp.QuoteMeta(funcName) + `\(t \*testing\.T\)`)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		content, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		require.NoError(t, readErr)

		if declPattern.Match(content) {
			return true
		}
	}

	return false
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	// Deterministic order for a passing test's log line only; not asserted.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}

	return keys
}
