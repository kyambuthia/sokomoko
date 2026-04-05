package db

import "database/sql"

const (
	defaultWarehouseID   = 1
	defaultWarehouseName = "Default Warehouse"
	defaultWarehouseSlug = "default"

	stockMovementInitial    = "initial"
	stockMovementAdjustment = "adjustment"
	stockMovementAllocation = "allocation"
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

func recordStockMovementTx(tx *sql.Tx, productID int, warehouseID int64, movementType string, quantityDelta int, note string) error {
	_, err := tx.Exec(
		`INSERT INTO stock_movements (product_id, warehouse_id, movement_type, quantity_delta, note)
		 VALUES (?, ?, ?, ?, ?)`,
		productID, warehouseID, movementType, quantityDelta, nullIfEmpty(note),
	)
	return err
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
