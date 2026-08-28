// Package obs wires the observability exporters — Sentry for errors and panics,
// Prometheus for metrics, OpenTelemetry for traces — and the request middleware
// that correlates them with the log stream.
//
// Every exporter is off until an operator configures it, because MediKube makes
// no outbound request nobody asked for.
//
// It sits on the PocketBase side of the import boundary.
package obs
