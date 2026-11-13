package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/money"
)

// ListOrdersForFulfillment returns every order that is not cancelled and is
// either cash on delivery or paid online.
func (s *Store) ListOrdersForFulfillment(ctx context.Context) ([]Order, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.listOrders(ctx, "WHERE o.status <> 'cancelled' AND "+paymentSettledSQL+" ORDER BY o.created_at DESC, o.id DESC")
}

// partnerOwnsOrderSQL matches open orders with at least one line where every
// line belongs to the partner in parameter $1.
const partnerOwnsOrderSQL = `o.status <> 'cancelled' AND ` + paymentSettledSQL + `
	AND EXISTS (SELECT 1 FROM order_items i WHERE i.order_id = o.id)
	AND NOT EXISTS (SELECT 1 FROM order_items i WHERE i.order_id = o.id AND i.partner_id IS DISTINCT FROM $1)`

// ListOrdersForPartner returns the open orders wholly owned by one partner.
// Orders that mix partners stay with admins and staff.
func (s *Store) ListOrdersForPartner(ctx context.Context, partnerID int) ([]Order, error) {
	if partnerID <= 0 {
		return []Order{}, nil
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.listOrders(ctx, "WHERE "+partnerOwnsOrderSQL+" ORDER BY o.created_at DESC, o.id DESC", partnerID)
}

// PartnerOwnsOrder reports whether orderID is an open order wholly owned by
// partnerID.
func (s *Store) PartnerOwnsOrder(ctx context.Context, partnerID, orderID int) (bool, error) {
	if partnerID <= 0 || orderID <= 0 {
		return false, nil
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var owned bool
	err := s.DB.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM orders o WHERE o.id = $2 AND "+partnerOwnsOrderSQL+")",
		partnerID, orderID,
	).Scan(&owned)
	return owned, err
}

func partnerOwnsOrderTx(ctx context.Context, tx *sql.Tx, partnerID, orderID int) (bool, error) {
	var owned bool
	err := tx.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM orders o WHERE o.id = $2 AND "+partnerOwnsOrderSQL+")",
		partnerID, orderID,
	).Scan(&owned)
	return owned, err
}

func (s *Store) ListAllOrders(ctx context.Context) ([]Order, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.listOrders(ctx, "ORDER BY o.created_at DESC, o.id DESC")
}

func (s *Store) ListAllOrdersPage(ctx context.Context, limit, offset int) ([]Order, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if limit < 1 {
		limit = 50
	} else if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM orders").Scan(&total); err != nil {
		return nil, 0, err
	}
	orders := []Order{}
	if orderPageHasRows(total, limit, offset) {
		orders, err = listOrdersQuery(ctx, tx, "ORDER BY o.created_at DESC, o.id DESC LIMIT $1 OFFSET $2", limit, offset)
		if err != nil {
			return nil, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, wrapDBError(err)
	}
	return orders, total, nil
}

const (
	deliveryBoardSummarySQL = "SELECT " +
		"count(*) FILTER (WHERE o.status <> 'cancelled' AND o.delivery_status = 'queued'), " +
		"count(*) FILTER (WHERE o.status <> 'cancelled' AND o.delivery_status = 'processing'), " +
		"count(*) FILTER (WHERE o.status <> 'cancelled' AND o.delivery_status = 'shipped'), " +
		"count(*) FILTER (WHERE o.status <> 'cancelled' AND o.delivery_status = 'delivered'), " +
		"count(*) FILTER (WHERE o.status <> 'cancelled' AND o.delivery_status <> 'delivered') " +
		"FROM orders o"
	pendingDeliveryOrdersPredicate = "o.status <> 'cancelled' AND o.delivery_status <> 'delivered'"
)

