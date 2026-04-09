package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	defaultWarehouseID   = 1
	defaultWarehouseName = "Default Warehouse"
	defaultWarehouseSlug = "default"

	stockMovementInitial     = "initial"
	stockMovementAdjustment  = "adjustment"
	stockMovementReservation = "reservation"
	stockMovementRelease     = "release"
	stockMovementAllocation  = "allocation"
)

const (
	StockReservationStatusActive    = "active"
	StockReservationStatusReleased  = "released"
	StockReservationStatusExpired   = "expired"
	StockReservationStatusConverted = "converted"
)

func ensureDefaultWarehouseTx(tx *sql.Tx) (int64, error) {
	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO warehouses (id, name, slug, is_default)
		 VALUES (?, ?, ?, 1)`,
		defaultWarehouseID, defaultWarehouseName, defaultWarehouseSlug,
	); err != nil {
		return 0, err
	}

	var warehouseID int64
	if err := tx.QueryRow(
		"SELECT id FROM warehouses WHERE slug = ?",
		defaultWarehouseSlug,
	).Scan(&warehouseID); err != nil {
		return 0, err
	}
	return warehouseID, nil
}

func ensureInventoryStockTx(tx *sql.Tx, productID int, fallbackAvailable int) (int64, error) {
	warehouseID, err := ensureDefaultWarehouseTx(tx)
	if err != nil {
		return 0, err
	}

	if fallbackAvailable < 0 {
		fallbackAvailable = 0
	}

	if _, err := tx.Exec(
		`INSERT OR IGNORE INTO inventory_stocks (product_id, warehouse_id, on_hand_quantity, reserved_quantity, allocated_quantity)
		 VALUES (?, ?, ?, 0, 0)`,
		productID, warehouseID, fallbackAvailable,
	); err != nil {
		return 0, err
	}

	return warehouseID, nil
}

func ensureDefaultWarehouseConn(conn *sql.Conn) (int64, error) {
	ctx := context.Background()

	if _, err := conn.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO warehouses (id, name, slug, is_default)
		 VALUES (?, ?, ?, 1)`,
		defaultWarehouseID, defaultWarehouseName, defaultWarehouseSlug,
	); err != nil {
		return 0, err
	}

	var warehouseID int64
	if err := conn.QueryRowContext(
		ctx,
		"SELECT id FROM warehouses WHERE slug = ?",
		defaultWarehouseSlug,
	).Scan(&warehouseID); err != nil {
		return 0, err
	}
	return warehouseID, nil
}

func ensureInventoryStockConn(conn *sql.Conn, productID int, fallbackAvailable int) (int64, error) {
	warehouseID, err := ensureDefaultWarehouseConn(conn)
	if err != nil {
		return 0, err
	}

	if fallbackAvailable < 0 {
		fallbackAvailable = 0
	}

	if _, err := conn.ExecContext(
		context.Background(),
		`INSERT OR IGNORE INTO inventory_stocks (product_id, warehouse_id, on_hand_quantity, reserved_quantity, allocated_quantity)
		 VALUES (?, ?, ?, 0, 0)`,
		productID, warehouseID, fallbackAvailable,
	); err != nil {
		return 0, err
	}

	return warehouseID, nil
}

