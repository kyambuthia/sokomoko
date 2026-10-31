package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const chatPageSize = 30

type chatRecord struct {
	ID, CustomerID, OtherID, PartnerID, ProductID, ServiceID, OrderID, DeliveryID, AssignmentID, RequesterID, LastMessageID int
	Kind, State, CustomerName, OtherName, PartnerName, ProductName, ServiceName                                             string
	UpdatedAt                                                                                                               time.Time
	Live, Unread                                                                                                            bool
	BlockedByCustomer, BlockedByOther                                                                                       bool
}

// No platform role gets implicit access to conversations. Seller membership
// and the courier assignment are checked on every read, including polling.
const chatAccessSQL = `
 ((actor.role = 'user' AND actor.id = c.customer_user_id)
 OR (c.kind = 'direct' AND actor.role = 'user' AND actor.id = c.other_user_id)
 OR (c.kind = 'seller' AND actor.role = 'partner' AND p.status = 'active'
     AND EXISTS (SELECT 1 FROM partner_users pu WHERE pu.user_id = actor.id AND pu.partner_id = c.partner_id))
 OR (c.kind = 'delivery' AND actor.role = 'courier' AND actor.id = c.other_user_id
     AND d.courier_user_id = actor.id AND c.assignment_event_id =
       (SELECT e.id FROM courier_delivery_events e WHERE e.delivery_id = d.id
        AND e.action IN ('assigned', 'reassigned') ORDER BY e.id DESC LIMIT 1)))
 AND (c.order_id IS NULL OR o.user_id = c.customer_user_id)`

const chatSelectSQL = `SELECT c.id, c.kind, c.customer_user_id, COALESCE(c.other_user_id, 0),
 COALESCE(c.partner_id, 0), COALESCE(c.product_id, 0), COALESCE(c.service_id, 0), COALESCE(c.order_id, 0),
 COALESCE(c.delivery_id, 0), COALESCE(c.assignment_event_id, 0), c.requester_user_id,
 c.state, c.updated_at, c.last_message_id, cu.username, COALESCE(ou.username, ''), COALESCE(p.name, ''), COALESCE(prod.name, ''), COALESCE(svc.title, ''),
 COALESCE(cu.deleted_at IS NULL AND cu.role = 'user' AND CASE c.kind
 WHEN 'direct' THEN ou.deleted_at IS NULL AND ou.role = 'user'
 WHEN 'seller' THEN p.status = 'active'
   AND EXISTS (SELECT 1 FROM partner_users pu JOIN users u ON u.id = pu.user_id
               WHERE pu.partner_id = p.id AND u.role = 'partner' AND u.deleted_at IS NULL)
   AND (c.service_id IS NULL OR (svc.state = 'published' AND svc.partner_id = c.partner_id))
   AND (c.product_id IS NULL OR (prod.deleted_at IS NULL AND prod.partner_id = c.partner_id))
   AND (c.order_id IS NULL OR EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = c.order_id AND oi.partner_id = c.partner_id))
 WHEN 'delivery' THEN ou.deleted_at IS NULL AND ou.role = 'courier'
   AND d.courier_user_id = c.other_user_id AND d.status <> 'delivered' AND o.status NOT IN ('cancelled','delivered') AND o.delivery_status <> 'delivered'
   AND c.assignment_event_id = (SELECT e.id FROM courier_delivery_events e WHERE e.delivery_id = d.id
                              AND e.action IN ('assigned', 'reassigned') ORDER BY e.id DESC LIMIT 1)
 END, false),
 (EXISTS (SELECT 1 FROM chat_encrypted_events e JOIN chat_devices cd ON cd.id=e.sender_device_id WHERE e.conversation_id=c.id AND cd.user_id<>$1 AND e.kind='message' AND e.id>COALESCE((SELECT er.last_event_id FROM chat_encrypted_reads er WHERE er.conversation_id=c.id AND er.user_id=$1),0)) OR EXISTS (SELECT 1 FROM chat_messages m WHERE m.conversation_id = c.id AND m.sender_user_id <> $1
   AND m.id > COALESCE((SELECT cr.last_read_message_id FROM chat_reads cr WHERE cr.conversation_id = c.id AND cr.user_id = $1), 0))),
 EXISTS (SELECT 1 FROM chat_blocks b WHERE b.owner_user_id=c.customer_user_id
   AND COALESCE(b.target_user_id,0)=COALESCE(c.other_user_id,0) AND COALESCE(b.target_partner_id,0)=COALESCE(c.partner_id,0)),
 EXISTS (SELECT 1 FROM chat_blocks b WHERE b.target_user_id=c.customer_user_id
   AND COALESCE(b.owner_user_id,0)=COALESCE(c.other_user_id,0) AND COALESCE(b.owner_partner_id,0)=COALESCE(c.partner_id,0))
 FROM chat_conversations c
 JOIN users actor ON actor.id = $1 AND actor.deleted_at IS NULL
 JOIN users cu ON cu.id = c.customer_user_id
 LEFT JOIN users ou ON ou.id = c.other_user_id
 LEFT JOIN partners p ON p.id = c.partner_id
 LEFT JOIN products prod ON prod.id = c.product_id
 LEFT JOIN service_listings svc ON svc.id = c.service_id
 LEFT JOIN orders o ON o.id = c.order_id
 LEFT JOIN courier_deliveries d ON d.id = c.delivery_id
 WHERE ` + chatAccessSQL

