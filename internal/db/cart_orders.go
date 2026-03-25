package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) ensureCart(userID int) (int64, error) {
	var cartID int64
	err := s.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID)
	if err == nil {
		return cartID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	res, err := s.DB.Exec("INSERT INTO carts (user_id) VALUES (?)", userID)
	if err != nil {
		if queryErr := s.DB.QueryRow("SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID); queryErr == nil {
			return cartID, nil
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AddToCart(userID, productID, quantity int) error {
	if quantity <= 0 {
		return fmt.Errorf("quantity must be greater than zero")
	}
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}

	_, err = s.DB.Exec(
		`INSERT INTO cart_items (cart_id, product_id, quantity)
		 VALUES (?, ?, ?)
		 ON CONFLICT(cart_id, product_id) DO UPDATE SET
		   quantity = quantity + excluded.quantity,
		   updated_at = CURRENT_TIMESTAMP`,
		cartID, productID, quantity,
	)
	return err
}

func (s *Store) UpdateCartQuantity(userID, productID, quantity int) error {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}

	if quantity <= 0 {
		_, err = s.DB.Exec("DELETE FROM cart_items WHERE cart_id = ? AND product_id = ?", cartID, productID)
		return err
	}

	_, err = s.DB.Exec(
		"UPDATE cart_items SET quantity = ?, updated_at = CURRENT_TIMESTAMP WHERE cart_id = ? AND product_id = ?",
		quantity, cartID, productID,
	)
	return err
}

func (s *Store) RemoveFromCart(userID, productID int) error {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("DELETE FROM cart_items WHERE cart_id = ? AND product_id = ?", cartID, productID)
	return err
}

func (s *Store) ClearCart(userID int) error {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("DELETE FROM cart_items WHERE cart_id = ?", cartID)
	return err
}

func (s *Store) GetCartItems(userID int) ([]CartItem, float64, error) {
	cartID, err := s.ensureCart(userID)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.DB.Query(
		`SELECT p.id, p.name, p.slug, p.price, p.stock_quantity, ci.quantity
		 FROM cart_items ci
		 JOIN products p ON p.id = ci.product_id
		 WHERE ci.cart_id = ? AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		cartID,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []CartItem{}
	subtotal := 0.0
	productIDs := []int{}
	for rows.Next() {
		item := CartItem{}
		if err := rows.Scan(
			&item.ProductID,
			&item.ProductName,
			&item.ProductSlug,
			&item.UnitPrice,
			&item.StockQuantity,
			&item.Quantity,
		); err != nil {
			return nil, 0, err
		}
		item.ProductImageURL = "/static/images/placeholder.png"
		item.LineTotal = item.UnitPrice * float64(item.Quantity)
		subtotal += item.LineTotal
		items = append(items, item)
		productIDs = append(productIDs, item.ProductID)
	}

	imagesByProduct, err := s.GetProductImagesByProductIDs(productIDs)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		if imgs := imagesByProduct[items[i].ProductID]; len(imgs) > 0 {
			items[i].ProductImageURL = imgs[0].URL
		}
	}
	return items, subtotal, nil
}

func (s *Store) PlaceOrderFromCart(userID int, deliveryAddress string) (int64, error) {
	return s.placeOrderFromCart(userID, deliveryAddress, 0, "", false)
}

func (s *Store) PlaceOrderFromCartWithPricing(userID int, deliveryAddress string, totalAmount float64, deliveryNotice string) (int64, error) {
	return s.placeOrderFromCart(userID, deliveryAddress, totalAmount, deliveryNotice, true)
}

func (s *Store) placeOrderFromCart(userID int, deliveryAddress string, totalAmount float64, deliveryNotice string, useCustomPricing bool) (int64, error) {
	address := strings.TrimSpace(deliveryAddress)
	if address == "" {
		return 0, ErrDeliveryAddressRequired
	}
	if useCustomPricing && totalAmount < 0 {
		return 0, ErrNegativeTotalAmount
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var cartID int64
	err = tx.QueryRow("SELECT id FROM carts WHERE user_id = ?", userID).Scan(&cartID)
	if err == sql.ErrNoRows {
		return 0, ErrCartEmpty
	}
	if err != nil {
		return 0, err
	}

	type cartLine struct {
		ProductID     int
		ProductName   string
		Quantity      int
		UnitPrice     float64
		StockQuantity int
	}
	lines := []cartLine{}
	rows, err := tx.Query(
		`SELECT p.id, p.name, ci.quantity, p.price, p.stock_quantity
		 FROM cart_items ci
		 JOIN products p ON p.id = ci.product_id
		 WHERE ci.cart_id = ? AND p.deleted_at IS NULL
		 ORDER BY p.name`,
		cartID,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	subtotal := 0.0
	for rows.Next() {
		line := cartLine{}
		if scanErr := rows.Scan(&line.ProductID, &line.ProductName, &line.Quantity, &line.UnitPrice, &line.StockQuantity); scanErr != nil {
			return 0, scanErr
		}
		if line.Quantity > line.StockQuantity {
			return 0, fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
		}
		lines = append(lines, line)
		subtotal += line.UnitPrice * float64(line.Quantity)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(lines) == 0 {
		return 0, ErrCartEmpty
	}

	orderTotal := subtotal
	if useCustomPricing {
		orderTotal = totalAmount
	}
	notice := strings.TrimSpace(deliveryNotice)
	if notice == "" {
		notice = "Order received. Awaiting partner acceptance."
	}

	res, err := tx.Exec(
		`INSERT INTO orders (user_id, status, partner_status, delivery_status, total_amount, delivery_address, delivery_notice)
		 VALUES (?, 'pending', 'new', 'queued', ?, ?, ?)`,
		userID, orderTotal, address, notice,
	)
	if err != nil {
		return 0, err
	}
	orderID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, line := range lines {
		if _, err = tx.Exec(
			`INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price)
			 VALUES (?, ?, ?, ?, ?)`,
			orderID, line.ProductID, line.ProductName, line.Quantity, line.UnitPrice,
		); err != nil {
			return 0, err
		}

		result, updateErr := tx.Exec(
			`UPDATE products
			 SET stock_quantity = stock_quantity - ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ? AND stock_quantity >= ?`,
			line.Quantity, line.ProductID, line.Quantity,
		)
		if updateErr != nil {
			return 0, updateErr
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return 0, rowsErr
		}
		if affected == 0 {
			return 0, fmt.Errorf("%w: %s", ErrInsufficientStock, line.ProductName)
		}
	}

	if _, err = tx.Exec("DELETE FROM cart_items WHERE cart_id = ?", cartID); err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return orderID, nil
}

func (s *Store) listOrderItems(orderID int) ([]OrderItem, error) {
	itemsByOrder, err := s.listOrderItemsByOrderIDs([]int{orderID})
	if err != nil {
		return nil, err
	}
	return itemsByOrder[orderID], nil
}

func (s *Store) listOrderItemsByOrderIDs(orderIDs []int) (map[int][]OrderItem, error) {
	itemsByOrder := map[int][]OrderItem{}
	if len(orderIDs) == 0 {
		return itemsByOrder, nil
	}

	placeholders := make([]string, len(orderIDs))
	args := make([]interface{}, 0, len(orderIDs))
	for i, id := range orderIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(
		`SELECT order_id, product_id, product_name, quantity, unit_price
		 FROM order_items
		 WHERE order_id IN (%s)
		 ORDER BY order_id, id`,
		strings.Join(placeholders, ","),
	)
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var orderID int
		item := OrderItem{}
		if err := rows.Scan(&orderID, &item.ProductID, &item.ProductName, &item.Quantity, &item.UnitPrice); err != nil {
			return nil, err
		}
		item.LineTotal = item.UnitPrice * float64(item.Quantity)
		itemsByOrder[orderID] = append(itemsByOrder[orderID], item)
	}
	return itemsByOrder, nil
}

func (s *Store) ListOrdersByUser(userID int) ([]CustomerOrder, error) {
	rows, err := s.DB.Query(
		`SELECT id, status, partner_status, delivery_status, delivery_address, COALESCE(delivery_notice, ''), total_amount, created_at
		 FROM orders
		 WHERE user_id = ?
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []CustomerOrder{}
	orderIDs := []int{}
	for rows.Next() {
		order := CustomerOrder{}
		if err := rows.Scan(
			&order.ID,
			&order.Status,
			&order.PartnerStatus,
			&order.DeliveryStatus,
			&order.DeliveryAddress,
			&order.DeliveryNotice,
			&order.TotalAmount,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, order)
		orderIDs = append(orderIDs, order.ID)
	}

	itemsByOrder, err := s.listOrderItemsByOrderIDs(orderIDs)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}
	return orders, nil
}

func (s *Store) ListOrdersForFulfillment() ([]FulfillmentOrder, error) {
	rows, err := s.DB.Query(
		`SELECT o.id, o.user_id, u.username, o.status, o.partner_status, o.delivery_status, COALESCE(o.delivery_notice, ''), o.delivery_address, o.total_amount, o.created_at
		 FROM orders o
		 JOIN users u ON u.id = o.user_id
		 WHERE o.status != 'cancelled'
		 ORDER BY o.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []FulfillmentOrder{}
	orderIDs := []int{}
	for rows.Next() {
		order := FulfillmentOrder{}
		if err := rows.Scan(
			&order.ID,
			&order.CustomerUserID,
			&order.CustomerName,
			&order.Status,
			&order.PartnerStatus,
			&order.DeliveryStatus,
			&order.DeliveryNotice,
			&order.DeliveryAddress,
			&order.TotalAmount,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, order)
		orderIDs = append(orderIDs, order.ID)
	}

	itemsByOrder, err := s.listOrderItemsByOrderIDs(orderIDs)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}
	return orders, nil
}
