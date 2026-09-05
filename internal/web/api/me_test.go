package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	domainidentity "medikube/internal/domain/identity"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	"medikube/internal/testsupport"
	"medikube/internal/web"
)

// T193. contracts/account.md's four operations. The account surface is the one
// place a person changes something about themselves, so every assertion here is
// paired with a read of the STORED record: a response that reports a change it
// did not make is the failure this file exists to catch.

//nolint:gosec // test credentials, not credentials
const (
	changedPassword = "a-different-perfectly-fine-password"
	wrongPassword   = "not-this-accounts-password"
)

func TestTheAccountBehindTheSessionIsReadBackWhole(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).get(meURL)
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	me := answer.me(t)
	assert.Equal(t, testsupport.AccountAID, me.ID)
	assert.Equal(t, testsupport.AccountAEmail, me.Email)
	assert.Equal(t, testsupport.AccountAName, me.Name)
	assert.Equal(t, string(domainidentity.RoleUser), me.Role)
	assert.Equal(t, string(domainidentity.DefaultUnitSystem), me.UnitSystem)
	assert.Equal(t, domainidentity.DefaultLocale, me.Locale)
	assert.Equal(t, string(domainidentity.DefaultDateFormat), me.DateFormat)
	assert.Equal(t, string(domainidentity.DefaultTheme), me.Theme)
	assert.NotEmpty(t, me.CreatedAt)

	// The body is somebody's own profile: it must not be written down anywhere,
	// and there is no concurrency question for a validator to answer.
	assert.Equal(t, "private, no-store", answer.Header.Get("Cache-Control"))
	assert.Empty(t, answer.Header.Get("ETag"), "an account carries no version to write back against")
}

// T093/contracts/active-patient.md's amendment to GET /api/v1/me: the person
// in view (seeded at the account's own self-record, testsupport/seed's
// applyActivePatients) and how many the account owns.
func TestTheAccountReadsBackTheActivePatientAndOwnedCount(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).get(meURL)
	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	me := answer.me(t)
	require.NotNil(t, me.ActivePatient, "the seeded account has a self-record but none is reported active")
	assert.Equal(t, testsupport.AccountAPatientSelfID, me.ActivePatient.ID)
	assert.Equal(t, 3, me.Patients.OwnedCount, "account A owns three seeded patients")
}

// T093: active_patient is read-only from this side. MePatch has no member
// for it, so a caller naming it is refused by shape before any handler runs
// — the same enforcement FR-012 already relies on for role and
// email_confirmed.
func TestPatchingTheAccountRejectsActivePatientAsAnUnknownField(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).
		do(http.MethodPatch, meURL, body("active_patient", quoted(testsupport.AccountAPatientChildID)), nil)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	envelope := answer.envelope(t)
	require.Len(t, envelope.Error.Fields, 1)
	assert.Equal(t, "active_patient", envelope.Error.Fields[0].Field)
	assert.Equal(t, "unknown_field", envelope.Error.Fields[0].Code)

	record := instance.stored(t, testsupport.AccountAEmail)
	assert.Equal(t, testsupport.AccountAPatientSelfID, store.UserActivePatientID(record),
		"a refused patch changed the pointer anyway")
}

func TestReadingTheAccountWithoutASessionIsRefused(t *testing.T) {
	t.Parallel()

	answer := newRig(t).anonymous().get(meURL)

	require.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
	assert.Equal(t, web.CodeUnauthenticated, answer.envelope(t).Error.Code)
	assert.NotContains(t, answer.Body, testsupport.AccountAEmail)
}

