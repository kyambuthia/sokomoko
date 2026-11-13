package domain

import (
	"errors"
	"time"
)

var (
	ErrChatNotFound           = errors.New("conversation or contact not found")
	ErrChatInvalid            = errors.New("invalid message request")
	ErrChatClosed             = errors.New("conversation is not open for messages")
	ErrChatBlocked            = errors.New("messaging is blocked for this contact")
	ErrChatLimited            = errors.New("messaging rate limit reached")
	ErrChatConflict           = errors.New("message submission has already been used")
	ErrChatEncryptionRequired = errors.New("end-to-end encryption is required")
	ErrChatDeviceApproval     = errors.New("this device requires approval from an existing device")
	ErrChatEventOrder         = errors.New("encrypted events changed; refresh before retrying")
	ErrChatRosterChanged      = errors.New("encrypted conversation membership changed")
)

// ChatTarget is an untrusted request. Exactly one of Handle, ProductID, ServiceID,
// OrderID or DeliveryOrderID must be supplied; PartnerID qualifies OrderID.
type ChatTarget struct {
	Handle          string `json:"handle,omitempty"`
	ProductID       int    `json:"product_id,omitempty"`
	ServiceID       int    `json:"service_id,omitempty"`
	OrderID         int    `json:"order_id,omitempty"`
	PartnerID       int    `json:"partner_id,omitempty"`
	DeliveryOrderID int    `json:"delivery_order_id,omitempty"`
}

type ChatContact struct {
	ConversationID int        `json:"conversation_id,omitempty"`
	Target         ChatTarget `json:"target"`
	Name           string     `json:"name"`
	Context        string     `json:"context"`
	Shared         bool       `json:"shared"`
	Request        bool       `json:"request"`
}

// ChatConversation is a presentation model: it contains no email, delivery
// address, financial information, credentials or other participants' orders.
type ChatConversation struct {
	CustomerUserID  int       `json:"customer_user_id"`
	ID              int       `json:"id"`
	LastMessageID   int       `json:"last_message_id"`
	Kind            string    `json:"kind"`
	Name            string    `json:"name"`
	Context         string    `json:"context"`
	State           string    `json:"state"`
	UpdatedAt       time.Time `json:"updated_at"`
	Unread          bool      `json:"unread"`
	IncomingRequest bool      `json:"incoming_request"`
	CanSend         bool      `json:"can_send"`
	CanAccept       bool      `json:"can_accept"`
	CanBlock        bool      `json:"can_block"`
	BlockedByMe     bool      `json:"blocked_by_me"`
	Blocked         bool      `json:"blocked"`
	Closed          bool      `json:"closed"`
	Shared          bool      `json:"shared"`
}

type ChatMessage struct {
	ID        int       `json:"id"`
	Sender    string    `json:"sender"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	Mine      bool      `json:"mine"`
	CanReport bool      `json:"can_report"`
}

type ChatThread struct {
	Conversation ChatConversation `json:"conversation"`
	Messages     []ChatMessage    `json:"messages"`
	OlderBefore  int              `json:"older_before,omitempty"`
	More         bool             `json:"more"`
}

type ChatInbox struct {
	Conversations []ChatConversation `json:"conversations"`
	NextBefore    int                `json:"next_before,omitempty"`
}

type ChatReport struct {
	Encrypted bool
	ID        int
	Sender    string
	Reporter  string
	Body      string
	Reason    string
	CreatedAt time.Time
}

// EncryptedRequest payload is an exact JSON string. The device signs its hash;
// it contains only public keys, ciphertext, or explicit moderator disclosures, never MLS state.
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
