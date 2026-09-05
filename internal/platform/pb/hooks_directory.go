package pb

import (
	"context"
	"errors"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
)

// The hook ids for the directory audit, distinct from hooks_records.go's own:
// PocketBase's Bind Id must be unique per hook or one silently replaces the
// other, and practitioners/facilities are audited by a second Bind because
// they are not a kind.Kind (research D-05) and BindRecordAudit takes
// []kind.Kind.
const (
	directoryAuditCreateHookID = "medikube_audit_directory_create"
	directoryAuditUpdateHookID = "medikube_audit_directory_update"
	directoryAuditDeleteHookID = "medikube_audit_directory_delete"
)

// DirectoryAudit is everything the three directory hooks need. It is
// structurally RecordAudit with Kinds replaced by an explicit
// collection-to-target map, because the directory has no kind.Kind to derive
// one from.
type DirectoryAudit struct {
	Trail Trail

	// Collections is what to audit: collection name to the audit target it
	// writes rows under. A collection absent from it is not audited.
	Collections map[string]audit.TargetKind

	Actor   func(context.Context) (access.Actor, bool)
	Request func(context.Context) string
}

// BindDirectoryAudit binds the same three post-commit hooks BindRecordAudit
// does, keyed by an explicit collection map instead of a kind table.
func BindDirectoryAudit(app core.App, config DirectoryAudit) error {
	if app == nil {
		return errors.New("pb: the directory audit is bound to no application")
	}

	if config.Trail == nil {
		return errors.New("pb: the directory audit is bound with no trail, so every write would go unrecorded")
	}

	if len(config.Collections) == 0 {
		return errors.New("pb: the directory audit is bound to no collections, so it would record nothing")
	}

	record := RecordAudit{Trail: config.Trail, Actor: config.Actor, Request: config.Request}

	bind(app.OnRecordAfterCreateSuccess(), directoryAuditCreateHookID, record, config.Collections, audit.ActionCreate)
	bind(app.OnRecordAfterUpdateSuccess(), directoryAuditUpdateHookID, record, config.Collections, audit.ActionUpdate)
	bind(app.OnRecordAfterDeleteSuccess(), directoryAuditDeleteHookID, record, config.Collections, audit.ActionDelete)

	return nil
}
