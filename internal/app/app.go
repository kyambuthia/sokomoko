package app

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/db"
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
	AllProducts(ctx context.Context) ([]catalogsvc.Product, error)
	Browse(ctx context.Context, filter catalogsvc.Filter) ([]catalogsvc.Product, error)
	Categories(ctx context.Context) ([]catalogsvc.Category, error)
	Page(ctx context.Context, page, limit int) ([]catalogsvc.Product, int, error)
	ProductBySlug(ctx context.Context, slug string) (*catalogsvc.Product, error)
	Search(ctx context.Context, query string) ([]catalogsvc.Product, error)
	SearchInCategory(ctx context.Context, query, category string) ([]catalogsvc.Product, error)
	SearchPage(ctx context.Context, query, category string, page, limit int) ([]catalogsvc.Product, int, error)
}

type AccountService interface {
	OrderForUser(context.Context, int, int) (*accountsvc.Order, error)
	OrdersForUserPage(context.Context, int, int, int) ([]accountsvc.Order, int, error)
	CancelOrder(ctx context.Context, userID, orderID int) error
}

type CommerceService interface {
	AddToCart(ctx context.Context, userID, productID, quantity int) error
	Cart(ctx context.Context, userID int) (commerceSvc.Cart, error)
	ItemCount(ctx context.Context, userID int) (int, error)
	RemoveFromCart(ctx context.Context, userID, productID int) error
	UpdateCartItem(ctx context.Context, userID, productID, quantity int) error
}

type CheckoutService interface {
	CheckoutWithPayment(ctx context.Context, userID int, deliveryAddress, paymentMethod, idempotencyKey string) (int64, checkoutsvc.Summary, error)
	PreparedCheckout(ctx context.Context, userID int, checkoutToken string) (checkoutsvc.PageState, error)
	SaveDraft(ctx context.Context, userID int, checkoutToken string, deliveryAddress string, paymentMethod string) (checkoutsvc.PageState, error)
	Prepare(ctx context.Context, userID int, reservationKey string) error
}

type PaymentService interface {
	GenerateIdempotencyKey() (string, error)
	MethodLabel(method string) string
	MethodOptions(ctx context.Context) []paymentsvc.MethodOption
	IsOnline(method string) bool
	Start(ctx context.Context, req paymentsvc.StartRequest) (paymentsvc.StartOutcome, error)
	Status(ctx context.Context, userID, orderID int) (paymentsvc.StatusView, error)
	HandleReturn(ctx context.Context, userID int, reference string) (int, error)
	HandleWebhook(ctx context.Context, providerID string, r *http.Request, body []byte) error
	HandleCallback(ctx context.Context, providerID, token string, r *http.Request, body []byte) error
	Providers(ctx context.Context) ([]paymentsvc.ProviderView, error)
	PaymentExceptions(ctx context.Context) ([]db.PaymentException, error)
	RequestRefund(context.Context, *paymentsvc.Actor, int, string) (*db.PaymentException, error)
	RefreshRefund(context.Context, *paymentsvc.Actor, int) (*db.PaymentException, error)
	ResolveRefund(context.Context, *paymentsvc.Actor, int, string) (*db.PaymentException, error)
	ExceptionsNeedingAttention(context.Context) (int, error)
	SaveProvider(ctx context.Context, actor *paymentsvc.Actor, id string, input paymentsvc.ProviderInput) error
	Pricing(ctx context.Context) (paymentsvc.Pricing, error)
	SavePricing(ctx context.Context, actor *paymentsvc.Actor, input paymentsvc.PricingInput) (paymentsvc.Pricing, error)
	SecretsConfigured() bool
}

