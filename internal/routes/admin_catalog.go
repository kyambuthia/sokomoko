package routes

import (
	"errors"
	"log"
	"net/http"

	"github.com/kyambuthia/sokomoko/internal/app"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
)

// AdminCategories lists categories and handles create and delete actions.
func AdminCategories(a *app.App) http.HandlerFunc {
	svc := a.Admin

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
			return
		}
		metrics, err := loadAdminMetrics(svc)
		if err != nil {
			serverError(w, r)
			return
		}
		page := newAdminPage(metrics, adminRoleFromContext(r), "Categories", "Organise the catalog into departments customers can browse")
		status := http.StatusOK

		if r.Method == http.MethodPost {
			actor := adminActorFromContext(r)
			var message string
			switch r.FormValue("action") {
			case "delete":
				err = svc.DeleteCategory(actor, r.FormValue("category_id"))
				message = "Category deleted"
			default:
				page.CategoryForm = adminsvc.CategoryInput{
					Name: r.FormValue("name"), Slug: r.FormValue("slug"), Description: r.FormValue("description"),
				}
				err = svc.CreateCategory(actor, page.CategoryForm)
				message = "Category created"
			}
			switch {
			case err == nil:
				redirectWithFlash(w, r, "/categories", "success", message)
				return
			case errors.Is(err, adminsvc.ErrForbiddenCatalogAction):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, adminsvc.ErrInvalidCategory):
				page.FormError = "Enter a name (up to 80 characters). Slugs use lowercase letters, numbers, and single dashes."
			case errors.Is(err, adminsvc.ErrDuplicateCategory):
				page.FormError = "A category with that name or slug already exists."
			case errors.Is(err, adminsvc.ErrCategoryNotFound):
				page.FormError = "That category no longer exists."
			default:
				log.Printf("admin categories: %v", err)
				page.FormError = "Unable to save the category."
			}
			status = http.StatusBadRequest
		} else if flash := popFlash(w, r); flash.Message != "" {
			page.FormMessage = flash.Message
		}

		if page.Categories, err = svc.Categories(); err != nil {
			serverError(w, r)
			return
		}
		renderAdminPage(a, w, r, status, page)
	}
}
