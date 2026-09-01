package identity_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/service/identity"
	"medikube/internal/service/identity/identitytest"
)

const replacement = "a-second-perfectly-ordinary-passphrase"

// TestEveryRecoveryRequestIsAnsweredWithOneValue is FR-073. Six branches — an
// address with an account, one without, one that is not an address at all, an
// empty one, a store that would not answer and a mailer that would not send —
// and one answer.
//
// The mailer failure is in the table deliberately: it is the branch that only
// exists for an address that HAS an account, so an implementation that let it
// change the answer would have built the oracle out of a status code instead of
// a message.
func TestEveryRecoveryRequestIsAnsweredWithOneValue(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		email    string
		sabotage func(harness)
	}{
		"an address with an account":    {email: identitytest.Email},
		"an address with no account":    {email: identitytest.StrangerEmail},
		"not an address at all":         {email: "not-an-address"},
		"an empty address":              {email: ""},
		"a mailer that will not send":   {email: identitytest.Email, sabotage: func(h harness) { h.mailer.Fail(io.ErrClosedPipe) }},
		"a store that will not look up": {email: identitytest.Email, sabotage: func(h harness) { h.repository.Fail(io.ErrUnexpectedEOF) }},
	}

	answers := make(map[string]domainidentity.Acknowledgement, len(cases))

	for name, attempt := range cases {
		h := newHarness(t)

		if attempt.sabotage != nil {
			attempt.sabotage(h)
		}

		acknowledgement, _ := h.service.RequestPasswordReset(t.Context(), access.Anonymous(requestID), attempt.email)

		answers[name] = acknowledgement

		assert.Emptyf(t, h.auditor.Events(),
			"%s wrote an audit row; a request records nothing, because the only thing there is to record is the typed address", name)
	}

	for name, answer := range answers {
		assert.Equalf(t, domainidentity.AcknowledgeRecovery(), answer,
			"%s is answered differently, so the form says who has an account here", name)
	}
}

// TestARecoveryRequestSendsToAnAccountAndNeverToAnAddress is FR-077: the mailer
// is handed an account id and nothing else, so there is no path by which a
// typed address reaches a message, a log line or a trail.
func TestARecoveryRequestSendsToAnAccountAndNeverToAnAddress(t *testing.T) {
	t.Parallel()

	t.Run("an address with an account", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.service.RequestPasswordReset(t.Context(), access.Anonymous(requestID), identitytest.Email)
		require.NoError(t, err)

		assert.Equal(t, []string{h.account.ID}, h.mailer.Resets())
	})

	t.Run("an address with no account", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		_, err := h.service.RequestPasswordReset(t.Context(), access.Anonymous(requestID), identitytest.StrangerEmail)
		require.NoError(t, err)

		assert.Empty(t, h.mailer.Resets(), "a message was sent for an address with no account")
	})
}

// TestTheRecoveryRequestHasOneExitAndOneAnswer is the structural half of T223i
// at the layer that decides, asserted out of the source rather than with a
// clock (Constitution VIII, ANALYSIS N13).
//
// One return statement means the branch cannot answer; one call to the
// acknowledgement's constructor means the two branches cannot each build their
// own. Both are properties a later edit has to break visibly — adding an early
// return on the no-account branch is exactly the edit this fails on, and it is
// the edit somebody makes while "tidying up".
func TestTheRecoveryRequestHasOneExitAndOneAnswer(t *testing.T) {
	t.Parallel()

	body := methodBody(t, "recovery.go", "RequestPasswordReset")

	var (
		exits   int
		answers int
	)

	ast.Inspect(body, func(node ast.Node) bool {
		switch found := node.(type) {
		case *ast.ReturnStmt:
			exits++
		case *ast.CallExpr:
			if selector, ok := found.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "AcknowledgeRecovery" {
				answers++
			}
		}

		return true
	})

	assert.Equal(t, 1, exits,
		"RequestPasswordReset has %d return statements: one of them is reached only when the address has no account, and that is the oracle FR-073 closes", exits)
	assert.Equal(t, 1, answers,
		"RequestPasswordReset builds its answer in %d places, so the branches can drift apart", answers)
}

// methodBody finds one method of the service in one of this package's own
// source files. It reads the source rather than the compiled form because what
// is being asserted is the shape of the code, which is what the next author
// edits.
func methodBody(t *testing.T, file, method string) *ast.BlockStmt {
	t.Helper()

	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)

	for _, declaration := range parsed.Decls {
		function, isFunction := declaration.(*ast.FuncDecl)
		if !isFunction || function.Name.Name != method || function.Recv == nil {
			continue
		}

		require.NotNil(t, function.Body)

		return function.Body
	}

	t.Fatalf("%s declares no method %s; the assertion below would pass having read nothing", file, method)

	return nil
}

