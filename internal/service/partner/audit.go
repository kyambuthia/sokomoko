package partner

import (
	"context"
	"log/slog"
)

func (s *Service) logAudit(ctx context.Context, actorUserID int, action, targetType string, targetID int, details string) {
	if err := s.store.CreateAuditLog(ctx, actorUserID, action, targetType, targetID, details); err != nil {
		slog.Error("audit log write failed",
			"action", action,
			"target_type", targetType,
			"target_id", targetID,
			"error", err,
		)
	}
}
