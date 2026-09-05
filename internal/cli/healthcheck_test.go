package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/cli"
)

func TestHealthcheckExitsZeroOnAReadyInstance(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/readyz", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, HealthcheckAddr: strings.TrimPrefix(server.URL, "http://")}

	handled, err := cli.Dispatch([]string{"healthcheck"}, deps)
	require.True(t, handled)
	require.NoError(t, err)
	assert.Empty(t, stdout.Bytes(), "healthcheck prints nothing on success")
}

func TestHealthcheckExitsNonZeroOnA503(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, HealthcheckAddr: strings.TrimPrefix(server.URL, "http://")}

	handled, err := cli.Dispatch([]string{"healthcheck"}, deps)
	require.True(t, handled)
	assert.Error(t, err)
}

func TestHealthcheckExitsNonZeroOnAnUnreachableAddress(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	// Port 0's listener never exists to dial and no local firewall is
	// involved, so this fails fast rather than waiting out the healthcheck's
	// own timeout.
	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, HealthcheckAddr: "127.0.0.1:0"}

	handled, err := cli.Dispatch([]string{"healthcheck"}, deps)
	require.True(t, handled)
	assert.Error(t, err)
}

func TestHealthcheckAddrFlagOverridesTheDefault(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer

	deps := cli.Deps{Stdout: &stdout, Stderr: &stderr, HealthcheckAddr: "127.0.0.1:0"}

	handled, err := cli.Dispatch([]string{"healthcheck", "--addr", strings.TrimPrefix(server.URL, "http://")}, deps)
	require.True(t, handled)
	require.NoError(t, err)
}
