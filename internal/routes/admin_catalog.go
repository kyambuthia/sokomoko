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

// AdminPartners lists partners and their logins and handles onboarding,
// suspension, and partner account creation.
func AdminPartners(a *app.App) http.HandlerFunc {
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
		page := newAdminPage(metrics, adminRoleFromContext(r), "Partners", "Onboard vendors, give them workspace logins, and suspend them when needed")
		status := http.StatusOK

		if r.Method == http.MethodPost {
			actor := adminActorFromContext(r)
			var message string
			switch r.FormValue("action") {
			case "status":
				newStatus := r.FormValue("status")
				err = svc.SetPartnerStatus(actor, r.FormValue("partner_id"), newStatus)
				message = "Partner reactivated"
				if newStatus == "suspended" {
					message = "Partner suspended. Their products are hidden from customers."
				}
			case "create_account":
				page.AccountForm = adminsvc.PartnerAccountInput{
					PartnerID: r.FormValue("partner_id"), Username: r.FormValue("username"), Email: r.FormValue("email"),
				}
				input := page.AccountForm
				input.Password = r.FormValue("password")
				err = svc.CreatePartnerAccount(actor, input)
				message = "Partner login created"
			default:
				page.PartnerForm = adminsvc.PartnerInput{
					Name: r.FormValue("name"), Slug: r.FormValue("slug"), ContactEmail: r.FormValue("contact_email"),
				}
				err = svc.CreatePartner(actor, page.PartnerForm)
				message = "Partner created"
			}
			var inputErr *adminsvc.AccountInputError
			switch {
			case err == nil:
				redirectWithFlash(w, r, "/partners", "success", message)
				return
			case errors.Is(err, adminsvc.ErrForbiddenPartnerAction):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.As(err, &inputErr):
				page.FormError = inputErr.Reason
			case errors.Is(err, adminsvc.ErrInvalidPartner):
				page.FormError = "Enter a partner name. Slugs use lowercase letters, numbers, and single dashes; the contact email must be valid."
			case errors.Is(err, adminsvc.ErrDuplicatePartner):
				page.FormError = "A partner with that slug already exists."
			case errors.Is(err, adminsvc.ErrDuplicateAccount):
				page.FormError = "That username or email is already in use."
			case errors.Is(err, adminsvc.ErrPartnerNotFound):
				page.FormError = "Choose an existing partner."
			default:
				log.Printf("admin partners: %v", err)
				page.FormError = "Unable to save the partner change."
			}
			status = http.StatusBadRequest
		} else if flash := popFlash(w, r); flash.Message != "" {
			page.FormMessage = flash.Message
		}

		if page.Partners, page.PartnerAccounts, err = svc.Partners(); err != nil {
			serverError(w, r)
			return
		}
		renderAdminPage(a, w, r, status, page)
	}
}
