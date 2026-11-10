package db

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func chatNonce(n int) string { return fmt.Sprintf("%032x", n) }
func chatHandle(t *testing.T, s *Store, id int) string {
	t.Helper()
	u, err := s.GetUserByID(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return u.Username
}
func chatMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func chatWantError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
}

func TestChatDirectRequestAccessAndRetries(t *testing.T) {
	s := newTestStore(t)
	a, b, outside, admin := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleAdmin)
	target := ChatTarget{Handle: strings.ToUpper(chatHandle(t, s, b))}
	id, err := s.StartChat(t.Context(), a, target, "hello", chatNonce(1))
	chatMust(t, err)
	duplicate, err := s.StartChat(t.Context(), a, target, "hello", chatNonce(1))
	chatMust(t, err)
	if id != duplicate {
		t.Fatal("duplicate conversation")
	}
	_, err = s.StartChat(t.Context(), a, target, "changed", chatNonce(1))
	chatWantError(t, err, ErrChatConflict)
	for _, actor := range []int{outside, admin} {
		_, err = s.ChatThread(t.Context(), actor, id, 0, 0)
		chatWantError(t, err, ErrChatNotFound)
		_, err = s.SendChatMessage(t.Context(), actor, id, "intrude", chatNonce(2))
		chatWantError(t, err, ErrChatNotFound)
		inbox, err := s.ChatInbox(t.Context(), actor, 0)
		chatMust(t, err)
		if len(inbox.Conversations) != 0 {
			t.Fatal("outsider inbox leak")
		}
	}
	for _, actor := range []int{a, b} {
		_, err = s.SendChatMessage(t.Context(), actor, id, "too early", chatNonce(2))
		chatWantError(t, err, ErrChatClosed)
	}
	chatWantError(t, s.ChatAction(t.Context(), a, id, "accept", 0, ""), ErrChatClosed)
	thread, err := s.ChatThread(t.Context(), b, id, 0, 0)
	chatMust(t, err)
	if !thread.Conversation.CanAccept || !thread.Conversation.Unread || len(thread.Messages) != 1 {
		t.Fatalf("request = %+v", thread)
	}
	chatMust(t, s.ChatAction(t.Context(), b, id, "accept", 0, ""))
	messageID, err := s.SendChatMessage(t.Context(), b, id, "accepted", chatNonce(3))
	chatMust(t, err)
	chatWantError(t, s.ChatAction(t.Context(), a, id, "read", messageID+999, ""), ErrChatNotFound)
	chatMust(t, s.ChatAction(t.Context(), a, id, "read", messageID, ""))
	thread, err = s.ChatThread(t.Context(), a, id, 0, 0)
	chatMust(t, err)
	if thread.Conversation.Unread || len(thread.Messages) != 2 || !thread.Conversation.CanSend {
		t.Fatalf("thread = %+v", thread)
	}
	chatMust(t, s.ChatAction(t.Context(), b, id, "block", 0, ""))
	inbox, err := s.ChatInbox(t.Context(), a, 0)
	chatMust(t, err)
	if len(inbox.Conversations) != 1 || inbox.Conversations[0].CanSend || !inbox.Conversations[0].Blocked {
		t.Fatal("blocked inbox allows sending")
	}

	_, err = s.SendChatMessage(t.Context(), a, id, "blocked", chatNonce(4))
	chatWantError(t, err, ErrChatBlocked)
	chatMust(t, s.ChatAction(t.Context(), a, id, "unblock", 0, "")) // cannot undo the recipient's block
	_, err = s.SendChatMessage(t.Context(), a, id, "still blocked", chatNonce(5))
	chatWantError(t, err, ErrChatBlocked)
	chatMust(t, s.ChatAction(t.Context(), b, id, "unblock", 0, ""))
	_, err = s.SendChatMessage(t.Context(), a, id, "unblocked", chatNonce(6))
	chatMust(t, err)
}

func chatShop(t *testing.T, s *Store) (partner, member, product int) {
	t.Helper()
	member = mustCreateUser(t, s, RolePartner)
	suffix := nextSuffix()
	id, err := s.CreatePartner(t.Context(), Partner{Name: "Shop " + suffix, Slug: "shop-" + suffix, Status: "active"})
	chatMust(t, err)
	partner = int(id)
	_, err = s.DB.Exec(`INSERT INTO partner_users(user_id,partner_id) VALUES($1,$2)`, member, partner)
	chatMust(t, err)
	product = mustCreateProduct(t, s, 1000, 10)
	_, err = s.DB.Exec(`UPDATE products SET partner_id=$1 WHERE id=$2`, partner, product)
	chatMust(t, err)
	return
}