func scanChat(row interface{ Scan(...any) error }) (chatRecord, error) {
	var c chatRecord
	err := row.Scan(&c.ID, &c.Kind, &c.CustomerID, &c.OtherID, &c.PartnerID, &c.ProductID, &c.ServiceID,
		&c.OrderID, &c.DeliveryID, &c.AssignmentID, &c.RequesterID, &c.State, &c.UpdatedAt, &c.LastMessageID,
		&c.CustomerName, &c.OtherName, &c.PartnerName, &c.ProductName, &c.ServiceName, &c.Live, &c.Unread, &c.BlockedByCustomer, &c.BlockedByOther)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrChatNotFound
	}
	return c, err
}

func (c chatRecord) view(actorID int) ChatConversation {
	v := ChatConversation{ID: c.ID, CustomerUserID: c.CustomerID, LastMessageID: c.LastMessageID, Kind: c.Kind, State: c.State, UpdatedAt: c.UpdatedAt,
		Unread: c.Unread, CanSend: c.State == "accepted" && c.Live, Closed: !c.Live || c.State == "declined",
		CanBlock: true, Shared: c.Kind == "seller"}
	v.IncomingRequest = c.State == "pending" && ((actorID == c.CustomerID) != (c.RequesterID == c.CustomerID))
	v.Blocked = c.BlockedByCustomer || c.BlockedByOther
	v.BlockedByMe = c.BlockedByOther
	if actorID == c.CustomerID {
		v.BlockedByMe = c.BlockedByCustomer
	}
	v.CanAccept = v.IncomingRequest && c.Live && !v.Blocked
	v.CanSend = v.CanSend && !v.Blocked
	v.Name = "@" + c.CustomerName
	if actorID == c.CustomerID {
		v.Name = "@" + c.OtherName
		if c.Kind == "seller" {
			v.Name = c.PartnerName
		}
	}
	switch {
	case c.Kind == "delivery":
		v.Context = fmt.Sprintf("Delivery · order #%d", c.OrderID)
	case c.OrderID > 0:
		v.Context = fmt.Sprintf("Order #%d · %s", c.OrderID, c.PartnerName)
	case c.ServiceID > 0:
		v.Context = "Service · " + c.ServiceName
	case c.ProductID > 0:
		v.Context = c.ProductName
	case c.Kind == "seller":
		v.Context = "Shop conversation"
	default:
		v.Context = "Direct conversation"
	}
	return v
}

type chatParty struct{ user, partner int }

