package admin

import (
	"context"
	"errors"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/slug"
)

var (
	ErrForbiddenCatalogAction = errors.New("forbidden catalog action")
	ErrInvalidCategory        = errors.New("invalid category")
	ErrDuplicateCategory      = errors.New("duplicate category")
	ErrCategoryNotFound       = errors.New("category not found")
)

const (
	maxCategoryName        = 80
	maxCategoryDescription = 500
)

type Category struct {
	ID           int
	Name         string
	Slug         string
	Description  string
	ProductCount int
}

type CategoryInput struct {
	Name        string
	Slug        string
	Description string
}

// canManageCatalog reports whether actor may manage categories.
func canManageCatalog(actor *Actor) bool {
	return actor != nil && (actor.Role == db.RoleAdmin || actor.Role == db.RoleStaff)
}

// Categories lists active categories with their active product counts.
func (s *Service) Categories() ([]Category, error) {
	categories, err := s.store.GetAllCategories()
	if err != nil {
		return nil, err
	}
	counts, err := s.store.CountProductsByCategory()
	if err != nil {
		return nil, err
	}
	mapped := make([]Category, 0, len(categories))
	for _, c := range categories {
		mapped = append(mapped, Category{
			ID: c.ID, Name: c.Name, Slug: c.Slug, Description: c.Description,
			ProductCount: counts[c.ID],
		})
	}
	return mapped, nil
}

// CreateCategory adds a category. An empty slug is derived from the name.
func (s *Service) CreateCategory(actor *Actor, input CategoryInput) error {
	if !canManageCatalog(actor) {
		return ErrForbiddenCatalogAction
	}
	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	categorySlug := strings.ToLower(strings.TrimSpace(input.Slug))
	if categorySlug == "" {
		categorySlug = slug.Make(name)
	}
	if name == "" || len([]rune(name)) > maxCategoryName || len([]rune(description)) > maxCategoryDescription || !slug.Valid(categorySlug) {
		return ErrInvalidCategory
	}
	id, err := s.store.CreateCategory(db.Category{Name: name, Slug: categorySlug, Description: description})
	if db.IsUniqueConstraintError(err) {
		return ErrDuplicateCategory
	}
	if err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(context.Background(), actor.ID, "category.create", "category", int(id), "slug="+categorySlug)
	return nil
}

// UpdateCategory changes a category's name and description. The slug stays
// the same so category links keep working.
func (s *Service) UpdateCategory(actor *Actor, idRaw string, input CategoryInput) error {
	if !canManageCatalog(actor) {
		return ErrForbiddenCatalogAction
	}
	id, err := parsePositiveInt(idRaw)
	if err != nil {
		return ErrCategoryNotFound
	}
	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	if name == "" || len([]rune(name)) > maxCategoryName || len([]rune(description)) > maxCategoryDescription {
		return ErrInvalidCategory
	}
	category, err := s.store.GetCategoryByID(id)
	if err != nil {
		return err
	}
	if category == nil {
		return ErrCategoryNotFound
	}
	before := category.Name
	category.Name, category.Description = name, description
	err = s.store.UpdateCategory(*category)
	if db.IsUniqueConstraintError(err) {
		return ErrDuplicateCategory
	}
	if err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(context.Background(), actor.ID, "category.update", "category", id, before+" -> "+name)
	return nil
}

// DeleteCategory soft-deletes a category. Its products become uncategorized.
func (s *Service) DeleteCategory(actor *Actor, idRaw string) error {
	if !canManageCatalog(actor) {
		return ErrForbiddenCatalogAction
	}
	id, err := parsePositiveInt(idRaw)
	if err != nil {
		return ErrCategoryNotFound
	}
	category, err := s.store.GetCategoryByID(id)
	if err != nil {
		return err
	}
	if category == nil {
		return ErrCategoryNotFound
	}
	if err := s.store.DeleteCategory(id); err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(context.Background(), actor.ID, "category.delete", "category", id, "slug="+category.Slug)
	return nil
}
