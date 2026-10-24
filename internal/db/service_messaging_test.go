package db

import (
	"strings"
	"testing"
)

func TestEncryptedServiceEnquiriesRespectVisibilityAndOwnership(t *testing.T) {
	s := newTestStore(t)
	partner, owner, product := chatShop(t, s)
	_, outsider, _ := chatShop(t, s)
	buyer := mustCreateUser(t, s, RoleUser)
	device := newEncryptedTestDevice(t, s, buyer, 400)
	v, err := s.SaveService(t.Context(), owner, serviceFixture("service-enquiry"))
	chatMust(t, err)
	target := ChatTarget{ServiceID: v.ID}
	_, err = s.ChatContacts(t.Context(), buyer, target)
	chatWantError(t, err, ErrChatNotFound)
	_, err = device.call(t, s, "start", map[string]any{"target": target})
	chatWantError(t, err, ErrChatNotFound)
	v.State = "published"
	v, err = s.SaveService(t.Context(), owner, *v)
	chatMust(t, err)
	contacts, err := s.ChatContacts(t.Context(), buyer, target)
	chatMust(t, err)
	if len(contacts) != 1 || !contacts[0].Shared || contacts[0].Context != "Service · Demo haircut" {
		t.Fatalf("contacts=%+v", contacts)
	}
	id := encryptedStart(t, s, device, target)
	again := encryptedStart(t, s, device, target)
	if again != id {
		t.Fatal("duplicate service conversation")
	}
	otherService := serviceFixture("another-service-enquiry")
	otherService.State = "published"
	second, err := s.SaveService(t.Context(), owner, otherService)
	chatMust(t, err)
	secondID := encryptedStart(t, s, device, ChatTarget{ServiceID: second.ID})
	if secondID == id {
		t.Fatal("distinct service enquiries merged")
	}
	productID := encryptedStart(t, s, device, ChatTarget{ProductID: product})
	if id == productID {
		t.Fatal("service enquiry merged into product conversation")
	}
	thread, err := s.ChatThread(t.Context(), owner, id, 0, 0)
	chatMust(t, err)
	if !thread.Conversation.CanSend || !thread.Conversation.Shared || !strings.Contains(thread.Conversation.Context, v.Title) {
		t.Fatalf("thread=%+v", thread.Conversation)
	}
	_, err = s.ChatThread(t.Context(), outsider, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
	_, err = s.ChatContacts(t.Context(), owner, target)
	chatWantError(t, err, ErrChatNotFound)
	// The transport is still opaque; creating an enquiry stores no plaintext intro.
	var count int
	chatMust(t, s.DB.QueryRowContext(t.Context(), "SELECT count(*) FROM chat_messages WHERE conversation_id=$1", id).Scan(&count))
	if count != 0 {
		t.Fatal("encrypted enquiry created plaintext content")
	}
	sellerDevice := newEncryptedTestDevice(t, s, owner, 401)
	encryptedPublish(t, s, sellerDevice, 402)
	session := encryptedNewSession(t, s, device, id, 403)
	first, err := device.call(t, s, "send", encryptedSendInput(id, session, 0))
	chatMust(t, err)
	reply, err := sellerDevice.call(t, s, "send", encryptedSendInput(id, session, first.ID))
	chatMust(t, err)
	history, err := sellerDevice.call(t, s, "events", map[string]any{"conversation_id": id})
	chatMust(t, err)
	if len(history.Events) != 2 {
		t.Fatalf("encrypted history events=%d", len(history.Events))
	}
	chatMust(t, s.ChatAction(t.Context(), owner, id, "block", 0, ""))
	_, err = device.call(t, s, "start", map[string]any{"target": target})
	chatWantError(t, err, ErrChatBlocked)
	chatMust(t, s.ChatAction(t.Context(), owner, id, "unblock", 0, ""))
	v.State = "archived"
	_, err = s.SaveService(t.Context(), owner, *v)
	chatMust(t, err)
	thread, err = s.ChatThread(t.Context(), buyer, id, 0, 0)
	chatMust(t, err)
	if thread.Conversation.CanSend || !thread.Conversation.Closed {
		t.Fatal("archived service remained open")
	}
	_, err = device.call(t, s, "roster", map[string]any{"conversation_id": id})
	chatMust(t, err)
	_, err = sellerDevice.call(t, s, "send", encryptedSendInput(id, session, reply.ID))
	chatWantError(t, err, ErrChatClosed)
	history, err = sellerDevice.call(t, s, "events", map[string]any{"conversation_id": id})
	chatMust(t, err)
	if len(history.Events) != 2 {
		t.Fatal("archiving removed encrypted history")
	}
	_, err = device.call(t, s, "claim", map[string]any{"conversation_id": id, "roster_hash": "irrelevant-after-closure"})
	chatWantError(t, err, ErrChatClosed)
	_, err = s.ChatContacts(t.Context(), buyer, target)
	chatWantError(t, err, ErrChatNotFound)
	_, err = device.call(t, s, "start", map[string]any{"target": target})
	chatWantError(t, err, ErrChatNotFound)
	// Shop suspension continues to revoke seller access to its conversations.
	_, err = s.DB.ExecContext(t.Context(), "UPDATE partners SET status='suspended' WHERE id=$1", partner)
	chatMust(t, err)
	_, err = s.ChatThread(t.Context(), owner, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
}
