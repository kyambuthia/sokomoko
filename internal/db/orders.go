package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/money"
)

const defaultDeliveryNotice = "Order received. Awaiting partner acceptance."

var (
	validOrderStatuses = map[string]bool{
		OrderStatusPending: true, OrderStatusProcessing: true, OrderStatusShipped: true,
		OrderStatusDelivered: true, OrderStatusCancelled: true,
	}
	validPartnerStatuses = map[string]bool{
		"new": true, "accepted": true, "packing": true, "dispatched": true, "completed": true, "cancelled": true,
	}
	validDeliveryStatuses = map[string]bool{
		"queued": true, "processing": true, "shipped": true, "delivered": true,
	}
	partnerStatusToOrderStatus = map[string]string{
		"new":        OrderStatusPending,
		"accepted":   OrderStatusProcessing,
		"packing":    OrderStatusProcessing,
		"dispatched": OrderStatusShipped,
		"completed":  OrderStatusDelivered,
		"cancelled":  OrderStatusCancelled,
	}
	allowedPartnerTransitions = map[string]map[string]bool{
		"new":        {"new": true, "accepted": true, "cancelled": true},
		"accepted":   {"accepted": true, "packing": true, "cancelled": true},
		"packing":    {"packing": true, "dispatched": true, "cancelled": true},
		"dispatched": {"dispatched": true, "completed": true},
		"completed":  {"completed": true},
		"cancelled":  {"cancelled": true},
	}
	allowedDeliveryTransitions = map[string]map[string]bool{
		"queued":     {"queued": true, "processing": true, "shipped": true, "delivered": true},
		"processing": {"processing": true, "shipped": true, "delivered": true},
		"shipped":    {"shipped": true, "delivered": true},
		"delivered":  {"delivered": true},
	}
)

func isAllowedPartnerTransition(from, to string) bool  { return allowedPartnerTransitions[from][to] }
func isAllowedDeliveryTransition(from, to string) bool { return allowedDeliveryTransitions[from][to] }

// orderLine is a priced line ready to be written to order_items.
type orderLine struct {
	ProductID   int
	PartnerID   sql.NullInt64
	ProductName string
	Quantity    int
	UnitPrice   money.Cents
}

// allocation describes where an order's stock comes from: either the caller's
// active reservation (reservationKey set) or directly from available stock.
type allocation struct {
	userID         int
	reservationKey string
}

// createOrderTx writes the order and its items and allocates stock for every
// line. Lines must be sorted by product id.
func createOrderTx(ctx context.Context, tx *sql.Tx, userID int, address, notice string, totals OrderTotals, lines []orderLine, alloc allocation) (int64, error) {
	if strings.TrimSpace(address) == "" {
		return 0, ErrDeliveryAddressRequired
	}
	if len(lines) == 0 {
		return 0, ErrCartEmpty
	}
	if strings.TrimSpace(notice) == "" {
		notice = defaultDeliveryNotice
	}

	warehouseID, err := defaultWarehouseID(ctx, tx)
	if err != nil {
		return 0, err
	}

	var orderID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO orders (user_id, subtotal_cents, shipping_cents, tax_cents, total_cents, delivery_address, delivery_notice)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		userID, int64(totals.Subtotal), int64(totals.ShippingFee), int64(totals.TaxAmount), int64(totals.Total),
		strings.TrimSpace(address), strings.TrimSpace(notice),
	).Scan(&orderID); err != nil {
		return 0, wrapDBError(err)
	}

	reference := fmt.Sprintf("order:%d", orderID)
	for _, line := range lines {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO order_items (order_id, product_id, partner_id, warehouse_id, product_name, quantity, unit_price_cents)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			orderID, line.ProductID, line.PartnerID, warehouseID, line.ProductName, line.Quantity, int64(line.UnitPrice),
		); err != nil {
			return 0, err
		}

		stock, err := lockInventoryStock(ctx, tx, line.ProductID, warehouseID)
		if err != nil {
			return 0, err
		}

		if alloc.reservationKey != "" {
			var reserved int
			if err := tx.QueryRowContext(ctx,
				`UPDATE stock_reservations
				 SET status = 'converted', released_at = now()
				 WHERE user_id = $1 AND reservation_key = $2 AND product_id = $3 AND warehouse_id = $4
				   AND status = 'active' AND expires_at > now()
				 RETURNING quantity`,
				alloc.userID, alloc.reservationKey, line.ProductID, warehouseID,
			).Scan(&reserved); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return 0, fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
				}
				return 0, err
			}
			if reserved < line.Quantity {
				return 0, fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE inventory_stocks
				 SET reserved_quantity = reserved_quantity - $1, allocated_quantity = allocated_quantity + $2
				 WHERE product_id = $3 AND warehouse_id = $4`,
				reserved, line.Quantity, line.ProductID, warehouseID,
			); err != nil {
				return 0, wrapDBError(err)
			}
		} else {
			if line.Quantity > stock.AvailableQuantity {
				return 0, fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE inventory_stocks SET allocated_quantity = allocated_quantity + $1
				 WHERE product_id = $2 AND warehouse_id = $3`,
				line.Quantity, line.ProductID, warehouseID,
			); err != nil {
				return 0, wrapDBError(err)
			}
		}
		if err := recordStockMovement(ctx, tx, line.ProductID, warehouseID, stockMovementAllocation, -line.Quantity, reference); err != nil {
			return 0, err
		}
	}
	return orderID, nil
}

