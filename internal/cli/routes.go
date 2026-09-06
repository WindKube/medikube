package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"medikube/internal/httproute"
)

// RouteRow is `medikube routes --json`'s wire contract: e2e/routes.ts
// consumes it, so the member names are published and stable.
type RouteRow struct {
	OpID     string `json:"op_id"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Auth     string `json:"auth"`
	Landmark string `json:"landmark,omitempty"`
	SmokeURL string `json:"smoke_url,omitempty"`
	// SmokeVariants is US9's addition (contracts/pages.md §3.5): the status
	// views a page's own SmokeURL narrows into, each a concrete URL the
	// browser gate also visits, emitted inside this route's own row rather
	// than as separate rows — they are not pages (research L2).
	SmokeVariants []string `json:"smoke_variants,omitempty"`
	Summary       string   `json:"summary"`
}

func rowOf(route httproute.Route) RouteRow {
	return RouteRow{
		OpID:          route.OpID,
		Method:        route.Method,
		Path:          route.Path,
		Kind:          string(route.Kind),
		Auth:          string(route.Auth),
		Landmark:      route.Landmark,
		SmokeURL:      route.SmokeURL,
		SmokeVariants: route.SmokeVariants,
		Summary:       route.Summary,
	}
}

func runRoutes(args []string, deps Deps) error {
	flags := flag.NewFlagSet(commandRoutes, flag.ContinueOnError)
	flags.SetOutput(deps.Stderr)

	asJSON := flags.Bool("json", false, "print the machine form e2e/routes.ts consumes")

	if err := flags.Parse(args); err != nil {
		return err
	}

	routes := deps.Routes()

	rows := make([]RouteRow, 0, len(routes))
	for _, route := range routes {
		rows = append(rows, rowOf(route))
	}

	if *asJSON {
		return writeJSON(deps.Stdout, rows)
	}

	return writeRoutesTable(deps.Stdout, rows)
}

func writeJSON(w io.Writer, rows []RouteRow) error {
	encoded, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Errorf("cli: encoding the route inventory: %w", err)
	}

	_, err = fmt.Fprintln(w, string(encoded))

	return err
}

func writeRoutesTable(w io.Writer, rows []RouteRow) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)

	if _, err := fmt.Fprintln(tw, "METHOD\tPATH\tAUTH\tLANDMARK\tSUMMARY"); err != nil {
		return fmt.Errorf("cli: printing the route inventory: %w", err)
	}

	for _, row := range rows {
		landmark := row.Landmark
		if landmark == "" {
			landmark = "-"
		}

		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", row.Method, row.Path, row.Auth, landmark, row.Summary); err != nil {
			return fmt.Errorf("cli: printing the route inventory: %w", err)
		}
	}

	return tw.Flush()
}