type AdminService interface {
	AuditLogs(ctx context.Context, limit int) ([]adminsvc.AuditLog, error)
	Categories(ctx context.Context) ([]adminsvc.Category, error)
	CreateCategory(ctx context.Context, actor *adminsvc.Actor, input adminsvc.CategoryInput) error
	CreatePartner(ctx context.Context, actor *adminsvc.Actor, input adminsvc.PartnerInput) error
	CreatePartnerAccount(ctx context.Context, actor *adminsvc.Actor, input adminsvc.PartnerAccountInput) error
	Partners(ctx context.Context) ([]adminsvc.Partner, []adminsvc.PartnerAccount, error)
	SetPartnerStatus(ctx context.Context, actor *adminsvc.Actor, idRaw, status string) error
	UpdatePartner(ctx context.Context, actor *adminsvc.Actor, idRaw string, input adminsvc.PartnerInput) error
	DeleteCategory(ctx context.Context, actor *adminsvc.Actor, idRaw string) error
	UpdateCategory(ctx context.Context, actor *adminsvc.Actor, idRaw string, input adminsvc.CategoryInput) error
	DeactivateUser(ctx context.Context, actor *adminsvc.Actor, userIDRaw string) error
	ReactivateUser(ctx context.Context, actor *adminsvc.Actor, userIDRaw string) error
	DeactivatedMembers(ctx context.Context) ([]adminsvc.TeamMember, error)
	Deliveries(ctx context.Context) (adminsvc.DeliveryBoard, error)
	Metrics(ctx context.Context) (adminsvc.Metrics, error)
	Orders(ctx context.Context) ([]adminsvc.Order, error)
	OrdersPage(context.Context, int, int) ([]adminsvc.Order, int, error)
	Products(ctx context.Context) ([]adminsvc.Product, error)
	ProductsPage(context.Context, int, int) ([]adminsvc.Product, int, error)
	Reports(ctx context.Context) (adminsvc.SalesReport, error)
	TeamMembers(ctx context.Context, query string, page, limit int) ([]adminsvc.TeamMember, int, error)
	UpdateOrder(ctx context.Context, actor *adminsvc.Actor, input adminsvc.UpdateOrderInput) error
}

type PartnerService interface {
	CreateProductForActor(ctx context.Context, actor *partnersvc.Actor, input partnersvc.CreateProductInput) error
	ProductForEdit(ctx context.Context, actor *partnersvc.Actor, id string) (int, partnersvc.ProductInput, error)
	UpdateProductForActor(ctx context.Context, actor *partnersvc.Actor, id string, input partnersvc.ProductInput) error
	DashboardForActor(context.Context, *partnersvc.Actor) (partnersvc.DashboardData, error)
	OrdersForActor(ctx context.Context, actor *partnersvc.Actor, filter string) (partnersvc.OrdersData, error)
	OrdersPageForActor(context.Context, *partnersvc.Actor, string, int, int) (partnersvc.OrdersData, int, error)
	ProductsForActor(ctx context.Context, actor *partnersvc.Actor) (partnersvc.ProductsData, error)
	ProductsPageForActor(context.Context, *partnersvc.Actor, int, int) (partnersvc.ProductsData, int, error)
	SaveStoreSettings(ctx context.Context, actor *partnersvc.Actor, input partnersvc.StoreSettingsInput) (partnersvc.StoreSettings, error)
	StoreSettings(ctx context.Context) (*partnersvc.StoreSettings, error)
	UpdateOrder(ctx context.Context, actor *partnersvc.Actor, input partnersvc.UpdateOrderInput) error
}

type AuthService interface {
	APIAuthMiddleware(next http.Handler) http.Handler
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
	PasswordChange(tmpl *template.Template, action, backHref string) http.HandlerFunc
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
	// PublicBaseURL is the public origin used in payment return and callback
	// URLs. When empty, development derives it from the request.
	PublicBaseURL string
	Production    bool
}

type App struct {
	Readiness     ReadinessChecker
	Catalog       CatalogService
	Account       AccountService
	Commerce      CommerceService
	Checkout      CheckoutService
	Payment       PaymentService
	Admin         AdminService
	Partner       PartnerService
	Auth          AuthService
	Templates     *ui.Templates
	StaticFS      embed.FS
	PublicBaseURL string
	Production    bool
}

func New(deps Dependencies) *App {
	return &App{
		Readiness:     deps.Readiness,
		Catalog:       deps.Catalog,
		Account:       deps.Account,
		Commerce:      deps.Commerce,
		Checkout:      deps.Checkout,
		Payment:       deps.Payment,
		Admin:         deps.Admin,
		Partner:       deps.Partner,
		Auth:          deps.Auth,
		Templates:     deps.Templates,
		StaticFS:      deps.StaticFS,
		PublicBaseURL: deps.PublicBaseURL,
		Production:    deps.Production,
	}
}

// PublicBase is the public origin used in payment return and callback URLs:
// the configured PUBLIC_BASE_URL, or in development the storefront origin
// derived from the request (workspace host prefixes removed). Production
// without PUBLIC_BASE_URL returns "", and online payments refuse to start.
func (a *App) PublicBase(r *http.Request) string {
	if a.PublicBaseURL != "" {
		return strings.TrimRight(a.PublicBaseURL, "/")
	}
	if a.Production {
		return ""
	}
	host := strings.ToLower(r.Host)
	for _, prefix := range []string{"admin.", "partner."} {
		host = strings.TrimPrefix(host, prefix)
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + host
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
