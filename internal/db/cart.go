package db

import (
	"context"
	"database/sql"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/money"
)

const maxCartLineQuantity = 999

func ensureCartTx(ctx context.Context, q querier, userID int) (int64, error) {
	var cartID int64
	err := q.QueryRowContext(ctx,
		`INSERT INTO carts (user_id) VALUES ($1)
		 ON CONFLICT (user_id) DO UPDATE SET updated_at = now()
		 RETURNING id`,
		userID,
	).Scan(&cartID)
	return cartID, err
}

// AddToCart adds quantity of a product to the user's cart, capping the line at
// the maximum line quantity. Any open checkout is invalidated.
func (s *Store) AddToCart(userID, productID, quantity int) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		cartID, err := ensureCartTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO cart_items (cart_id, product_id, quantity)
			 VALUES ($1, $2, LEAST($3::INTEGER, $4::INTEGER))
			 ON CONFLICT (cart_id, product_id) DO UPDATE SET
			   quantity = LEAST(cart_items.quantity + EXCLUDED.quantity, $4::INTEGER)`,
			cartID, productID, quantity, maxCartLineQuantity,
		); err != nil {
			return wrapDBError(err)
		}
		return invalidateCheckoutsTx(ctx, tx, userID)
	})
}

// UpdateCartQuantity sets a line's quantity; zero or less removes the line.
func (s *Store) UpdateCartQuantity(userID, productID, quantity int) error {
	if quantity > maxCartLineQuantity {
		return ErrInvalidQuantity
	}
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		cartID, err := ensureCartTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		if quantity <= 0 {
			_, err = tx.ExecContext(ctx, "DELETE FROM cart_items WHERE cart_id = $1 AND product_id = $2", cartID, productID)
		} else {
			_, err = tx.ExecContext(ctx,
				"UPDATE cart_items SET quantity = $1 WHERE cart_id = $2 AND product_id = $3",
				quantity, cartID, productID,
			)
		}
		if err != nil {
			return err
		}
		return invalidateCheckoutsTx(ctx, tx, userID)
	})
}

func (s *Store) RemoveFromCart(userID, productID int) error {
	return s.UpdateCartQuantity(userID, productID, 0)
}

func (s *Store) ClearCart(userID int) error {
	ctx, cancel := s.ctx()
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM cart_items WHERE cart_id = (SELECT id FROM carts WHERE user_id = $1)", userID,
		); err != nil {
			return err
		}
		return invalidateCheckoutsTx(ctx, tx, userID)
	})
}

// CartItemCount returns the total quantity of active products in the cart.
func (s *Store) CartItemCount(userID int) (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	var count int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(ci.quantity), 0)
		 FROM carts c
		 JOIN cart_items ci ON ci.cart_id = c.id
		 JOIN products p ON p.id = ci.product_id AND p.deleted_at IS NULL
		 WHERE c.user_id = $1`,
		userID,
	).Scan(&count)
	return count, err
}

// GetCartItems returns cart lines with current prices and available stock.
func (s *Store) GetCartItems(userID int) ([]CartItem, money.Cents, error) {
	return s.getCartItems(userID, "")
}

// GetCartItemsForCheckout is GetCartItems, except stock the user already holds
// under reservationKey counts as available to them.
func (s *Store) GetCartItemsForCheckout(userID int, reservationKey string) ([]CartItem, money.Cents, error) {
	return s.getCartItems(userID, strings.TrimSpace(reservationKey))
}

func (s *Store) getCartItems(userID int, reservationKey string) ([]CartItem, money.Cents, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		`SELECT p.id, p.name, p.slug, p.price_cents,
		        stock.available_quantity + COALESCE(own.reserved_quantity, 0),
		        ci.quantity,
		        COALESCE(img.url, '')
		 FROM carts c
		 JOIN cart_items ci ON ci.cart_id = c.id
		 JOIN products p ON p.id = ci.product_id AND p.deleted_at IS NULL
		 CROSS JOIN LATERAL (
		     SELECT COALESCE(SUM(s.on_hand_quantity - s.reserved_quantity - s.allocated_quantity), 0)::INTEGER AS available_quantity
		     FROM inventory_stocks s WHERE s.product_id = p.id
		 ) stock
		 LEFT JOIN LATERAL (
		     SELECT SUM(r.quantity)::INTEGER AS reserved_quantity
		     FROM stock_reservations r
		     WHERE $2 <> '' AND r.user_id = c.user_id AND r.reservation_key = $2
		       AND r.product_id = p.id AND r.status = 'active'
		 ) own ON true
		 LEFT JOIN LATERAL (
		     SELECT url FROM product_images WHERE product_id = p.id ORDER BY position, id LIMIT 1
		 ) img ON true
		 WHERE c.user_id = $1
		 ORDER BY lower(p.name), p.id`,
		userID, reservationKey,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := []CartItem{}
	var subtotal money.Cents
	for rows.Next() {
		var item CartItem
		if err := rows.Scan(
			&item.ProductID,
			&item.ProductName,
			&item.ProductSlug,
			&item.UnitPrice,
			&item.StockQuantity,
			&item.Quantity,
			&item.ProductImageURL,
		); err != nil {
			return nil, 0, err
		}
		item.LineTotal = item.UnitPrice * money.Cents(item.Quantity)
		subtotal += item.LineTotal
		items = append(items, item)
	}
	return items, subtotal, rows.Err()
}

type cartLine struct {
	ProductID   int
	PartnerID   sql.NullInt64
	ProductName string
	Quantity    int
	UnitPrice   money.Cents
}

// cartLinesTx reads the user's active cart lines inside a transaction, ordered
// by product id so that inventory rows are always locked in the same order.
func cartLinesTx(ctx context.Context, tx *sql.Tx, userID int) ([]cartLine, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT p.id, p.partner_id, p.name, ci.quantity, p.price_cents
		 FROM carts c
		 JOIN cart_items ci ON ci.cart_id = c.id
		 JOIN products p ON p.id = ci.product_id AND p.deleted_at IS NULL
		 WHERE c.user_id = $1
		 ORDER BY p.id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := []cartLine{}
	for rows.Next() {
		var line cartLine
		if err := rows.Scan(&line.ProductID, &line.PartnerID, &line.ProductName, &line.Quantity, &line.UnitPrice); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

// AvailableForUser returns how many units of a product the user may hold in
// their cart: available stock plus whatever the user already has reserved.
// It returns ErrProductNotFound for missing or deleted products.
func (s *Store) AvailableForUser(userID, productID int) (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	var available int
	err := s.DB.QueryRowContext(ctx,
		`SELECT
		    COALESCE((SELECT SUM(on_hand_quantity - reserved_quantity - allocated_quantity)
		              FROM inventory_stocks WHERE product_id = p.id), 0)
		  + COALESCE((SELECT SUM(quantity) FROM stock_reservations
		              WHERE product_id = p.id AND user_id = $1 AND status = 'active'), 0)
		 FROM products p
		 WHERE p.id = $2 AND p.deleted_at IS NULL`,
		userID, productID,
	).Scan(&available)
	if err == sql.ErrNoRows {
		return 0, ErrProductNotFound
	}
	return available, err
}

// CartQuantity returns the quantity of one product in the user's cart.
func (s *Store) CartQuantity(userID, productID int) (int, error) {
	ctx, cancel := s.ctx()
	defer cancel()

	var quantity int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COALESCE((SELECT ci.quantity FROM carts c
		                  JOIN cart_items ci ON ci.cart_id = c.id
		                  WHERE c.user_id = $1 AND ci.product_id = $2), 0)`,
		userID, productID,
	).Scan(&quantity)
	return quantity, err
}