// ListDeliveryBoardPage returns delivery counts and one oldest-first page of pending orders.
func (s *Store) ListDeliveryBoardPage(ctx context.Context, limit, offset int) ([]Order, DeliveryBoardSummary, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if limit < 1 {
		limit = 50
	} else if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, DeliveryBoardSummary{}, 0, err
	}
	defer tx.Rollback()

	var summary DeliveryBoardSummary
	if err := tx.QueryRowContext(ctx, deliveryBoardSummarySQL).Scan(
		&summary.QueuedCount, &summary.ProcessingCount, &summary.ShippedCount,
		&summary.DeliveredCount, &summary.PendingCount,
	); err != nil {
		return nil, DeliveryBoardSummary{}, 0, err
	}
	total := summary.PendingCount
	orders := []Order{}
	if orderPageHasRows(total, limit, offset) {
		orders, err = listOrdersQuery(ctx, tx,
			"WHERE "+pendingDeliveryOrdersPredicate+" ORDER BY o.created_at ASC, o.id ASC LIMIT $1 OFFSET $2",
			limit, offset)
		if err != nil {
			return nil, DeliveryBoardSummary{}, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, DeliveryBoardSummary{}, 0, wrapDBError(err)
	}
	return orders, summary, total, nil
}

func fulfillmentPredicate(partnerID int) string {
	if partnerID > 0 {
		return partnerOwnsOrderSQL
	}
	return "o.status <> 'cancelled' AND " + paymentSettledSQL
}

// ListFulfillmentOrdersPage applies role scope and status filtering in
// SQL before loading order lines. partnerID=0 is the admin/staff queue.
func (s *Store) ListFulfillmentOrdersPage(ctx context.Context, partnerID int, status string, limit, offset int) ([]Order, int, error) {
	orders, _, total, err := s.listFulfillmentOrdersPage(ctx, partnerID, status, limit, offset, false)
	return orders, total, err
}

// ListFulfillmentOrdersPageWithSummary returns the queue page, its total, and
// the workspace status counts from one read-only database snapshot.
func (s *Store) ListFulfillmentOrdersPageWithSummary(ctx context.Context, partnerID int, status string, limit, offset int) ([]Order, PartnerOrderSummary, int, error) {
	return s.listFulfillmentOrdersPage(ctx, partnerID, status, limit, offset, true)
}

func (s *Store) listFulfillmentOrdersPage(ctx context.Context, partnerID int, status string, limit, offset int, includeSummary bool) ([]Order, PartnerOrderSummary, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	if limit < 1 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	predicate := fulfillmentPredicate(partnerID)
	args := []any{}
	if partnerID > 0 {
		args = append(args, partnerID)
	}
	switch status {
	case "completed":
		predicate += " AND o.status = 'delivered'"
	case "dispatched":
		predicate += " AND o.partner_status = 'dispatched' AND o.status <> 'delivered'"
	case "", "all":
	default:
		args = append(args, status)
		predicate += fmt.Sprintf(" AND o.partner_status = $%d", len(args))
	}

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, PartnerOrderSummary{}, 0, err
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM orders o WHERE "+predicate, args...).Scan(&total); err != nil {
		return nil, PartnerOrderSummary{}, 0, err
	}
	var summary PartnerOrderSummary
	if includeSummary {
		summary, err = fulfillmentSummaryQuery(ctx, tx, partnerID)
		if err != nil {
			return nil, PartnerOrderSummary{}, 0, err
		}
	}
	orders := []Order{}
	if orderPageHasRows(total, limit, offset) {
		limitArg := fmt.Sprintf("$%d", len(args)+1)
		offsetArg := fmt.Sprintf("$%d", len(args)+2)
		orders, err = listOrdersQuery(ctx, tx, "WHERE "+predicate+" ORDER BY o.created_at DESC, o.id DESC LIMIT "+limitArg+" OFFSET "+offsetArg,
			append(args, limit, offset)...)
		if err != nil {
			return nil, PartnerOrderSummary{}, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, PartnerOrderSummary{}, 0, wrapDBError(err)
	}
	return orders, summary, total, nil
}

func (s *Store) FulfillmentSummary(ctx context.Context, partnerID int) (PartnerOrderSummary, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return fulfillmentSummaryQuery(ctx, s.DB, partnerID)
}

