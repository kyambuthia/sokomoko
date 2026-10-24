package db

import (
	"errors"
	"time"
)

var (
	ErrChatNotFound = errors.New("conversation or contact not found")
	ErrChatInvalid  = errors.New("invalid message request")
	ErrChatClosed   = errors.New("conversation is not open for messages")
	ErrChatBlocked  = errors.New("messaging is blocked for this contact")
	ErrChatLimited  = errors.New("messaging rate limit reached")
	ErrChatConflict = errors.New("message submission has already been used")
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
