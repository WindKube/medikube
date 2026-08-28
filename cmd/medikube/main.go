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

func main() {
	if _, err := fmt.Fprintf(os.Stdout, "medikube %s\n", version); err != nil {
		os.Exit(1)
	}
}
