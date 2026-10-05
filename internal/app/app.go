package app

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"

	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type ReadinessChecker interface {
	PingContext(ctx context.Context) error
}

type CatalogService interface {
	AllProducts() ([]catalogsvc.Product, error)
	Browse(filter catalogsvc.Filter) ([]catalogsvc.Product, error)
	Categories() ([]catalogsvc.Category, error)
	ProductBySlug(slug string) (*catalogsvc.Product, error)
	Search(query string) ([]catalogsvc.Product, error)
	SearchInCategory(query, category string) ([]catalogsvc.Product, error)
}

type AccountService interface {
	OrderForUser(userID, orderID int) (*accountsvc.Order, error)
	OrdersForUser(userID int) ([]accountsvc.Order, error)
}

type CommerceService interface {
	AddToCart(userID, productID, quantity int) error
	Cart(userID int) (commerceSvc.Cart, error)
	ItemCount(userID int) (int, error)
	RemoveFromCart(userID, productID int) error
	UpdateCartItem(userID, productID, quantity int) error
}

type CheckoutService interface {
	CheckoutWithPayment(userID int, deliveryAddress, paymentMethod, idempotencyKey string) (int64, checkoutsvc.Summary, error)
	PreparedCheckout(userID int, checkoutToken string) (checkoutsvc.PageState, error)
	SaveDraft(userID int, checkoutToken string, deliveryAddress string, paymentMethod string) (checkoutsvc.PageState, error)
	Prepare(userID int, reservationKey string) error
}

type PaymentService interface {
	GenerateIdempotencyKey() (string, error)
	MethodLabel(method string) string
	SupportedMethodOptions() []paymentsvc.MethodOption
}

type AdminService interface {
	AuditLogs(limit int) ([]adminsvc.AuditLog, error)
	DeactivateUser(actor *adminsvc.Actor, userIDRaw string) error
	Metrics() (adminsvc.Metrics, error)
	Orders() ([]adminsvc.Order, error)
	Products() ([]adminsvc.Product, error)
	Reports() (adminsvc.SalesReport, error)
	TeamMembers() ([]adminsvc.TeamMember, error)
	UpdateOrder(actor *adminsvc.Actor, input adminsvc.UpdateOrderInput) error
}

type PartnerService interface {
	CreateProduct(input partnersvc.CreateProductInput) error
	Dashboard() (partnersvc.DashboardData, error)
	Orders(filter string) (partnersvc.OrdersData, error)
	Products() (partnersvc.ProductsData, error)
	SaveStoreSettings(input partnersvc.StoreSettingsInput) (partnersvc.StoreSettings, error)
	StoreSettings() (*partnersvc.StoreSettings, error)
	UpdateOrder(actor *partnersvc.Actor, input partnersvc.UpdateOrderInput) error
}

type AuthService interface {
	AdminLogin(tmpl *template.Template) http.HandlerFunc
	AdminSetup(tmpl *template.Template) http.HandlerFunc
	AuthMiddleware(next http.Handler) http.Handler
	CSRFMiddleware(next http.Handler) http.Handler
	CSRFToken(r *http.Request) string
	Login(tmpl *template.Template) http.HandlerFunc
	Logout() http.HandlerFunc
	OptionalUser(next http.Handler) http.Handler
	PasswordResetConfirm(tmpl *template.Template, allowedRoles []string, title string, helper string, loginPath string) http.HandlerFunc
	PasswordResetRequest(tmpl *template.Template, allowedRoles []string, title string, helper string) http.HandlerFunc
	SignUp(tmpl *template.Template) http.HandlerFunc
	StaffSignUp(tmpl *template.Template) http.HandlerFunc
	WorkspaceLogin(tmpl *template.Template) http.HandlerFunc
}

type Dependencies struct {
	Readiness ReadinessChecker
	Catalog   CatalogService
	Account   AccountService
	Commerce  CommerceService
	Checkout  CheckoutService
	Payment   PaymentService
	Admin     AdminService
	Partner   PartnerService
	Auth      AuthService
	Templates *ui.Templates
	StaticFS  embed.FS
}

type App struct {
	Readiness ReadinessChecker
	Catalog   CatalogService
	Account   AccountService
	Commerce  CommerceService
	Checkout  CheckoutService
	Payment   PaymentService
	Admin     AdminService
	Partner   PartnerService
	Auth      AuthService
	Templates *ui.Templates
	StaticFS  embed.FS
}

func New(deps Dependencies) *App {
	return &App{
		Readiness: deps.Readiness,
		Catalog:   deps.Catalog,
		Account:   deps.Account,
		Commerce:  deps.Commerce,
		Checkout:  deps.Checkout,
		Payment:   deps.Payment,
		Admin:     deps.Admin,
		Partner:   deps.Partner,
		Auth:      deps.Auth,
		Templates: deps.Templates,
		StaticFS:  deps.StaticFS,
	}
}

// Render executes tmpl into a buffer and writes it with a 200 status. Rendering
// to a buffer first means a template error produces a clean error page rather
// than a truncated document.
func (a *App) Render(w http.ResponseWriter, tmpl *template.Template, data any) {
	a.RenderStatus(w, tmpl, http.StatusOK, data)
}

// RenderStatus is Render with an explicit status code.
func (a *App) RenderStatus(w http.ResponseWriter, tmpl *template.Template, status int, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "root_template", data); err != nil {
		log.Printf("template execution error: %v", err)
		RenderErrorPage(w, nil, http.StatusInternalServerError, "Internal Server Error", "We could not render this page.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("response write error: %v", err)
	}
}

// JSON writes v as a JSON response.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}
