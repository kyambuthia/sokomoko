// Package messaging validates public messaging inputs. The store rechecks
// account, shop and delivery relationships inside each database operation.
package messaging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kyambuthia/sokomoko/internal/db"
)

type Actor struct {
	ID   int
	Role string
}

type store interface {
	EncryptedChat(context.Context, int, db.EncryptedRequest) (db.EncryptedResponse, error)
	AllowChatLookup(context.Context, int) error
	ChatContacts(context.Context, int, db.ChatTarget) ([]db.ChatContact, error)
	ChatInbox(context.Context, int, int) (db.ChatInbox, error)
	ChatThread(context.Context, int, int, int, int) (db.ChatThread, error)
	StartChat(context.Context, int, db.ChatTarget, string, string) (int, error)
	SendChatMessage(context.Context, int, int, string, string) (int, error)
	ChatAction(context.Context, int, int, string, int, string) error
	ChatReports(context.Context, int) ([]db.ChatReport, error)
	ResolveChatReport(context.Context, int, int) error
}

var _ store = (*db.Store)(nil)

type Service struct{ store store }

func New(s store) *Service { return &Service{store: s} }

var handlePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$`)
var noncePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func Nonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func validActor(a *Actor) bool {
	return a != nil && a.ID > 0 && (a.Role == db.RoleUser || a.Role == db.RolePartner || a.Role == db.RoleCourier)
}

func normalizeTarget(t db.ChatTarget) (db.ChatTarget, error) {
	t.Handle = strings.TrimPrefix(strings.TrimSpace(t.Handle), "@")
	if t.ProductID < 0 || t.OrderID < 0 || t.PartnerID < 0 || t.DeliveryOrderID < 0 {
		return t, db.ErrChatInvalid
	}
	count := 0
	if t.Handle != "" {
		count++
		if !handlePattern.MatchString(t.Handle) {
			return t, db.ErrChatInvalid
		}
	}
	if t.ProductID > 0 {
		count++
	}
	if t.OrderID > 0 {
		count++
	}
	if t.DeliveryOrderID > 0 {
		count++
	}
	if count != 1 || (t.PartnerID > 0 && t.OrderID == 0) {
		return t, db.ErrChatInvalid
	}
	return t, nil
}

func normalizeMessage(body, nonce string) (string, error) {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	if !utf8.ValidString(body) || body == "" || len(body) > 16000 || utf8.RuneCountInString(body) > 4000 || !noncePattern.MatchString(nonce) {
		return "", db.ErrChatInvalid
	}
	for _, r := range body {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", db.ErrChatInvalid
		}
	}
	return body, nil
}

func (s *Service) Contacts(ctx context.Context, a *Actor, t db.ChatTarget) ([]db.ChatContact, error) {
	if !validActor(a) {
		return nil, db.ErrChatNotFound
	}
	if err := s.store.AllowChatLookup(ctx, a.ID); err != nil {
		return nil, err
	}
	t, err := normalizeTarget(t)
	if err != nil {
		return nil, err
	}
	return s.store.ChatContacts(ctx, a.ID, t)
}
func (s *Service) Inbox(ctx context.Context, a *Actor, before int) (db.ChatInbox, error) {
	if !validActor(a) {
		return db.ChatInbox{}, db.ErrChatNotFound
	}
	if before < 0 {
		return db.ChatInbox{}, db.ErrChatInvalid
	}
	return s.store.ChatInbox(ctx, a.ID, before)
}
func (s *Service) Thread(ctx context.Context, a *Actor, id, before, after int) (db.ChatThread, error) {
	if !validActor(a) {
		return db.ChatThread{}, db.ErrChatNotFound
	}
	if id <= 0 || before < 0 || after < 0 || (before > 0 && after > 0) {
		return db.ChatThread{}, db.ErrChatInvalid
	}
	return s.store.ChatThread(ctx, a.ID, id, before, after)
}
func (s *Service) Start(ctx context.Context, a *Actor, t db.ChatTarget, body, nonce string) (int, error) {
	if !validActor(a) {
		return 0, db.ErrChatNotFound
	}
	if err := s.store.AllowChatLookup(ctx, a.ID); err != nil {
		return 0, err
	}
	if _, err := normalizeTarget(t); err != nil {
		return 0, err
	}
	if _, err := normalizeMessage(body, nonce); err != nil {
		return 0, err
	}
	return 0, db.ErrChatEncryptionRequired
}
func (s *Service) Send(ctx context.Context, a *Actor, id int, body, nonce string) (int, error) {
	if !validActor(a) {
		return 0, db.ErrChatNotFound
	}
	if id <= 0 {
		return 0, db.ErrChatInvalid
	}
	if _, err := normalizeMessage(body, nonce); err != nil {
		return 0, err
	}
	return 0, db.ErrChatEncryptionRequired
}
func (s *Service) Action(ctx context.Context, a *Actor, id int, action string, messageID int, reason string) error {
	if !validActor(a) {
		return db.ErrChatNotFound
	}
	if id <= 0 || messageID < 0 {
		return db.ErrChatInvalid
	}
	return s.store.ChatAction(ctx, a.ID, id, action, messageID, reason)
}
func (s *Service) Reports(ctx context.Context, a *Actor) ([]db.ChatReport, error) {
	if a == nil || a.ID <= 0 || a.Role != db.RoleAdmin {
		return nil, db.ErrChatNotFound
	}
	return s.store.ChatReports(ctx, a.ID)
}
func (s *Service) ResolveReport(ctx context.Context, a *Actor, id int) error {
	if a == nil || a.ID <= 0 || a.Role != db.RoleAdmin {
		return db.ErrChatNotFound
	}
	if id == 0 {
		return db.ErrChatInvalid
	}
	return s.store.ResolveChatReport(ctx, a.ID, id)
}

// Encrypted requests carry only device-signed public metadata and ciphertext.
func (s *Service) Encrypted(ctx context.Context, a *Actor, r db.EncryptedRequest) (db.EncryptedResponse, error) {
	if !validActor(a) {
		return db.EncryptedResponse{}, db.ErrChatNotFound
	}
	if r.Op == "start" {
		if err := s.store.AllowChatLookup(ctx, a.ID); err != nil {
			return db.EncryptedResponse{}, err
		}
		var input struct {
			Target db.ChatTarget `json:"target"`
		}
		if json.Unmarshal([]byte(r.Payload), &input) != nil {
			return db.EncryptedResponse{}, db.ErrChatInvalid
		}
		if _, err := normalizeTarget(input.Target); err != nil {
			return db.EncryptedResponse{}, err
		}
	}
	return s.store.EncryptedChat(ctx, a.ID, r)
}
