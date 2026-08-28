// Command medikube is the MediKube server: one static binary with an embedded
// PocketBase.
//
// This file is the composition root. Everything is constructed here and handed
// down, and this is the only place in MediKube permitted to panic — a
// dependency that cannot be built at startup is programmer error, and there is
// nobody left to return an error to.
//
// It sits on the PocketBase side of the import boundary.
package main

import (
	"fmt"
	"os"
)

// The one deliberate write to a file descriptor in MediKube's own code, and
// this package is one of three named in internal/logging/singlestream_test.go's
// operator surface. Principle VI governs the log stream, not stdout: a version
// line, `medikube routes` and the serve banner PocketBase's own cobra command
// prints are operator output, and a JSON-only stdout is not implementable on
// this stack. The exemption is a directory list rather than a set of function
// names, so a handler writing to os.Stdout is caught wherever it lives and
// however it spells the call.
//
// This body is a placeholder. The composition root is T132, and when it lands
// the version belongs on RootCmd.Version and on the boot log line.
func main() {
	if _, err := fmt.Fprintf(os.Stdout, "medikube %s\n", version); err != nil {
		os.Exit(1)
	}
}
