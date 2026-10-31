package db

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var encryptedID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func EncryptedSigningText(userID int, r EncryptedRequest) string {
	h := sha256.Sum256([]byte(r.Payload))
	return fmt.Sprintf("sokomoko-e2ee-v1\n%s\n%d\n%s\n%s\n%s", r.Op, userID, r.DeviceID, r.Nonce, base64.StdEncoding.EncodeToString(h[:]))
}
func encryptedVerify(userID int, r EncryptedRequest, key []byte) bool {
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	return err == nil && len(key) == ed25519.PublicKeySize && ed25519.Verify(key, []byte(EncryptedSigningText(userID, r)), sig)
}
func encryptedDevice(ctx context.Context, q querier, userID int, id string) (ChatDevice, error) {
	var d ChatDevice
	err := q.QueryRowContext(ctx, `SELECT d.id,d.user_id,u.username,d.name,d.public_key,d.approved_at IS NOT NULL,d.revoked_at IS NOT NULL
 FROM chat_devices d JOIN users u ON u.id=d.user_id AND u.deleted_at IS NULL
 WHERE d.id=$1 AND d.user_id=$2`, id, userID).Scan(&d.ID, &d.UserID, &d.Username, &d.Name, &d.PublicKey, &d.Approved, &d.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrChatNotFound
	}
	return d, err
}
func (s *Store) EncryptedChat(ctx context.Context, userID int, r EncryptedRequest) (EncryptedResponse, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var out EncryptedResponse
	if !encryptedID.MatchString(r.DeviceID) || !encryptedID.MatchString(r.Nonce) || len(r.Payload) > 900000 || !json.Valid([]byte(r.Payload)) {
		return out, ErrChatInvalid
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		out = EncryptedResponse{}
		// Device-directory changes exclude in-flight encrypted operations.
		// Reads share this lock; unrelated conversations remain concurrent.
		lockSQL := `SELECT pg_advisory_xact_lock_shared(491308772091)`
		if r.Op == "register" || r.Op == "approve" || r.Op == "revoke" {
			lockSQL = `SELECT pg_advisory_xact_lock(491308772091)`
		}
		if _, err := tx.ExecContext(ctx, lockSQL); err != nil {
			return err
		}

		var role string
		roleQuery := `SELECT role FROM users WHERE id=$1 AND deleted_at IS NULL`
		switch r.Op {
		case "start", "roster", "claim", "session", "events", "send", "report", "read":
			// Conversation operations lock the actor after any order and
			// delivery rows, preserving the courier dispatch lock order.
		default:
			// Device mutations do not need assignment locks, so pin the
			// account while authorizing them here.
			roleQuery += " FOR SHARE"
		}
		if err := tx.QueryRowContext(ctx, roleQuery, userID).Scan(&role); err != nil {
			return ErrChatNotFound
		}
		if role != RoleUser && role != RolePartner && role != RoleCourier {
			return ErrChatNotFound
		}
		if r.Op == "register" {
			return registerEncryptedDevice(ctx, tx, userID, r, &out)
		}
		d, err := encryptedDevice(ctx, tx, userID, r.DeviceID)
		if err != nil {
			return err
		}
		if d.Revoked || !encryptedVerify(userID, r, d.PublicKey) {
			return ErrChatNotFound
		}
		if r.Op == "devices" {
			out.Device = &d
			out.Devices, err = listEncryptedDevices(ctx, tx, userID)
			if err != nil {
				return err
			}
			err = tx.QueryRowContext(ctx, `SELECT count(*) FROM chat_device_packages WHERE device_id=$1 AND claimed_at IS NULL AND created_at > now()-interval '30 days'`, d.ID).Scan(&out.PackagesAvailable)
			return err
		}
		if !d.Approved {
			return ErrChatDeviceApproval
		}
		// Serializes approval/revocation and every encrypted request for this device.
		// Recheck after waiting for concurrent revocation.
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "chat-device-request:"+d.ID); err != nil {
			return err
		}
		d, err = encryptedDevice(ctx, tx, userID, r.DeviceID)
		if err != nil {
			return err
		}
		if d.Revoked {
			return ErrChatNotFound
		}
		// Cached replies must not bypass current membership or courier assignment.
		if r.Op == "roster" || r.Op == "claim" || r.Op == "session" || r.Op == "events" || r.Op == "send" || r.Op == "report" || r.Op == "read" {
			p, err := encryptedPayload[encryptedConversationInput](r.Payload)
			if err != nil {
				return err
			}
			if _, err = loadChatTx(ctx, tx, userID, p.ConversationID, true); err != nil {
				return err
			}
		}
		digest := sha256.Sum256([]byte(EncryptedSigningText(userID, r)))
		hash := hex.EncodeToString(digest[:])
		var savedHash string
		var saved []byte
		err = tx.QueryRowContext(ctx, `SELECT digest,response FROM chat_encrypted_requests WHERE device_id=$1 AND nonce=$2`, d.ID, r.Nonce).Scan(&savedHash, &saved)
		if err == nil {
			if savedHash != hash {
				return ErrChatConflict
			}
			return json.Unmarshal(saved, &out)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		switch r.Op {
		case "approve", "revoke":
			err = changeEncryptedDevice(ctx, tx, userID, d, r, &out)
		case "packages":
			err = publishEncryptedPackages(ctx, tx, d, r, &out)
		case "start":
			err = startEncryptedChat(ctx, tx, userID, r, &out)
		case "roster", "claim", "session", "events", "send", "report", "read":
			err = encryptedConversation(ctx, tx, userID, d, r, &out)
		default:
			err = ErrChatInvalid
		}
		if err != nil {
			return err
		}
		// Reads are deliberately not retained in the request cache; retries use a fresh nonce.
		if r.Op == "roster" || r.Op == "events" {
			return nil
		}
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO chat_encrypted_requests(device_id,nonce,digest,response)VALUES($1,$2,$3,$4)`, d.ID, r.Nonce, hash, data)
		return err
	})
	return out, err
}
func listEncryptedDevices(ctx context.Context, tx *sql.Tx, userID int) ([]ChatDevice, error) {
	rows, err := tx.QueryContext(ctx, `SELECT d.id,d.user_id,u.username,d.name,d.public_key,d.approved_at IS NOT NULL,d.revoked_at IS NOT NULL FROM chat_devices d JOIN users u ON u.id=d.user_id WHERE d.user_id=$1 ORDER BY d.created_at,d.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ChatDevice{}
	for rows.Next() {
		var d ChatDevice
		if err := rows.Scan(&d.ID, &d.UserID, &d.Username, &d.Name, &d.PublicKey, &d.Approved, &d.Revoked); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func registerEncryptedDevice(ctx context.Context, tx *sql.Tx, userID int, r EncryptedRequest, out *EncryptedResponse) error {
	p, err := encryptedPayload[struct {
		Name      string `json:"name"`
		PublicKey []byte `json:"public_key"`
	}](r.Payload)
	if err != nil || len(p.PublicKey) != 32 || len(p.Name) == 0 || len(p.Name) > 60 || !encryptedVerify(userID, r, p.PublicKey) {
		return ErrChatInvalid
	}
	// User-level advisory lock makes first-device trust atomic. Never bootstrap
	// again after losing/revoking every device: password reset is not key recovery.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("chat-device-user:%d", userID)); err != nil {
		return err
	}
	existing, err := encryptedDevice(ctx, tx, userID, r.DeviceID)
	if err == nil {
		if !encryptedVerify(userID, r, existing.PublicKey) {
			return ErrChatConflict
		}
		out.Device = &existing
		return nil
	}
	if !errors.Is(err, ErrChatNotFound) {
		return err
	}
	var total, pending int
	err = tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER(WHERE approved_at IS NULL AND revoked_at IS NULL) FROM chat_devices WHERE user_id=$1`, userID).Scan(&total, &pending)
	if err != nil {
		return err
	}
	if total >= 30 || pending >= 5 {
		return ErrChatLimited
	}
	if err = chatRate(ctx, tx, userID, "device_register", 3600, 5); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO chat_devices(id,user_id,name,public_key,approved_at) VALUES($1,$2,$3,$4,CASE WHEN $5::boolean THEN now() END)`, r.DeviceID, userID, p.Name, p.PublicKey, total == 0)
	if err != nil {
		return err
	}
	d, err := encryptedDevice(ctx, tx, userID, r.DeviceID)
	out.Device = &d
	return err
}
func changeEncryptedDevice(ctx context.Context, tx *sql.Tx, userID int, d ChatDevice, r EncryptedRequest, out *EncryptedResponse) error {
	p, err := encryptedPayload[struct {
		ID        string `json:"id"`
		PublicKey []byte `json:"public_key"`
	}](r.Payload)
	if err != nil || !encryptedID.MatchString(p.ID) {
		return ErrChatInvalid
	}
	target, err := encryptedDevice(ctx, tx, userID, p.ID)
	if err != nil {
		return err
	}
	if base64.StdEncoding.EncodeToString(p.PublicKey) != base64.StdEncoding.EncodeToString(target.PublicKey) {
		return ErrChatConflict
	}
	if r.Op == "approve" {
		if target.Revoked || target.Approved {
			return ErrChatConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE chat_devices SET approved_at=now(),approved_by=$2 WHERE id=$1`, target.ID, d.ID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE chat_devices SET revoked_at=COALESCE(revoked_at,now()) WHERE id=$1`, target.ID)
	}
	return err
}
func publishEncryptedPackages(ctx context.Context, tx *sql.Tx, d ChatDevice, r EncryptedRequest, out *EncryptedResponse) error {
	p, err := encryptedPayload[struct {
		Packages []ChatPackage `json:"packages"`
	}](r.Payload)
	if err != nil || len(p.Packages) < 1 || len(p.Packages) > 20 {
		return ErrChatInvalid
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM chat_device_packages WHERE device_id=$1 AND claimed_at IS NULL AND created_at > now()-interval '30 days'`, d.ID).Scan(&count); err != nil {
		return err
	}
	if count+len(p.Packages) > 30 {
		return ErrChatLimited
	}
	for _, kp := range p.Packages {
		if !encryptedID.MatchString(kp.ID) || len(kp.Package) < 64 || len(kp.Package) > 16384 {
			return ErrChatInvalid
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO chat_device_packages(id,device_id,package)VALUES($1,$2,$3)`, kp.ID, d.ID, kp.Package); err != nil {
			return err
		}
	}
	return nil
}
