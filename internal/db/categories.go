package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const categoryColumns = "id, name, slug, description, parent_id, created_at, updated_at, deleted_at"

func scanCategory(row interface{ Scan(...any) error }) (*Category, error) {
	category := &Category{}
	err := row.Scan(
		&category.ID,
		&category.Name,
		&category.Slug,
		&category.Description,
		&category.ParentID,
		&category.CreatedAt,
		&category.UpdatedAt,
		&category.DeletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return category, nil
}

func (s *Store) CreateCategory(ctx context.Context, category Category) (int64, error) {
	return s.createCategory(ctx, 0, "", category, false)
}

func (s *Store) CreateCategoryForActor(ctx context.Context, actorID int, actorRole string, category Category) (int64, error) {
	return s.createCategory(ctx, actorID, actorRole, category, true)
}

func (s *Store) createCategory(ctx context.Context, actorID int, actorRole string, category Category, requireActor bool) (int64, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	var id int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if requireActor {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, actorRole); err != nil {
				return err
			}
		}
		return tx.QueryRowContext(ctx,
			`INSERT INTO categories (name, slug, description, parent_id)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			strings.TrimSpace(category.Name),
			strings.TrimSpace(category.Slug),
			strings.TrimSpace(category.Description),
			category.ParentID,
		).Scan(&id)
	})
	return id, wrapDBError(err)
}

func (s *Store) GetCategoryByID(ctx context.Context, id int) (*Category, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanCategory(s.DB.QueryRowContext(ctx,
		"SELECT "+categoryColumns+" FROM categories WHERE id = $1 AND deleted_at IS NULL", id,
	))
}

func (s *Store) GetCategoryBySlug(ctx context.Context, slug string) (*Category, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return scanCategory(s.DB.QueryRowContext(ctx,
		"SELECT "+categoryColumns+" FROM categories WHERE slug = $1 AND deleted_at IS NULL",
		strings.TrimSpace(slug),
	))
}

func (s *Store) UpdateCategory(ctx context.Context, category Category) error {
	return s.updateCategory(ctx, 0, "", category, false)
}

func (s *Store) UpdateCategoryForActor(ctx context.Context, actorID int, actorRole string, category Category) error {
	return s.updateCategory(ctx, actorID, actorRole, category, true)
}

func (s *Store) updateCategory(ctx context.Context, actorID int, actorRole string, category Category, requireActor bool) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	return s.withTx(ctx, func(tx *sql.Tx) error {
		if requireActor {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, actorRole); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE categories SET name = $1, slug = $2, description = $3, parent_id = $4
			 WHERE id = $5 AND deleted_at IS NULL`,
			strings.TrimSpace(category.Name),
			strings.TrimSpace(category.Slug),
			strings.TrimSpace(category.Description),
			category.ParentID,
			category.ID,
		)
		return wrapDBError(err)
	})
}

func (s *Store) DeleteCategory(ctx context.Context, id int) error {
	return s.deleteCategory(ctx, 0, "", id, false)
}

func (s *Store) DeleteCategoryForActor(ctx context.Context, actorID int, actorRole string, id int) error {
	return s.deleteCategory(ctx, actorID, actorRole, id, true)
}

func (s *Store) deleteCategory(ctx context.Context, actorID int, actorRole string, id int, requireActor bool) error {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	return s.withTx(ctx, func(tx *sql.Tx) error {
		if requireActor {
			if _, _, err := lockWorkspaceActorTx(ctx, tx, actorID, actorRole); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE categories SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL", id)
		return err
	})
}

// CountProductsByCategory returns the number of active products in each
// category, keyed by category id.
func (s *Store) CountProductsByCategory(ctx context.Context) (map[int]int, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()
	rows, err := s.DB.QueryContext(ctx,
		"SELECT category_id, count(*) FROM products WHERE deleted_at IS NULL AND category_id IS NOT NULL GROUP BY category_id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[int]int{}
	for rows.Next() {
		var id, count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (s *Store) GetAllCategories(ctx context.Context) ([]Category, error) {
	ctx, cancel := s.queryContext(ctx)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx,
		"SELECT "+categoryColumns+" FROM categories WHERE deleted_at IS NULL ORDER BY lower(name)",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := []Category{}
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, *category)
	}
	return categories, rows.Err()
}