func fulfillmentSummaryQuery(ctx context.Context, q querier, partnerID int) (PartnerOrderSummary, error) {
	args := []any{}
	if partnerID > 0 {
		args = append(args, partnerID)
	}
	var summary PartnerOrderSummary
	err := q.QueryRowContext(ctx,
		`SELECT count(*) FILTER (WHERE o.partner_status = 'new'),
		        count(*) FILTER (WHERE o.partner_status IN ('accepted', 'packing')),
		        count(*) FILTER (WHERE o.partner_status = 'dispatched' AND o.status <> 'delivered'),
		        count(*) FILTER (WHERE o.status = 'delivered'),
		        count(*) FILTER (WHERE o.partner_status IN ('new', 'accepted') AND o.created_at <= now() - interval '2 hours')
		 FROM orders o WHERE `+fulfillmentPredicate(partnerID), args...).Scan(
		&summary.NewCount, &summary.InProgressCount, &summary.DispatchedCount, &summary.CompletedCount, &summary.OverdueCount)
	return summary, err
}

type lockedOrder struct {
	Status         string
	PartnerStatus  string
	DeliveryStatus string
	InventoryState string
}

func lockOrderTx(ctx context.Context, tx *sql.Tx, orderID int) (*lockedOrder, error) {
	var o lockedOrder
	err := tx.QueryRowContext(ctx,
		"SELECT status, partner_status, delivery_status, inventory_state FROM orders WHERE id = $1 FOR UPDATE",
		orderID,
	).Scan(&o.Status, &o.PartnerStatus, &o.DeliveryStatus, &o.InventoryState)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// UpdateOrderFulfillment applies a partner fulfillment update. Partners control
// preparation; dispatching ships stock and sets delivery to shipped. The
// courier workflow owns later delivery-state changes.
func (s *Store) UpdateOrderFulfillment(ctx context.Context, orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error {
	return s.updateOrderFulfillment(ctx, 0, "", orderID, partnerStatus, deliveryStatus, deliveryNotice)
}

// UpdateOrderFulfillmentForActor checks workspace authorization and order
// ownership in the same transaction as the fulfillment update.
func (s *Store) UpdateOrderFulfillmentForActor(ctx context.Context, actorID int, actorRole string, orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error {
	if actorID <= 0 {
		return ErrOrderNotFound
	}
	return s.updateOrderFulfillment(ctx, actorID, actorRole, orderID, partnerStatus, deliveryStatus, deliveryNotice)
}

func (s *Store) updateOrderFulfillment(ctx context.Context, actorID int, actorRole string, orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error {
	nextStatus, ok := partnerStatusToOrderStatus[partnerStatus]
	if !ok {
		return ErrInvalidPartnerStatus
	}
	if !validDeliveryStatuses[deliveryStatus] {
		return ErrInvalidDeliveryStatus
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		partnerID, isPartner := 0, false
		if actorID > 0 {
			var err error
			partnerID, isPartner, err = lockWorkspaceActorTx(ctx, tx, actorID, actorRole)
			if errors.Is(err, ErrWorkspaceActorNotFound) {
				return ErrOrderNotFound
			}
			if err != nil {
				return err
			}
		}
		current, err := lockOrderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		paymentSettled, err := orderPaymentSettledTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if !paymentSettled {
			return ErrInvalidPartnerTransition
		}
		if isPartner {
			owned, err := partnerOwnsOrderTx(ctx, tx, partnerID, orderID)
			if err != nil {
				return err
			}
			if !owned {
				return ErrOrderNotFound
			}
		}
		if !isAllowedPartnerTransition(current.PartnerStatus, partnerStatus) {
			return ErrInvalidPartnerTransition
		}
		// Partners control preparation, while courier accounts control the
		// delivery handoff. Once dispatched, a partner cannot complete or alter
		// delivery state. Dispatching advances the order to shipped atomically.
		if current.PartnerStatus == "dispatched" {
			if partnerStatus != "dispatched" {
				return ErrInvalidPartnerTransition
			}
			// A retried dispatch must not reopen a courier-completed order.
			nextStatus = current.Status
			deliveryStatus = current.DeliveryStatus
		} else if partnerStatus == "dispatched" {
			deliveryStatus = "shipped"
		} else {
			deliveryStatus = current.DeliveryStatus
		}
		if !validDeliveryStatuses[deliveryStatus] || !isAllowedDeliveryTransition(current.DeliveryStatus, deliveryStatus) {
			return ErrInvalidDeliveryTransition
		}
		return writeOrderStateTx(ctx, tx, orderID, current, nextStatus, partnerStatus, deliveryStatus, deliveryNotice)
	})
}

// UpdateOrderByAdmin lets a trusted internal caller set any valid status
// combination. Request handlers should use UpdateOrderByAdminForActor.
func (s *Store) UpdateOrderByAdmin(ctx context.Context, orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error {
	return s.updateOrderByAdmin(ctx, 0, orderID, status, partnerStatus, deliveryStatus, deliveryNotice, false)
}

// UpdateOrderByAdminForActor revalidates the admin in the order transaction.
func (s *Store) UpdateOrderByAdminForActor(ctx context.Context, actorID, orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error {
	return s.updateOrderByAdmin(ctx, actorID, orderID, status, partnerStatus, deliveryStatus, deliveryNotice, true)
}

func (s *Store) updateOrderByAdmin(ctx context.Context, actorID, orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string, requireAdmin bool) error {
	if !validOrderStatuses[status] || !validPartnerStatuses[partnerStatus] || !validDeliveryStatuses[deliveryStatus] {
		return ErrInvalidOrderState
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := lockOrderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if requireAdmin {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, RoleAdmin); err != nil {
				return err
			}
		}
		if current.InventoryState == "released" && inventoryStateFor(status, partnerStatus) != "released" {
			return ErrInvalidOrderState
		}
		return writeOrderStateTx(ctx, tx, orderID, current, status, partnerStatus, deliveryStatus, deliveryNotice)
	})
}

func writeOrderStateTx(ctx context.Context, tx *sql.Tx, orderID int, current *lockedOrder, status, partnerStatus, deliveryStatus, notice string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE orders
		 SET status = $1, partner_status = $2, delivery_status = $3, delivery_notice = $4
		 WHERE id = $5`,
		status, partnerStatus, deliveryStatus, strings.TrimSpace(notice), orderID,
	); err != nil {
		return err
	}
	return applyOrderInventoryTransition(ctx, tx, orderID, current.InventoryState, inventoryStateFor(status, partnerStatus))
}

func (s *Store) GetPartnerOrderSummary(ctx context.Context) (PartnerOrderSummary, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var summary PartnerOrderSummary
	err := s.DB.QueryRowContext(ctx,
		`SELECT
		    count(*) FILTER (WHERE partner_status = 'new'),
		    count(*) FILTER (WHERE partner_status IN ('accepted', 'packing')),
		    count(*) FILTER (WHERE partner_status = 'dispatched' AND status <> 'delivered'),
		    count(*) FILTER (WHERE status = 'delivered'),
		    count(*) FILTER (WHERE partner_status IN ('new', 'accepted') AND created_at <= now() - interval '2 hours')
		 FROM orders
		 WHERE status <> 'cancelled'`,
	).Scan(&summary.NewCount, &summary.InProgressCount, &summary.DispatchedCount, &summary.CompletedCount, &summary.OverdueCount)
	return summary, err
}

// GetOrderReportData returns revenue and order-status totals from one snapshot.
func (s *Store) GetOrderReportData(ctx context.Context) (money.Cents, map[string]int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	var revenue money.Cents
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(total_cents), 0) FROM orders WHERE status <> 'cancelled'",
	).Scan(&revenue); err != nil {
		return 0, nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT status, count(*) FROM orders GROUP BY status")
	if err != nil {
		return 0, nil, err
	}
	counts := make(map[string]int, len(validOrderStatuses))
	for status := range validOrderStatuses {
		counts[status] = 0
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return 0, nil, err
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, nil, err
	}
	if err := rows.Close(); err != nil {
		return 0, nil, err
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, wrapDBError(err)
	}
	return revenue, counts, nil
}

func (s *Store) GetOrderStatusCounts(ctx context.Context) (map[string]int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx, "SELECT status, count(*) FROM orders GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	for status := range validOrderStatuses {
		counts[status] = 0
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

// SumOrderRevenue totals non-cancelled orders.
func (s *Store) SumOrderRevenue(ctx context.Context) (money.Cents, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var total money.Cents
	err := s.DB.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(total_cents), 0) FROM orders WHERE status <> 'cancelled'",
	).Scan(&total)
	return total, err
}
