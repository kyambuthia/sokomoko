package db

import (
	"context"
	"database/sql"
	"errors"
)

// lockWorkspaceActorTx pins the actor account and, for partners, the active
// partner membership until the caller's workspace mutation commits.
func lockWorkspaceActorTx(ctx context.Context, tx *sql.Tx, actorID int, actorRole string) (int, bool, error) {
	if actorID <= 0 {
		return 0, false, ErrWorkspaceActorNotFound
	}
	switch actorRole {
	case RolePartner:
		var partnerID int
		err := tx.QueryRowContext(ctx, `SELECT p.id
			FROM partners p
			JOIN partner_users pu ON pu.partner_id = p.id
			JOIN users u ON u.id = pu.user_id
			WHERE u.id = $1 AND u.role = $2 AND u.deleted_at IS NULL AND p.status = 'active'
			FOR SHARE OF p, pu, u`,
			actorID, RolePartner,
		).Scan(&partnerID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, ErrWorkspaceActorNotFound
		}
		if err != nil {
			return 0, false, err
		}
		return partnerID, true, nil
	case RoleAdmin, RoleStaff:
		var authorizedUserID int
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM users WHERE id = $1 AND role = $2 AND deleted_at IS NULL FOR SHARE`,
			actorID, actorRole,
		).Scan(&authorizedUserID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, ErrWorkspaceActorNotFound
		}
		if err != nil {
			return 0, false, err
		}
		return 0, false, nil
	default:
		return 0, false, ErrWorkspaceActorNotFound
	}
}
