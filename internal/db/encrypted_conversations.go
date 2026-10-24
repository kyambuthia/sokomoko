package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

func startEncryptedChat(ctx context.Context, tx *sql.Tx, actorID int, r EncryptedRequest, out *EncryptedResponse) error {
	p, err := encryptedPayload[struct {
		Target ChatTarget `json:"target"`
	}](r.Payload)
	if err != nil {
		return err
	}
	p.Target.Handle = strings.TrimPrefix(strings.TrimSpace(p.Target.Handle), "@")
	c, err := resolveChatTarget(ctx, tx, actorID, p.Target)
	if err != nil {
		return err
	}
	if err = c.lockPair(ctx, tx); err != nil {
		return err
	}
	mine, theirs, err := chatBlockState(ctx, tx, c, actorID)
	if err != nil {
		return err
	}
	if mine || theirs {
		return ErrChatBlocked
	}
	id, err := findChat(ctx, tx, c)
	if err != nil {
		return err
	}
	if id == 0 {
		if err = chatRate(ctx, tx, actorID, "start", 3600, 10); err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO chat_conversations(kind,customer_user_id,other_user_id,partner_id,product_id,order_id,delivery_id,assignment_event_id,requester_user_id,state,last_message_id)
  VALUES($1,$2,NULLIF($3,0),NULLIF($4,0),NULLIF($5,0),NULLIF($6,0),NULLIF($7,0),NULLIF($8,0),$9,$10,nextval(pg_get_serial_sequence('chat_messages','id')))RETURNING id`, c.Kind, c.CustomerID, c.OtherID, c.PartnerID, c.ProductID, c.OrderID, c.DeliveryID, c.AssignmentID, actorID, c.State).Scan(&id)
	}
	out.ConversationID = id
	return err
}
func encryptedRoster(ctx context.Context, tx *sql.Tx, c chatRecord) (EncryptedRoster, error) {
	result := EncryptedRoster{Devices: []ChatDevice{}}
	if c.PartnerID > 0 {
		if _, err := tx.ExecContext(ctx, `SELECT id FROM partners WHERE id=$1 FOR SHARE`, c.PartnerID); err != nil {
			return result, err
		}
		if _, err := tx.ExecContext(ctx, `SELECT user_id FROM partner_users WHERE partner_id=$1 FOR SHARE`, c.PartnerID); err != nil {
			return result, err
		}
	}

	rows, err := tx.QueryContext(ctx, `SELECT d.id,d.user_id,u.username,d.name,d.public_key,true,false
 FROM chat_devices d JOIN users u ON u.id=d.user_id AND u.deleted_at IS NULL
 WHERE d.approved_at IS NOT NULL AND d.revoked_at IS NULL AND
 ((u.id=$1 AND u.role='user') OR ($2::text='direct' AND u.id=$3 AND u.role='user') OR
 ($2='delivery' AND u.id=$3 AND u.role='courier') OR ($2='seller' AND u.role='partner' AND EXISTS(SELECT 1 FROM partner_users pu JOIN partners p ON p.id=pu.partner_id AND p.status='active' WHERE pu.user_id=u.id AND pu.partner_id=$4)))
 ORDER BY d.id LIMIT 33 FOR SHARE OF d,u`, c.CustomerID, c.Kind, c.OtherID, c.PartnerID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var customer, other bool
	for rows.Next() {
		var d ChatDevice
		if err := rows.Scan(&d.ID, &d.UserID, &d.Username, &d.Name, &d.PublicKey, &d.Approved, &d.Revoked); err != nil {
			return result, err
		}
		result.Devices = append(result.Devices, d)
		if d.UserID == c.CustomerID {
			customer = true
		} else {
			other = true
		}
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.Devices) > 32 {
		return result, ErrChatLimited
	}
	encoded, err := json.Marshal(result.Devices)
	if err != nil {
		return result, err
	}
	hash := sha256.Sum256(encoded)
	result.Hash = hex.EncodeToString(hash[:])
	result.Ready = customer && other
	return result, nil
}

type encryptedConversationInput struct {
	ConversationID int      `json:"conversation_id"`
	SessionID      string   `json:"session_id"`
	RosterHash     string   `json:"roster_hash"`
	Packages       []string `json:"packages"`
	Welcome        []byte   `json:"welcome"`
	Ciphertext     []byte   `json:"ciphertext"`
	Kind           string   `json:"kind"`
	After          int      `json:"after"`
	EventID        int      `json:"event_id"`
	Reason         string   `json:"reason"`
	DisclosedText  string   `json:"disclosed_text"`
}

func encryptedConversation(ctx context.Context, tx *sql.Tx, userID int, d ChatDevice, r EncryptedRequest, out *EncryptedResponse) error {
	p, err := encryptedPayload[encryptedConversationInput](r.Payload)
	if err != nil || p.ConversationID <= 0 || p.After < 0 {
		return ErrChatInvalid
	}
	c, err := loadChatTx(ctx, tx, userID, p.ConversationID, true)
	if err != nil {
		return err
	}
	v, err := chatView(ctx, tx, c, userID)
	if err != nil {
		return err
	}
	out.Conversation = &v
	roster, err := encryptedRoster(ctx, tx, c)
	if err != nil {
		return err
	}
	out.Roster = &roster
	var current sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT encrypted_session_id FROM chat_conversations WHERE id=$1`, c.ID).Scan(&current); err != nil {
		return err
	}
	if r.Op == "read" {
		var exists bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM chat_encrypted_events e JOIN chat_encrypted_sessions s ON s.id=e.session_id WHERE e.id=$1 AND e.conversation_id=$2 AND s.roster @> jsonb_build_array(jsonb_build_object('id',$3::text)))`, p.EventID, c.ID, d.ID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return ErrChatNotFound
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO chat_encrypted_reads(conversation_id,user_id,last_event_id)VALUES($1,$2,$3)ON CONFLICT(conversation_id,user_id)DO UPDATE SET last_event_id=GREATEST(chat_encrypted_reads.last_event_id,excluded.last_event_id)`, c.ID, userID, p.EventID)
		return err
	}
	if r.Op == "events" {
		return encryptedEvents(ctx, tx, c, d, p, out)
	}
	if r.Op == "report" {
		return reportEncrypted(ctx, tx, userID, c, d, p, out)
	}
	if current.Valid {
		session, err := loadEncryptedSession(ctx, tx, current.String)
		if err != nil {
			return err
		}
		out.Session = &session
	}
	if r.Op == "roster" {
		return nil
	}
	if v.Blocked {
		return ErrChatBlocked
	}
	if !c.Live || c.State == "declined" {
		return ErrChatClosed
	}
	if !roster.Ready {
		return ErrChatDeviceApproval
	}
	if p.RosterHash != roster.Hash {
		return ErrChatRosterChanged
	}
	if !slices.ContainsFunc(roster.Devices, func(x ChatDevice) bool { return x.ID == d.ID }) {
		return ErrChatNotFound
	}
	switch r.Op {
	case "claim":
		if err = chatRate(ctx, tx, userID, "package_claim", 60, 5); err != nil {
			return err
		}
		if out.Session != nil && out.Session.RosterHash == roster.Hash {
			return ErrChatConflict
		}
		if out.Session != nil && !slices.ContainsFunc(out.Session.Roster, func(x ChatDevice) bool { return x.ID == d.ID }) {
			return ErrChatDeviceApproval
		}
		for _, peer := range roster.Devices {
			if peer.ID == d.ID {
				continue
			}
			var kp ChatPackage
			err = tx.QueryRowContext(ctx, `UPDATE chat_device_packages SET claimed_at=now(),claimed_by=$2,conversation_id=$3 WHERE id=(SELECT id FROM chat_device_packages WHERE device_id=$1 AND claimed_at IS NULL AND created_at > now()-interval '30 days' ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,device_id,package`, peer.ID, d.ID, c.ID).Scan(&kp.ID, &kp.DeviceID, &kp.Package)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrChatDeviceApproval
			}
			if err != nil {
				return err
			}
			out.Packages = append(out.Packages, kp)
		}
		return nil
	case "session":
		if !encryptedID.MatchString(p.SessionID) || len(p.Welcome) < 64 || len(p.Welcome) > 262144 || len(p.Packages) != len(roster.Devices)-1 {
			return ErrChatInvalid
		}
		if out.Session != nil && out.Session.RosterHash == roster.Hash {
			return ErrChatConflict
		}
		if out.Session != nil && !slices.ContainsFunc(out.Session.Roster, func(x ChatDevice) bool { return x.ID == d.ID }) {
			return ErrChatDeviceApproval
		}
		seen := map[string]bool{}
		for _, id := range p.Packages {
			var owner string
			err = tx.QueryRowContext(ctx, `SELECT device_id FROM chat_device_packages WHERE id=$1 AND claimed_by=$2 AND conversation_id=$3 AND used_session_id IS NULL`, id, d.ID, c.ID).Scan(&owner)
			if err != nil || owner == d.ID || seen[owner] || !slices.ContainsFunc(roster.Devices, func(x ChatDevice) bool { return x.ID == owner }) {
				return ErrChatConflict
			}
			seen[owner] = true
		}
		data, err := json.Marshal(roster.Devices)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO chat_encrypted_sessions(id,conversation_id,creator_device_id,roster,roster_hash,welcome)VALUES($1,$2,$3,$4,$5,$6)`, p.SessionID, c.ID, d.ID, data, roster.Hash, p.Welcome)
		if err != nil {
			return err
		}
		for _, id := range p.Packages {
			if _, err = tx.ExecContext(ctx, `UPDATE chat_device_packages SET used_session_id=$2 WHERE id=$1`, id, p.SessionID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE chat_conversations SET encrypted_session_id=$2 WHERE id=$1`, c.ID, p.SessionID)
		if err != nil {
			return err
		}
		session, err := loadEncryptedSession(ctx, tx, p.SessionID)
		out.Session = &session
		return err
	case "send":
		if out.Session == nil || p.SessionID != out.Session.ID || out.Session.RosterHash != roster.Hash {
			return ErrChatRosterChanged
		}
		var latest int
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(id),0) FROM chat_encrypted_events WHERE session_id=$1`, p.SessionID).Scan(&latest); err != nil {
			return err
		}
		if latest != p.After {
			return ErrChatEventOrder
		}
		if len(p.Ciphertext) < 32 || len(p.Ciphertext) > 65536 || (p.Kind != "message" && p.Kind != "update") {
			return ErrChatInvalid
		}
		if c.State != "accepted" {
			var count int
			if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM chat_encrypted_events WHERE conversation_id=$1 AND kind='message'`, c.ID).Scan(&count); err != nil {
				return err
			}
			if p.Kind != "message" || count > 0 || c.RequesterID != userID {
				return ErrChatClosed
			}
		}
		if p.Kind == "update" {
			if err = chatRate(ctx, tx, userID, "encrypted_update", 60, 60); err != nil {
				return err
			}
		} else {
			if err = chatRate(ctx, tx, userID, "send_minute", 60, 30); err != nil {
				return err
			}
			if err = chatRate(ctx, tx, userID, "send_day", 86400, 500); err != nil {
				return err
			}
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO chat_encrypted_events(conversation_id,session_id,sender_device_id,kind,ciphertext,nonce)VALUES($1,$2,$3,$4,$5,$6)RETURNING id`, c.ID, p.SessionID, d.ID, p.Kind, p.Ciphertext, r.Nonce).Scan(&out.ID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE chat_conversations SET updated_at=now(),last_message_id=$2 WHERE id=$1`, c.ID, out.ID)
		return err
	}
	return ErrChatInvalid
}
func loadEncryptedSession(ctx context.Context, q querier, id string) (EncryptedSession, error) {
	var s EncryptedSession
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT id,creator_device_id,roster,roster_hash,welcome FROM chat_encrypted_sessions WHERE id=$1`, id).Scan(&s.ID, &s.Creator, &raw, &s.RosterHash, &s.Welcome)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s.Roster)
	return s, err
}
func encryptedEvents(ctx context.Context, tx *sql.Tx, c chatRecord, d ChatDevice, p encryptedConversationInput, out *EncryptedResponse) error {
	rows, err := tx.QueryContext(ctx, `SELECT e.id,e.session_id,e.sender_device_id,e.kind,e.ciphertext,e.nonce,e.created_at FROM chat_encrypted_events e JOIN chat_encrypted_sessions s ON s.id=e.session_id
 WHERE e.conversation_id=$1 AND e.id>$2 AND s.roster @> jsonb_build_array(jsonb_build_object('id',$3::text)) ORDER BY e.id LIMIT 51`, c.ID, p.After, d.ID)
	if err != nil {
		return err
	}
	out.Events = []EncryptedEvent{}
	ids := map[string]bool{}
	for rows.Next() {
		var e EncryptedEvent
		if err := rows.Scan(&e.ID, &e.SessionID, &e.DeviceID, &e.Kind, &e.Ciphertext, &e.Nonce, &e.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		if len(out.Events) == 50 {
			out.More = true
			break
		}
		out.Events = append(out.Events, e)
		ids[e.SessionID] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for id := range ids {
		s, err := loadEncryptedSession(ctx, tx, id)
		if err != nil {
			return err
		}
		out.Sessions = append(out.Sessions, s)
	}
	return nil
}
func reportEncrypted(ctx context.Context, tx *sql.Tx, userID int, c chatRecord, d ChatDevice, p encryptedConversationInput, out *EncryptedResponse) error {
	if !validChatReportReason(p.Reason) || len(p.DisclosedText) == 0 || len(p.DisclosedText) > 16000 || !utf8.ValidString(p.DisclosedText) || utf8.RuneCountInString(p.DisclosedText) > 4000 {
		return ErrChatInvalid
	}
	var sender int
	err := tx.QueryRowContext(ctx, `SELECT cd.user_id FROM chat_encrypted_events e JOIN chat_devices cd ON cd.id=e.sender_device_id JOIN chat_encrypted_sessions s ON s.id=e.session_id
 WHERE e.id=$1 AND e.conversation_id=$2 AND e.kind='message' AND s.roster @> jsonb_build_array(jsonb_build_object('id',$3::text))`, p.EventID, c.ID, d.ID).Scan(&sender)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrChatNotFound
	}
	if err != nil {
		return err
	}
	if sender == userID || (c.Kind == "seller" && sender != c.CustomerID && userID != c.CustomerID) {
		return ErrChatInvalid
	}
	if err = chatRate(ctx, tx, userID, "report", 3600, 10); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO chat_encrypted_reports(event_id,reporter_user_id,reason,disclosed_text)VALUES($1,$2,$3,$4)ON CONFLICT DO NOTHING`, p.EventID, userID, p.Reason, p.DisclosedText)
	return err
}

// Stable context included in the MLS group ID and checked by every endpoint.
func EncryptedGroupID(conversationID int, s EncryptedSession) string {
	return fmt.Sprintf("sokomoko:%d:%s:%s", conversationID, s.ID, s.RosterHash)
}
