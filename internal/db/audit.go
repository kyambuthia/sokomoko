package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type auditContextKey struct{}

type pendingAudit struct {
	ActorUserID int
	Action      string
	TargetType  string
	TargetID    int
	Details     string
}

// WithAudit attaches an audit row to the next transaction started with this
// context. The row commits with that transaction, and a failed insert rolls
// the business change back.
func WithAudit(ctx context.Context, actorUserID int, action, targetType string, targetID int, details string) context.Context {
	return context.WithValue(ctx, auditContextKey{}, &pendingAudit{
		ActorUserID: actorUserID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Details:     details,
	})
}

// SetAuditTarget fills the audit target when the transaction creates the row
// and the id is not known before the insert.
func SetAuditTarget(ctx context.Context, targetID int) {
	entry, ok := ctx.Value(auditContextKey{}).(*pendingAudit)
	if !ok || entry == nil || targetID <= 0 || entry.TargetID != 0 {
		return
	}
	entry.TargetID = targetID
}

// ClearAudit drops a pending audit when the transaction changes no row.
func ClearAudit(ctx context.Context) {
	entry, ok := ctx.Value(auditContextKey{}).(*pendingAudit)
	if !ok || entry == nil {
		return
	}
	entry.Action = ""
}

func writeContextAudit(ctx context.Context, tx *sql.Tx) error {
	entry, ok := ctx.Value(auditContextKey{}).(*pendingAudit)
	if !ok || entry == nil || strings.TrimSpace(entry.Action) == "" {
		return nil
	}
	if strings.TrimSpace(entry.Action) == "" || strings.TrimSpace(entry.TargetType) == "" {
		return errors.New("action and target_type are required")
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, details)
		 VALUES ($1, $2, $3, $4, $5)`,
		nullInt64(entry.ActorUserID), strings.TrimSpace(entry.Action), strings.TrimSpace(entry.TargetType), nullInt64(entry.TargetID), strings.TrimSpace(entry.Details),
	)
	return err
}

// CreateAuditLog records an action that has already happened, so a caller
// whose request is cancelled still gets its entry written.
func (s *Store) CreateAuditLog(ctx context.Context, actorUserID int, action, targetType string, targetID int, details string) error {
	if strings.TrimSpace(action) == "" || strings.TrimSpace(targetType) == "" {
		return errors.New("action and target_type are required")
	}
	ctx, cancel := s.queryContext(context.WithoutCancel(ctx))
	defer cancel()

	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, details)
		 VALUES ($1, $2, $3, $4, $5)`,
		nullInt64(actorUserID), strings.TrimSpace(action), strings.TrimSpace(targetType), nullInt64(targetID), strings.TrimSpace(details),
	)
	return err
}

func (s *Store) ListAuditLogs(ctx context.Context, limit int) ([]AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT a.id, a.actor_user_id, COALESCE(u.username, ''), a.action, a.target_type, a.target_id, a.details, a.created_at
		 FROM audit_logs a
		 LEFT JOIN users u ON u.id = a.actor_user_id
		 ORDER BY a.created_at DESC, a.id DESC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []AuditLog{}
	for rows.Next() {
		var entry AuditLog
		if err := rows.Scan(&entry.ID, &entry.ActorUserID, &entry.ActorName, &entry.Action, &entry.TargetType, &entry.TargetID, &entry.Details, &entry.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}
