package auth_test

import "strings"

// splitIDs reads an aria-describedby, which is a space-separated list of ids:
// a control described by both its refusal and the published rules names two,
// and a test that treated the attribute as one id would report the pair as a
// dangling reference.
func splitIDs(value string) []string { return strings.Fields(value) }