func TestConfirmPasswordResetSetsTheNewPasswordAndRecordsIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	token, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)

	require.NoError(t, h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), token, replacement))

	require.NoError(t, h.authenticator.Verify(t.Context(), h.account.ID, replacement))
	assert.ErrorIs(t, h.authenticator.Verify(t.Context(), h.account.ID, identitytest.Password),
		domain.ErrUnauthenticated, "the password that was replaced still works")

	event := h.only(t, audit.ActionPasswordChange)
	assert.Equal(t, h.account.ID, event.TargetID)
	assert.Equal(t, h.account.ID, event.ActorID,
		"the row names nobody, so the trail cannot say whose password was replaced")
}

// TestARecoveredPasswordIsHeldToTheRulesRegistrationPublishes is FR-004 and
// FR-074 met at the service, and it compares the two refusals rather than
// asserting each: a recovery path with its own weaker check would produce a
// different one, and nothing else in the suite would notice.
func TestARecoveredPasswordIsHeldToTheRulesRegistrationPublishes(t *testing.T) {
	t.Parallel()

	for _, chosen := range []string{"short", "", identitytest.Email, identitytest.Name} {
		t.Run(chosen, func(t *testing.T) {
			t.Parallel()

			open := newOpenHarness(t)

			_, registration := open.service.Register(t.Context(), access.Anonymous(requestID), identity.Registration{
				Email:    identitytest.Email,
				Name:     identitytest.Name,
				Password: chosen,
			})

			h := newHarness(t)

			token, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
			require.NoError(t, err)

			recovery := h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), token, chosen)

			var (
				atRegistration *domain.ValidationError
				atRecovery     *domain.ValidationError
			)

			require.ErrorAs(t, registration, &atRegistration)
			require.ErrorAs(t, recovery, &atRecovery)

			assert.Equal(t, string(mustMarshal(t, atRegistration)), string(mustMarshal(t, atRecovery)),
				"a password set through a recovery link is held to different rules than one chosen at registration")
		})
	}
}

// TestARefusedPasswordLeavesTheLinkUsable. The token is spent by the write and
// not by the attempt, so one typo does not cost another round trip through a
// mailbox (FR-074's "offer to request another" is for a link that expired, not
// for a password that was too short).
func TestARefusedPasswordLeavesTheLinkUsable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	token, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)

	require.Error(t, h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), token, "short"))
	assert.Empty(t, h.auditor.Events())

	require.NoError(t, h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), token, replacement))
}

// TestARecoveredPasswordEndsEverySessionIssuedBeforeIt is FR-074's last
// sentence, asserted the same way T200 asserts FR-010: through a credential
// minted before the change, because a link and a session token are signed with
// the same record key (research D-16).
func TestARecoveredPasswordEndsEverySessionIssuedBeforeIt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	token, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)

	other, err := h.authenticator.Token(identity.TokenEmailConfirmation, h.account.ID)
	require.NoError(t, err)

	require.NoError(t, h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), token, replacement))

	_, err = h.authenticator.Redeem(t.Context(), identity.TokenEmailConfirmation, other)
	assert.ErrorIs(t, err, identity.ErrInvalidToken,
		"a credential issued before the recovery still resolves, so the sessions it stands for are still open")
}

// TestEveryUnusableRecoveryLinkIsOneRefusal is FR-074: expired, already used
// and altered are one `invalid_token` with one message, because distinguishing
// them tells an attacker which tokens once existed.
func TestEveryUnusableRecoveryLinkIsOneRefusal(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	spent, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)
	require.NoError(t, h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), spent, replacement))
	h.auditor.Forget()

	confirmation, err := h.authenticator.Token(identity.TokenEmailConfirmation, h.account.ID)
	require.NoError(t, err)

	links := map[string]string{
		"already used":            spent,
		"minted for another flow": confirmation,
		"never minted at all":     "not-a-token-this-instance-issued",
		"empty":                   "",
		"shaped like a real one":  string(identity.TokenPasswordReset) + ":" + h.account.ID + ":99",
	}

	messages := make(map[string]string, len(links))

	for name, link := range links {
		err := h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), link, replacement)

		require.ErrorIsf(t, err, identity.ErrInvalidToken, "a link %s was not refused as one refusal", name)

		messages[name] = err.Error()
	}

	for name, message := range messages {
		assert.Equalf(t, messages["never minted at all"], message,
			"a link %s reads differently, so which links once existed can be read out of the difference", name)
	}

	assert.Empty(t, h.auditor.Events(), "a refused link recorded a password change that did not happen")
}

// TestALinkThatCouldNotBeCheckedIsNotAnInvalidLink. Folding an outage into
// `invalid_token` would tell somebody their link had expired on the strength of
// a failed read, and would hide the outage behind a 400 nobody investigates.
func TestALinkThatCouldNotBeCheckedIsNotAnInvalidLink(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.authenticator.Fail(io.ErrUnexpectedEOF)

	err := h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), "any-link", replacement)

	require.Error(t, err)
	assert.NotErrorIs(t, err, identity.ErrInvalidToken)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

// TestRequestVerificationTakesNoAddress. Accepting one would let any signed-in
// caller aim MediKube's mailer at a stranger, and the only defence that cannot
// be forgotten is not having a parameter to put it in (contracts/auth.md).
func TestRequestVerificationTakesNoAddress(t *testing.T) {
	t.Parallel()

	method, found := reflect.TypeOf(&identity.Service{}).MethodByName("RequestVerification")
	require.True(t, found)

	assert.Equal(t, 3, method.Type.NumIn(),
		"RequestVerification takes an argument beyond the context and the actor, which is somewhere an address could go")
}

