package obs

import (
	"context"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/config"
	"medikube/internal/domain/kind"
	"medikube/internal/httproute"
)

// The route patterns a MediKube instance registers are the whole label domain.
// Two are enough to prove the allowlist admits what is in it and refuses what
// is not; the real set arrives from internal/httproute at boot.
// The patterns are spelled through the registry's own Pattern() rather than by
// hand, because the allowlist and the registry have to agree on the shape of a
// pattern: if Pattern() ever stopped being "METHOD path", a test that spelled
// it itself would keep passing while every production label read "other".
var testPatterns = []string{
	httproute.Route{Method: http.MethodGet, Path: recordsPath}.Pattern(),
	httproute.Route{Method: http.MethodGet, Path: recordsPath + "/{id}"}.Pattern(),
}

// recordsPath is spelled through the kind table rather than by hand: the
// segment is declared once (research D-05) and internal/architecture fails a
// second spelling.
var recordsPath = "/api/v1/records/" + kind.Medication.Segment()

// startMetrics is the fixture: a registry, an exposed listener and a scrape
// helper, torn down with the test.
func startMetrics(t *testing.T, cfg config.MetricsConfig) (*Metrics, *MetricsServer) {
	t.Helper()

	metrics := NewMetrics()
	metrics.PublishRoutes(testPatterns...)

	server, err := StartMetrics(t.Context(), cfg, metrics, zerolog.Nop())
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		require.NoError(t, server.Shutdown(ctx))
	})

	return metrics, server
}

// A pooled connection that never carried a request keeps http.Server.Shutdown
// waiting for its first five seconds, which is the whole cleanup budget.
var scrapeClient = &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

// scrape reads the exposition the way a Prometheus server would.
func scrape(t *testing.T, address, token string) (int, string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+address+"/metrics", nil)
	require.NoError(t, err)

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := scrapeClient.Do(request)
	require.NoError(t, err)

	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, string(body)
}

// FR-055: the measurements are on a channel an ordinary visitor cannot reach.
//
// The bound address is what settles it and it settles it everywhere: a
// listener on 0.0.0.0 reports the unspecified address, a listener on loopback
// reports 127.0.0.1, and the two are distinguishable without a second
// interface, a second process or a network. The last row is not decoration —
// it is what proves the first two rows can fail.
func TestTheMetricsListenerBindsWhereItIsToldAndTheDefaultIsLoopback(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		addr         string
		wantLoopback bool
	}{
		{name: "the documented default host", addr: "127.0.0.1:0", wantLoopback: true},
		{name: "loopback by name", addr: "localhost:0", wantLoopback: true},
		{
			name: "every interface, which an operator may only do in production with a token",
			addr: "0.0.0.0:0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, server := startMetrics(t, config.MetricsConfig{Enabled: true, Addr: tc.addr})

			host, _, err := net.SplitHostPort(server.Addr())
			require.NoError(t, err)

			ip := net.ParseIP(host)
			require.NotNil(t, ip, "the listener reported %q, which is not an address", host)

			assert.Equal(t, tc.wantLoopback, ip.IsLoopback(),
				"the metrics listener bound %s", server.Addr())
		})
	}
}

// The default is a documented setting and not a constant buried in the server,
// so the assertion is made against the tag an operator's environment overrides
// rather than against a literal repeated here.
func TestTheDocumentedMetricsDefaultAddressIsLoopback(t *testing.T) {
	t.Parallel()

	field, found := reflect.TypeOf(config.MetricsConfig{}).FieldByName("Addr")
	require.True(t, found, "config.MetricsConfig has no Addr field, so this test read nothing")

	declared := field.Tag.Get("envDefault")
	require.NotEmpty(t, declared, "MetricsConfig.Addr declares no envDefault, so there is no default to assert about")

	host, _, err := net.SplitHostPort(declared)
	require.NoError(t, err)

	ip := net.ParseIP(host)
	require.NotNil(t, ip, "the default address %q does not name an IP", declared)

	assert.True(t, ip.IsLoopback(),
		"the default metrics address %q is reachable from off the machine; exposing metrics must be an explicit act", declared)
}

// The socket-level half of FR-055: MediKube's own port is bound to every
// interface, and the metrics port is not, so a caller who can reach the
// application cannot reach the measurements.
func TestTheMetricsListenerIsUnreachableFromANonLoopbackAddress(t *testing.T) {
	t.Parallel()

	_, server := startMetrics(t, config.MetricsConfig{Enabled: true, Addr: "127.0.0.1:0"})

	status, body := scrape(t, server.Addr(), "")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "go_goroutines", "the loopback scrape returned no exposition, so the refusal below proves nothing")

	_, port, err := net.SplitHostPort(server.Addr())
	require.NoError(t, err)

	external := nonLoopbackIPv4(t)
	if external == "" {
		t.Skip("this machine has no non-loopback IPv4 address; the bound-address assertion above carries the claim")
	}

	dialer := net.Dialer{Timeout: 2 * time.Second}

	conn, err := dialer.DialContext(t.Context(), "tcp", net.JoinHostPort(external, port))
	if err == nil {
		conn.Close()
	}

	assert.Error(t, err,
		"the metrics listener answered on %s, so anything that can route to this host can scrape it", external)
}

