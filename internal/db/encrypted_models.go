package db

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrChatEncryptionRequired = errors.New("end-to-end encryption is required")
var ErrChatDeviceApproval = errors.New("this device requires approval from an existing device")
var ErrChatEventOrder = errors.New("encrypted events changed; refresh before retrying")
var ErrChatRosterChanged = errors.New("encrypted conversation membership changed")

// Payload is an exact JSON string. The device signs its hash; it contains only
// public keys, ciphertext, or explicit moderator disclosures, never MLS state.
type EncryptedRequest struct {
	Op        string `json:"op"`
	DeviceID  string `json:"device_id"`
	Nonce     string `json:"nonce"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}
type ChatDevice struct {
	ID        string `json:"id"`
	UserID    int    `json:"user_id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
	Approved  bool   `json:"approved"`
	Revoked   bool   `json:"revoked"`
}
type EncryptedRoster struct {
	Devices []ChatDevice `json:"devices"`
	Hash    string       `json:"hash"`
	Ready   bool         `json:"ready"`
}
type ChatPackage struct {
	ID       string `json:"id"`
	DeviceID string `json:"device_id"`
	Package  []byte `json:"package"`
}
type EncryptedSession struct {
	ID         string       `json:"id"`
	Creator    string       `json:"creator"`
	Roster     []ChatDevice `json:"roster"`
	RosterHash string       `json:"roster_hash"`
	Welcome    []byte       `json:"welcome"`
}
type EncryptedEvent struct {
	ID         int       `json:"id"`
	SessionID  string    `json:"session_id"`
	DeviceID   string    `json:"device_id"`
	Kind       string    `json:"kind"`
	Ciphertext []byte    `json:"ciphertext"`
	Nonce      string    `json:"nonce"`
	CreatedAt  time.Time `json:"created_at"`
}
type EncryptedResponse struct {
	PackagesAvailable int                `json:"packages_available"`
	Device            *ChatDevice        `json:"device,omitempty"`
	Devices           []ChatDevice       `json:"devices,omitempty"`
	ConversationID    int                `json:"conversation_id,omitempty"`
	Conversation      *ChatConversation  `json:"conversation,omitempty"`
	Roster            *EncryptedRoster   `json:"roster,omitempty"`
	Packages          []ChatPackage      `json:"packages,omitempty"`
	Session           *EncryptedSession  `json:"session,omitempty"`
	Sessions          []EncryptedSession `json:"sessions,omitempty"`
	Events            []EncryptedEvent   `json:"events,omitempty"`
	ID                int                `json:"id,omitempty"`
	More              bool               `json:"more,omitempty"`
}

func encryptedPayload[T any](raw string) (T, error) {
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return v, ErrChatInvalid
	}
	return v, nil
}