func TestChatSellerTeamAndBlocksAcrossContexts(t *testing.T) {
	s := newTestStore(t)
	buyer := mustCreateUser(t, s, RoleUser)
	partner, member, product := chatShop(t, s)
	_, otherMember, _ := chatShop(t, s)
	teammate := mustCreateUser(t, s, RolePartner)
	_, err := s.DB.Exec(`INSERT INTO partner_users(user_id,partner_id) VALUES($1,$2)`, teammate, partner)
	chatMust(t, err)
	id, err := s.StartChat(t.Context(), buyer, ChatTarget{ProductID: product}, "product inquiry", chatNonce(1))
	chatMust(t, err)
	for _, actor := range []int{member, teammate} {
		v, err := s.ChatThread(t.Context(), actor, id, 0, 0)
		chatMust(t, err)
		if !v.Conversation.CanSend || !v.Conversation.Shared {
			t.Fatal("shop team cannot reply")
		}
	}
	_, err = s.ChatThread(t.Context(), otherMember, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
	reply, err := s.SendChatMessage(t.Context(), teammate, id, "shop reply", chatNonce(2))
	chatMust(t, err)
	chatWantError(t, s.ChatAction(t.Context(), member, id, "report", reply, "spam"), ErrChatInvalid)
	chatMust(t, s.ChatAction(t.Context(), buyer, id, "block", 0, ""))
	_, err = s.StartChat(t.Context(), member, ChatTarget{Handle: chatHandle(t, s, buyer)}, "new route", chatNonce(3))
	chatWantError(t, err, ErrChatBlocked)
	_, err = s.SendChatMessage(t.Context(), teammate, id, "another staff member", chatNonce(4))
	chatWantError(t, err, ErrChatBlocked)
	chatMust(t, s.ChatAction(t.Context(), buyer, id, "unblock", 0, ""))
	_, err = s.DB.Exec(`DELETE FROM partner_users WHERE user_id=$1`, teammate)
	chatMust(t, err)
	_, err = s.ChatThread(t.Context(), teammate, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
	_, err = s.DB.Exec(`UPDATE partners SET status='suspended' WHERE id=$1`, partner)
	chatMust(t, err)
	_, err = s.ChatThread(t.Context(), member, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
	_, err = s.SendChatMessage(t.Context(), buyer, id, "suspended shop", chatNonce(5))
	chatWantError(t, err, ErrChatClosed)
}

func chatOrder(t *testing.T, s *Store, buyer int, products ...int) int {
	t.Helper()
	for _, p := range products {
		chatMust(t, s.AddToCart(t.Context(), buyer, p, 1))
	}
	id, err := s.PlaceOrderFromCart(t.Context(), buyer, "Sample address")
	chatMust(t, err)
	return int(id)
}

func TestChatMixedOrderContactsStayWithinShop(t *testing.T) {
	s := newTestStore(t)
	buyer, outside := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser)
	p1, m1, product1 := chatShop(t, s)
	p2, m2, product2 := chatShop(t, s)
	order := chatOrder(t, s, buyer, product1, product2)
	contacts, err := s.ChatContacts(t.Context(), buyer, ChatTarget{OrderID: order})
	chatMust(t, err)
	if len(contacts) != 2 {
		t.Fatalf("contacts=%d", len(contacts))
	}
	contacts, err = s.ChatContacts(t.Context(), m1, ChatTarget{OrderID: order})
	chatMust(t, err)
	if len(contacts) != 1 || contacts[0].Target.PartnerID != p1 {
		t.Fatal("other seller leaked into order contacts")
	}
	_, err = s.ChatContacts(t.Context(), outside, ChatTarget{OrderID: order})
	chatWantError(t, err, ErrChatNotFound)
	_, err = s.StartChat(t.Context(), m1, ChatTarget{OrderID: order, PartnerID: p2}, "wrong shop", chatNonce(1))
	chatWantError(t, err, ErrChatNotFound)
	id, err := s.StartChat(t.Context(), m1, ChatTarget{OrderID: order, PartnerID: p1}, "order question", chatNonce(2))
	chatMust(t, err)
	_, err = s.ChatThread(t.Context(), m2, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
	v, err := s.ChatThread(t.Context(), buyer, id, 0, 0)
	chatMust(t, err)
	if !v.Conversation.CanSend {
		t.Fatal("buyer cannot reply")
	}
}

func TestChatCourierReassignmentRevokesHistoryAndCreatesNewThread(t *testing.T) {
	s := newTestStore(t)
	buyer, c1, c2, admin := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleCourier), mustCreateUser(t, s, RoleCourier), mustCreateUser(t, s, RoleAdmin)
	_, _, product := chatShop(t, s)
	order := chatOrder(t, s, buyer, product)
	_, err := s.DB.Exec(`UPDATE orders SET status='shipped',delivery_status='shipped',partner_status='dispatched',inventory_state='shipped' WHERE id=$1`, order)
	chatMust(t, err)
	chatMust(t, s.AssignCourierToOrder(t.Context(), order, c1, admin))
	id, err := s.StartChat(t.Context(), buyer, ChatTarget{DeliveryOrderID: order}, "delivery instructions", chatNonce(1))
	chatMust(t, err)
	_, err = s.ChatThread(t.Context(), c1, id, 0, 0)
	chatMust(t, err)
	_, err = s.StartChat(t.Context(), buyer, ChatTarget{Handle: chatHandle(t, s, c1)}, "bypass assignment", chatNonce(2))
	chatWantError(t, err, ErrChatNotFound)
	chatMust(t, s.AssignCourierToOrder(t.Context(), order, c2, admin))
	for _, courier := range []int{c1, c2} {
		_, err = s.ChatThread(t.Context(), courier, id, 0, 0)
		chatWantError(t, err, ErrChatNotFound)
		_, err = s.SendChatMessage(t.Context(), courier, id, "old thread", chatNonce(3))
		chatWantError(t, err, ErrChatNotFound)
	}
	v, err := s.ChatThread(t.Context(), buyer, id, 0, 0)
	chatMust(t, err)
	if !v.Conversation.Closed {
		t.Fatal("old assignment still writable")
	}
	_, err = s.SendChatMessage(t.Context(), buyer, id, "old assignment", chatNonce(4))
	chatWantError(t, err, ErrChatClosed)
	newID, err := s.StartChat(t.Context(), c2, ChatTarget{DeliveryOrderID: order}, "new courier", chatNonce(5))
	chatMust(t, err)
	if newID == id {
		t.Fatal("reassigned courier inherited old thread")
	}
	chatMust(t, s.AssignCourierToOrder(t.Context(), order, c1, admin))
	_, err = s.ChatThread(t.Context(), c1, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
	thirdID, err := s.StartChat(t.Context(), c1, ChatTarget{DeliveryOrderID: order}, "new assignment", chatNonce(6))
	chatMust(t, err)
	if thirdID == id || thirdID == newID {
		t.Fatal("assignment epochs were reused")
	}
	_, err = s.DB.Exec(`UPDATE orders SET status='cancelled' WHERE id=$1`, order)
	chatMust(t, err)
	_, err = s.SendChatMessage(t.Context(), c1, thirdID, "cancelled", chatNonce(7))
	chatWantError(t, err, ErrChatClosed)
}

func TestChatReportsOnlyExposeSelectedMessage(t *testing.T) {
	s := newTestStore(t)
	a, b, admin := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleAdmin)
	id, err := s.StartChat(t.Context(), a, ChatTarget{Handle: chatHandle(t, s, b)}, "reported text", chatNonce(1))
	chatMust(t, err)
	thread, err := s.ChatThread(t.Context(), b, id, 0, 0)
	chatMust(t, err)
	message := thread.Messages[0].ID
	chatWantError(t, s.ChatAction(t.Context(), a, id, "report", message, "scam"), ErrChatInvalid)
	chatWantError(t, s.ChatAction(t.Context(), b, id, "report", message, "invalid"), ErrChatInvalid)
	chatMust(t, s.ChatAction(t.Context(), b, id, "report", message, "scam"))
	chatMust(t, s.ChatAction(t.Context(), b, id, "report", message, "scam"))
	reports, err := s.ChatReports(t.Context(), admin)
	chatMust(t, err)
	if len(reports) != 1 || reports[0].Body != "reported text" {
		t.Fatalf("reports=%+v", reports)
	}
	_, err = s.ChatThread(t.Context(), admin, id, 0, 0)
	chatWantError(t, err, ErrChatNotFound)
	chatWantError(t, s.ResolveChatReport(t.Context(), b, reports[0].ID), ErrChatNotFound)
	chatMust(t, s.ResolveChatReport(t.Context(), admin, reports[0].ID))
	reports, err = s.ChatReports(t.Context(), admin)
	chatMust(t, err)
	if len(reports) != 0 {
		t.Fatal("resolved report still open")
	}
}

func TestChatConcurrentFirstMessageIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	a, b := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser)
	target := ChatTarget{Handle: chatHandle(t, s, b)}
	var wg sync.WaitGroup
	ids := make([]int, 4)
	errs := make([]error, 4)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = s.StartChat(t.Context(), a, target, "one introduction", chatNonce(1))
		}()
	}
	wg.Wait()
	for i := range ids {
		chatMust(t, errs[i])
		if ids[i] != ids[0] {
			t.Fatal("duplicate conversation")
		}
	}
	thread, err := s.ChatThread(t.Context(), b, ids[0], 0, 0)
	chatMust(t, err)
	if len(thread.Messages) != 1 {
		t.Fatal("duplicate message")
	}
}

