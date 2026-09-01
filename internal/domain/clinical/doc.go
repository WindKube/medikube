// Package clinical holds the clinical entities and their enumerations.
//
// Its types marshal into the log stream by identifier alone: a diagnosis, a
// medication name or a note must never reach a log, a trace or a metric, and
// redacting marshallers are what makes that structural rather than remembered.
package clinical