func TestRequestVerificationSendsToTheSignedInAccount(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	require.NoError(t, h.service.RequestVerification(t.Context(), h.actor()))

	assert.Equal(t, []string{h.account.ID}, h.mailer.Verifications())
	assert.Empty(t, h.auditor.Events(), "asking for a message again is not an event in the trail")
}

// TestRequestVerificationOnAConfirmedAddressSendsNothing. contracts/auth.md
// answers it exactly as an unconfirmed one: there is nothing to disclose — the
// caller owns the account — and nothing to fail at.
func TestRequestVerificationOnAConfirmedAddressSendsNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	confirmed := h.account
	confirmed.EmailConfirmed = true
	_, err := h.repository.Update(t.Context(), confirmed)
	require.NoError(t, err)

	require.NoError(t, h.service.RequestVerification(t.Context(), h.actor()))

	assert.Empty(t, h.mailer.Verifications())
}

// TestConfirmVerificationConfirmsTheAddressAndWritesAnUpdate.
//
// `update` and NOT `email_change`, however obviously named that constant is:
// data-model §3 says no phase in 001–006 writes email_change, and reaching for
// it would break the vocabulary counts every later phase asserts.
func TestConfirmVerificationConfirmsTheAddressAndWritesAnUpdate(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	token, err := h.authenticator.Token(identity.TokenEmailConfirmation, h.account.ID)
	require.NoError(t, err)

	require.NoError(t, h.service.ConfirmVerification(t.Context(), access.Anonymous(requestID), token))

	stored, exists := h.repository.Stored(h.account.ID)
	require.True(t, exists)
	assert.True(t, stored.EmailConfirmed)

	event := h.only(t, audit.ActionUpdate)
	assert.NotEqual(t, audit.ActionEmailChange, event.Action)
	assert.Equal(t, h.account.ID, event.TargetID)
	assert.Equal(t, h.account.ID, event.ActorID)
}

// TestASecondUseOfAConfirmationLinkIsAnsweredExactlyAsAnAlteredOne is T223d,
// and it is MediKube's own behaviour rather than PocketBase's: SetVerified
// rotates no token key, so a spent confirmation link stays resolvable for its
// full 24 hours (measured against v0.40.1 — two consecutive confirmations both
// answered 204). The already-confirmed account IS the used token.
func TestASecondUseOfAConfirmationLinkIsAnsweredExactlyAsAnAlteredOne(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	token, err := h.authenticator.Token(identity.TokenEmailConfirmation, h.account.ID)
	require.NoError(t, err)

	require.NoError(t, h.service.ConfirmVerification(t.Context(), access.Anonymous(requestID), token))
	h.auditor.Forget()

	second := h.service.ConfirmVerification(t.Context(), access.Anonymous(requestID), token)
	altered := h.service.ConfirmVerification(t.Context(), access.Anonymous(requestID), "not-a-token-this-instance-issued")

	require.ErrorIs(t, second, identity.ErrInvalidToken)
	require.ErrorIs(t, altered, identity.ErrInvalidToken)
	assert.Equal(t, altered.Error(), second.Error(),
		"a link used twice reads differently from one that was never valid")

	assert.Empty(t, h.auditor.Events(), "a refused link recorded a change that did not happen")
}

// TestNoRecoveryPathIntroducesANewActionValue is T221's rule read from the
// other end: a password replaced through a link writes the same
// `password_change` row a deliberate change writes, and a confirmed address
// writes `update`. The vocabulary counts data-model §3 fixes are unchanged, so
// the migrations later phases assert against do not move.
func TestNoRecoveryPathIntroducesANewActionValue(t *testing.T) {
	t.Parallel()

	permitted := []audit.Action{audit.ActionPasswordChange, audit.ActionUpdate}

	h := newHarness(t)

	reset, err := h.authenticator.Token(identity.TokenPasswordReset, h.account.ID)
	require.NoError(t, err)

	_, err = h.service.RequestPasswordReset(t.Context(), access.Anonymous(requestID), identitytest.Email)
	require.NoError(t, err)
	require.NoError(t, h.service.ConfirmPasswordReset(t.Context(), access.Anonymous(requestID), reset, replacement))
	require.NoError(t, h.service.RequestVerification(t.Context(), h.actor()))

	confirmation, err := h.authenticator.Token(identity.TokenEmailConfirmation, h.account.ID)
	require.NoError(t, err)
	require.NoError(t, h.service.ConfirmVerification(t.Context(), access.Anonymous(requestID), confirmation))

	written := h.auditor.Actions()
	require.NotEmpty(t, written, "the recovery flows wrote nothing at all, so this passed having read nothing")

	for _, action := range written {
		assert.Truef(t, slices.Contains(permitted, action),
			"the recovery flows wrote %q, which is outside the two values data-model §3 allots them", action)
	}
}