func getInventoryStockTx(tx *sql.Tx, productID int, warehouseID int64) (*InventoryStock, error) {
	stock := &InventoryStock{}
	err := tx.QueryRow(
		`SELECT id, product_id, warehouse_id, on_hand_quantity, reserved_quantity, allocated_quantity, created_at, updated_at
		 FROM inventory_stocks
		 WHERE product_id = ? AND warehouse_id = ?`,
		productID, warehouseID,
	).Scan(
		&stock.ID,
		&stock.ProductID,
		&stock.WarehouseID,
		&stock.OnHandQuantity,
		&stock.ReservedQuantity,
		&stock.AllocatedQuantity,
		&stock.CreatedAt,
		&stock.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stock.AvailableQuantity = stock.OnHandQuantity - stock.ReservedQuantity - stock.AllocatedQuantity
	return stock, nil
}

func syncProductStockQuantityTx(tx *sql.Tx, productID int) error {
	_, err := tx.Exec(
		`UPDATE products
		 SET stock_quantity = MAX(
		     COALESCE((
		         SELECT SUM(on_hand_quantity - reserved_quantity - allocated_quantity)
		         FROM inventory_stocks
		         WHERE product_id = ?
		     ), 0),
		     0
		 ),
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		productID, productID,
	)
	return err
}

func syncProductStockQuantity(conn *sql.Conn, productID int) error {
	_, err := conn.ExecContext(
		context.Background(),
		`UPDATE products
		 SET stock_quantity = MAX(
		     COALESCE((
		         SELECT SUM(on_hand_quantity - reserved_quantity - allocated_quantity)
		         FROM inventory_stocks
		         WHERE product_id = ?
		     ), 0),
		     0
		 ),
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		productID, productID,
	)
	return err
}

func recordStockMovementTx(tx *sql.Tx, productID int, warehouseID int64, movementType string, quantityDelta int, note string) error {
	_, err := tx.Exec(
		`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, quantity_delta, note)
		 VALUES (?, ?, ?, ?, ?)`,
		productID, warehouseID, movementType, quantityDelta, nullIfEmpty(note),
	)
	return err
}

func recordStockMovement(conn *sql.Conn, productID int, warehouseID int64, movementType string, quantityDelta int, note string) error {
	_, err := conn.ExecContext(
		context.Background(),
		`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, quantity_delta, note)
		 VALUES (?, ?, ?, ?, ?)`,
		productID, warehouseID, movementType, quantityDelta, nullIfEmpty(note),
	)
	return err
}

func expireActiveStockReservationsTx(tx *sql.Tx, now time.Time) error {
	rows, err := tx.Query(
		`SELECT id, product_id, warehouse_id, quantity
		 FROM stock_reservations
		 WHERE status = ? AND expires_at IS NOT NULL AND expires_at <= ?`,
		StockReservationStatusActive,
		now.UTC(),
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type expiredReservation struct {
		ID          int
		ProductID   int
		WarehouseID int
		Quantity    int
	}
	expired := []expiredReservation{}
	for rows.Next() {
		reservation := expiredReservation{}
		if err := rows.Scan(&reservation.ID, &reservation.ProductID, &reservation.WarehouseID, &reservation.Quantity); err != nil {
			return err
		}
		expired = append(expired, reservation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, reservation := range expired {
		if _, err := tx.Exec(
			`UPDATE inventory_stocks
			 SET reserved_quantity = CASE
			     WHEN reserved_quantity >= ? THEN reserved_quantity - ?
			     ELSE 0
			 END,
			     updated_at = CURRENT_TIMESTAMP
			 WHERE product_id = ? AND warehouse_id = ?`,
			reservation.Quantity,
			reservation.Quantity,
			reservation.ProductID,
			reservation.WarehouseID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE stock_reservations
			 SET status = ?, released_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			StockReservationStatusExpired,
			reservation.ID,
		); err != nil {
			return err
		}
		if err := recordStockMovementTx(tx, reservation.ProductID, int64(reservation.WarehouseID), stockMovementRelease, reservation.Quantity, "reservation expired"); err != nil {
			return err
		}
		if err := syncProductStockQuantityTx(tx, reservation.ProductID); err != nil {
			return err
		}
	}

	return nil
}

func expireActiveStockReservations(conn *sql.Conn, now time.Time) error {
	ctx := context.Background()

	rows, err := conn.QueryContext(
		ctx,
		`SELECT id, product_id, warehouse_id, quantity
		 FROM stock_reservations
		 WHERE status = ? AND expires_at IS NOT NULL AND expires_at <= ?`,
		StockReservationStatusActive,
		now.UTC(),
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type expiredReservation struct {
		ID          int
		ProductID   int
		WarehouseID int
		Quantity    int
	}
	expired := []expiredReservation{}
	for rows.Next() {
		reservation := expiredReservation{}
		if err := rows.Scan(&reservation.ID, &reservation.ProductID, &reservation.WarehouseID, &reservation.Quantity); err != nil {
			return err
		}
		expired = append(expired, reservation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, reservation := range expired {
		if _, err := conn.ExecContext(
			ctx,
			`UPDATE inventory_stocks
			 SET reserved_quantity = CASE
			     WHEN reserved_quantity >= ? THEN reserved_quantity - ?
			     ELSE 0
			 END,
			     updated_at = CURRENT_TIMESTAMP
			 WHERE product_id = ? AND warehouse_id = ?`,
			reservation.Quantity,
			reservation.Quantity,
			reservation.ProductID,
			reservation.WarehouseID,
		); err != nil {
			return err
		}
		if _, err := conn.ExecContext(
			ctx,
			`UPDATE stock_reservations
			 SET status = ?, released_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			StockReservationStatusExpired,
			reservation.ID,
		); err != nil {
			return err
		}
		if err := recordStockMovement(conn, reservation.ProductID, int64(reservation.WarehouseID), stockMovementRelease, reservation.Quantity, "reservation expired"); err != nil {
			return err
		}
		if err := syncProductStockQuantity(conn, reservation.ProductID); err != nil {
			return err
		}
	}

	return nil
}

func releaseActiveStockReservationsForUserTx(tx *sql.Tx, userID int) error {
	rows, err := tx.Query(
		`SELECT id, product_id, warehouse_id, quantity, reservation_key
		 FROM stock_reservations
		 WHERE user_id = ? AND status = ?`,
		userID,
		StockReservationStatusActive,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type activeReservation struct {
		ID             int
		ProductID      int
		WarehouseID    int
		Quantity       int
		ReservationKey sql.NullString
	}
	reservations := []activeReservation{}
	for rows.Next() {
		reservation := activeReservation{}
		if err := rows.Scan(&reservation.ID, &reservation.ProductID, &reservation.WarehouseID, &reservation.Quantity, &reservation.ReservationKey); err != nil {
			return err
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, reservation := range reservations {
		if _, err := tx.Exec(
			`UPDATE inventory_stocks
			 SET reserved_quantity = CASE
			     WHEN reserved_quantity >= ? THEN reserved_quantity - ?
			     ELSE 0
			 END,
			     updated_at = CURRENT_TIMESTAMP
			 WHERE product_id = ? AND warehouse_id = ?`,
			reservation.Quantity,
			reservation.Quantity,
			reservation.ProductID,
			reservation.WarehouseID,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			`UPDATE stock_reservations
			 SET status = ?, released_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			StockReservationStatusReleased,
			reservation.ID,
		); err != nil {
			return err
		}
		note := "reservation released"
		if reservation.ReservationKey.Valid && strings.TrimSpace(reservation.ReservationKey.String) != "" {
			note = fmt.Sprintf("reservation:%s released", reservation.ReservationKey.String)
		}
		if err := recordStockMovementTx(tx, reservation.ProductID, int64(reservation.WarehouseID), stockMovementRelease, reservation.Quantity, note); err != nil {
			return err
		}
		if err := syncProductStockQuantityTx(tx, reservation.ProductID); err != nil {
			return err
		}
	}

	return nil
}

func releaseActiveStockReservationsForUser(conn *sql.Conn, userID int) error {
	ctx := context.Background()

	rows, err := conn.QueryContext(
		ctx,
		`SELECT id, product_id, warehouse_id, quantity, reservation_key
		 FROM stock_reservations
		 WHERE user_id = ? AND status = ?`,
		userID,
		StockReservationStatusActive,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type activeReservation struct {
		ID             int
		ProductID      int
		WarehouseID    int
		Quantity       int
		ReservationKey sql.NullString
	}
	reservations := []activeReservation{}
	for rows.Next() {
		reservation := activeReservation{}
		if err := rows.Scan(&reservation.ID, &reservation.ProductID, &reservation.WarehouseID, &reservation.Quantity, &reservation.ReservationKey); err != nil {
			return err
		}
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, reservation := range reservations {
		if _, err := conn.ExecContext(
			ctx,
			`UPDATE inventory_stocks
			 SET reserved_quantity = CASE
			     WHEN reserved_quantity >= ? THEN reserved_quantity - ?
			     ELSE 0
			 END,
			     updated_at = CURRENT_TIMESTAMP
			 WHERE product_id = ? AND warehouse_id = ?`,
			reservation.Quantity,
			reservation.Quantity,
			reservation.ProductID,
			reservation.WarehouseID,
		); err != nil {
			return err
		}
		if _, err := conn.ExecContext(
			ctx,
			`UPDATE stock_reservations
			 SET status = ?, released_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			StockReservationStatusReleased,
			reservation.ID,
		); err != nil {
			return err
		}
		note := "reservation released"
		if reservation.ReservationKey.Valid && strings.TrimSpace(reservation.ReservationKey.String) != "" {
			note = fmt.Sprintf("reservation:%s released", reservation.ReservationKey.String)
		}
		if err := recordStockMovement(conn, reservation.ProductID, int64(reservation.WarehouseID), stockMovementRelease, reservation.Quantity, note); err != nil {
			return err
		}
		if err := syncProductStockQuantity(conn, reservation.ProductID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) ReserveCartForCheckout(userID int, reservationKey string, expiresAt time.Time) error {
	key := strings.TrimSpace(reservationKey)
	if key == "" {
		return fmt.Errorf("reservation key is required")
	}

	conn, err := s.DB.Conn(context.Background())
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		return err
	}

	var doRollback bool
	defer func() {
		if doRollback {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	if err := expireActiveStockReservations(conn, time.Now().UTC()); err != nil {
		doRollback = true
		return err
	}
	if err := releaseActiveStockReservationsForUser(conn, userID); err != nil {
		doRollback = true
		return err
	}

	var cartID int64
	err = conn.QueryRowContext(context.Background(), "SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID)
	if err == sql.ErrNoRows {
		doRollback = true
		return ErrCartEmpty
	}
	if err != nil {
		doRollback = true
		return err
	}

	type reservationLine struct {
		ProductID     int
		ProductName   string
		Quantity      int
		StockQuantity int
	}

	rows, err := conn.QueryContext(context.Background(),
		`SELECT p.id, p.name, ci.quantity,
		        COALESCE(inv.available_quantity, p.stock_quantity) AS available_quantity
		 FROM cart_items ci
		 JOIN products p ON p.id = ci.product_id
		 LEFT JOIN (
		     SELECT product_id, SUM(on_hand_quantity - reserved_quantity - allocated_quantity) AS available_quantity
		     FROM inventory_stocks
		     GROUP BY product_id
		 ) inv ON inv.product_id = p.id
		 WHERE ci.cart_id = ? AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		cartID,
	)
	if err != nil {
		doRollback = true
		return err
	}

	lines := []reservationLine{}
	for rows.Next() {
		line := reservationLine{}
		if err := rows.Scan(&line.ProductID, &line.ProductName, &line.Quantity, &line.StockQuantity); err != nil {
			_ = rows.Close()
			doRollback = true
			return err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		doRollback = true
		return err
	}
	if err := rows.Close(); err != nil {
		doRollback = true
		return err
	}
	if len(lines) == 0 {
		doRollback = true
		return ErrCartEmpty
	}

	for _, line := range lines {
		warehouseID, err := ensureInventoryStockConn(conn, line.ProductID, line.StockQuantity)
		if err != nil {
			doRollback = true
			return err
		}

		var stock InventoryStock
		stockErr := conn.QueryRowContext(context.Background(),
			`SELECT id, product_id, warehouse_id, on_hand_quantity, reserved_quantity, allocated_quantity, created_at, updated_at
			 FROM inventory_stocks
			 WHERE product_id = ? AND warehouse_id = ?`,
			line.ProductID, warehouseID,
		).Scan(
			&stock.ID,
			&stock.ProductID,
			&stock.WarehouseID,
			&stock.OnHandQuantity,
			&stock.ReservedQuantity,
			&stock.AllocatedQuantity,
			&stock.CreatedAt,
			&stock.UpdatedAt,
		)
		if stockErr == sql.ErrNoRows {
			doRollback = true
			return fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
		}
		if stockErr != nil {
			doRollback = true
			return stockErr
		}
		stock.AvailableQuantity = stock.OnHandQuantity - stock.ReservedQuantity - stock.AllocatedQuantity

		if line.Quantity > stock.AvailableQuantity {
			doRollback = true
			return fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
		}

		result, updateErr := conn.ExecContext(context.Background(),
			`UPDATE inventory_stocks
			 SET reserved_quantity = reserved_quantity + ?, updated_at = CURRENT_TIMESTAMP
			 WHERE product_id = ? AND warehouse_id = ?
			   AND (on_hand_quantity - reserved_quantity - allocated_quantity) >= ?`,
			line.Quantity,
			line.ProductID,
			warehouseID,
			line.Quantity,
		)
		if updateErr != nil {
			doRollback = true
			return updateErr
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			doRollback = true
			return rowsErr
		}
		if affected == 0 {
			doRollback = true
			return fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
		}

		if _, err := conn.ExecContext(context.Background(),
			`INSERT INTO stock_reservations (product_id, warehouse_id, user_id, reservation_key, quantity, status, expires_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			line.ProductID,
			warehouseID,
			userID,
			key,
			line.Quantity,
			StockReservationStatusActive,
			expiresAt.UTC(),
		); err != nil {
			doRollback = true
			return err
		}
		if err := recordStockMovement(conn, line.ProductID, warehouseID, stockMovementReservation, -line.Quantity, fmt.Sprintf("reservation:%s", key)); err != nil {
			doRollback = true
			return err
		}
		if err := syncProductStockQuantity(conn, line.ProductID); err != nil {
			doRollback = true
			return err
		}
	}

	if _, err := conn.ExecContext(context.Background(), "COMMIT"); err != nil {
		doRollback = true
		return err
	}

	return nil
}

func (s *Store) ListActiveStockReservationsByKey(userID int, reservationKey string) ([]StockReservation, error) {
	rows, err := s.DB.Query(
		`SELECT id, product_id, warehouse_id, user_id, reservation_key, quantity, status, expires_at, released_at, created_at, updated_at
		 FROM stock_reservations
		 WHERE user_id = ? AND reservation_key = ? AND status = ?
		 ORDER BY id`,
		userID,
		strings.TrimSpace(reservationKey),
		StockReservationStatusActive,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reservations := []StockReservation{}
	for rows.Next() {
		reservation := StockReservation{}
		if err := rows.Scan(
			&reservation.ID,
			&reservation.ProductID,
			&reservation.WarehouseID,
			&reservation.UserID,
			&reservation.ReservationKey,
			&reservation.Quantity,
			&reservation.Status,
			&reservation.ExpiresAt,
			&reservation.ReleasedAt,
			&reservation.CreatedAt,
			&reservation.UpdatedAt,
		); err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	return reservations, rows.Err()
}

func (s *Store) ListInventoryStocksByProductID(productID int) ([]InventoryStock, error) {
	rows, err := s.DB.Query(
		`SELECT id, product_id, warehouse_id, on_hand_quantity, reserved_quantity, allocated_quantity, created_at, updated_at
		 FROM inventory_stocks
		 WHERE product_id = ?
		 ORDER BY warehouse_id`,
		productID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stocks := []InventoryStock{}
	for rows.Next() {
		stock := InventoryStock{}
		if err := rows.Scan(
			&stock.ID,
			&stock.ProductID,
			&stock.WarehouseID,
			&stock.OnHandQuantity,
			&stock.ReservedQuantity,
			&stock.AllocatedQuantity,
			&stock.CreatedAt,
			&stock.UpdatedAt,
		); err != nil {
			return nil, err
		}
		stock.AvailableQuantity = stock.OnHandQuantity - stock.ReservedQuantity - stock.AllocatedQuantity
		stocks = append(stocks, stock)
	}
	return stocks, rows.Err()
}

func (s *Store) ListStockMovementsByProductID(productID int) ([]StockMovement, error) {
	rows, err := s.DB.Query(
		`SELECT id, product_id, warehouse_id, movement_type, quantity_delta, note, created_at
		 FROM stock_movements
		 WHERE product_id = ?
		 ORDER BY id`,
		productID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	movements := []StockMovement{}
	for rows.Next() {
		movement := StockMovement{}
		if err := rows.Scan(
			&movement.ID,
			&movement.ProductID,
			&movement.WarehouseID,
			&movement.MovementType,
			&movement.QuantityDelta,
			&movement.Note,
			&movement.CreatedAt,
		); err != nil {
			return nil, err
		}
		movements = append(movements, movement)
	}
	return movements, rows.Err()
}
