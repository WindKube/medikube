package identity_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/service/identity"
	"medikube/internal/service/identity/identitytest"
)

// T207. Two walks, in opposite directions, because a hand-written list of six
// actions rots the moment a seventh is written and neither direction alone
// notices.
//
// The first walk goes over the SERVICE'S METHOD SET: every method must be
// exercised here and must declare which rows it writes, so a method added next
// year fails this test on the day it is added rather than shipping unaudited.
//
// The second goes over THE DECLARED VOCABULARY: every one of the twenty values
// audit.Actions() publishes must be accounted for, so a value nobody can say
// who writes is an offence rather than a green tick.

// where an action value is written. The point of the four constants is that
// "this service writes it" and "nothing in 001–006 writes it" are different
// claims, and a value nobody has classified is neither.
const (
	byThisService = "internal/service/identity"
	byAHook       = "internal/platform/pb/hooks.go"
	byAnother     = "another service in this phase"
	byALaterPhase = "no phase in 001–006"
)

// The whole of audit.Actions(), each said to be written somewhere. A value
// missing from here fails the second walk below.
var writtenBy = map[audit.Action]string{
	audit.ActionCreate:         byThisService,
	audit.ActionUpdate:         byThisService,
	audit.ActionLoginFailed:    byThisService,
	audit.ActionLogout:         byThisService,
	audit.ActionPasswordChange: byThisService,
	audit.ActionAccountDelete:  byThisService,

	// NOT this service's, and that is the whole of research D-14. PocketBase's
	// own auth route stays reachable, so a `login` row written from a MediKube
	// handler would leave every sign-in through that route unaudited while
	// looking exactly like coverage. OnRecordAuthRequest fires for both paths
	// (T205, T221, T221b).
	audit.ActionLogin:        byAHook,
	audit.ActionAdminSession: byAHook,

	audit.ActionDelete:       byAnother,
	audit.ActionAccessDenied: byAnother,

	audit.ActionReadSensitive: byALaterPhase,
	audit.ActionShareGrant:    byALaterPhase,
	audit.ActionShareRevoke:   byALaterPhase,
	audit.ActionShareExpire:   byALaterPhase,
	audit.ActionInviteSend:    byALaterPhase,
	audit.ActionInviteRespond: byALaterPhase,
	audit.ActionExport:        byALaterPhase,
	audit.ActionBackupCreate:  byALaterPhase,
	audit.ActionBackupRestore: byALaterPhase,

	// Declared and written by nothing in 001–006 (data-model §3). Confirming an
	// address writes `update`, however obviously named this constant is.
	audit.ActionEmailChange: byALaterPhase,
}

// exercise is one call of one method and the rows it is expected to leave
// behind. Declaring "none" is a decision like any other: a method that writes
// nothing says so here, so that a method that started writing something is a
// failure rather than a silent change.
type exercise struct {
	name  string
	run   func(t *testing.T) harness
	wrote []audit.Action
}

// The service's whole method set. A method missing from here fails the first
// walk, which is the property that survives the next author.
var coverage = map[string][]exercise{
	"Register": {{
		name: "an open instance",
		run: func(t *testing.T) harness {
			h := newOpenHarness(t)

			_, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
				Email:    "registering@example.test",
				Name:     "Registering",
				Password: identitytest.Password,
			})
			require.NoError(t, err)

			return h
		},
		wrote: []audit.Action{audit.ActionCreate},
	}, {
		name: "a closed instance",
		run: func(t *testing.T) harness {
			h := newHarness(t)

			_, err := h.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
				Email:    "registering@example.test",
				Name:     "Registering",
				Password: identitytest.Password,
			})
			require.Error(t, err)

			return h
		},
	}},

	"SignIn": {{
		name: "a credential that matches",
		run: func(t *testing.T) harness {
			h := newHarness(t)

			_, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), identity.Credentials{
				Email:    identitytest.Email,
				Password: identitytest.Password,
			})
			require.NoError(t, err)

			return h
		},
		// The hook writes `login`, for MediKube's route and PocketBase's alike.
	}, {
		name: "a credential that does not",
		run: func(t *testing.T) harness {
			h := newHarness(t)

			_, err := h.service.SignIn(t.Context(), access.Anonymous(requestID), identity.Credentials{
				Email:    identitytest.Email,
				Password: "not-the-password-on-the-account",
			})
			require.Error(t, err)

			return h
		},
		wrote: []audit.Action{audit.ActionLoginFailed},
	}},

	"SignOut": {{
		name: "a signed-in caller",
		run: func(t *testing.T) harness {
			h := newHarness(t)
			require.NoError(t, h.service.SignOut(t.Context(), h.actor()))

			return h
		},
		wrote: []audit.Action{audit.ActionLogout},
	}},

	"Me": {{
		name: "a signed-in caller",
		run: func(t *testing.T) harness {
			h := newHarness(t)

			_, err := h.service.Me(t.Context(), h.actor())
			require.NoError(t, err)

			return h
		},
	}},

	"UpdateProfile": {{
		name: "a changed display name",
		run: func(t *testing.T) harness {
			h := newHarness(t)
			name := "Renamed"

			_, err := h.service.UpdateProfile(t.Context(), h.actor(), identity.Profile{Name: &name})
			require.NoError(t, err)

			return h
		},
		wrote: []audit.Action{audit.ActionUpdate},
	}},

	"ChangePassword": {{
		name: "the current password supplied",
		run: func(t *testing.T) harness {
			h := newHarness(t)
			require.NoError(t, h.service.ChangePassword(t.Context(), h.actor(), identitytest.Password, replacement))

			return h
		},
		wrote: []audit.Action{audit.ActionPasswordChange},
	}},

	"DeleteAccount": {{
		name: "the password and the phrase",
		run: func(t *testing.T) harness {
			h := newHarness(t)
			require.NoError(t, h.service.DeleteAccount(
				t.Context(), h.actor(), identitytest.Password, domainidentity.DeleteConfirmationPhrase))

			return h
		},
		wrote: []audit.Action{audit.ActionAccountDelete},
	}},

	"RequestPasswordReset": {{
		name: "an address with an account",
		run: func(t *testing.T) harness {
			h := newHarness(t)

			_, err := h.service.RequestPasswordReset(t.Context(), access.Anonymous(requestID), identitytest.Email)
			require.NoError(t, err)

			return h
		},
		// Nothing: there is nothing yet to record about an account that may not
		// exist, and the only thing there is to record is the typed address
		// (contracts/auth.md).
	}},

	"ConfirmPasswordReset": {{
		name: "a link that resolves",
		run: func(t *testing.T) harness {
			h := newHarness(t)

			token, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
			require.NoError(t, err)

			require.NoError(t, h.service.ConfirmPasswordReset(
				t.Context(), access.Anonymous(requestID), token, replacement))

			return h
		},
		wrote: []audit.Action{audit.ActionPasswordChange},
	}},

	"RequestVerification": {{
		name: "an unconfirmed address",
		run: func(t *testing.T) harness {
			h := newHarness(t)
			require.NoError(t, h.service.RequestVerification(t.Context(), h.actor()))

			return h
		},
	}},

	"ConfirmVerification": {{
		name: "a link that resolves",
		run: func(t *testing.T) harness {
			h := newHarness(t)

			token, err := h.authenticator.Token(identity.TokenEmailConfirmation, h.account.ID)
			require.NoError(t, err)

			require.NoError(t, h.service.ConfirmVerification(t.Context(), access.Anonymous(requestID), token))

			return h
		},
		wrote: []audit.Action{audit.ActionUpdate},
	}},

	"RegistrationOpen": {{
		name: "read",
		run: func(t *testing.T) harness {
			h := newHarness(t)
			h.service.RegistrationOpen()

			return h
		},
	}},
}

