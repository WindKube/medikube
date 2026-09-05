// Package shared holds view components more than one directory or record kind
// uses, starting with the type-ahead picker (FR-039).
package shared

import "strconv"

func jsQuoteShared(value string) string { return strconv.Quote(value) }
