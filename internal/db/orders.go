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
		"dispatched": {"dispatched": true},
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
		`INSERT INTO orders (user_id, subtotal_cents, shipping_cents, tax_cents, total_cents, delivery_address, delivery_notice, currency)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE(NULLIF($8, ''), (SELECT currency FROM commerce_settings WHERE id = 1)))
		 RETURNING id`,
		userID, int64(totals.Subtotal), int64(totals.ShippingFee), int64(totals.TaxAmount), int64(totals.Total),
		strings.TrimSpace(address), strings.TrimSpace(notice), totals.Currency,
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
func (s *Store) PlaceOrderFromCart(ctx context.Context, userID int, deliveryAddress string) (int64, error) {
	if strings.TrimSpace(deliveryAddress) == "" {
		return 0, ErrDeliveryAddressRequired
	}
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var orderID int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := lockCartTx(ctx, tx, userID); err != nil {
			return err
		}
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
			lineTotal, err := c.UnitPrice.Mul(int64(c.Quantity))
			if err != nil {
				return err
			}
			subtotal, err = subtotal.Add(lineTotal)
			if err != nil {
				return err
			}
			lines = append(lines, orderLine(c))
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
       o.subtotal_cents, o.shipping_cents, o.tax_cents, o.total_cents, o.created_at, o.updated_at,
       COALESCE(pay.method, ''), COALESCE(pay.status, ''), COALESCE(pay.review_required, FALSE)
FROM orders o
JOIN users u ON u.id = o.user_id
LEFT JOIN LATERAL (
    SELECT p.method, p.status,
           EXISTS (
               SELECT 1 FROM payment_exceptions e
               WHERE e.payment_id = p.id
                 AND e.reason IN ('amount_mismatch', 'amount_unverified')
                 AND e.status <> 'refunded'
                 AND (p.status <> 'captured' OR e.affects_payment)
           ) AS review_required
    FROM payments p WHERE p.order_id = o.id ORDER BY p.id DESC LIMIT 1
) pay ON true`

// paymentSettledSQL matches orders a partner may fulfil: cash on delivery, or
// captured online payments without an unresolved amount mismatch.
const paymentSettledSQL = `NOT EXISTS (
    SELECT 1 FROM payments pp
    WHERE pp.order_id = o.id AND pp.method <> 'cash_on_delivery'
      AND (pp.status <> 'captured' OR EXISTS (
          SELECT 1 FROM payment_exceptions e
          WHERE e.payment_id = pp.id AND e.affects_payment AND e.status <> 'refunded'
      )))`

func orderPaymentSettledTx(ctx context.Context, tx *sql.Tx, orderID int) (bool, error) {
	var settled bool
	err := tx.QueryRowContext(ctx,
		"SELECT "+paymentSettledSQL+" FROM orders o WHERE o.id = $1", orderID,
	).Scan(&settled)
	return settled, err
}

func scanOrder(row interface{ Scan(...any) error }) (*Order, error) {
	order := &Order{}
	err := row.Scan(
		&order.ID, &order.UserID, &order.CustomerName, &order.Status, &order.PartnerStatus,
		&order.DeliveryStatus, &order.InventoryState, &order.DeliveryAddress, &order.DeliveryNotice,
		&order.Currency, &order.Subtotal, &order.ShippingFee, &order.TaxAmount, &order.TotalAmount,
		&order.CreatedAt, &order.UpdatedAt, &order.PaymentMethod, &order.PaymentStatus, &order.PaymentReviewRequired,
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
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	orders, err := listOrdersQuery(ctx, tx, suffix, args...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err)
	}
	return orders, nil
}

func listOrdersQuery(ctx context.Context, q querier, suffix string, args ...any) ([]Order, error) {
	rows, err := q.QueryContext(ctx, orderSelect+" "+suffix, args...)
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

	items, err := orderItemsByOrderIDsQuery(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = items[orders[i].ID]
	}
	return orders, nil
}

func orderPageHasRows(total, limit, offset int) bool {
	if total == 0 {
		return false
	}
	lastOffset := ((total - 1) / limit) * limit
	return offset <= lastOffset
}

func orderItemsByOrderIDsQuery(ctx context.Context, q querier, orderIDs []int) (map[int][]OrderItem, error) {
	itemsByOrder := map[int][]OrderItem{}
	if len(orderIDs) == 0 {
		return itemsByOrder, nil
	}
	rows, err := q.QueryContext(ctx,
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
func (s *Store) GetOrderForUser(ctx context.Context, userID, orderID int) (*Order, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	orders, err := s.listOrders(ctx, "WHERE o.id = $1 AND o.user_id = $2", orderID, userID)
	if err != nil || len(orders) == 0 {
		return nil, err
	}
	return &orders[0], nil
}

func (s *Store) ListOrdersByUser(ctx context.Context, userID int) ([]Order, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.listOrders(ctx, "WHERE o.user_id = $1 ORDER BY o.created_at DESC, o.id DESC", userID)
}

func (s *Store) ListOrdersByUserPage(ctx context.Context, userID, limit, offset int) ([]Order, int, error) {
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

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM orders WHERE user_id = $1", userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	orders := []Order{}
	if orderPageHasRows(total, limit, offset) {
		orders, err = listOrdersQuery(ctx, tx,
			"WHERE o.user_id = $1 ORDER BY o.created_at DESC, o.id DESC LIMIT $2 OFFSET $3", userID, limit, offset)
		if err != nil {
			return nil, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, wrapDBError(err)
	}
	return orders, total, nil
}
