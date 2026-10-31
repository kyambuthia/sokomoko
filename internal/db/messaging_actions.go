package db

import (
	"context"
	"database/sql"
	"errors"
)

func insertChatMessage(ctx context.Context, tx *sql.Tx, c chatRecord, actorID int, body, nonce string, fresh bool) (int, error) {
	var existingID int
	var existingBody string
	err := tx.QueryRowContext(ctx, `SELECT id,body FROM chat_messages WHERE conversation_id=$1 AND sender_user_id=$2 AND client_nonce=$3`,
		c.ID, actorID, nonce).Scan(&existingID, &existingBody)
	if err == nil {
		if existingBody != body {
			return 0, ErrChatConflict
		}
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	mine, theirs, err := chatBlockState(ctx, tx, c, actorID)
	if err != nil {
		return 0, err
	}
	if mine || theirs {
		return 0, ErrChatBlocked
	}
	if !c.Live || (c.State != "accepted" && !(fresh && c.State == "pending" && c.RequesterID == actorID)) {
		return 0, ErrChatClosed
	}
	if err := chatRate(ctx, tx, actorID, "send_minute", 60, 30); err != nil {
		return 0, err
	}
	if err := chatRate(ctx, tx, actorID, "send_day", 86400, 500); err != nil {
		return 0, err
	}
	var id int
	err = tx.QueryRowContext(ctx, `INSERT INTO chat_messages(conversation_id,sender_user_id,body,client_nonce)
      VALUES($1,$2,$3,$4) RETURNING id`, c.ID, actorID, body, nonce).Scan(&id)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE chat_conversations SET updated_at=now(),last_message_id=$2 WHERE id=$1`, c.ID, id)
	// Sending must not mark unseen incoming messages as read.
	return id, err
}

func (s *Store) SendChatMessage(ctx context.Context, actorID, conversationID int, body, nonce string) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var id int
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		c, err := loadChatTx(ctx, tx, actorID, conversationID, true)
		if err != nil {
			return err
		}
		id, err = insertChatMessage(ctx, tx, c, actorID, body, nonce, false)
		return err
	})
	return id, err
}

func (s *Store) ChatAction(ctx context.Context, actorID, conversationID int, action string, messageID int, reason string) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		c, err := loadChatTx(ctx, tx, actorID, conversationID, true)
		if err != nil {
			return err
		}
		switch action {
		case "accept", "decline":
			v, err := chatView(ctx, tx, c, actorID)
			if err != nil {
				return err
			}
			if !v.IncomingRequest || !c.Live {
				return ErrChatClosed
			}
			if action == "accept" && v.Blocked {
				return ErrChatBlocked
			}
			state := "accepted"
			if action == "decline" {
				state = "declined"
			}
			_, err = tx.ExecContext(ctx, `UPDATE chat_conversations SET state=$2,updated_at=now() WHERE id=$1`, c.ID, state)
			return err
		case "block", "unblock":
			a, b := c.parties(actorID)
			if action == "unblock" {
				_, err = tx.ExecContext(ctx, `DELETE FROM chat_blocks WHERE COALESCE(owner_user_id,0)=$1 AND COALESCE(owner_partner_id,0)=$2
                  AND COALESCE(target_user_id,0)=$3 AND COALESCE(target_partner_id,0)=$4`, a.user, a.partner, b.user, b.partner)
			} else {
				_, err = tx.ExecContext(ctx, `INSERT INTO chat_blocks(owner_user_id,owner_partner_id,target_user_id,target_partner_id,created_by_user_id)
                  VALUES(NULLIF($1,0),NULLIF($2,0),NULLIF($3,0),NULLIF($4,0),$5) ON CONFLICT DO NOTHING`, a.user, a.partner, b.user, b.partner, actorID)
			}
			return err
		case "read":
			var exists bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM chat_messages WHERE id=$1 AND conversation_id=$2)`, messageID, c.ID).Scan(&exists)
			if err != nil {
				return err
			}
			if !exists {
				return ErrChatNotFound
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO chat_reads(conversation_id,user_id,last_read_message_id) VALUES($1,$2,$3)
              ON CONFLICT(conversation_id,user_id) DO UPDATE SET last_read_message_id=GREATEST(chat_reads.last_read_message_id,excluded.last_read_message_id)`, c.ID, actorID, messageID)
			return err
		case "report":
			if !validChatReportReason(reason) {
				return ErrChatInvalid
			}
			var senderID int
			err = tx.QueryRowContext(ctx, `SELECT sender_user_id FROM chat_messages WHERE id=$1 AND conversation_id=$2`, messageID, c.ID).Scan(&senderID)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatNotFound
			}
			if err != nil {
				return err
			}
			sameSide := senderID == actorID
			if c.Kind == "seller" {
				sameSide = (senderID == c.CustomerID) == (actorID == c.CustomerID)
			}
			if sameSide {
				return ErrChatInvalid
			}
			if err := chatRate(ctx, tx, actorID, "report", 3600, 10); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO chat_reports(conversation_id,message_id,reporter_user_id,reason)
              VALUES($1,$2,$3,$4) ON CONFLICT(message_id,reporter_user_id) DO NOTHING`, c.ID, messageID, actorID, reason)
			return err
		default:
			return ErrChatInvalid
		}
	})
}