func (c chatRecord) parties(actorID int) (chatParty, chatParty) {
	a, b := chatParty{user: c.CustomerID}, chatParty{user: c.OtherID, partner: c.PartnerID}
	if actorID != c.CustomerID {
		return b, a
	}
	return a, b
}

func (c chatRecord) lockPair(ctx context.Context, tx *sql.Tx) error {
	a, b := c.parties(c.CustomerID)
	keys := []string{fmt.Sprintf("u%d:p%d", a.user, a.partner), fmt.Sprintf("u%d:p%d", b.user, b.partner)}
	slices.Sort(keys)
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "chat:"+strings.Join(keys, ":"))
	return err
}

func chatBlockState(ctx context.Context, q querier, c chatRecord, actorID int) (mine, theirs bool, err error) {
	a, b := c.parties(actorID)
	err = q.QueryRowContext(ctx, `SELECT
      EXISTS (SELECT 1 FROM chat_blocks WHERE COALESCE(owner_user_id,0)=$1 AND COALESCE(owner_partner_id,0)=$2
        AND COALESCE(target_user_id,0)=$3 AND COALESCE(target_partner_id,0)=$4),
      EXISTS (SELECT 1 FROM chat_blocks WHERE COALESCE(owner_user_id,0)=$3 AND COALESCE(owner_partner_id,0)=$4
        AND COALESCE(target_user_id,0)=$1 AND COALESCE(target_partner_id,0)=$2)`, a.user, a.partner, b.user, b.partner).Scan(&mine, &theirs)
	return
}

func chatView(ctx context.Context, q querier, c chatRecord, actorID int) (ChatConversation, error) {
	v := c.view(actorID)
	mine, theirs, err := chatBlockState(ctx, q, c, actorID)
	v.BlockedByMe, v.Blocked = mine, mine || theirs
	v.CanSend, v.CanAccept = v.CanSend && !v.Blocked, v.CanAccept && !v.Blocked
	return v, err
}

