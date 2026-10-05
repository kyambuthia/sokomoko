package db

import (
	"errors"
	"strings"
)

func (s *Store) CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error {
	if strings.TrimSpace(action) == "" || strings.TrimSpace(targetType) == "" {
		return errors.New("action and target_type are required")
	}
	ctx, cancel := s.ctx()
	defer cancel()

	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, details)
		 VALUES ($1, $2, $3, $4, $5)`,
		nullInt64(actorUserID), strings.TrimSpace(action), strings.TrimSpace(targetType), nullInt64(targetID), strings.TrimSpace(details),
	)
	return err
}

func (s *Store) ListAuditLogs(limit int) ([]AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	ctx, cancel := s.ctx()
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