func TestChatPaginationAndLimits(t *testing.T) {
	s := newTestStore(t)
	a, b := mustCreateUser(t, s, RoleUser), mustCreateUser(t, s, RoleUser)
	id, err := s.StartChat(t.Context(), a, ChatTarget{Handle: chatHandle(t, s, b)}, "first", chatNonce(1))
	chatMust(t, err)
	chatMust(t, s.ChatAction(t.Context(), b, id, "accept", 0, ""))
	for i := 2; i <= 55; i++ {
		_, err = s.DB.Exec(`INSERT INTO chat_messages(conversation_id,sender_user_id,body,client_nonce)VALUES($1,$2,$3,$4)`, id, a, fmt.Sprint(i), chatNonce(i))
		chatMust(t, err)
	}
	thread, err := s.ChatThread(t.Context(), b, id, 0, 0)
	chatMust(t, err)
	if len(thread.Messages) != 50 || thread.OlderBefore == 0 || thread.Messages[0].Body != "6" {
		t.Fatal("latest page incorrect")
	}
	older, err := s.ChatThread(t.Context(), b, id, thread.OlderBefore, 0)
	chatMust(t, err)
	if len(older.Messages) != 5 || older.More {
		t.Fatal("older page incorrect")
	}
	later, err := s.ChatThread(t.Context(), b, id, 0, older.Messages[0].ID)
	chatMust(t, err)
	if len(later.Messages) != 50 || !later.More || later.Messages[0].Body != "2" {
		t.Fatal("incremental page lost ordering")
	}
	for range 20 {
		chatMust(t, s.AllowChatLookup(t.Context(), a))
	}
	chatWantError(t, s.AllowChatLookup(t.Context(), a), ErrChatLimited)
}

