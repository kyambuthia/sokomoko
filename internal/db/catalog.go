package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// activePartnerSQL excludes products whose partner is suspended. Products
// without a partner stay visible. It expects the products table aliased as p.
const activePartnerSQL = "NOT EXISTS (SELECT 1 FROM partners sp WHERE sp.id = p.partner_id AND sp.status <> 'active')"

// productSelect projects a product with its category, partner, and default-warehouse
// available stock. The LATERAL sum is driven by the inventory_stocks primary key.
const productSelect = `
SELECT p.id, p.name, p.slug, p.description, p.price_cents, p.currency,
       stock.available_quantity,
       COALESCE(c.name, ''), p.category_id, p.partner_id, COALESCE(pt.name, ''), COALESCE(pt.slug, ''),
       p.created_at, p.updated_at
FROM products p
LEFT JOIN categories c ON c.id = p.category_id AND c.deleted_at IS NULL
LEFT JOIN partners pt ON pt.id = p.partner_id
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(s.on_hand_quantity - s.reserved_quantity - s.allocated_quantity), 0)::INTEGER AS available_quantity
    FROM inventory_stocks s
    WHERE s.product_id = p.id AND s.warehouse_id = (SELECT id FROM warehouses WHERE is_default)
) stock`

func scanProduct(row interface{ Scan(...any) error }) (*Product, error) {
	product := &Product{}
	err := row.Scan(
		&product.ID,
		&product.Name,
		&product.Slug,
		&product.Description,
		&product.Price,
		&product.Currency,
		&product.StockQuantity,
		&product.Category,
		&product.CategoryID,
		&product.PartnerID,
		&product.PartnerName,
		&product.PartnerSlug,
		&product.CreatedAt,
		&product.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return product, nil
}

func lockProductMutationActorTx(ctx context.Context, tx *sql.Tx, actorID int, actorRole string) (int, bool, error) {
	partnerID, isPartner, err := lockWorkspaceActorTx(ctx, tx, actorID, actorRole)
	if errors.Is(err, ErrWorkspaceActorNotFound) {
		return 0, false, ErrProductNotFound
	}
	return partnerID, isPartner, err
}

// CreateProduct inserts a product, its opening stock in the default warehouse,
// and any uploaded media in one transaction.
func (s *Store) CreateProduct(ctx context.Context, product Product) (int64, error) {
	return s.createProduct(ctx, product, 0, "")
}

func (s *Store) CreateProductForActor(ctx context.Context, product Product, actorID int, actorRole string) (int64, error) {
	if actorID <= 0 {
		return 0, ErrProductNotFound
	}
	return s.createProduct(ctx, product, actorID, actorRole)
}

func (s *Store) createProduct(ctx context.Context, product Product, actorID int, actorRole string) (int64, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	if product.Price < 0 {
		return 0, fmt.Errorf("price must not be negative")
	}
	if product.StockQuantity < 0 {
		return 0, ErrInvalidQuantity
	}
	if err := validateProductVideo(product.Video); err != nil {
		return 0, err
	}
	currency := strings.ToUpper(strings.TrimSpace(product.Currency))

	var id int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if actorID > 0 {
			partnerID, isPartner, err := lockProductMutationActorTx(ctx, tx, actorID, actorRole)
			if err != nil {
				return err
			}
			if isPartner {
				product.PartnerID = sql.NullInt64{Int64: int64(partnerID), Valid: true}
			}
		}
		var currentCurrency string
		if err := tx.QueryRowContext(ctx, "SELECT currency FROM commerce_settings WHERE id = 1 FOR SHARE").Scan(&currentCurrency); err != nil {
			return err
		}
		if currency != "" && currency != currentCurrency {
			return ErrProductCurrencyChanged
		}
		currency = currentCurrency

		warehouseID, err := defaultWarehouseID(ctx, tx)
		if err != nil {
			return err
		}

		if err := tx.QueryRowContext(ctx,
			`INSERT INTO products (name, slug, description, price_cents, currency, category_id, partner_id)
			 VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, ''), (SELECT currency FROM commerce_settings WHERE id = 1)), $6, $7)
			 RETURNING id`,
			strings.TrimSpace(product.Name),
			strings.TrimSpace(product.Slug),
			strings.TrimSpace(product.Description),
			int64(product.Price),
			currency,
			product.CategoryID,
			product.PartnerID,
		).Scan(&id); err != nil {
			return wrapProductCreateError(err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO inventory_stocks (product_id, warehouse_id, on_hand_quantity)
			 VALUES ($1, $2, $3)`,
			id, warehouseID, product.StockQuantity,
		); err != nil {
			return err
		}
		if product.StockQuantity > 0 {
			if err := recordStockMovement(ctx, tx, int(id), warehouseID, stockMovementInitial, product.StockQuantity, "product creation"); err != nil {
				return err
			}
		}

		for i, img := range product.Images {
			position := img.DisplayOrder
			if position <= 0 {
				position = i
			}
			if err := insertProductImageTx(ctx, tx, id, img, position); err != nil {
				return err
			}
		}
		if product.Video != nil {
			if err := saveProductVideoTx(ctx, tx, int(id), product.Video); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) GetProductByID(ctx context.Context, id int) (*Product, error) {
	return s.getProduct(ctx, "p.id = $1", id)
}

func (s *Store) GetProductBySlug(ctx context.Context, slug string) (*Product, error) {
	return s.getProduct(ctx, "p.slug = $1", strings.TrimSpace(slug))
}

// GetPublicProductBySlug is GetProductBySlug restricted to products customers
// may see: not deleted and not from a suspended partner.
func (s *Store) GetPublicProductBySlug(ctx context.Context, slug string) (*Product, error) {
	return s.getProduct(ctx, "p.slug = $1 AND "+activePartnerSQL, strings.TrimSpace(slug))
}

func (s *Store) getProduct(ctx context.Context, predicate string, arg any) (*Product, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	product, err := scanProduct(tx.QueryRowContext(ctx,
		productSelect+" WHERE "+predicate+" AND p.deleted_at IS NULL", arg,
	))
	if err != nil || product == nil {
		return product, err
	}

	images, err := getProductImagesByProductIDs(ctx, tx, []int{product.ID})
	if err != nil {
		return nil, err
	}
	product.Images = images[product.ID]
	var hasVideo bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM product_video_blobs WHERE product_id = $1)", product.ID,
	).Scan(&hasVideo); err != nil {
		return nil, err
	}
	if hasVideo {
		product.VideoURL = fmt.Sprintf("/media/product-videos/%d", product.ID)
	}
	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err)
	}
	return product, nil
}

// UpdateProduct updates catalog fields and sets the available quantity in the
// default warehouse to product.StockQuantity, keeping reserved and allocated
// stock untouched. Image rows change only when product.Images carries new
// upload bytes, or when RemoveUploadedImages is set. New bytes replace prior
// blob-backed images. RemoveUploadedImages deletes blob-backed images and
// leaves URL-only images in place. Either image change commits with the rest
// of the update.
func (s *Store) UpdateProduct(ctx context.Context, product Product) error {
	return s.updateProduct(ctx, product, 0, "")
}

func (s *Store) UpdateProductForActor(ctx context.Context, product Product, actorID int, actorRole string) error {
	if actorID <= 0 {
		return ErrProductNotFound
	}
	return s.updateProduct(ctx, product, actorID, actorRole)
}

func (s *Store) updateProduct(ctx context.Context, product Product, actorID int, actorRole string) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	if product.StockQuantity < 0 {
		return ErrInvalidQuantity
	}
	if err := validateProductVideo(product.Video); err != nil {
		return err
	}

	return s.withTx(ctx, func(tx *sql.Tx) error {
		partnerID, isPartner := 0, false
		if actorID > 0 {
			var err error
			partnerID, isPartner, err = lockProductMutationActorTx(ctx, tx, actorID, actorRole)
			if err != nil {
				return err
			}
		}
		var currentCurrency string
		if err := tx.QueryRowContext(ctx, "SELECT currency FROM commerce_settings WHERE id = 1 FOR SHARE").Scan(&currentCurrency); err != nil {
			return err
		}
		if expected := strings.ToUpper(strings.TrimSpace(product.Currency)); expected != "" && expected != currentCurrency {
			return ErrProductCurrencyChanged
		}

		updateQuery := `UPDATE products
			SET name = $1, slug = $2, description = $3, price_cents = $4, category_id = $5
			WHERE id = $6 AND deleted_at IS NULL`
		updateArgs := []any{
			strings.TrimSpace(product.Name),
			strings.TrimSpace(product.Slug),
			strings.TrimSpace(product.Description),
			int64(product.Price),
			product.CategoryID,
			product.ID,
		}
		if isPartner {
			updateQuery += ` AND partner_id = $7`
			updateArgs = append(updateArgs, partnerID)
		}
		result, err := tx.ExecContext(ctx, updateQuery, updateArgs...)
		if err != nil {
			return wrapProductCreateError(err)
		}
		affected, err := rowsAffected(result)
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrProductNotFound
		}

		if hasUploadedProductImage(product.Images) {
			if err := deleteUploadedProductImagesTx(ctx, tx, product.ID); err != nil {
				return err
			}
			newImageCount := 0
			for _, image := range product.Images {
				if len(image.Data) > 0 {
					newImageCount++
				}
			}
			if _, err := tx.ExecContext(ctx,
				"UPDATE product_images SET position = position + $1 WHERE product_id = $2",
				newImageCount, product.ID,
			); err != nil {
				return err
			}
			for i, image := range product.Images {
				if len(image.Data) == 0 {
					continue
				}
				if err := insertProductImageTx(ctx, tx, int64(product.ID), image, i); err != nil {
					return err
				}
			}
		} else if product.RemoveUploadedImages {
			if err := deleteUploadedProductImagesTx(ctx, tx, product.ID); err != nil {
				return err
			}
		}

		if product.Video != nil {
			if err := saveProductVideoTx(ctx, tx, product.ID, product.Video); err != nil {
				return err
			}
		} else if product.RemoveUploadedVideo {
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM product_video_blobs WHERE product_id = $1", product.ID,
			); err != nil {
				return err
			}
		}

		warehouseID, err := defaultWarehouseID(ctx, tx)
		if err != nil {
			return err
		}
		stock, err := lockInventoryStock(ctx, tx, product.ID, warehouseID)
		if err != nil {
			return err
		}

		targetOnHand := product.StockQuantity + stock.ReservedQuantity + stock.AllocatedQuantity
		delta := targetOnHand - stock.OnHandQuantity
		if delta == 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE inventory_stocks SET on_hand_quantity = $1
			 WHERE product_id = $2 AND warehouse_id = $3`,
			targetOnHand, product.ID, warehouseID,
		); err != nil {
			return err
		}
		return recordStockMovement(ctx, tx, product.ID, warehouseID, stockMovementAdjustment, delta, "product update")
	})
}

