package openapi_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/openapi"
)

// T102. Three properties of the document as a whole: it declares OpenAPI 3.1,
// its operationIds are unique and are exactly contracts/README.md's inventory,
// and every operation states its authorization rule where a reader will see it.

// contracts/README.md's "Operation inventory — 22", transcribed. This is the
// one place in the codebase that holds it, and it is deliberately a literal
// list: a gate that computes the expected inventory from the thing under test
// asserts nothing. The eleven documented PocketBase-native paths are counted
// separately, below, because contracts/README.md names them by path and the
// registry is what gave them operationIds.
var contractInventory = []string{
	"getAuthConfig",
	"register",
	"login",
	"refreshSession",
	"logout",
	"getMe",
	"updateMe",
	"deleteMe",
	"changePassword",
	"listRecords",
	"listRecordsOfKind",
	"createRecord",
	"getRecord",
	"updateRecord",
	"deleteRecord",
	"streamRecords",
	"healthz",
	"readyz",
	"requestPasswordReset",
	"confirmPasswordReset",
	"requestEmailVerification",
	"confirmEmailVerification",
	"listPatients",
	"createPatient",
	"getPatient",
	"updatePatient",
	"deletePatient",
	"getPatientChart",
	"putPatientPhoto",
	"getPatientPhoto",
	"deletePatientPhoto",
	"setActivePatient",
}

// contracts/practitioners.md and contracts/facilities.md's ten operations
// (phase 002-patient-core, US5), transcribed the same way and kept separate
// from contractInventory: that literal is contracts/README.md's own count and
// stays what phase 001 pinned.
var directoryInventory = []string{
	"listPractitioners",
	"createPractitioner",
	"getPractitioner",
	"updatePractitioner",
	"deletePractitioner",
	"listFacilities",
	"createFacility",
	"getFacility",
	"updateFacility",
	"deleteFacility",
}

// The operations that resolve an id or a kind out of the stored data. For these
// the authorization rule is not merely "a session is required": another
// account's id and an id that never existed answer identically (FR-033).
var ownerScoped = []string{
	"listRecords",
	"listRecordsOfKind",
	"createRecord",
	"getRecord",
	"updateRecord",
	"deleteRecord",
	"streamRecords",
	"listPatients",
	"createPatient",
	"getPatient",
	"updatePatient",
	"deletePatient",
	"getPatientChart",
	"putPatientPhoto",
	"getPatientPhoto",
	"deletePatientPhoto",
	"listPractitioners",
	"createPractitioner",
	"getPractitioner",
	"updatePractitioner",
	"deletePractitioner",
	"listFacilities",
	"createFacility",
	"getFacility",
	"updateFacility",
	"deleteFacility",
}

func TestTheDocumentDeclaresOpenAPI31(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	assert.Equal(t, "3.1.0", loaded.OpenAPI)

	// The string alone is not enough. kin-openapi's version switch is a
	// hardcoded list, an unrecognised spelling such as "3.1.3" yields an empty
	// major/minor, and the document is then validated as 3.0 with no error at
	// all — so a document that SAYS 3.1 can be checked as if it did not.
	assert.True(t, loaded.IsOpenAPI31OrLater(), "the version string is not one kin-openapi recognises as 3.1")
	assert.False(t, loaded.IsOpenAPI30())
}

func TestTheDocumentIsIdentified(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	require.NotNil(t, loaded.Info)
	assert.NotEmpty(t, loaded.Info.Title)
	assert.Equal(t, fixtureVersion, loaded.Info.Version)
}

func TestGenerateRefusesADocumentWithNoVersion(t *testing.T) {
	t.Parallel()

	in := twoKindInput()
	in.Version = ""

	_, err := openapi.Generate(in)
	require.Error(t, err)
}

func TestTheAPIOperationsAreExactlyTheContractInventory(t *testing.T) {
	t.Parallel()

	require.Len(t, contractInventory, 32, "contracts/README.md's inventory plus 002-patient-core's patient operations is 32 operations")

	loaded := roundTrip(t, generate(t, twoKindInput()))
	documented := operationsByID(t, loaded)

	var externals []string

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind == httproute.KindExternal {
			externals = append(externals, route.OpID)
		}
	}

	assert.Len(t, externals, 11, "contracts/README.md documents eleven PocketBase-native paths")

	found := make([]string, 0, len(documented))
	for opID := range documented {
		found = append(found, opID)
	}

	expected := append(append([]string{}, contractInventory...), directoryInventory...)
	assert.ElementsMatch(t, append(expected, externals...), found)
}

// kin-openapi does check operationId uniqueness — and returns on the FIRST
// duplicate even under EnableMultiError, so with forty-three operations a bulk
// rename can hide behind one report. operationsByID asserts every one.
func TestEveryOperationIDIsUnique(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))

	assert.Len(t, operationsByID(t, loaded), 53)
}

func TestEveryOperationCarriesItsAuthorizationRule(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))
	documented := operationsByID(t, loaded)

	auth := make(map[string]httproute.Auth)
	for _, route := range documentedRoutes(t) {
		auth[route.OpID] = route.Auth
	}

	for opID, op := range documented {
		t.Run(opID, func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, op.op.Summary, "an operation nobody described is an operation nobody can review")

			description := op.op.Description
			require.NotEmpty(t, description)
			require.Truef(t, strings.HasPrefix(description, "Authorization:"),
				"%s's description does not open with its authorization rule: %q", opID, description)

			switch auth[opID] {
			case httproute.AuthPublic:
				assert.Contains(t, description, "public")
			case httproute.AuthUser:
				assert.Contains(t, description, "requires a session")
			case httproute.AuthAdmin:
				assert.Contains(t, description, "superuser")
			}
		})
	}
}

// The universal rule of contracts/README.md: on owner-scoped data an
// authorization failure and a genuine not-found are the same answer. A
// published description that does not say so invites a client to treat 404 as
// "gone" and retry.
func TestOwnerScopedOperationsPublishTheFourOhFourRule(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))
	documented := operationsByID(t, loaded)

	for _, opID := range ownerScoped {
		op, present := documented[opID]
		require.Truef(t, present, "%s is not in the document", opID)

		assert.Containsf(t, op.op.Description, "404",
			"%s is owner-scoped but its description does not publish the 404 rule (FR-033)", opID)
	}
}

func TestPublicOperationsDeclareNoSecurityAndTheRestDeclareSome(t *testing.T) {
	t.Parallel()

	loaded := roundTrip(t, generate(t, twoKindInput()))
	documented := operationsByID(t, loaded)

	require.NotNil(t, loaded.Components)
	require.NotEmpty(t, loaded.Components.SecuritySchemes)

	for _, route := range documentedRoutes(t) {
		op, present := documented[route.OpID]
		require.True(t, present)

		security := op.op.Security
		require.NotNilf(t, security, "%s declares no security at all, so it inherits the document's", route.OpID)

		if route.Auth == httproute.AuthPublic {
			assert.Emptyf(t, *security, "%s is public but demands a credential", route.OpID)

			continue
		}

		require.NotEmptyf(t, *security, "%s requires %s but documents no credential", route.OpID, route.Auth)

		for _, requirement := range *security {
			for scheme := range requirement {
				_, declared := loaded.Components.SecuritySchemes[scheme]
				assert.Truef(t, declared, "%s requires the undeclared security scheme %q", route.OpID, scheme)
			}
		}
	}
}