func TestChatShopProfileTarget(t *testing.T) {
	s := newTestStore(t)
	buyer := mustCreateUser(t, s, RoleUser)
	partner, member, product := chatShop(t, s)
	contacts, err := s.ChatContacts(t.Context(), buyer, ChatTarget{PartnerID: partner})
	chatMust(t, err)
	if len(contacts) != 1 || contacts[0].Name == "" || contacts[0].Context != "Shop conversation" || contacts[0].Target.PartnerID != partner {
		t.Fatalf("contacts=%+v", contacts)
	}
	id, err := s.StartChat(t.Context(), buyer, ChatTarget{PartnerID: partner}, "hello shop", chatNonce(1))
	chatMust(t, err)
	productThread, err := s.StartChat(t.Context(), buyer, ChatTarget{ProductID: product}, "about this", chatNonce(2))
	chatMust(t, err)
	if id == productThread {
		t.Fatal("shop thread collapsed into the product thread")
	}
	view, err := s.ChatThread(t.Context(), member, id, 0, 0)
	chatMust(t, err)
	if view.Conversation.Context != "Shop conversation" || !view.Conversation.CanSend || !view.Conversation.Shared {
		t.Fatalf("shop view=%+v", view.Conversation)
	}
	_, err = s.ChatContacts(t.Context(), member, ChatTarget{PartnerID: partner})
	chatWantError(t, err, ErrChatNotFound)
	_, err = s.StartChat(t.Context(), buyer, ChatTarget{PartnerID: partner + 99999}, "missing", chatNonce(3))
	chatWantError(t, err, ErrChatNotFound)
}