// DeleteProduct soft-deletes a product for trusted internal callers.
func (s *Store) DeleteProduct(ctx context.Context, id int) error {
	return s.deleteProduct(ctx, id, 0, "")
}

// DeleteProductForActor rechecks the actor and product ownership in the
// deletion transaction so a concurrent suspension or membership removal wins.
func (s *Store) DeleteProductForActor(ctx context.Context, id, actorID int, actorRole string) error {
	if actorID <= 0 {
		return ErrProductNotFound
	}
	return s.deleteProduct(ctx, id, actorID, actorRole)
}

func (s *Store) deleteProduct(ctx context.Context, id, actorID int, actorRole string) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		partnerID, isPartner := 0, false
		if actorID > 0 {
			var err error
			partnerID, isPartner, err = lockProductMutationActorTx(ctx, tx, actorID, actorRole)
			if err != nil {
				return err
			}
		}
		deleteQuery := `UPDATE products SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
		args := []any{id}
		if isPartner {
			deleteQuery += ` AND partner_id = $2`
			args = append(args, partnerID)
		}

		result, err := tx.ExecContext(ctx, deleteQuery, args...)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated == 0 {
			return ErrProductNotFound
		}

		rows, err := tx.QueryContext(ctx,
			`SELECT c.user_id
			 FROM carts c
			 JOIN cart_items ci ON ci.cart_id = c.id
			 WHERE ci.product_id = $1
			 ORDER BY c.user_id
			 FOR UPDATE OF c`,
			id,
		)
		if err != nil {
			return err
		}
		affectedUsers := []int{}
		for rows.Next() {
			var userID int
			if err := rows.Scan(&userID); err != nil {
				rows.Close()
				return err
			}
			affectedUsers = append(affectedUsers, userID)
		}
		err = rows.Err()
		if closeErr := rows.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE product_id = $1`, id); err != nil {
			return err
		}
		for _, userID := range affectedUsers {
			if err := invalidateCheckoutsTx(ctx, tx, userID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM product_images WHERE product_id = $1`, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM product_video_blobs WHERE product_id = $1`, id)
		return err
	})
}

