package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	stockMovementInitial      = "initial"
	stockMovementAdjustment   = "adjustment"
	stockMovementReservation  = "reservation"
	stockMovementRelease      = "release"
	stockMovementAllocation   = "allocation"
	stockMovementDeallocation = "deallocation"
	stockMovementShipment     = "shipment"
)

// defaultWarehouseID returns the warehouse that new stock is booked into and
// orders are sourced from.
func defaultWarehouseID(ctx context.Context, q querier) (int, error) {
	var id int
	err := q.QueryRowContext(ctx, "SELECT id FROM warehouses WHERE is_default").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("no default warehouse configured")
	}
	return id, err
}

// lockInventoryStock returns the inventory row for update, creating an empty
// row when the product has never been stocked in the warehouse.
func lockInventoryStock(ctx context.Context, tx *sql.Tx, productID, warehouseID int) (*InventoryStock, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO inventory_stocks (product_id, warehouse_id) VALUES ($1, $2)
		 ON CONFLICT (product_id, warehouse_id) DO NOTHING`,
		productID, warehouseID,
	); err != nil {
		return nil, err
	}

	stock := &InventoryStock{}
	err := tx.QueryRowContext(ctx,
		`SELECT product_id, warehouse_id, on_hand_quantity, reserved_quantity, allocated_quantity, updated_at
		 FROM inventory_stocks
		 WHERE product_id = $1 AND warehouse_id = $2
		 FOR UPDATE`,
		productID, warehouseID,
	).Scan(&stock.ProductID, &stock.WarehouseID, &stock.OnHandQuantity, &stock.ReservedQuantity, &stock.AllocatedQuantity, &stock.UpdatedAt)
	if err != nil {
		return nil, err
	}
	stock.AvailableQuantity = stock.OnHandQuantity - stock.ReservedQuantity - stock.AllocatedQuantity
	return stock, nil
}

func recordStockMovement(ctx context.Context, q querier, productID, warehouseID int, movementType string, quantityDelta int, reference string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, quantity_delta, reference)
		 VALUES ($1, $2, $3, $4, $5)`,
		productID, warehouseID, movementType, quantityDelta, strings.TrimSpace(reference),
	)
	return err
}

type releasedReservation struct {
	ProductID      int
	WarehouseID    int
	Quantity       int
	ReservationKey string
}