// FR-011: the display name and the four preferences, changed and read back.
func TestTheFiveThingsAPersonMayChangeAboutThemselves(t *testing.T) {
	t.Parallel()

	instance := newRig(t)
	signedIn := instance.as(testsupport.AccountAEmail)

	answer := signedIn.do(http.MethodPatch, meURL, body(
		"name", quoted("Amara O."),
		"unit_system", quoted(string(domainidentity.UnitSystemImperial)),
		"locale", quoted("en-GB"),
		"date_format", quoted(string(domainidentity.DateFormatDMY)),
		"theme", quoted(string(domainidentity.ThemeDark)),
	), nil)

	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	me := answer.me(t)
	assert.Equal(t, "Amara O.", me.Name)
	assert.Equal(t, string(domainidentity.UnitSystemImperial), me.UnitSystem)
	assert.Equal(t, "en-GB", me.Locale)
	assert.Equal(t, string(domainidentity.DateFormatDMY), me.DateFormat)
	assert.Equal(t, string(domainidentity.ThemeDark), me.Theme)

	// The response is a claim; the database is the fact.
	stored, err := store.UserFromRecord(instance.stored(t, testsupport.AccountAEmail))
	require.NoError(t, err)

	assert.Equal(t, "Amara O.", stored.Name)
	assert.Equal(t, domainidentity.UnitSystemImperial, stored.UnitSystem)
	assert.Equal(t, "en-GB", stored.Locale)
	assert.Equal(t, domainidentity.DateFormatDMY, stored.DateFormat)
	assert.Equal(t, domainidentity.ThemeDark, stored.Theme)
	assert.Equal(t, testsupport.AccountAEmail, stored.Email, "the address moved on a profile change")

	// A second session sees the new theme, which is what makes a preference a
	// property of the account rather than of the browser (FR-045).
	assert.Equal(t, string(domainidentity.ThemeDark),
		instance.as(testsupport.AccountAEmail).get(meURL).me(t).Theme)
}

// A patch mentioning one member leaves the other four where they were.
func TestAPatchChangesOnlyWhatItMentions(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).
		do(http.MethodPatch, meURL, body("theme", quoted(string(domainidentity.ThemeLight))), nil)

	require.Equal(t, http.StatusOK, answer.Status, answer.Body)

	me := answer.me(t)
	assert.Equal(t, string(domainidentity.ThemeLight), me.Theme)
	assert.Equal(t, testsupport.AccountAName, me.Name)
	assert.Equal(t, domainidentity.DefaultLocale, me.Locale)
}

// FR-027 again, on the account: every offending value at once.
func TestEveryInvalidPreferenceIsReportedAtOnce(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).do(http.MethodPatch, meURL, body(
		"name", quoted(""),
		"unit_system", quoted("furlongs"),
		"locale", quoted("english"),
		"theme", quoted("neon"),
	), nil)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)

	fields := make([]string, 0, 4)
	for _, reported := range answer.envelope(t).Error.Fields {
		fields = append(fields, reported.Field)
	}

	assert.ElementsMatch(t, []string{"name", "unit_system", "locale", "theme"}, fields)

	stored, err := store.UserFromRecord(instance.stored(t, testsupport.AccountAEmail))
	require.NoError(t, err)
	assert.Equal(t, testsupport.AccountAName, stored.Name, "a refused patch changed the record anyway")
}

// contracts/account.md's theme case, by its code rather than by its prose.
func TestAThemeOutsideTheVocabularyIsRefusedByItsCode(t *testing.T) {
	t.Parallel()

	answer := newRig(t).as(testsupport.AccountAEmail).
		do(http.MethodPatch, meURL, body("theme", quoted("neon")), nil)

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Equal(t, [][2]string{{"theme", domain.CodeInvalidValue}}, answer.envelope(t).fieldCodes())
}

