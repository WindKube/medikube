package cli

import "flag"

// runSeed takes no flags: there is no --force escape from the production
// refusal (FR-060).
func runSeed(args []string, deps Deps) error {
	flags := flag.NewFlagSet(commandSeed, flag.ContinueOnError)
	flags.SetOutput(deps.Stderr)

	if err := flags.Parse(args); err != nil {
		return err
	}

	return deps.Seed(deps.Stdout)
}
