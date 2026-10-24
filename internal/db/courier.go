package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var courierFailureReasons = map[string]bool{
	"Recipient unavailable": true,
	"Address issue":         true,
	"Item damaged":          true,
	"Recipient declined":    true,
	"Other":                 true,
}

// ListCourierUsers returns active courier accounts for dispatcher controls.
func (s *Store) ListCourierUsers(ctx context.Context) ([]CourierUser, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.DB.QueryContext(ctx,
		`SELECT u.id, u.username,
         count(cd.id) FILTER (WHERE cd.status IN ('assigned','picked_up','out_for_delivery') AND o.status NOT IN ('cancelled','delivered')),
         count(cd.id) FILTER (WHERE cd.status = 'delivery_failed' AND o.status NOT IN ('cancelled','delivered'))
         FROM users u
         LEFT JOIN courier_deliveries cd ON cd.courier_user_id=u.id
           AND cd.status IN ('assigned','picked_up','out_for_delivery','delivery_failed')
         LEFT JOIN orders o ON o.id=cd.order_id
         WHERE u.role=$1 AND u.deleted_at IS NULL
         GROUP BY u.id ORDER BY lower(u.username), u.id LIMIT 200`, RoleCourier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]CourierUser, 0)
	for rows.Next() {
		var user CourierUser
		if err := rows.Scan(&user.ID, &user.Username, &user.ActiveDeliveries, &user.FailedDeliveries); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// ListCourierDispatchOrders returns shipped orders awaiting delivery or
// requiring a new courier after a failed attempt. It excludes payment fields.
func (s *Store) ListCourierDispatchOrders(ctx context.Context) ([]CourierDelivery, error) {
	return s.listCourierOrders(ctx, 0, true)
}

// ListCourierDeliveries returns only the deliveries assigned to courierUserID.
// Completed history omits recipient addresses; active and failed jobs retain them.
func (s *Store) ListCourierDeliveries(ctx context.Context, courierUserID int) ([]CourierDelivery, error) {
	if courierUserID <= 0 {
		return nil, ErrCourierDeliveryForbidden
	}
	return s.listCourierOrders(ctx, courierUserID, false)
}

func (s *Store) listCourierOrders(ctx context.Context, courierUserID int, dispatch bool) ([]CourierDelivery, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	where := ""
	args := []any{}
	if dispatch {
		where = `o.status = 'shipped' AND o.delivery_status <> 'delivered'`
		where += ` AND (cd.status IS NULL OR cd.status <> 'delivered')`
	} else {
		args = append(args, courierUserID)
		where = `cd.courier_user_id = $1 AND o.status <> 'cancelled'`
	}

	rows, err := s.DB.QueryContext(ctx,
		`WITH selected AS (
		 SELECT o.id,
		        CASE WHEN COALESCE(cd.status, '') = 'delivered' THEN 1 ELSE 0 END AS delivery_rank,
		        COALESCE(cd.assigned_at, o.created_at) AS queued_at
		 FROM orders o
		 LEFT JOIN courier_deliveries cd ON cd.order_id = o.id
		 WHERE `+where+`
		 ORDER BY delivery_rank, queued_at, o.id
		 LIMIT 200
		)
		 SELECT COALESCE(cd.id, 0), o.id, COALESCE(cd.courier_user_id, 0),
		        COALESCE(courier.username, ''), COALESCE(cd.assigned_by_user_id, 0),
		        customer.username, CASE WHEN cd.status='delivered' THEN '' ELSE o.delivery_address END, o.status, o.partner_status,
		        o.delivery_status, COALESCE(cd.status, ''), COALESCE(cd.failure_note, ''),
		        o.created_at, COALESCE(cd.assigned_at, o.created_at),
	        COALESCE(cd.updated_at, o.updated_at), oi.product_name, oi.quantity
		 FROM selected
		 JOIN orders o ON o.id = selected.id
		 JOIN users customer ON customer.id = o.user_id
		 LEFT JOIN courier_deliveries cd ON cd.order_id = o.id
		 LEFT JOIN users courier ON courier.id = cd.courier_user_id
		 LEFT JOIN order_items oi ON oi.order_id = o.id
		 ORDER BY selected.delivery_rank, selected.queued_at, o.id, oi.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := make(map[int]int)
	deliveries := make([]CourierDelivery, 0)
	for rows.Next() {
		var delivery CourierDelivery
		var productName sql.NullString
		var quantity sql.NullInt64
		if err := rows.Scan(
			&delivery.ID, &delivery.OrderID, &delivery.CourierUserID,
			&delivery.CourierName, &delivery.AssignedByID, &delivery.CustomerName,
			&delivery.DeliveryAddress, &delivery.OrderStatus, &delivery.PartnerStatus,
			&delivery.DeliveryStatus, &delivery.Status, &delivery.FailureNote,
			&delivery.CreatedAt, &delivery.AssignedAt, &delivery.UpdatedAt,
			&productName, &quantity,
		); err != nil {
			return nil, err
		}
		index, ok := byID[delivery.OrderID]
		if !ok {
			index = len(deliveries)
			byID[delivery.OrderID] = index
			delivery.Items = make([]CourierDeliveryItem, 0)
			deliveries = append(deliveries, delivery)
		}
		if productName.Valid && quantity.Valid {
			deliveries[index].Items = append(deliveries[index].Items, CourierDeliveryItem{
				ProductName: productName.String,
				Quantity:    int(quantity.Int64),
			})
		}
	}
	return deliveries, rows.Err()
}

// AssignCourierToOrder assigns or reassigns an order only after seller
// fulfillment has shipped it. Failed attempts may be reassigned; once a rider
// picks up the order, only that rider can move it forward.
func (s *Store) AssignCourierToOrder(ctx context.Context, orderID, courierUserID, actorUserID int) error {
	if orderID <= 0 || courierUserID <= 0 || actorUserID <= 0 {
		return ErrDeliveryNotReady
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		currentOrder, err := lockOrderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if currentOrder.Status != OrderStatusShipped || currentOrder.DeliveryStatus == "delivered" {
			return ErrDeliveryNotReady
		}
		var role string
		err = tx.QueryRowContext(ctx,
			`SELECT role FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, courierUserID,
		).Scan(&role)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && role != RoleCourier) {
			return ErrCourierUserNotFound
		}
		if err != nil {
			return err
		}

		var deliveryID int
		var previousCourierID int
		var previousStatus string
		err = tx.QueryRowContext(ctx,
			`SELECT id, courier_user_id, status FROM courier_deliveries WHERE order_id = $1 FOR UPDATE`, orderID,
		).Scan(&deliveryID, &previousCourierID, &previousStatus)
		if errors.Is(err, sql.ErrNoRows) {
			err = tx.QueryRowContext(ctx,
				`INSERT INTO courier_deliveries (order_id, courier_user_id, assigned_by_user_id)
				 VALUES ($1, $2, $3) RETURNING id`, orderID, courierUserID, actorUserID,
			).Scan(&deliveryID)
			if err != nil {
				return wrapDBError(err)
			}
			return recordCourierDeliveryEventTx(ctx, tx, deliveryID, actorUserID, "assigned", "", CourierDeliveryAssigned, 0, courierUserID, "")
		}
		if err != nil {
			return err
		}
		if previousStatus != CourierDeliveryAssigned && previousStatus != CourierDeliveryFailed {
			return ErrDeliveryAlreadyStarted
		}
		if previousCourierID == courierUserID && previousStatus == CourierDeliveryAssigned {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE courier_deliveries
			 SET courier_user_id = $1, assigned_by_user_id = $2,
			     status = 'assigned', failure_note = '', assigned_at = now(),
			     picked_up_at = NULL, out_for_delivery_at = NULL, delivered_at = NULL, failed_at = NULL
			 WHERE id = $3`, courierUserID, actorUserID, deliveryID,
		); err != nil {
			return wrapDBError(err)
		}
		return recordCourierDeliveryEventTx(ctx, tx, deliveryID, actorUserID, "reassigned", previousStatus, CourierDeliveryAssigned, previousCourierID, courierUserID, "")
	})
}

// UpdateCourierDelivery moves an assigned delivery forward for its owning
// courier. The order row is locked first, matching dispatcher lock order.
func (s *Store) UpdateCourierDelivery(ctx context.Context, deliveryID, courierUserID int, nextStatus, note string) error {
	if deliveryID <= 0 || courierUserID <= 0 {
		return ErrCourierDeliveryForbidden
	}
	note = strings.TrimSpace(note)
	if nextStatus == CourierDeliveryFailed {
		if !courierFailureReasons[note] {
			return ErrInvalidCourierTransition
		}
	} else {
		// Only predefined failure categories are retained; free-form customer
		// information does not belong in the delivery event log.
		note = ""
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		var orderID int
		if err := tx.QueryRowContext(ctx,
			`SELECT order_id FROM courier_deliveries WHERE id = $1`, deliveryID,
		).Scan(&orderID); errors.Is(err, sql.ErrNoRows) {
			return ErrCourierDeliveryNotFound
		} else if err != nil {
			return err
		}
		currentOrder, err := lockOrderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}

		var currentStatus, currentNote string
		var assignedCourierID int
		if err := tx.QueryRowContext(ctx,
			`SELECT status, courier_user_id, failure_note FROM courier_deliveries WHERE id = $1 FOR UPDATE`, deliveryID,
		).Scan(&currentStatus, &assignedCourierID, &currentNote); errors.Is(err, sql.ErrNoRows) {
			return ErrCourierDeliveryNotFound
		} else if err != nil {
			return err
		}
		if assignedCourierID != courierUserID {
			return ErrCourierDeliveryForbidden
		}
		if currentOrder.Status == OrderStatusCancelled {
			return ErrInvalidCourierTransition
		}
		// A repeated submission after a lost response is a no-op. Ownership is
		// checked first; failure retries must match the original reason exactly.
		if currentStatus == nextStatus && nextStatus != CourierDeliveryAssigned && currentNote == note {
			return nil
		}
		if !allowedCourierTransition(currentStatus, nextStatus) {
			return ErrInvalidCourierTransition
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE courier_deliveries SET status = $1, failure_note = $2,
			 picked_up_at = CASE WHEN $1 = 'picked_up' THEN now() WHEN $1 = 'out_for_delivery' THEN COALESCE(picked_up_at, now()) ELSE picked_up_at END,
			 out_for_delivery_at = CASE WHEN $1 = 'out_for_delivery' THEN now() ELSE out_for_delivery_at END,
			 delivered_at = CASE WHEN $1 = 'delivered' THEN now() ELSE delivered_at END,
			 failed_at = CASE WHEN $1 = 'delivery_failed' THEN now() ELSE failed_at END
			 WHERE id = $3`, nextStatus, note, deliveryID,
		); err != nil {
			return wrapDBError(err)
		}

		if err := recordCourierDeliveryEventTx(ctx, tx, deliveryID, courierUserID, nextStatus, currentStatus, nextStatus, courierUserID, courierUserID, note); err != nil {
			return err
		}

		switch nextStatus {
		case CourierDeliveryDelivered:
			return writeOrderStateTx(ctx, tx, orderID, currentOrder, OrderStatusDelivered,
				currentOrder.PartnerStatus, "delivered", "Order delivered.")
		case CourierDeliveryFailed:
			_, err := tx.ExecContext(ctx,
				`UPDATE orders SET delivery_notice = 'Delivery attempt unsuccessful. The team will follow up.' WHERE id = $1`, orderID,
			)
			return err
		default:
			return nil
		}
	})
}

func allowedCourierTransition(from, to string) bool {
	switch from {
	case CourierDeliveryAssigned:
		return to == CourierDeliveryPickedUp || to == CourierDeliveryFailed
	case CourierDeliveryPickedUp:
		return to == CourierDeliveryOutForDelivery || to == CourierDeliveryFailed
	case CourierDeliveryOutForDelivery:
		return to == CourierDeliveryDelivered || to == CourierDeliveryFailed
	default:
		return false
	}
}

func recordCourierDeliveryEventTx(ctx context.Context, tx *sql.Tx, deliveryID, actorUserID int, action, previousStatus, status string, previousCourierID, courierUserID int, note string) error {
	var previousCourier any
	if previousCourierID > 0 {
		previousCourier = previousCourierID
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO courier_delivery_events
		 (delivery_id, actor_user_id, action, previous_status, status, previous_courier_user_id, courier_user_id, note)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		deliveryID, actorUserID, action, previousStatus, status, previousCourier, courierUserID, note,
	)
	return err
}
