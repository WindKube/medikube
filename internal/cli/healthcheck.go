package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"
)

// healthcheckTimeout bounds the whole dial-and-read, not just readyz's own
// 2s budget: a container HEALTHCHECK needs this to fail as a probe rather
// than hang.
const healthcheckTimeout = 5 * time.Second

var errNotReady = errors.New("cli: the instance is not ready")

// runHealthcheck dials http://{addr}/api/v1/readyz and exits 0 on 200, non-zero
// otherwise. It prints nothing on success — there is no shell in the
// distroless image to redirect chatter to /dev/null.
func runHealthcheck(args []string, deps Deps) error {
	flags := flag.NewFlagSet(commandHealthcheck, flag.ContinueOnError)
	flags.SetOutput(deps.Stderr)

	addr := flags.String("addr", deps.HealthcheckAddr, "the host:port readyz answers on")

	if err := flags.Parse(args); err != nil {
		return err
	}

	return dialReadyz(*addr)
}

func dialReadyz(addr string) error {
	ctx, cancel := context.WithTimeout(context.Background(), healthcheckTimeout)
	defer cancel()

	url := "http://" + addr + "/api/v1/readyz"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("cli: building the healthcheck request: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cli: %s did not answer: %w", addr, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s answered %d", errNotReady, addr, res.StatusCode)
	}

	return nil
}
