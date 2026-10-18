package routes

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
	"github.com/kyambuthia/sokomoko/internal/ui"
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

// AdminPayments manages the store currency, pricing, and payment providers.
func AdminPayments(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodGet, http.MethodPost)
			return
		}
		metrics, err := loadAdminMetrics(a.Admin)
		if err != nil {
			serverError(w, r)
			return
		}
		page := newAdminPage(metrics, adminRoleFromContext(r), "Payments", "")
		status := http.StatusOK
		user := requestUserFromContext(r)
		actor := &paymentsvc.Actor{ID: user.ID, Role: user.Role}

		if r.Method == http.MethodPost {
			var message string
			switch r.FormValue("action") {
			case "refund", "refund_refresh", "refund_resolve":
				var id int
				id, err = strconv.Atoi(r.FormValue("exception_id"))
				if err == nil && id > 0 {
					switch r.FormValue("action") {
					case "refund_refresh":
						_, err = a.Payment.RefreshRefund(r.Context(), actor, id)
						message = "Refund status checked"
					case "refund_resolve":
						_, err = a.Payment.ResolveRefund(actor, id, r.FormValue("refund_reference"))
						message = "Refund marked as completed"
					default:
						_, err = a.Payment.RequestRefund(r.Context(), actor, id, r.FormValue("refund_reference"))
						message = "Refund request recorded"
					}
				} else {
					err = db.ErrPaymentExceptionNotFound
				}
			case "pricing":
				var saved paymentsvc.Pricing
				saved, err = a.Payment.SavePricing(actor, paymentsvc.PricingInput{
					Currency:              r.FormValue("currency"),
					ShippingFlat:          r.FormValue("shipping_flat"),
					FreeShippingThreshold: r.FormValue("free_shipping_threshold"),
					TaxRatePercent:        r.FormValue("tax_rate"),
				})
				if err == nil {
					ui.SetCurrency(saved.Currency)
				}
				message = "Currency and pricing saved"
			default:
				id := r.FormValue("provider")
				input := paymentsvc.ProviderInput{
					Enabled: r.FormValue("enabled") == "1",
					Mode:    r.FormValue("mode"),
					Values:  map[string]string{},
				}
				for key, values := range r.PostForm {
					if field, ok := strings.CutPrefix(key, "field_"); ok && len(values) > 0 {
						input.Values[field] = values[0]
					}
				}
				err = a.Payment.SaveProvider(actor, id, input)
				message = "Payment method saved"
				page.FormProvider = id
			}
			switch {
			case err == nil:
				redirectWithFlash(w, r, "/payments", "success", message)
				return
			case errors.Is(err, paymentsvc.ErrForbidden):
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case errors.Is(err, db.ErrCurrencyLocked):
				page.FormError = "The currency cannot change after orders exist, because existing prices are stored in the current currency."
			case errors.Is(err, paymentsvc.ErrSecretsKeyMissing):
				page.FormError = "Set SECRETS_KEY on the server to store payment credentials."
			case errors.Is(err, paymentsvc.ErrLastMethod):
				page.FormError = "Keep at least one payment method enabled, or customers cannot check out."
			case errors.Is(err, paymentsvc.ErrRefundReferenceRequired):
				page.FormError = "Enter the M-Pesa reversal receipt after completing the refund in M-Pesa."
			case errors.Is(err, db.ErrPaymentExceptionNotFound):
				page.FormError = "That payment exception no longer exists."
			case errors.Is(err, paymentsvc.ErrRefundNotReady), errors.Is(err, paymentsvc.ErrRefundUnsupported),
				errors.Is(err, paymentsvc.ErrRefundNotResolvable):
				page.FormError = sentence(err)
			case errors.Is(err, paymentsvc.ErrMissingFields), errors.Is(err, paymentsvc.ErrCurrencyMismatch),
				errors.Is(err, paymentsvc.ErrInvalidPricing), errors.Is(err, paymentsvc.ErrUnknownProvider):
				page.FormError = sentence(err)
			default:
				log.Printf("admin payments: %v", err)
				page.FormError = "Unable to save payment settings."
			}
			status = http.StatusBadRequest
		} else if flash := popFlash(w, r); flash.Message != "" {
			page.FormMessage = flash.Message
		}

		if page.Pricing, err = a.Payment.Pricing(); err != nil {
			serverError(w, r)
			return
		}
		if page.Providers, err = a.Payment.Providers(); err != nil {
			serverError(w, r)
			return
		}
		if page.PaymentExceptions, err = a.Payment.PaymentExceptions(); err != nil {
			serverError(w, r)
			return
		}
		page.Currencies = money.Currencies
		page.SecretsConfigured = a.Payment.SecretsConfigured()
		page.PublicBase = a.PublicBase(r)
		renderAdminPage(a, w, r, status, page)
	}
}

// sentence capitalises an error message for display.
func sentence(err error) string {
	msg := err.Error()
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:] + "."
}
