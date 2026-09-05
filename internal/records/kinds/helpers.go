package kinds

import (
	"strconv"

	"medikube/internal/domain/clinical"
)

// joinWords is a wiring failure's field list, human-readable.
func joinWords(words []string) string {
	switch len(words) {
	case 1:
		return words[0]
	case 2:
		return words[0] + " and no " + words[1]
	default:
		return words[0] + ", no " + joinWords(words[1:])
	}
}

// boolFilter converts a `?x=true`/`?x=false` narrowing. Anything else,
// including absence, means unnarrowed.
func boolFilter(values []string) *bool {
	if len(values) == 0 {
		return nil
	}

	parsed, err := strconv.ParseBool(values[0])
	if err != nil {
		return nil
	}

	return &parsed
}

func conditionStatuses(values []string) []clinical.ConditionStatus {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.ConditionStatus, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.ConditionStatus(value))
	}

	return converted
}

func conditionStatusStrings() []string {
	statuses := clinical.ConditionStatuses()
	values := make([]string, 0, len(statuses))

	for _, status := range statuses {
		values = append(values, string(status))
	}

	return values
}

func severities(values []string) []clinical.Severity {
	if len(values) == 0 {
		return nil
	}

	converted := make([]clinical.Severity, 0, len(values))
	for _, value := range values {
		converted = append(converted, clinical.Severity(value))
	}

	return converted
}

func severityStrings() []string {
	severities := clinical.Severities()
	values := make([]string, 0, len(severities))

	for _, severity := range severities {
		values = append(values, string(severity))
	}

	return values
}

// boolStrings is the vocabulary a boolean filter is checked against.
func boolStrings() []string {
	return []string{"true", "false"}
}