// TestEveryServiceMethodSaysWhatItWritesToTheTrail is the first walk.
func TestEveryServiceMethodSaysWhatItWritesToTheTrail(t *testing.T) {
	t.Parallel()

	serviceType := reflect.TypeOf(&identity.Service{})
	require.GreaterOrEqual(t, serviceType.NumMethod(), 12,
		"the walk found almost no methods; it is not looking at the service it thinks it is")

	for i := range serviceType.NumMethod() {
		method := serviceType.Method(i)

		t.Run(method.Name, func(t *testing.T) {
			t.Parallel()

			exercises, classified := coverage[method.Name]
			require.Truef(t, classified,
				"%s is exercised by nothing here: add it, and say which audit rows it writes", method.Name)
			require.NotEmptyf(t, exercises, "%s is registered with no exercise at all", method.Name)

			for _, run := range exercises {
				t.Run(run.name, func(t *testing.T) {
					t.Parallel()

					h := run.run(t)

					assert.Equalf(t, run.wrote, nilIfEmpty(h.auditor.Actions()),
						"%s with %s wrote a different set of rows than it declares", method.Name, run.name)
				})
			}
		})
	}
}

func nilIfEmpty(actions []audit.Action) []audit.Action {
	if len(actions) == 0 {
		return nil
	}

	return actions
}

// TestEveryDeclaredActionSaysWhoWritesIt is the second walk, and it is driven
// from audit.Actions() rather than from a list here: a twenty-first value added
// to the vocabulary with nobody writing it is exactly the kind of orphan that
// is invisible to lint and to every other test.
func TestEveryDeclaredActionSaysWhoWritesIt(t *testing.T) {
	t.Parallel()

	declared := audit.Actions()
	require.Len(t, declared, 20, "the declared vocabulary has moved; data-model §3 fixes it at twenty")

	for _, action := range declared {
		writer, classified := writtenBy[action]

		assert.Truef(t, classified,
			"the vocabulary declares %q and nothing here says who writes it", action)
		assert.NotEmptyf(t, writer, "%q is classified as being written by nobody at all", action)
	}
}

// TestThisServiceWritesExactlyTheActionsItClaims is the two walks joined, and
// it is where the `login` row is pinned OUT of this package.
//
// Every action this file says the service writes must actually be written by
// some exercise above, and every action written by an exercise must be one this
// file says the service writes. A `login` row added to the sign-in handler
// therefore fails here — which is the failure research D-14 exists to cause,
// because papering over it silently unaudits PocketBase's own auth route.
func TestThisServiceWritesExactlyTheActionsItClaims(t *testing.T) {
	t.Parallel()

	var claimed []audit.Action

	for _, action := range audit.Actions() {
		if writtenBy[action] == byThisService {
			claimed = append(claimed, action)
		}
	}

	require.Len(t, claimed, 6, "T207's six identity actions have become %d", len(claimed))

	var observed []audit.Action

	for method, exercises := range coverage {
		for _, run := range exercises {
			h := run.run(t)

			for _, action := range h.auditor.Actions() {
				assert.Equalf(t, byThisService, writtenBy[action],
					"%s with %s wrote %q, which this file says is written by %s",
					method, run.name, action, writtenBy[action])

				if !slices.Contains(observed, action) {
					observed = append(observed, action)
				}
			}
		}
	}

	for _, action := range claimed {
		assert.Containsf(t, observed, action,
			"%q is claimed by this service and no exercise here produces it, so nothing asserts it is ever written", action)
	}

	assert.NotContains(t, observed, audit.ActionLogin,
		"the service wrote a `login` row; it is the hook's, so PocketBase's own auth route would now be unaudited (research D-14)")
}
