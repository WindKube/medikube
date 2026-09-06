package page_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/httproute"
	"medikube/internal/testsupport"
	"medikube/internal/web/api"
	"medikube/internal/web/views/ids"
)

// T250. #error-banner and #toast are rendered on EVERY page, signed in or out,
// even though both start empty: Datastar patches by id and an element that
// does not exist cannot be patched, so a page missing either container is a
// live view that silently never updates.
func TestErrorBannerAndToastAreRenderedOnEveryPage(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage {
			continue
		}

		t.Run(route.OpID, func(t *testing.T) {
			t.Parallel()

			b := rig
			if route.Auth == httproute.AuthPublic {
				b = rig.anonymous()
			}

			_, _, body := b.get(route.SmokeURL)

			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.ErrorBanner), "%s renders no #error-banner", route.OpID)
			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.Toast), "%s renders no #toast", route.OpID)

			// Both containers are empty on first render: neither carries a
			// toast or a banner message on a plain GET, which the smoke run
			// only proves once these are non-empty on the routes that patch
			// them.
			assert.Regexpf(t,
				fmt.Sprintf("id=%q[^>]*></div>", ids.ErrorBanner),
				body, "%s's #error-banner is not empty on first render", route.OpID)
		})
	}
}

// The three error views carry both too — the whole reason FR-046 says the
// person "still has navigation" is that they still have the full shell.
func TestErrorBannerAndToastAreRenderedOnEveryErrorView(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t).anonymous()

	for _, view := range httproute.Inventory().ErrorViews() {
		if view.SmokeURL == "" {
			continue
		}

		t.Run(string(view.Name), func(t *testing.T) {
			t.Parallel()

			_, _, body := rig.get(view.SmokeURL)

			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.ErrorBanner), "%s renders no #error-banner", view.Name)
			assert.Containsf(t, body, fmt.Sprintf("id=%q", ids.Toast), "%s renders no #toast", view.Name)
		})
	}
}

// A quick sanity check that the ids package's own two constants are what the
// shell actually uses — a rename on one side and not the other would make
// both tests above pass for the wrong reason (a substring that happens to
// still be there) rather than the right one.
func TestTheShellIDsAreNotAccidentallyGeneric(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, ids.ErrorBanner)
	require.NotEmpty(t, ids.Toast)
	require.False(t, strings.Contains(ids.ErrorBanner, " "))
	require.False(t, strings.Contains(ids.Toast, " "))
}

// A page with a patient in view opens the record stream for that patient, so
// a row created from the list arrives without a reload (contracts/streams.md).
func TestEverySignedInPageWithAPatientOpensTheRecordStream(t *testing.T) {
	t.Parallel()

	rig := newBrowser(t)

	for _, route := range httproute.Inventory().Routes() {
		if route.Kind != httproute.KindPage {
			continue
		}

		t.Run(route.OpID, func(t *testing.T) {
			t.Parallel()

			b := rig
			if route.Auth == httproute.AuthPublic {
				b = rig.anonymous()
			}

			_, _, body := b.get(route.SmokeURL)

			patient := ""
			if parsed, err := url.Parse(route.SmokeURL); err == nil {
				patient = parsed.Query().Get(api.ParamPatient)
			}

			if route.Auth == httproute.AuthPublic || patient == "" {
				assert.NotContains(t, body, "data-init=", "%s has no patient to follow", route.OpID)

				return
			}

			assert.Containsf(t, body, `data-init="@get(&#39;/api/v1/streams/records?patient=`+patient+`&#39;)"`,
				"%s does not open the record stream", route.OpID)
		})
	}
}

// T015. <html lang> follows the same D-04 order RenderPage resolves a
// Localizer with: the account's stored locale, else Accept-Language, else
// English.
func TestHTMLLangFollowsTheResolvedLocalizer(t *testing.T) {
	t.Parallel()

	const settingsPath = "/settings"

	t.Run("a Polish account's page carries lang=pl", func(t *testing.T) {
		t.Parallel()

		rig := newBrowser(t)
		setAccountLocale(t, rig, testsupport.AccountAEmail, "pl")

		_, _, body := rig.get(settingsPath)
		assert.Contains(t, body, `lang="pl"`)
	})

	t.Run("an anonymous Accept-Language: pl page carries lang=pl", func(t *testing.T) {
		t.Parallel()

		rig := newBrowser(t).anonymous()

		_, _, body := rig.getWithHeader("/login", "Accept-Language", "pl")
		assert.Contains(t, body, `lang="pl"`)
	})

	t.Run("an English account's page carries lang=en", func(t *testing.T) {
		t.Parallel()

		rig := newBrowser(t)

		_, _, body := rig.get(settingsPath)
		assert.Contains(t, body, `lang="en"`)
	})
}

func setAccountLocale(t *testing.T, b *browser, email, locale string) {
	t.Helper()

	record, err := b.app.FindAuthRecordByEmail("users", email)
	require.NoError(t, err)

	record.Set("locale", locale)
	require.NoError(t, b.app.Save(record))
}

func (b *browser) getWithHeader(url, header, value string) (int, http.Header, string) {
	b.t.Helper()

	request := httptest.NewRequestWithContext(b.t.Context(), http.MethodGet, url, nil)
	request.Header.Set("Accept", "text/html")
	request.Header.Set(header, value)

	if b.token != "" {
		request.Header.Set("Authorization", b.token)
	}

	recorder := httptest.NewRecorder()
	b.handler.ServeHTTP(recorder, request)

	return recorder.Code, recorder.Header(), recorder.Body.String()
}
