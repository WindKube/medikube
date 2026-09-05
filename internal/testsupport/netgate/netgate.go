//go:build netgate

// Package netgate proves FR-047: with no external destination configured,
// nothing this phase's endpoints do opens a network connection off the
// process. It installs a process-wide net.Dialer.Control hook that refuses
// every dial attempt and records the address, so a caller can assert not
// merely that a call failed but that nothing even tried to leave.
//
// This phase owns the harness; phases 003, 005 and 006 extend the exercise
// dial_test.go drives rather than declaring a second one (cross-artifact
// finding M6's rule, applied to egress).
package netgate

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"syscall"
)

// Trap is a net.Dialer.Control hook that refuses every dial it sees.
type Trap struct {
	mu    sync.Mutex
	dials []string
}

// Control satisfies net.Dialer's Control field.
func (tr *Trap) Control(network, address string, _ syscall.RawConn) error {
	tr.mu.Lock()
	tr.dials = append(tr.dials, network+" "+address)
	tr.mu.Unlock()

	return fmt.Errorf("netgate: outbound dial to %s %q refused; this exercise configured no destination for it", network, address)
}

// Dials returns every address the trap refused, in the order it saw them.
func (tr *Trap) Dials() []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	return append([]string(nil), tr.dials...)
}

// Arm installs the trap as the dialer behind http.DefaultTransport and
// http.DefaultClient — where Go's own HTTP stack dials when nothing
// overrides it, and where every outbound client this repository's
// dependencies use (Sentry's transport, the OTLP exporter) reaches unless it
// was handed a destination of its own. restore puts the previous transport
// back; a caller runs it in t.Cleanup so one exercise cannot leave the trap
// armed for the next.
func Arm() (trap *Trap, restore func()) {
	trap = &Trap{}

	dialer := &net.Dialer{Control: trap.Control}
	transport := &http.Transport{DialContext: dialer.DialContext}

	previousDefaultTransport := http.DefaultTransport
	previousClientTransport := http.DefaultClient.Transport

	http.DefaultTransport = transport
	http.DefaultClient.Transport = transport

	return trap, func() {
		http.DefaultTransport = previousDefaultTransport
		http.DefaultClient.Transport = previousClientTransport
	}
}
