package audit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// One body for three vocabularies. `valid` and `all` arrive as method
// expressions, so the test names the function it asserts rather than a closure
// that could quietly call a different one.
func assertVocabulary[T ~string](t *testing.T, want []T, all func() []T, valid func(T) bool, rejected []string) {
	t.Helper()

	require.Equal(t, want, all(),
		"the accessor publishes the vocabulary the migration declares, in the declared order")

	for _, value := range want {
		t.Run("accepts "+string(value), func(t *testing.T) {
			t.Parallel()

			assert.True(t, valid(value), "%q is declared and must therefore be accepted", value)
		})
	}

	for _, value := range rejected {
		t.Run("refuses "+value, func(t *testing.T) {
			t.Parallel()

			assert.False(t, valid(T(value)), "%q is not declared and must be refused", value)
		})
	}

	t.Run("no value is declared twice", func(t *testing.T) {
		t.Parallel()

		seen := make(map[T]struct{}, len(want))
		for _, value := range want {
			_, duplicate := seen[value]
			require.False(t, duplicate, "%q is declared twice; a select field would silently keep one", value)
			seen[value] = struct{}{}
		}
	})

	t.Run("every value is lower snake_case", func(t *testing.T) {
		t.Parallel()

		for _, value := range want {
			assert.Equal(t, strings.ToLower(string(value)), string(value),
				"the spelling is the stored select value and the wire value; %q is not it", value)
			assert.NotContains(t, string(value), "-", "%q is snake_case, not kebab-case", value)
			assert.NotContains(t, string(value), " ", "%q carries a space", value)
		}
	})

	t.Run("the accessor clones", func(t *testing.T) {
		t.Parallel()

		got := all()
		require.NotEmpty(t, got)
		got[0] = T("mutated")

		assert.Equal(t, want, all(), "a caller reordered or overwrote the vocabulary for everybody")
	})
}

// data-model §3's three vocabularies, declared in full. The counts are asserted
// separately from the values because the count is the thing a later phase
// breaks: `action` and `target_kind` are declared complete in this phase's
// migration precisely so no later phase writes an undeclared select value.
func TestEachVocabularyIsExactlyWhatTheMigrationDeclares(t *testing.T) {
	t.Parallel()

	t.Run("ActorKind", func(t *testing.T) {
		t.Parallel()

		want := []ActorKind{ActorKindUser, ActorKindAdmin, ActorKindSuperuser, ActorKindSystem}

		assert.Len(t, ActorKinds(), 4)
		assertVocabulary(t, want, ActorKinds, ActorKind.Valid,
			[]string{"", " ", "User", "SYSTEM", "anonymous", "guest", "medication"})
	})

	t.Run("Action", func(t *testing.T) {
		t.Parallel()

		want := []Action{
			ActionCreate,
			ActionUpdate,
			ActionDelete,
			ActionAccessDenied,
			ActionLogin,
			ActionLoginFailed,
			ActionLogout,
			ActionPasswordChange,
			ActionAccountDelete,
			ActionAdminSession,
			ActionReadSensitive,
			ActionShareGrant,
			ActionShareRevoke,
			ActionShareExpire,
			ActionInviteSend,
			ActionInviteRespond,
			ActionExport,
			ActionBackupCreate,
			ActionBackupRestore,
			ActionEmailChange,
		}

		assert.Len(t, Actions(), 20,
			"data-model §3: nineteen shared-contract values plus access_denied, declared in full")
		assertVocabulary(t, want, Actions, Action.Valid,
			[]string{"", " ", "Create", "read", "access-denied", "accessDenied", "user"})
	})

	t.Run("TargetKind", func(t *testing.T) {
		t.Parallel()

		want := []TargetKind{
			TargetKindMedication,
			TargetKindAllergy,
			TargetKindCondition,
			TargetKindEncounter,
			TargetKindProcedure,
			TargetKindTreatment,
			TargetKindSymptom,
			TargetKindVitals,
			TargetKindImmunization,
			TargetKindInjury,
			TargetKindInsurance,
			TargetKindEquipment,
			TargetKindEmergencyContact,
			TargetKindFamilyMember,
			TargetKindLabResult,
			TargetKindPatient,
			TargetKindUser,
			TargetKindShare,
			TargetKindInvitation,
			TargetKindAttachment,
			TargetKindExport,
			TargetKindBackup,
			TargetKindSystem,
		}

		assert.Len(t, TargetKinds(), 23,
			"data-model §3: fifteen record kinds and eight platform kinds, declared in full")
		assertVocabulary(t, want, TargetKinds, TargetKind.Valid,
			[]string{"", " ", "Medication", "medications", "practitioner", "facility", "tag", "search", "login"})
	})
}

// The mechanical half of "the fifteen record kinds are the same fifteen".
// Phase 003 adds thirteen kinds and phase 004 the fifteenth; a kind whose audit
// target is undeclared would fail select validation the first time somebody
// created one, in production, rather than here.
func TestEveryRecordKindIsADeclaredTargetKind(t *testing.T) {
	t.Parallel()

	for _, k := range kind.Kinds() {
		t.Run(string(k), func(t *testing.T) {
			t.Parallel()

			assert.True(t, TargetKind(k).Valid(),
				"kind %q has no audit target kind, so its rows could not be written", k)
		})
	}
}