func lockChatActorTx(ctx context.Context, tx *sql.Tx, actorID int) error {
	var lockedActorID int
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR SHARE`, actorID).Scan(&lockedActorID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrChatNotFound
	}
	return err
}

// Hold assignment and actor rows while authorizing and mutating. Dispatch
// locks the order before the delivery; keep that order before locking the actor
// so assignment changes and account deactivation cannot race with message access.
func loadChatTx(ctx context.Context, tx *sql.Tx, actorID, id int, mutate bool) (chatRecord, error) {
	var deliveryID sql.NullInt64
	var orderID, serviceID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT delivery_id,order_id,service_id FROM chat_conversations WHERE id=$1`, id).Scan(&deliveryID, &orderID, &serviceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return chatRecord{}, ErrChatNotFound
		}
		return chatRecord{}, err
	}
	if deliveryID.Valid {
		// Dispatch locks the order before the assignment. Match that order,
		// including the FK locks required when a first message is inserted.
		if _, err := tx.ExecContext(ctx, `SELECT id FROM orders WHERE id=$1 FOR SHARE`, orderID.Int64); err != nil {
			return chatRecord{}, err
		}
		if _, err := tx.ExecContext(ctx, `SELECT id FROM courier_deliveries WHERE id=$1 FOR SHARE`, deliveryID.Int64); err != nil {
			return chatRecord{}, err
		}
	}
	if mutate {
		if err := lockChatActorTx(ctx, tx, actorID); err != nil {
			return chatRecord{}, err
		}
	}
	if serviceID.Valid && mutate {
		// SaveService locks the actor before the listing; use the same order
		// while pinning the service state against archiving.
		if _, err := tx.ExecContext(ctx, `SELECT id FROM service_listings WHERE id=$1 FOR SHARE`, serviceID.Int64); err != nil {
			return chatRecord{}, err
		}
	}
	c, err := scanChat(tx.QueryRowContext(ctx, chatSelectSQL+` AND c.id=$2`, actorID, id))
	if err != nil || !mutate {
		return c, err
	}
	if err := c.lockPair(ctx, tx); err != nil {
		return c, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT id FROM chat_conversations WHERE id=$1 FOR UPDATE`, id); err != nil {
		return c, err
	}
	return scanChat(tx.QueryRowContext(ctx, chatSelectSQL+` AND c.id=$2`, actorID, id))
}

func (s *Store) ChatInbox(ctx context.Context, actorID, before int) (ChatInbox, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.DB.QueryContext(ctx, chatSelectSQL+` AND ($2::bigint=0 OR c.last_message_id<$2) ORDER BY c.last_message_id DESC LIMIT $3`, actorID, before, chatPageSize+1)
	if err != nil {
		return ChatInbox{}, err
	}
	defer rows.Close()
	result := ChatInbox{Conversations: []ChatConversation{}}
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return result, err
		}
		if len(result.Conversations) == chatPageSize {
			result.NextBefore = result.Conversations[len(result.Conversations)-1].LastMessageID
			break
		}
		result.Conversations = append(result.Conversations, c.view(actorID))
	}
	return result, rows.Err()
}

func (s *Store) ChatThread(ctx context.Context, actorID, id, before, after int) (ChatThread, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var result ChatThread
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		result = ChatThread{Messages: []ChatMessage{}}
		c, err := loadChatTx(ctx, tx, actorID, id, false)
		if err != nil {
			return err
		}
		result.Conversation, err = chatView(ctx, tx, c, actorID)
		if err != nil {
			return err
		}
		direction := "DESC"
		if after > 0 {
			direction = "ASC"
		}
		rows, err := tx.QueryContext(ctx, `SELECT m.id, u.username, m.body, m.created_at, m.sender_user_id=$2,
          CASE WHEN $5::text='seller' THEN (m.sender_user_id=$6) <> ($2=$6) ELSE m.sender_user_id<>$2 END
          FROM chat_messages m JOIN users u ON u.id=m.sender_user_id
          WHERE m.conversation_id=$1 AND ($3::bigint=0 OR m.id<$3) AND ($4::bigint=0 OR m.id>$4)
          ORDER BY m.id `+direction+` LIMIT 51`, id, actorID, before, after, c.Kind, c.CustomerID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m ChatMessage
			if err := rows.Scan(&m.ID, &m.Sender, &m.Body, &m.CreatedAt, &m.Mine, &m.CanReport); err != nil {
				return err
			}
			if len(result.Messages) == 50 {
				result.More = true
				break
			}
			result.Messages = append(result.Messages, m)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if after == 0 {
			slices.Reverse(result.Messages)
		}
		if result.More && after == 0 {
			result.OlderBefore = result.Messages[0].ID
		}
		return nil
	})
	return result, err
}

// A shared, persistent budget for authenticated handle lookups (including
// unsuccessful ones). Mutations use separate budgets in their transactions.
func (s *Store) AllowChatLookup(ctx context.Context, actorID int) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error { return chatRate(ctx, tx, actorID, "lookup", 60, 20) })
}

func chatRate(ctx context.Context, tx *sql.Tx, actorID int, action string, seconds, limit int) error {
	var hits int
	err := tx.QueryRowContext(ctx, `INSERT INTO chat_rate_limits(user_id,action,bucket,hits,expires_at)
      SELECT id,$2,floor(extract(epoch FROM now())/$3)::bigint,1,now()+($3*2)*interval '1 second'
      FROM users WHERE id=$1 AND deleted_at IS NULL
      ON CONFLICT(user_id,action,bucket) DO UPDATE SET hits=chat_rate_limits.hits+1
        WHERE chat_rate_limits.hits<$4 RETURNING hits`, actorID, action, seconds, limit).Scan(&hits)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrChatLimited
	}
	if err != nil {
		return err
	}
	// Bound counter storage without requiring a separate cleanup worker.
	_, err = tx.ExecContext(ctx, `DELETE FROM chat_rate_limits WHERE user_id=$1 AND expires_at<now()`, actorID)
	return err
}
