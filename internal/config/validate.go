package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/rs/zerolog"
)

// environments are the values MEDIKUBE_ENV accepts. Only "production" changes
// behaviour; the rest exist so a typo is caught rather than silently treated as
// non-production. One spelling per environment: "dev" as a second name for
// "development" is a fork waiting to happen the first time something compares
// the string to anything other than "production".
var environments = []string{"production", "staging", "development"}

// Validate reports every problem at once (FR-051). One misspelled variable per
// container restart is how an operator loses an afternoon.
func (c Config) Validate() error {
	var errs []error

	if strings.TrimSpace(c.DataDir) == "" {
		// FR-061. There is deliberately no default: PocketBase's own fallback
		// puts pb_data beside the binary, which in the image is a read-only
		// layer, and that failure surfaces long after boot.
		errs = append(errs, errors.New("MEDIKUBE_DATA_DIR must be set to the directory that holds everything this instance keeps"))
	}

	if _, err := zerolog.ParseLevel(c.Log.Level); err != nil {
		errs = append(errs, fmt.Errorf("MEDIKUBE_LOG_LEVEL %q is not a level", c.Log.Level))
	}

	if !slices.Contains(environments, c.Env) {
		errs = append(errs, fmt.Errorf("MEDIKUBE_ENV %q must be one of %s", c.Env, strings.Join(environments, "|")))
	}

	if u, err := url.Parse(c.PublicURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("MEDIKUBE_PUBLIC_URL %q is not an absolute URL", c.PublicURL))
	}

	if c.DrainMax <= c.DrainDelay {
		errs = append(errs, fmt.Errorf("MEDIKUBE_DRAIN_MAX (%s) must exceed MEDIKUBE_DRAIN_DELAY (%s)", c.DrainMax, c.DrainDelay))
	}

	if c.Auth.SessionTTL <= 0 {
		errs = append(errs, errors.New("MEDIKUBE_AUTH_SESSION_TTL must be a positive duration"))
	}

	if c.Retention.AuditDays <= 0 {
		errs = append(errs, errors.New("MEDIKUBE_RETENTION_AUDIT_DAYS must be a positive number of days"))
	}

	if c.Sentry.SampleRate < 0 || c.Sentry.SampleRate > 1 {
		errs = append(errs, errors.New("MEDIKUBE_SENTRY_SAMPLE_RATE must be within [0,1]"))
	}

	if c.OTel.SampleRatio < 0 || c.OTel.SampleRatio > 1 {
		errs = append(errs, errors.New("MEDIKUBE_OTEL_SAMPLE_RATIO must be within [0,1]"))
	}

	if c.OTel.Enabled && strings.TrimSpace(c.OTel.Endpoint) == "" {
		errs = append(errs, errors.New("MEDIKUBE_OTEL_ENDPOINT is required when MEDIKUBE_OTEL_ENABLED is true"))
	}

	if c.Metrics.Enabled {
		if _, _, err := net.SplitHostPort(c.Metrics.Addr); err != nil {
			errs = append(errs, fmt.Errorf("MEDIKUBE_METRICS_ADDR %q is not host:port", c.Metrics.Addr))
		}
	}

	errs = append(errs, c.validateFiles()...)
	errs = append(errs, c.validateProduction()...)

	return errors.Join(errs...)
}

func (c Config) validateFiles() []error {
	var errs []error

	if c.Files.MaxUploadBytes <= 0 {
		errs = append(errs, errors.New("MEDIKUBE_FILES_MAX_UPLOAD_BYTES must be a positive number of bytes"))
	}

	if len(c.Files.AllowedMIME) == 0 {
		errs = append(errs, errors.New("MEDIKUBE_FILES_ALLOWED_MIME must list at least one media type"))
	}

	for _, mime := range c.Files.AllowedMIME {
		if !strings.Contains(mime, "/") {
			errs = append(errs, fmt.Errorf("MEDIKUBE_FILES_ALLOWED_MIME entry %q is not a media type", mime))
		}
	}

	return errs
}

// The rails that only make sense once the instance holds somebody's records.
func (c Config) validateProduction() []error {
	if c.Env != "production" {
		return nil
	}

	var errs []error

	if c.Dev {
		errs = append(errs, errors.New("MEDIKUBE_DEV must be false when MEDIKUBE_ENV is production"))
	}

	if c.Log.Pretty {
		errs = append(errs, errors.New("MEDIKUBE_LOG_PRETTY must be false when MEDIKUBE_ENV is production"))
	}

	if strings.HasPrefix(c.PublicURL, "http://") && !isLoopback(c.PublicURL) {
		errs = append(errs, errors.New("MEDIKUBE_PUBLIC_URL must be https when MEDIKUBE_ENV is production"))
	}

	// Bound to every interface, the endpoint is reachable from the pod network,
	// and metric labels name real route patterns.
	if c.Metrics.Enabled && !isLoopbackAddr(c.Metrics.Addr) && c.Metrics.Token == "" {
		errs = append(errs, errors.New("MEDIKUBE_METRICS_TOKEN is required when MEDIKUBE_METRICS_ADDR is not bound to loopback in production"))
	}

	return errs
}

func isLoopback(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return isLoopbackHost(u.Hostname())
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	return isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