func clearCartTx(ctx context.Context, tx *sql.Tx, userID int) error {
	_, err := tx.ExecContext(ctx,
		"DELETE FROM cart_items WHERE cart_id = (SELECT id FROM carts WHERE user_id = $1)", userID,
	)
	return err
}

// PlaceOrderFromCart places an order for the whole cart at current prices with
// no shipping or tax. The checkout service uses PlaceOrderFromCheckoutWithPayment
// instead; this path exists for operational tooling and tests.
func (s *Store) PlaceOrderFromCart(userID int, deliveryAddress string) (int64, error) {
	if strings.TrimSpace(deliveryAddress) == "" {
		return 0, ErrDeliveryAddressRequired
	}
	ctx, cancel := s.ctx()
	defer cancel()

	var orderID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := invalidateCheckoutsTx(ctx, tx, userID); err != nil {
			return err
		}
		cart, err := cartLinesTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		lines := make([]orderLine, 0, len(cart))
		var subtotal money.Cents
		for _, c := range cart {
			lines = append(lines, orderLine(c))
			subtotal += c.UnitPrice * money.Cents(c.Quantity)
		}
		totals := OrderTotals{Subtotal: subtotal, Total: subtotal}
		orderID, err = createOrderTx(ctx, tx, userID, deliveryAddress, "", totals, lines, allocation{})
		if err != nil {
			return err
		}
		return clearCartTx(ctx, tx, userID)
	})
	return orderID, err
}

const orderSelect = `
SELECT o.id, o.user_id, u.username, o.status, o.partner_status, o.delivery_status, o.inventory_state,
       o.delivery_address, o.delivery_notice, o.currency,
       o.subtotal_cents, o.shipping_cents, o.tax_cents, o.total_cents, o.created_at, o.updated_at
FROM orders o
JOIN users u ON u.id = o.user_id`

