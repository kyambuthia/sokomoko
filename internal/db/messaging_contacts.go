package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) ChatContacts(ctx context.Context, actorID int, target ChatTarget) ([]ChatContact, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	contacts := []ChatContact{}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		contacts = nil
		if target.OrderID > 0 && target.PartnerID == 0 {
			var err error
			contacts, err = orderChatContacts(ctx, tx, actorID, target.OrderID)
			return err
		}
		c, err := resolveChatTarget(ctx, tx, actorID, target)
		if err != nil {
			return err
		}
		mine, theirs, err := chatBlockState(ctx, tx, c, actorID)
		if err != nil {
			return err
		}
		if mine || theirs {
			return ErrChatBlocked
		}
		id, err := findChat(ctx, tx, c)
		if err != nil {
			return err
		}
		v := c.view(actorID)
		contacts = []ChatContact{{ConversationID: id, Target: target, Name: v.Name, Context: v.Context, Shared: v.Shared, Request: c.State == "pending"}}
		return nil
	})
	return contacts, err
}

func orderChatContacts(ctx context.Context, tx *sql.Tx, actorID, orderID int) ([]ChatContact, error) {
	var customerID int
	var customerName, role string
	err := tx.QueryRowContext(ctx, `SELECT o.user_id, cu.username, actor.role FROM orders o
      JOIN users cu ON cu.id=o.user_id
      JOIN users actor ON actor.id=$1 AND actor.deleted_at IS NULL
      WHERE o.id=$2 AND ((actor.role='user' AND o.user_id=actor.id)
       OR (actor.role='partner' AND EXISTS (SELECT 1 FROM partner_users pu JOIN order_items oi ON oi.partner_id=pu.partner_id
           WHERE pu.user_id=actor.id AND oi.order_id=o.id)))`, actorID, orderID).Scan(&customerID, &customerName, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrChatNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT p.id, p.name FROM order_items oi
      JOIN partners p ON p.id=oi.partner_id AND p.status='active'
      WHERE oi.order_id=$1 AND EXISTS (SELECT 1 FROM partner_users pu JOIN users u ON u.id=pu.user_id
        WHERE pu.partner_id=p.id AND u.deleted_at IS NULL AND u.role='partner')
      AND ($3='user' OR EXISTS (SELECT 1 FROM partner_users pu WHERE pu.partner_id=p.id AND pu.user_id=$2))
      ORDER BY p.name,p.id`, orderID, actorID, role)
	if err != nil {
		return nil, err
	}
	contacts := []ChatContact{}
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		if role == RolePartner {
			name = "@" + customerName
		}
		contacts = append(contacts, ChatContact{Target: ChatTarget{OrderID: orderID, PartnerID: id}, Name: name,
			Context: fmt.Sprintf("Seller · order #%d", orderID), Shared: true})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if role == RoleUser {
		var courier string
		err := tx.QueryRowContext(ctx, `SELECT u.username FROM courier_deliveries d JOIN users u ON u.id=d.courier_user_id
          JOIN orders o ON o.id=d.order_id
          WHERE d.order_id=$1 AND d.status<>'delivered' AND o.status<>'cancelled' AND u.deleted_at IS NULL AND u.role='courier'`, orderID).Scan(&courier)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			contacts = append(contacts, ChatContact{Target: ChatTarget{DeliveryOrderID: orderID}, Name: "@" + courier, Context: "Assigned courier"})
		}
	}
	return contacts, nil
}

func resolveChatTarget(ctx context.Context, tx *sql.Tx, actorID int, t ChatTarget) (chatRecord, error) {
	var role, username string
	err := tx.QueryRowContext(ctx, `SELECT role,username FROM users WHERE id=$1 AND deleted_at IS NULL`, actorID).Scan(&role, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return chatRecord{}, ErrChatNotFound
	}
	if err != nil {
		return chatRecord{}, err
	}
	c := chatRecord{RequesterID: actorID, State: "accepted", Live: true}
	switch {
	case t.Handle != "":
		var id int
		var targetRole, name string
		err = tx.QueryRowContext(ctx, `SELECT id,role,username FROM users WHERE lower(username)=lower($1) AND deleted_at IS NULL`, t.Handle).Scan(&id, &targetRole, &name)
		if errors.Is(err, sql.ErrNoRows) || id == actorID {
			return c, ErrChatNotFound
		}
		if err != nil {
			return c, err
		}
		c.State = "pending"
		switch {
		case role == RoleUser && targetRole == RoleUser:
			c.Kind, c.CustomerID, c.OtherID, c.CustomerName, c.OtherName = "direct", actorID, id, username, name
			if c.CustomerID > c.OtherID {
				c.CustomerID, c.OtherID = c.OtherID, c.CustomerID
				c.CustomerName, c.OtherName = c.OtherName, c.CustomerName
			}
		case (role == RoleUser && targetRole == RolePartner) || (role == RolePartner && targetRole == RoleUser):
			partnerUser := id
			c.CustomerID, c.CustomerName = actorID, username
			if role == RolePartner {
				partnerUser, c.CustomerID, c.CustomerName = actorID, id, name
			}
			c.Kind = "seller"
			err = tx.QueryRowContext(ctx, `SELECT p.id,p.name FROM partner_users pu JOIN partners p ON p.id=pu.partner_id AND p.status='active'
              WHERE pu.user_id=$1`, partnerUser).Scan(&c.PartnerID, &c.PartnerName)
		default:
			return c, ErrChatNotFound // couriers are contactable only through assignments
		}
	case t.ProductID > 0:
		if role != RoleUser {
			return c, ErrChatNotFound
		}
		c.Kind, c.CustomerID, c.CustomerName, c.ProductID = "seller", actorID, username, t.ProductID
		err = tx.QueryRowContext(ctx, `SELECT p.id,p.name,prod.name FROM products prod
          JOIN partners p ON p.id=prod.partner_id AND p.status='active'
          WHERE prod.id=$1 AND prod.deleted_at IS NULL
          AND EXISTS (SELECT 1 FROM partner_users pu JOIN users u ON u.id=pu.user_id
             WHERE pu.partner_id=p.id AND u.role='partner' AND u.deleted_at IS NULL)`, t.ProductID).Scan(&c.PartnerID, &c.PartnerName, &c.ProductName)
	case t.OrderID > 0 && t.PartnerID > 0:
		c.Kind, c.OrderID, c.PartnerID = "seller", t.OrderID, t.PartnerID
		err = tx.QueryRowContext(ctx, `SELECT o.user_id,cu.username,p.name FROM orders o JOIN users cu ON cu.id=o.user_id
          JOIN partners p ON p.id=$2 AND p.status='active'
          WHERE o.id=$1 AND cu.deleted_at IS NULL AND cu.role='user'
          AND EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id=o.id AND oi.partner_id=p.id)
          AND EXISTS (SELECT 1 FROM partner_users pu JOIN users u ON u.id=pu.user_id
             WHERE pu.partner_id=p.id AND u.role='partner' AND u.deleted_at IS NULL)
          AND (($4='user' AND o.user_id=$3) OR ($4='partner' AND EXISTS
             (SELECT 1 FROM partner_users pu WHERE pu.user_id=$3 AND pu.partner_id=p.id)))`,
			t.OrderID, t.PartnerID, actorID, role).Scan(&c.CustomerID, &c.CustomerName, &c.PartnerName)
	case t.DeliveryOrderID > 0:
		c.Kind, c.OrderID = "delivery", t.DeliveryOrderID
		if _, err := tx.ExecContext(ctx, `SELECT id FROM orders WHERE id=$1 FOR SHARE`, t.DeliveryOrderID); err != nil {
			return c, err
		}
		// Serialize resolution against reassignment before reading the event.
		err = tx.QueryRowContext(ctx, `SELECT id FROM courier_deliveries WHERE order_id=$1 FOR SHARE`, t.DeliveryOrderID).Scan(&c.DeliveryID)
		if errors.Is(err, sql.ErrNoRows) {
			return c, ErrChatNotFound
		}
		if err != nil {
			return c, err
		}
		err = tx.QueryRowContext(ctx, `SELECT o.user_id,cu.username,d.courier_user_id,rider.username,
          (SELECT e.id FROM courier_delivery_events e WHERE e.delivery_id=d.id AND e.action IN ('assigned','reassigned') ORDER BY e.id DESC LIMIT 1)
          FROM courier_deliveries d JOIN orders o ON o.id=d.order_id
          JOIN users cu ON cu.id=o.user_id AND cu.role='user' AND cu.deleted_at IS NULL
          JOIN users rider ON rider.id=d.courier_user_id AND rider.role='courier' AND rider.deleted_at IS NULL
          WHERE d.id=$1 AND d.status<>'delivered' AND o.status<>'cancelled'
          AND (($3='user' AND o.user_id=$2) OR ($3='courier' AND d.courier_user_id=$2))`,
			c.DeliveryID, actorID, role).Scan(&c.CustomerID, &c.CustomerName, &c.OtherID, &c.OtherName, &c.AssignmentID)
	default:
		return c, ErrChatInvalid
	}
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrChatNotFound
	}
	return c, err
}

func findChat(ctx context.Context, tx *sql.Tx, c chatRecord) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM chat_conversations WHERE kind=$1 AND customer_user_id=$2
      AND COALESCE(other_user_id,0)=$3 AND COALESCE(partner_id,0)=$4 AND COALESCE(product_id,0)=$5
      AND COALESCE(order_id,0)=$6 AND COALESCE(assignment_event_id,0)=$7`,
		c.Kind, c.CustomerID, c.OtherID, c.PartnerID, c.ProductID, c.OrderID, c.AssignmentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *Store) StartChat(ctx context.Context, actorID int, target ChatTarget, body, nonce string) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var id int
	var created bool
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		id = 0
		created = false
		c, err := resolveChatTarget(ctx, tx, actorID, target)
		if err != nil {
			return err
		}
		if err := c.lockPair(ctx, tx); err != nil {
			return err
		}
		existing, err := findChat(ctx, tx, c)
		if err != nil {
			return err
		}
		fresh := existing == 0
		created = fresh
		if fresh {
			mine, theirs, err := chatBlockState(ctx, tx, c, actorID)
			if err != nil {
				return err
			}
			if mine || theirs {
				return ErrChatBlocked
			}
			if err := chatRate(ctx, tx, actorID, "start", 3600, 10); err != nil {
				return err
			}
			err = tx.QueryRowContext(ctx, `INSERT INTO chat_conversations(kind,customer_user_id,other_user_id,partner_id,
              product_id,order_id,delivery_id,assignment_event_id,requester_user_id,state)
              VALUES($1,$2,NULLIF($3,0),NULLIF($4,0),NULLIF($5,0),NULLIF($6,0),NULLIF($7,0),NULLIF($8,0),$9,$10) RETURNING id`,
				c.Kind, c.CustomerID, c.OtherID, c.PartnerID, c.ProductID, c.OrderID, c.DeliveryID, c.AssignmentID, actorID, c.State).Scan(&existing)
			if err != nil {
				return err
			}
		}
		id = existing
		c, err = loadChatTx(ctx, tx, actorID, id, true)
		if err != nil {
			return err
		}
		_, err = insertChatMessage(ctx, tx, c, actorID, body, nonce, fresh)
		return err
	})
	if err != nil && created {
		id = 0
	}
	return id, err
}
