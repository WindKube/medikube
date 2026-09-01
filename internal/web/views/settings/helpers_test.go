package settings_test

import "strings"

// describedIDs reads an aria-describedby, which is a space-separated list: the
// new-password control names both its refusal and the published rules, and a
// test that treated the attribute as one id would report the pair as dangling.
func describedIDs(value string) []string { return strings.Fields(value) }