func scanOrder(row interface{ Scan(...any) error }) (*Order, error) {
	order := &Order{}
	err := row.Scan(
		&order.ID, &order.UserID, &order.CustomerName, &order.Status, &order.PartnerStatus,
		&order.DeliveryStatus, &order.InventoryState, &order.DeliveryAddress, &order.DeliveryNotice,
		&order.Currency, &order.Subtotal, &order.ShippingFee, &order.TaxAmount, &order.TotalAmount,
		&order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Store) listOrders(ctx context.Context, suffix string, args ...any) ([]Order, error) {
	rows, err := s.DB.QueryContext(ctx, orderSelect+" "+suffix, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []Order{}
	ids := []int{}
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *order)
		ids = append(ids, order.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items, err := s.orderItemsByOrderIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = items[orders[i].ID]
	}
	return orders, nil
}

func (s *Store) orderItemsByOrderIDs(ctx context.Context, orderIDs []int) (map[int][]OrderItem, error) {
	itemsByOrder := map[int][]OrderItem{}
	if len(orderIDs) == 0 {
		return itemsByOrder, nil
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT order_id, product_id, partner_id, product_name, quantity, unit_price_cents, line_total_cents
		 FROM order_items
		 WHERE order_id = ANY($1)
		 ORDER BY order_id, id`,
		int64Slice(orderIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var orderID int
		var item OrderItem
		if err := rows.Scan(&orderID, &item.ProductID, &item.PartnerID, &item.ProductName, &item.Quantity, &item.UnitPrice, &item.LineTotal); err != nil {
			return nil, err
		}
		itemsByOrder[orderID] = append(itemsByOrder[orderID], item)
	}
	return itemsByOrder, rows.Err()
}

// GetOrderForUser returns one of the user's orders, or nil when it does not
// exist or belongs to someone else.
func (s *Store) GetOrderForUser(userID, orderID int) (*Order, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	orders, err := s.listOrders(ctx, "WHERE o.id = $1 AND o.user_id = $2", orderID, userID)
	if err != nil || len(orders) == 0 {
		return nil, err
	}
	return &orders[0], nil
}

func (s *Store) ListOrdersByUser(userID int) ([]Order, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.listOrders(ctx, "WHERE o.user_id = $1 ORDER BY o.created_at DESC, o.id DESC", userID)
}

// ListOrdersForFulfillment returns every order that is not cancelled.
func (s *Store) ListOrdersForFulfillment() ([]Order, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.listOrders(ctx, "WHERE o.status <> 'cancelled' ORDER BY o.created_at DESC, o.id DESC")
}

func (s *Store) ListAllOrders() ([]Order, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.listOrders(ctx, "ORDER BY o.created_at DESC, o.id DESC")
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

// UpdateOrderFulfillment applies a partner fulfillment update. Partner status
// drives the overall order status, and both partner and delivery statuses may
// only move forward. Dispatching ships stock; cancelling releases it.
func (s *Store) UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error {
	nextStatus, ok := partnerStatusToOrderStatus[partnerStatus]
	if !ok {
		return ErrInvalidPartnerStatus
	}
	if !validDeliveryStatuses[deliveryStatus] {
		return ErrInvalidDeliveryStatus
	}
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := lockOrderTx(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if !isAllowedPartnerTransition(current.PartnerStatus, partnerStatus) {
			return ErrInvalidPartnerTransition
		}
		if !isAllowedDeliveryTransition(current.DeliveryStatus, deliveryStatus) {
			return ErrInvalidDeliveryTransition
		}
		return writeOrderStateTx(ctx, tx, orderID, current, nextStatus, partnerStatus, deliveryStatus, deliveryNotice)
	})
}

// UpdateOrderByAdmin lets an admin set any valid combination of statuses, with
// one guard: cancelled orders cannot be reopened because their stock has been
// released.
func (s *Store) UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error {
	if !validOrderStatuses[status] || !validPartnerStatuses[partnerStatus] || !validDeliveryStatuses[deliveryStatus] {
		return ErrInvalidOrderState
	}
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := lockOrderTx(ctx, tx, orderID)
		if err != nil {
			return err
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

func (s *Store) GetPartnerOrderSummary() (PartnerOrderSummary, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	var summary PartnerOrderSummary
	err := s.DB.QueryRowContext(ctx,
		`SELECT
		    count(*) FILTER (WHERE partner_status = 'new'),
		    count(*) FILTER (WHERE partner_status IN ('accepted', 'packing')),
		    count(*) FILTER (WHERE partner_status = 'dispatched'),
		    count(*) FILTER (WHERE partner_status = 'completed'),
		    count(*) FILTER (WHERE partner_status IN ('new', 'accepted') AND created_at <= now() - interval '2 hours')
		 FROM orders
		 WHERE status <> 'cancelled'`,
	).Scan(&summary.NewCount, &summary.InProgressCount, &summary.DispatchedCount, &summary.CompletedCount, &summary.OverdueCount)
	return summary, err
}

func (s *Store) GetOrderStatusCounts() (map[string]int, error) {
	ctx, cancel := s.ctx()
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
func (s *Store) SumOrderRevenue() (money.Cents, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	var total money.Cents
	err := s.DB.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(total_cents), 0) FROM orders WHERE status <> 'cancelled'",
	).Scan(&total)
	return total, err
}
