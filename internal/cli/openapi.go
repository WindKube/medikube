package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"medikube/internal/openapi"
)

// runOpenAPI writes the generated OpenAPI 3.1 document to stdout or --out.
// Generate -> RoundTrip -> Marshal is contracts/cli.md's required order:
// RoundTrip proves the document is valid as JSON (FACT 9), and the committed
// file is the reloaded document's own marshal.
func runOpenAPI(args []string, deps Deps) error {
	flags := flag.NewFlagSet(commandOpenAPI, flag.ContinueOnError)
	flags.SetOutput(deps.Stderr)

	out := flags.String("out", "", "write the document here instead of stdout")

	if err := flags.Parse(args); err != nil {
		return err
	}

	in, err := deps.OpenAPI()
	if err != nil {
		return fmt.Errorf("cli: building the OpenAPI input: %w", err)
	}

	document, err := openapi.Generate(in)
	if err != nil {
		return fmt.Errorf("cli: generating the OpenAPI document: %w", err)
	}

	loaded, _, err := openapi.RoundTrip(context.Background(), document)
	if err != nil {
		return fmt.Errorf("cli: the generated document does not round-trip: %w", err)
	}

	encoded, err := openapi.Marshal(loaded)
	if err != nil {
		return fmt.Errorf("cli: marshaling the OpenAPI document: %w", err)
	}

	return writeOpenAPI(deps.Stdout, *out, encoded)
}

func writeOpenAPI(stdout io.Writer, out string, encoded []byte) error {
	if out == "" {
		_, err := stdout.Write(encoded)

		return err
	}

	if err := os.WriteFile(out, encoded, 0o644); err != nil { //nolint:gosec // the document is public
		return fmt.Errorf("cli: writing %s: %w", out, err)
	}

	return nil
}
