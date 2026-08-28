package pb_test

import (
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// A locked route must be indistinguishable from a route that never existed.
//
// The status and the body were already asserted elsewhere. The headers were
// not, and that is where it was false: the lockdown short-circuits, so at any
// priority below PocketBase's security-headers middleware (-1010) a locked 404
// loses X-Content-Type-Options, X-Frame-Options, X-Xss-Protection and
// Cross-Origin-Opener-Policy while a genuine 404 keeps all four. One header
// answers the question the 404 is supposed to refuse.
//
// This asserts the whole header set rather than those four names, so the next
// middleware upstream adds does not reopen the gap silently.
func TestALockedRouteIsHeaderIdenticalToAGenuine404(t *testing.T) {
	t.Parallel()

	h := newHarness(t, bindMediKubeServe)

	genuine := h.do(t, http.MethodGet, "/api/this-route-has-never-existed", "", "")
	require.Equal(t, http.StatusNotFound, genuine.Status)

	locked := []string{
		"/api/collections/users/records",
		"/api/collections/" + kind.Medication.Collection() + "/records",
		"/api/collections/users/records/anyrecordid000",
	}

	for _, target := range locked {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			res := h.do(t, http.MethodGet, target, "", "")

			require.Equal(t, http.StatusNotFound, res.Status)
			assert.Equal(t, genuine.Body, res.Body)
			assert.Equal(t, headerNames(genuine.Header), headerNames(res.Header),
				"a locked 404 carrying a different header set than a genuine one tells an anonymous caller which route exists")

			for _, name := range headerNames(genuine.Header) {
				assert.Equal(t, genuine.Header.Values(name), res.Header.Values(name), name)
			}
		})
	}
}

func headerNames(h http.Header) []string {
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}
