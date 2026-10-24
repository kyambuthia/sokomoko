package db

import (
	"github.com/kyambuthia/sokomoko/internal/db/dbtest"
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

// Exercise the upgrade with pre-existing conversations, rather than relying
// solely on fresh databases where the new unique index starts empty.
func TestServiceConversationMigrationPreservesExistingThreads(t *testing.T) {
	s, err := OpenStore(dbtest.DSN(t))
	chatMust(t, err)
	t.Cleanup(func() { _ = s.Close() })
	migrations, err := loadMigrations()
	chatMust(t, err)
	for _, m := range migrations {
		if m.Version <= 10 {
			_, err = s.DB.ExecContext(t.Context(), m.SQL)
			chatMust(t, err)
		}
	}
	partner, owner, product := chatShop(t, s)
	buyer := mustCreateUser(t, s, RoleUser)
	var generalID, productID int
	chatMust(t, s.DB.QueryRowContext(t.Context(), `INSERT INTO chat_conversations(kind,customer_user_id,partner_id,requester_user_id,state) VALUES('seller',$1,$2,$1,'pending') RETURNING id`, buyer, partner).Scan(&generalID))
	chatMust(t, s.DB.QueryRowContext(t.Context(), `INSERT INTO chat_conversations(kind,customer_user_id,partner_id,product_id,requester_user_id,state) VALUES('seller',$1,$2,$3,$1,'accepted') RETURNING id`, buyer, partner, product).Scan(&productID))
	for _, m := range migrations {
		if m.Version == 11 {
			_, err = s.DB.ExecContext(t.Context(), m.SQL)
			chatMust(t, err)
		}
	}
	general, err := s.ChatContacts(t.Context(), buyer, ChatTarget{Handle: chatHandle(t, s, owner)})
	chatMust(t, err)
	products, err := s.ChatContacts(t.Context(), buyer, ChatTarget{ProductID: product})
	chatMust(t, err)
	if len(general) != 1 || general[0].ConversationID != generalID || len(products) != 1 || products[0].ConversationID != productID {
		t.Fatal("migration changed existing conversation identity")
	}
	listing := serviceFixture("upgrade-service")
	listing.State = "published"
	v, err := s.SaveService(t.Context(), owner, listing)
	chatMust(t, err)
	device := newEncryptedTestDevice(t, s, buyer, 410)
	serviceID := encryptedStart(t, s, device, ChatTarget{ServiceID: v.ID})
	if serviceID == generalID || serviceID == productID {
		t.Fatal("new context reused legacy thread")
	}
	general, err = s.ChatContacts(t.Context(), buyer, ChatTarget{Handle: chatHandle(t, s, owner)})
	chatMust(t, err)
	if len(general) != 1 || general[0].ConversationID != generalID {
		t.Fatal("new service context shadowed general conversation")
	}
}