func nonLoopbackIPv4(t *testing.T) string {
	t.Helper()

	addresses, err := net.InterfaceAddrs()
	require.NoError(t, err)

	for _, address := range addresses {
		ipNet, ok := address.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}

		if ipv4 := ipNet.IP.To4(); ipv4 != nil {
			return ipv4.String()
		}
	}

	return ""
}

// Off is off: no listener, nothing to reach, nothing to shut down. The enabled
// row is what proves the disabled row is an assertion about a socket rather
// than about a struct field.
func TestMetricsAreEntirelyOffWhenTheOperatorTurnsThemOff(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		enabled bool
	}{
		{name: "disabled"},
		{name: "enabled", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, server := startMetrics(t, config.MetricsConfig{Enabled: tc.enabled, Addr: "127.0.0.1:0"})

			if !tc.enabled {
				assert.Empty(t, server.Addr(), "a disabled metrics endpoint is listening on a port")

				return
			}

			require.NotEmpty(t, server.Addr())

			status, body := scrape(t, server.Addr(), "")
			assert.Equal(t, http.StatusOK, status)
			assert.Contains(t, body, "go_goroutines")
		})
	}
}

// A token turns the endpoint into one an operator holds a credential for. It
// exists because config.validateProduction requires one the moment the address
// is not loopback, and a required credential nothing checks is worse than none.
func TestTheMetricsEndpointRefusesAScrapeWithoutTheConfiguredToken(t *testing.T) {
	t.Parallel()

	const token = "a-scrape-token"

	for _, tc := range []struct {
		name       string
		configured string
		offered    string
		want       int
	}{
		{name: "no token configured, no token offered", want: http.StatusOK},
		{name: "no token configured, a token offered", offered: token, want: http.StatusOK},
		{name: "a token configured and matched", configured: token, offered: token, want: http.StatusOK},
		{name: "a token configured, none offered", configured: token, want: http.StatusUnauthorized},
		{name: "a token configured, the wrong one offered", configured: token, offered: "not-it", want: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, server := startMetrics(t, config.MetricsConfig{
				Enabled: true,
				Addr:    "127.0.0.1:0",
				Token:   tc.configured,
			})

			status, _ := scrape(t, server.Addr(), tc.offered)
			assert.Equal(t, tc.want, status)
		})
	}
}

// The listener carries the measurements and nothing else. A metrics port that
// also answers /debug/pprof is a second, undocumented surface on the channel
// FR-055 says is the operator's.
func TestTheMetricsListenerServesNothingButTheMeasurements(t *testing.T) {
	t.Parallel()

	_, server := startMetrics(t, config.MetricsConfig{Enabled: true, Addr: "127.0.0.1:0"})

	for _, path := range []string{"/", "/debug/pprof/", recordsPath, "/metrics/"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+server.Addr()+path, nil)
			require.NoError(t, err)

			response, err := scrapeClient.Do(request)
			require.NoError(t, err)

			defer response.Body.Close()

			assert.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	}
}

// FR-055's second clause: labels come from bounded, published sets.
//
// The mechanism is an allowlist built at construction out of the registered
// route patterns, not a sanitiser applied to whatever the middleware passes.
// A sanitiser has to recognise the thing it must not emit; an allowlist has to
// recognise the thing it may, and there are twenty of those and infinitely
// many of the other.
func TestNothingButAPublishedRoutePatternBecomesALabel(t *testing.T) {
	t.Parallel()

	metrics, server := startMetrics(t, config.MetricsConfig{Enabled: true, Addr: "127.0.0.1:0"})

	// What a middleware that reached for the resolved path instead of the
	// registered pattern would hand over. The id is a real 15-character
	// PocketBase id and the query is a search for a medication.
	metrics.ObserveRequest(recordsPath+"/rec0123456789ab?q="+medicationSentinel,
		http.MethodGet, http.StatusOK, 12*time.Millisecond)

	// A method is a token the client chose, so it is allowlisted too.
	metrics.ObserveRequest(testPatterns[0], "PROPFIND-"+medicationSentinel, http.StatusOK, time.Millisecond)

	// A status outside the HTTP range can only come from a handler that
	// invented one.
	metrics.ObserveRequest(testPatterns[0], http.MethodGet, 9000, time.Millisecond)

	// And the case that proves the allowlist admits anything at all.
	metrics.ObserveRequest(testPatterns[1], http.MethodPost, http.StatusCreated, 3*time.Millisecond)

	status, body := scrape(t, server.Addr(), "")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "medikube_http_requests_total",
		"the exposition carries no request counter, so the sweep below scanned nothing")

	// The sweep runs over the exposition text a scrape would actually receive,
	// because metric names, label values and HELP strings are the three places
	// a person's data reaches a metrics endpoint and the exposition format is
	// those three rendered.
	assert.NotContains(t, strings.ToLower(body), strings.ToLower(medicationSentinel),
		"a medication name reached the Prometheus exposition (FR-038, FR-055)")
	assert.NotContains(t, body, "rec0123456789ab",
		"a record id reached the Prometheus exposition as a label value")

	// The label sets themselves come from the gatherer rather than from the
	// text, so the assertion is about which series exist and not about how a
	// renderer happens to order label pairs.
	assert.ElementsMatch(t, []map[string]string{
		{"route": labelOther, "method": http.MethodGet, "status": "200"},
		{"route": testPatterns[0], "method": labelOther, "status": "200"},
		{"route": testPatterns[0], "method": http.MethodGet, "status": labelOther},
		{"route": testPatterns[1], "method": http.MethodPost, "status": "201"},
	}, labelSets(t, metrics, "medikube_http_requests_total"))
}