func validChatReportReason(reason string) bool {
	switch reason {
	case "spam", "harassment", "scam", "personal_information", "other":
		return true
	}
	return false
}

// Moderators can inspect selected reported messages, not arbitrary threads.
func (s *Store) ChatReports(ctx context.Context, actorID int) ([]ChatReport, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	// Bound each source before joining report details. Every global top-100 row
	// must be in its source's top 100; encrypted breaks cross-source ties.
	rows, err := s.DB.QueryContext(ctx, `SELECT r.id,sender.username,reporter.username,m.body,r.reason,r.created_at,false AS encrypted
      FROM (
        SELECT id,conversation_id,message_id,reporter_user_id,reason,created_at
        FROM chat_reports
        WHERE resolved_at IS NULL
          AND EXISTS (SELECT 1 FROM users WHERE id=$1 AND role='admin' AND deleted_at IS NULL)
        ORDER BY created_at,id
        LIMIT 100
      ) r
      JOIN chat_messages m ON m.id=r.message_id AND m.conversation_id=r.conversation_id
      JOIN users sender ON sender.id=m.sender_user_id JOIN users reporter ON reporter.id=r.reporter_user_id
      UNION ALL
      SELECT r.id,sender.username,reporter.username,r.disclosed_text,r.reason,r.created_at,true AS encrypted
      FROM (
        SELECT id,event_id,reporter_user_id,reason,disclosed_text,created_at
        FROM chat_encrypted_reports
        WHERE resolved_at IS NULL
          AND EXISTS (SELECT 1 FROM users WHERE id=$1 AND role='admin' AND deleted_at IS NULL)
        ORDER BY created_at,id
        LIMIT 100
      ) r
      JOIN chat_encrypted_events e ON e.id=r.event_id
      JOIN chat_devices d ON d.id=e.sender_device_id JOIN users sender ON sender.id=d.user_id
      JOIN users reporter ON reporter.id=r.reporter_user_id
      ORDER BY created_at,encrypted,id LIMIT 100`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := []ChatReport{}
	for rows.Next() {
		var r ChatReport
		if err := rows.Scan(&r.ID, &r.Sender, &r.Reporter, &r.Body, &r.Reason, &r.CreatedAt, &r.Encrypted); err != nil {
			return nil, err
		}
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (s *Store) ResolveChatReport(ctx context.Context, actorID, reportID int) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		table, kind := "chat_reports", "message_report"
		if reportID < 0 {
			table, kind = "chat_encrypted_reports", "encrypted_message_report"
			reportID = -reportID
		}
		result, err := tx.ExecContext(ctx, `UPDATE `+table+` SET resolved_at=now(),resolved_by_user_id=$1
          WHERE id=$2 AND resolved_at IS NULL AND EXISTS (SELECT 1 FROM users WHERE id=$1 AND role='admin' AND deleted_at IS NULL)`, actorID, reportID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrChatNotFound
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_logs(actor_user_id,action,target_type,target_id,details)
          VALUES($1,'resolve_message_report',$3,$2,'Reported message reviewed')`, actorID, reportID, kind)
		return err
	})
}