// ProductFilter narrows ListProducts. Zero values mean "no filter".
type ProductFilter struct {
	Query       string
	Category    string // category slug or case-insensitive name
	PartnerID   int
	InStockOnly bool
	PublicOnly  bool // hide products of suspended partners
	Limit       int
	Offset      int
}

// ListProducts returns active products matching the filter. A text query uses
// the full-text index and falls back to substring matching so partial words
// still match.
// ListProducts bounds the query by both the request deadline and the
// store query timeout.
func (s *Store) ListProducts(ctx context.Context, filter ProductFilter) ([]Product, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	products, err := listProductsTx(ctx, tx, filter)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, wrapDBError(err)
	}
	return products, nil
}

// ListProductsPage returns matching products and their total from one database
// snapshot. The image rows are loaded in that same read-only transaction.
func (s *Store) ListProductsPage(ctx context.Context, filter ProductFilter) ([]Product, int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	total, err := countFilteredProducts(ctx, tx, filter)
	if err != nil {
		return nil, 0, err
	}
	products := []Product{}
	if total > 0 && (filter.Offset <= 0 || filter.Offset < total) {
		products, err = listProductsTx(ctx, tx, filter)
		if err != nil {
			return nil, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, wrapDBError(err)
	}
	return products, total, nil
}

func listProductsTx(ctx context.Context, q querier, filter ProductFilter) ([]Product, error) {
	where, args, orderBy := productFilterSQL(filter)
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	query := productSelect + " WHERE " + where + " ORDER BY " + orderBy
	if filter.Limit > 0 {
		query += " LIMIT " + addArg(filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET " + addArg(filter.Offset)
	}

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := []Product{}
	ids := []int{}
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
		ids = append(ids, product.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	images, err := getProductImagesByProductIDs(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for i := range products {
		products[i].Images = images[products[i].ID]
	}
	return products, nil
}

func productFilterSQL(filter ProductFilter) (string, []any, string) {
	where := []string{"p.deleted_at IS NULL"}
	var args []any
	orderBy := "lower(p.name), p.id"
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if query := strings.TrimSpace(filter.Query); query != "" {
		tsArg := addArg(query)
		likeArg := addArg("%" + escapeLike(query) + "%")
		where = append(where, fmt.Sprintf(
			"(p.search_vector @@ websearch_to_tsquery('simple', %[1]s) OR p.name ILIKE %[2]s OR p.description ILIKE %[2]s)",
			tsArg, likeArg,
		))
		orderBy = fmt.Sprintf("ts_rank(p.search_vector, websearch_to_tsquery('simple', %s)) DESC, lower(p.name), p.id", tsArg)
	}
	if category := strings.TrimSpace(filter.Category); category != "" {
		arg := addArg(category)
		where = append(where, fmt.Sprintf("(c.slug = lower(%[1]s) OR lower(c.name) = lower(%[1]s))", arg))
	}
	if filter.PartnerID > 0 {
		where = append(where, "p.partner_id = "+addArg(filter.PartnerID))
	}
	if filter.InStockOnly {
		where = append(where, "stock.available_quantity > 0")
	}
	if filter.PublicOnly {
		where = append(where, activePartnerSQL)
	}

	return strings.Join(where, " AND "), args, orderBy
}

// CountFilteredProducts counts all matches without materializing them.
func (s *Store) CountFilteredProducts(ctx context.Context, filter ProductFilter) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return countFilteredProducts(ctx, s.DB, filter)
}

func countFilteredProducts(ctx context.Context, q querier, filter ProductFilter) (int, error) {
	where, args, _ := productFilterSQL(filter)
	from := `products p LEFT JOIN categories c ON c.id = p.category_id AND c.deleted_at IS NULL`
	if filter.InStockOnly {
		from += ` CROSS JOIN LATERAL (
			SELECT COALESCE(SUM(s.on_hand_quantity - s.reserved_quantity - s.allocated_quantity), 0)::INTEGER AS available_quantity
			FROM inventory_stocks s
			WHERE s.product_id = p.id AND s.warehouse_id = (SELECT id FROM warehouses WHERE is_default)
		) stock`
	}
	var count int
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM "+from+" WHERE "+where, args...).Scan(&count)
	return count, err
}

// GetAllProducts returns every active product ordered by name.
func (s *Store) GetAllProducts(ctx context.Context) ([]Product, error) {
	return s.ListProducts(ctx, ProductFilter{})
}

// SearchProducts matches products by name or description.
func (s *Store) SearchProducts(ctx context.Context, query string) ([]Product, error) {
	if strings.TrimSpace(query) == "" {
		return []Product{}, nil
	}
	return s.ListProducts(ctx, ProductFilter{Query: query})
}

// CountPublicProducts counts products customers may see.
func (s *Store) CountPublicProducts(ctx context.Context) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var count int
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM products p WHERE p.deleted_at IS NULL AND "+activePartnerSQL).Scan(&count)
	return count, err
}

func (s *Store) CountProducts(ctx context.Context) (int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	var count int
	err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM products WHERE deleted_at IS NULL").Scan(&count)
	return count, err
}
