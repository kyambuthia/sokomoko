package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kyambuthia/sokomoko/internal/auth"
	messagingSvc "github.com/kyambuthia/sokomoko/internal/service/messaging"
)

func messageActor(r *http.Request) *messagingSvc.Actor {
	u := auth.GetUserFromContext(r.Context())
	if u == nil {
		return nil
	}
	return &messagingSvc.Actor{ID: u.ID, Role: u.Role}
}
func messageCursor(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, messagingSvc.ErrChatInvalid
	}
	return n, nil
}
func messageAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, messagingSvc.ErrChatEventOrder):
		writeError(w, 409, "event_order_changed", "New encrypted events arrived. Refresh and retry your message.")
	case errors.Is(err, messagingSvc.ErrChatDeviceApproval):
		writeError(w, 409, "device_approval_required", "An approved device or contact key package is required")
	case errors.Is(err, messagingSvc.ErrChatRosterChanged):
		writeError(w, 409, "membership_changed", "Review the current devices before starting a new encrypted session")
	case errors.Is(err, messagingSvc.ErrChatEncryptionRequired):
		writeError(w, 409, "encryption_required", "Use the encrypted messaging client")
	case errors.Is(err, messagingSvc.ErrChatNotFound):
		writeError(w, 404, "not_found", "This conversation or contact is unavailable")
	case errors.Is(err, messagingSvc.ErrChatInvalid):
		writeError(w, 400, "invalid_message", "Check the contact and use a message of 1–4,000 characters with a valid submission nonce")
	case errors.Is(err, messagingSvc.ErrChatClosed):
		writeError(w, 409, "conversation_closed", "This conversation is waiting for acceptance or is closed")
	case errors.Is(err, messagingSvc.ErrChatBlocked):
		writeError(w, 403, "contact_unavailable", "Messaging is unavailable for this contact")
	case errors.Is(err, messagingSvc.ErrChatLimited):
		w.Header().Set("Retry-After", "60")
		writeError(w, 429, "rate_limited", "You've reached a messaging limit. Please try again later")
	case errors.Is(err, messagingSvc.ErrChatConflict):
		writeError(w, 409, "submission_conflict", "That submission nonce was already used for a different message")
	default:
		writeError(w, 500, "messaging_unavailable", "Messages are temporarily unavailable")
	}
}
func messageAPIMethod(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, 405, "method_not_allowed", "Method not allowed")
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		before, err := messageCursor(r.URL.Query().Get("before"))
		if err != nil {
			messageAPIError(w, err)
			return
		}
		result, err := h.app.Messaging.Inbox(r.Context(), messageActor(r), before)
		if err != nil {
			messageAPIError(w, err)
			return
		}
		writeData(w, 200, result)
	case http.MethodPost:
		var input struct {
			Target messagingSvc.ChatTarget `json:"target"`
			Body   string                  `json:"body"`
			Nonce  string                  `json:"nonce"`
		}
		if decodeJSON(r, &input) != nil {
			messageAPIError(w, messagingSvc.ErrChatInvalid)
			return
		}
		id, err := h.app.Messaging.Start(r.Context(), messageActor(r), input.Target, input.Body, input.Nonce)
		if err != nil {
			messageAPIError(w, err)
			return
		}
		writeData(w, 201, struct {
			ID int `json:"id"`
		}{id})
	default:
		messageAPIMethod(w, "GET, POST")
	}
}
func (h *Handler) messageContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		messageAPIMethod(w, "POST")
		return
	}
	var target messagingSvc.ChatTarget
	if decodeJSON(r, &target) != nil {
		messageAPIError(w, messagingSvc.ErrChatInvalid)
		return
	}
	contacts, err := h.app.Messaging.Contacts(r.Context(), messageActor(r), target)
	if err != nil {
		messageAPIError(w, err)
		return
	}
	writeData(w, 200, contacts)
}
func (h *Handler) messageThread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		messageAPIMethod(w, "GET")
		return
	}
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		messageAPIError(w, messagingSvc.ErrChatInvalid)
		return
	}
	before, err := messageCursor(r.URL.Query().Get("before"))
	if err != nil {
		messageAPIError(w, err)
		return
	}
	after, err := messageCursor(r.URL.Query().Get("after"))
	if err != nil {
		messageAPIError(w, err)
		return
	}
	thread, err := h.app.Messaging.Thread(r.Context(), messageActor(r), id, before, after)
	if err != nil {
		messageAPIError(w, err)
		return
	}
	writeData(w, 200, thread)
}
func (h *Handler) messageSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		messageAPIMethod(w, "POST")
		return
	}
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		messageAPIError(w, messagingSvc.ErrChatInvalid)
		return
	}
	var input struct {
		Body  string `json:"body"`
		Nonce string `json:"nonce"`
	}
	if decodeJSON(r, &input) != nil {
		messageAPIError(w, messagingSvc.ErrChatInvalid)
		return
	}
	messageID, err := h.app.Messaging.Send(r.Context(), messageActor(r), id, input.Body, input.Nonce)
	if err != nil {
		messageAPIError(w, err)
		return
	}
	nonce, err := messagingSvc.Nonce()
	if err != nil {
		messageAPIError(w, err)
		return
	}
	writeData(w, 201, struct {
		ID    int    `json:"id"`
		Nonce string `json:"nonce"`
	}{messageID, nonce})
}
func (h *Handler) messageAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		messageAPIMethod(w, "POST")
		return
	}
	id, ok := parsePositiveID(r.PathValue("id"))
	if !ok {
		messageAPIError(w, messagingSvc.ErrChatInvalid)
		return
	}
	var input struct {
		Action    string `json:"action"`
		MessageID int    `json:"message_id"`
		Reason    string `json:"reason"`
	}
	if decodeJSON(r, &input) != nil {
		messageAPIError(w, messagingSvc.ErrChatInvalid)
		return
	}
	if err := h.app.Messaging.Action(r.Context(), messageActor(r), id, input.Action, input.MessageID, input.Reason); err != nil {
		messageAPIError(w, err)
		return
	}
	writeData(w, 200, struct {
		OK bool `json:"ok"`
	}{true})
}

func (h *Handler) encryptedChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		messageAPIMethod(w, "POST")
		return
	}
	var input messagingSvc.EncryptedRequest
	if decodeJSON(r, &input) != nil {
		messageAPIError(w, messagingSvc.ErrChatInvalid)
		return
	}
	result, err := h.app.Messaging.Encrypted(r.Context(), messageActor(r), input)
	if err != nil {
		messageAPIError(w, err)
		return
	}
	writeData(w, 200, result)
}