// FR-009 and FR-010 together, which is the mandatory test contracts/account.md
// names: the session that made the change survives, every other one does not,
// the old password stops working and the new one starts.
func TestChangingThePasswordKeepsThisSessionAndEndsTheOthers(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	changing := instance.as(testsupport.AccountAEmail)
	elsewhere := instance.as(testsupport.AccountAEmail)

	require.Equal(t, http.StatusOK, elsewhere.get(meURL).Status)

	answer := changing.put(passwordURL, body(
		"current_password", quoted(testsupport.Password),
		"new_password", quoted(changedPassword),
	))

	require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)
	assert.Empty(t, answer.Body)

	fresh := answer.sessionCookie(t)
	require.NotNil(t, fresh, "the person who changed their password was signed out where they changed it")
	require.NotEmpty(t, fresh.Value)

	// The cookie the change handed back still works…
	assert.Equal(t, http.StatusOK, instance.anonymous().cookieGet(meURL, fresh.Value).Status)

	// …and every other session does not.
	assert.Equal(t, http.StatusUnauthorized, elsewhere.get(meURL).Status,
		"a session issued before the change survived it")

	// The old credential is gone and the new one works.
	old := instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail), "password", quoted(testsupport.Password)))
	assert.Equal(t, http.StatusUnauthorized, old.Status)

	replacement := instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail), "password", quoted(changedPassword)))
	assert.Equal(t, http.StatusOK, replacement.Status, replacement.Body)
}

func TestChangingThePasswordWithoutASessionIsRefused(t *testing.T) {
	t.Parallel()

	answer := newRig(t).anonymous().put(passwordURL, body(
		"current_password", quoted(testsupport.Password),
		"new_password", quoted(changedPassword),
	))

	require.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
	assert.Equal(t, web.CodeUnauthenticated, answer.envelope(t).Error.Code)
}

// FR-013. The phrase is compared exactly: not trimmed, not folded. An
// irreversible act asks for a deliberate one in return.
func TestDeletingAnAccountRequiresTheExactTypedPhrase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		typed   string
		wanted  string
		wantErr string
	}{
		{name: "the phrase itself", typed: domainidentity.DeleteConfirmationPhrase},
		{name: "in lower case", typed: "delete my account", wantErr: "confirmation"},
		{name: "with a trailing space", typed: domainidentity.DeleteConfirmationPhrase + " ", wantErr: "confirmation"},
		{name: "something else entirely", typed: "yes", wantErr: "confirmation"},
		{name: "nothing at all", typed: "", wantErr: "confirmation"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			instance := newRig(t)

			answer := instance.as(testsupport.AccountAEmail).deleteWithBody(meURL, body(
				"password", quoted(testsupport.Password),
				"confirmation", quoted(test.typed),
			))

			if test.wantErr == "" {
				require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)
				assert.False(t, instance.accountExists(t, testsupport.AccountAEmail))

				return
			}

			require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
			assert.Equal(t, [][2]string{{test.wantErr, "mismatch"}}, answer.envelope(t).fieldCodes())
			assert.True(t, instance.accountExists(t, testsupport.AccountAEmail),
				"a refused deletion deleted the account anyway")
		})
	}
}

func TestDeletingAnAccountRequiresThePasswordAgain(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).deleteWithBody(meURL, body(
		"password", quoted(wrongPassword),
		"confirmation", quoted(domainidentity.DeleteConfirmationPhrase),
	))

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Equal(t, [][2]string{{"password", "incorrect"}}, answer.envelope(t).fieldCodes())
	assert.True(t, instance.accountExists(t, testsupport.AccountAEmail))
}

// FR-014 and SC-012 at the edge: the account is gone, its credentials no longer
// sign in, and somebody else's data is untouched. The stored-data half of this
// — the medication cascade and the surviving audit row — is
// me_delete_integration_test.go's.
func TestADeletedAccountIsGoneAndNobodyElseIs(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.as(testsupport.AccountAEmail).deleteWithBody(meURL, body(
		"password", quoted(testsupport.Password),
		"confirmation", quoted(domainidentity.DeleteConfirmationPhrase),
	))

	require.Equal(t, http.StatusNoContent, answer.Status, answer.Body)

	cleared := answer.sessionCookie(t)
	require.NotNil(t, cleared, "the browser was left holding a cookie for an account that no longer exists")
	assert.Empty(t, cleared.Value)

	refused := instance.anonymous().post(loginURL, body(
		"email", quoted(testsupport.AccountAEmail), "password", quoted(testsupport.Password)))
	assert.Equal(t, http.StatusUnauthorized, refused.Status)

	assert.True(t, instance.accountExists(t, testsupport.AccountBEmail), "somebody else's account went too")
	assert.Equal(t, testsupport.AccountBMedicationCount,
		instance.as(testsupport.AccountBEmail).get(meURL).me(t).Counts[kind.Medication.Segment()])
}