// releaseReservationsTx moves matching active reservations to newStatus and
// returns their quantities to available stock. The predicate is appended to a
// WHERE clause that already restricts to active reservations; its parameters
// start at $2.
func releaseReservationsTx(ctx context.Context, tx *sql.Tx, newStatus, predicate string, args ...any) (int, error) {
	rows, err := tx.QueryContext(ctx,
		`UPDATE stock_reservations
		 SET status = $1, released_at = now()
		 WHERE status = 'active' AND `+predicate+`
		 RETURNING product_id, warehouse_id, quantity, reservation_key`,
		append([]any{newStatus}, args...)...,
	)
	if err != nil {
		return 0, err
	}
	released := []releasedReservation{}
	for rows.Next() {
		var r releasedReservation
		if err := rows.Scan(&r.ProductID, &r.WarehouseID, &r.Quantity, &r.ReservationKey); err != nil {
			rows.Close()
			return 0, err
		}
		released = append(released, r)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	// Touch inventory rows in a stable order to avoid deadlocks with
	// concurrent reservers.
	sort.Slice(released, func(i, j int) bool {
		if released[i].ProductID != released[j].ProductID {
			return released[i].ProductID < released[j].ProductID
		}
		return released[i].WarehouseID < released[j].WarehouseID
	})

	reason := "reservation released"
	if newStatus == StockReservationStatusExpired {
		reason = "reservation expired"
	}
	for _, r := range released {
		if _, err := tx.ExecContext(ctx,
			`UPDATE inventory_stocks
			 SET reserved_quantity = GREATEST(reserved_quantity - $1, 0)
			 WHERE product_id = $2 AND warehouse_id = $3`,
			r.Quantity, r.ProductID, r.WarehouseID,
		); err != nil {
			return 0, err
		}
		if err := recordStockMovement(ctx, tx, r.ProductID, r.WarehouseID, stockMovementRelease, r.Quantity,
			fmt.Sprintf("%s:%s", reason, r.ReservationKey)); err != nil {
			return 0, err
		}
	}
	return len(released), nil
}

// ReserveCartForCheckout holds stock for every cart line under reservationKey
// until expiresAt. Any reservations the user already holds are released first,
// so a user only ever holds stock for one checkout.
func (s *Store) ReserveCartForCheckout(userID int, reservationKey string, expiresAt time.Time) error {
	key := strings.TrimSpace(reservationKey)
	if key == "" {
		return errors.New("reservation key is required")
	}
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := releaseReservationsTx(ctx, tx, StockReservationStatusReleased, "user_id = $2", userID); err != nil {
			return err
		}

		lines, err := cartLinesTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			return ErrCartEmpty
		}

		warehouseID, err := defaultWarehouseID(ctx, tx)
		if err != nil {
			return err
		}

		for _, line := range lines {
			stock, err := lockInventoryStock(ctx, tx, line.ProductID, warehouseID)
			if err != nil {
				return err
			}
			if line.Quantity > stock.AvailableQuantity {
				return fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE inventory_stocks SET reserved_quantity = reserved_quantity + $1
				 WHERE product_id = $2 AND warehouse_id = $3`,
				line.Quantity, line.ProductID, warehouseID,
			); err != nil {
				return wrapDBError(err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO stock_reservations (product_id, warehouse_id, user_id, reservation_key, quantity, expires_at)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				line.ProductID, warehouseID, userID, key, line.Quantity, expiresAt.UTC(),
			); err != nil {
				return err
			}
			if err := recordStockMovement(ctx, tx, line.ProductID, warehouseID, stockMovementReservation, -line.Quantity, "reservation:"+key); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReleaseExpiredReservations returns expired reservation holds to available
// stock and expires their open checkouts. It is run periodically.
func (s *Store) ReleaseExpiredReservations() (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	released := 0
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		count, err := releaseReservationsTx(ctx, tx, StockReservationStatusExpired, "expires_at <= now()")
		if err != nil {
			return err
		}
		released = count
		_, err = tx.ExecContext(ctx,
			"UPDATE checkouts SET status = $1 WHERE status = $2 AND expires_at <= now()",
			CheckoutStatusExpired, CheckoutStatusOpen,
		)
		return err
	})
	return released, err
}

// ReleaseUserReservations drops the user's stock holds and cancels their open
// checkouts. It runs whenever the cart changes so a checkout never places an
// order for a stale snapshot of the cart.
func (s *Store) ReleaseUserReservations(userID int) error {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		return invalidateCheckoutsTx(ctx, tx, userID)
	})
}

func invalidateCheckoutsTx(ctx context.Context, tx *sql.Tx, userID int) error {
	if _, err := releaseReservationsTx(ctx, tx, StockReservationStatusReleased, "user_id = $2", userID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		"UPDATE checkouts SET status = $1 WHERE user_id = $2 AND status = $3",
		CheckoutStatusCancelled, userID, CheckoutStatusOpen,
	)
	return err
}

func (s *Store) ListActiveStockReservationsByKey(userID int, reservationKey string) ([]StockReservation, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, product_id, warehouse_id, user_id, reservation_key, quantity, status, expires_at, released_at, created_at
		 FROM stock_reservations
		 WHERE user_id = $1 AND reservation_key = $2 AND status = 'active'
		 ORDER BY id`,
		userID, strings.TrimSpace(reservationKey),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reservations := []StockReservation{}
	for rows.Next() {
		var r StockReservation
		if err := rows.Scan(&r.ID, &r.ProductID, &r.WarehouseID, &r.UserID, &r.ReservationKey, &r.Quantity, &r.Status, &r.ExpiresAt, &r.ReleasedAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		reservations = append(reservations, r)
	}
	return reservations, rows.Err()
}

func (s *Store) ListInventoryStocksByProductID(productID int) ([]InventoryStock, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT product_id, warehouse_id, on_hand_quantity, reserved_quantity, allocated_quantity, updated_at
		 FROM inventory_stocks
		 WHERE product_id = $1
		 ORDER BY warehouse_id`,
		productID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stocks := []InventoryStock{}
	for rows.Next() {
		var stock InventoryStock
		if err := rows.Scan(&stock.ProductID, &stock.WarehouseID, &stock.OnHandQuantity, &stock.ReservedQuantity, &stock.AllocatedQuantity, &stock.UpdatedAt); err != nil {
			return nil, err
		}
		stock.AvailableQuantity = stock.OnHandQuantity - stock.ReservedQuantity - stock.AllocatedQuantity
		stocks = append(stocks, stock)
	}
	return stocks, rows.Err()
}

func (s *Store) ListStockMovementsByProductID(productID int) ([]StockMovement, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, product_id, warehouse_id, movement_type, quantity_delta, reference, created_at
		 FROM stock_movements
		 WHERE product_id = $1
		 ORDER BY id`,
		productID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	movements := []StockMovement{}
	for rows.Next() {
		var m StockMovement
		if err := rows.Scan(&m.ID, &m.ProductID, &m.WarehouseID, &m.MovementType, &m.QuantityDelta, &m.Reference, &m.CreatedAt); err != nil {
			return nil, err
		}
		movements = append(movements, m)
	}
	return movements, rows.Err()
}

type orderStockLine struct {
	ProductID   int
	WarehouseID int
	Quantity    int
}

func orderStockLinesTx(ctx context.Context, tx *sql.Tx, orderID int) ([]orderStockLine, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT product_id, warehouse_id, quantity
		 FROM order_items WHERE order_id = $1
		 ORDER BY product_id, warehouse_id`,
		orderID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []orderStockLine{}
	for rows.Next() {
		var line orderStockLine
		if err := rows.Scan(&line.ProductID, &line.WarehouseID, &line.Quantity); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// applyOrderInventoryTransition moves an order's stock between allocated,
// shipped, and released. It is idempotent: calling it with the order's current
// state does nothing.
//
//	allocated -> shipped:  on_hand and allocated both drop (goods left the warehouse)
//	allocated -> released: allocated drops (order cancelled before shipping)
//
// Shipped and released are terminal for inventory.
func applyOrderInventoryTransition(ctx context.Context, tx *sql.Tx, orderID int, current, target string) error {
	if current == target || current != "allocated" {
		return nil
	}

	lines, err := orderStockLinesTx(ctx, tx, orderID)
	if err != nil {
		return err
	}

	for _, line := range lines {
		var (
			stmt         string
			movementType string
			delta        int
		)
		switch target {
		case "shipped":
			stmt = `UPDATE inventory_stocks
			        SET on_hand_quantity = on_hand_quantity - $1, allocated_quantity = allocated_quantity - $1
			        WHERE product_id = $2 AND warehouse_id = $3`
			movementType, delta = stockMovementShipment, -line.Quantity
		case "released":
			stmt = `UPDATE inventory_stocks
			        SET allocated_quantity = allocated_quantity - $1
			        WHERE product_id = $2 AND warehouse_id = $3`
			movementType, delta = stockMovementDeallocation, line.Quantity
		default:
			return fmt.Errorf("unknown inventory state %q", target)
		}
		if _, err := tx.ExecContext(ctx, stmt, line.Quantity, line.ProductID, line.WarehouseID); err != nil {
			return wrapDBError(err)
		}
		if err := recordStockMovement(ctx, tx, line.ProductID, line.WarehouseID, movementType, delta, fmt.Sprintf("order:%d", orderID)); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, "UPDATE orders SET inventory_state = $1 WHERE id = $2", target, orderID)
	return err
}

// inventoryStateFor maps order statuses to the inventory state they require.
func inventoryStateFor(status, partnerStatus string) string {
	switch {
	case status == OrderStatusCancelled || partnerStatus == "cancelled":
		return "released"
	case status == OrderStatusShipped || status == OrderStatusDelivered ||
		partnerStatus == "dispatched" || partnerStatus == "completed":
		return "shipped"
	default:
		return "allocated"
	}
}
