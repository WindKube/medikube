// Package phileak captures every sink patient data could escape through — the
// log stream, the metrics gatherer, the span recorder and Sentry — drives the
// application against sentinel-seeded data, and asserts no sentinel reached any
// of them.
//
// It sits on the PocketBase side of the import boundary.
package phileak
