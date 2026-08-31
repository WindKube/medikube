package api_test

import (
	"context"

	"github.com/stretchr/testify/assert"

	"medikube/internal/domain/audit"
	"medikube/internal/obs"
	"medikube/internal/platform/pb"
)

// refusingTrail is an audit trail that cannot be written to, which is what a
// database with a full disk looks like from the hook's side.
type refusingTrail struct{}

func (refusingTrail) Record(context.Context, audit.Event) error { return assert.AnError }

// breakTheTrail replaces the sign-in audit's trail on a wired instance,
// leaving every other part of the production wiring alone. PocketBase's
// hook.Bind replaces rather than appends when the id is the same, so this is
// the real binding with one dependency broken.
func breakTheTrail(r *rig) error {
	return pb.BindAuthAudit(r.instance.App, pb.AuthAudit{
		Trail:   refusingTrail{},
		Request: obs.CorrelationID,
	})
}