func TestDeletingAnAccountWithoutASessionIsRefused(t *testing.T) {
	t.Parallel()

	instance := newRig(t)

	answer := instance.anonymous().deleteWithBody(meURL, body(
		"password", quoted(testsupport.Password),
		"confirmation", quoted(domainidentity.DeleteConfirmationPhrase),
	))

	require.Equal(t, http.StatusUnauthorized, answer.Status, answer.Body)
	assert.True(t, instance.accountExists(t, testsupport.AccountAEmail))
}

// Every answer that carries the account, or changes the session that reaches
// it, is uncacheable.
//
// One assertion per operation rather than one for `getMe`, because the header
// is set at four call sites and a shared proxy caching ANY of them is the same
// disclosure: the next person through that proxy is handed somebody else's
// profile, or somebody else's Set-Cookie. The two 204s are in the table for the
// second reason — they carry a credential in a header, and a cached 204 with a
// Set-Cookie on it is worse than a cached body.
func TestEveryAnswerThatCarriesOrChangesTheAccountIsUncacheable(t *testing.T) {
	t.Parallel()

	answers := []struct {
		name     string
		expected int
		call     func(t *testing.T) response
	}{
		{
			name:     "reading the account",
			expected: http.StatusOK,
			call:     func(t *testing.T) response { return newRig(t).as(testsupport.AccountAEmail).get(meURL) },
		},
		{
			name:     "changing the account",
			expected: http.StatusOK,
			call: func(t *testing.T) response {
				return newRig(t).as(testsupport.AccountAEmail).patch(meURL, body("name", quoted("Renamed")), "")
			},
		},
		{
			name:     "signing in",
			expected: http.StatusOK,
			call: func(t *testing.T) response {
				return newRig(t).anonymous().post(loginURL, body(
					"email", quoted(testsupport.AccountAEmail), "password", quoted(testsupport.Password)))
			},
		},
		{
			name:     "registering",
			expected: http.StatusCreated,
			call: func(t *testing.T) response {
				return openRig(t).anonymous().post(registerURL, body(
					"email", quoted("cache@example.test"),
					"name", quoted("Cache Probe"),
					"password", quoted(testsupport.Password)))
			},
		},
		{
			name:     "renewing the session",
			expected: http.StatusOK,
			call:     func(t *testing.T) response { return newRig(t).as(testsupport.AccountAEmail).post(refreshURL, "") },
		},
		{
			name:     "changing the password",
			expected: http.StatusNoContent,
			call: func(t *testing.T) response {
				return newRig(t).as(testsupport.AccountAEmail).put(passwordURL, body(
					"current_password", quoted(testsupport.Password),
					"new_password", quoted("a-brand-new-password-entirely")))
			},
		},
		{
			name:     "signing out",
			expected: http.StatusNoContent,
			call:     func(t *testing.T) response { return newRig(t).as(testsupport.AccountAEmail).post(logoutURL, "") },
		},
		{
			name:     "deleting the account",
			expected: http.StatusNoContent,
			call: func(t *testing.T) response {
				return newRig(t).as(testsupport.AccountAEmail).deleteWithBody(meURL, body(
					"password", quoted(testsupport.Password),
					"confirmation", quoted(domainidentity.DeleteConfirmationPhrase)))
			},
		},
	}

	for _, answer := range answers {
		t.Run(answer.name, func(t *testing.T) {
			t.Parallel()

			got := answer.call(t)

			require.Equal(t, answer.expected, got.Status, got.Body)
			assert.Equal(t, "private, no-store", got.Header.Get("Cache-Control"),
				"%s is cacheable, so a shared cache may hand it to the next person through it", answer.name)
		})
	}
}