// labelSets is every series the named counter carries, as plain maps.
//
// The require on the count is the guard on the guard: an ElementsMatch against
// an empty slice is a comparison of two empty slices, and a gatherer that
// stopped carrying the counter would report that as agreement.
func labelSets(t *testing.T, metrics *Metrics, name string) []map[string]string {
	t.Helper()

	families, err := metrics.Registry().Gather()
	require.NoError(t, err)
	require.NotEmpty(t, families, "the registry gathered nothing at all")

	var sets []map[string]string

	for _, family := range families {
		if family.GetName() != name {
			continue
		}

		for _, metric := range family.GetMetric() {
			set := make(map[string]string, len(metric.GetLabel()))
			for _, pair := range metric.GetLabel() {
				set[pair.GetName()] = pair.GetValue()
			}

			sets = append(sets, set)
		}
	}

	require.NotEmpty(t, sets, "the registry carries no series named %s", name)

	return sets
}

// TestThePhaseTwoInstrumentsRecordAndAllowlistTheirLabels is T160: the four
// counters/histograms this phase adds, proving both halves — a call
// increments the right series, and a value off the published list still
// lands somewhere rather than becoming an unbounded label.
func TestThePhaseTwoInstrumentsRecordAndAllowlistTheirLabels(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics()

	metrics.RecordCreated("patient")
	metrics.RecordCreated("practitioner")
	metrics.RecordCreated("facility")
	metrics.RecordCreated("medication")
	metrics.RecordCreated("a patient nobody should be able to name as a label")

	assert.ElementsMatch(t, []map[string]string{
		{"kind": "patient"}, {"kind": "practitioner"}, {"kind": "facility"},
		{"kind": "medication"}, {"kind": labelOther},
	}, labelSets(t, metrics, "medikube_records_total"))

	metrics.ObservePhotoBytes(1 << 20)
	assert.Contains(t, labelSets(t, metrics, "medikube_files_photo_bytes"), map[string]string{})

	metrics.ObserveThumbDuration("100x100t", 5*time.Millisecond)
	metrics.ObserveThumbDuration("some size an operator typed in by hand", time.Millisecond)

	assert.ElementsMatch(t, []map[string]string{
		{"size": "100x100t"}, {"size": labelOther},
	}, labelSets(t, metrics, "medikube_files_thumb_duration_seconds"))

	metrics.PatientSwitch("ok")
	metrics.PatientSwitch("cleared")
	metrics.PatientSwitch("not_found")
	metrics.PatientSwitch("something a bug invented")

	assert.ElementsMatch(t, []map[string]string{
		{"outcome": "ok"}, {"outcome": "cleared"}, {"outcome": "not_found"}, {"outcome": labelOther},
	}, labelSets(t, metrics, "medikube_patients_switch_total"))
}

// TestThePhaseTwoInstrumentsAreNilSafe is FR-039/FR-056's other half: a
// *Metrics that was never wired (SetMetrics/SetTracer never called, or the
// composition root passed a nil pointer through) must not panic a caller
// that has no reason to nil-check an observability call.
func TestThePhaseTwoInstrumentsAreNilSafe(t *testing.T) {
	t.Parallel()

	var metrics *Metrics

	assert.NotPanics(t, func() {
		metrics.RecordCreated("patient")
		metrics.ObservePhotoBytes(1024)
		metrics.ObserveThumbDuration("100x100t", time.Millisecond)
		metrics.PatientSwitch("ok")
	})
}
