package app

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	bookingsvc "github.com/kyambuthia/sokomoko/internal/service/booking"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	directorysvc "github.com/kyambuthia/sokomoko/internal/service/directory"
	logisticsvc "github.com/kyambuthia/sokomoko/internal/service/logistics"
	messagingSvc "github.com/kyambuthia/sokomoko/internal/service/messaging"
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
	ProductImage(ctx context.Context, id int) (*db.ProductImage, error)
	ProductVideo(ctx context.Context, productID int) (*db.ProductVideo, error)
	Search(ctx context.Context, query string) ([]catalogsvc.Product, error)
	SearchInCategory(ctx context.Context, query, category string) ([]catalogsvc.Product, error)
	SearchPage(ctx context.Context, query, category string, page, limit int) ([]catalogsvc.Product, int, error)
	ProductCount(ctx context.Context, filter catalogsvc.Filter) (int, error)
	ActiveStore(ctx context.Context, slug string) (*db.Partner, error)
}

type DirectoryService interface {
	Browse(context.Context, db.ServiceFilter, int) ([]db.ServiceListing, int, error)
	Public(context.Context, string) (*db.ServiceListing, error)
	Own(context.Context, *directorysvc.Actor, int, int) ([]db.ServiceListing, int, error)
	OwnByState(context.Context, *directorysvc.Actor, string, int, int) ([]db.ServiceListing, int, error)
	Edit(context.Context, *directorysvc.Actor, int) (*db.ServiceListing, error)
	Save(context.Context, *directorysvc.Actor, int, directorysvc.Input) (*db.ServiceListing, error)
}

type BookingService interface {
	Request(context.Context, *bookingsvc.Actor, string, string, int, string) (*db.ServiceBooking, error)
	ForCustomer(context.Context, *bookingsvc.Actor, int, int) ([]db.ServiceBooking, int, error)
	ForPartner(context.Context, *bookingsvc.Actor, int, int) ([]db.ServiceBooking, int, error)
	Cancel(context.Context, *bookingsvc.Actor, int, int64) error
	Respond(context.Context, *bookingsvc.Actor, int, int64, string) error
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
	DeliveriesPage(context.Context, int, int) (adminsvc.DeliveryBoard, int, error)
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
	DeleteProductForActor(ctx context.Context, actor *partnersvc.Actor, id string) error
	DashboardForActor(context.Context, *partnersvc.Actor) (partnersvc.DashboardData, error)
	OrdersForActor(ctx context.Context, actor *partnersvc.Actor, filter string) (partnersvc.OrdersData, error)
	OrdersPageForActor(context.Context, *partnersvc.Actor, string, int, int) (partnersvc.OrdersData, int, error)
	ProductsForActor(ctx context.Context, actor *partnersvc.Actor) (partnersvc.ProductsData, error)
	ProductOptionsForActor(ctx context.Context, actor *partnersvc.Actor) (partnersvc.ProductsData, error)
	ProductsPageForActor(context.Context, *partnersvc.Actor, int, int) (partnersvc.ProductsData, int, error)
	SaveStoreSettings(ctx context.Context, actor *partnersvc.Actor, input partnersvc.StoreSettingsInput) (partnersvc.StoreSettings, error)
	StoreSettings(ctx context.Context) (*partnersvc.StoreSettings, error)
	UpdateOrder(ctx context.Context, actor *partnersvc.Actor, input partnersvc.UpdateOrderInput) error
}

type LogisticsService interface {
	DispatchBoard(context.Context, *logisticsvc.Actor) (logisticsvc.DispatchBoard, error)
	Assign(context.Context, *logisticsvc.Actor, string, string) error
	Jobs(context.Context, *logisticsvc.Actor) ([]db.CourierDelivery, error)
	JobsByView(context.Context, *logisticsvc.Actor, string) ([]db.CourierDelivery, error)
	Update(context.Context, *logisticsvc.Actor, string, string, string) error
}

type MessagingService interface {
	Encrypted(context.Context, *messagingSvc.Actor, db.EncryptedRequest) (db.EncryptedResponse, error)
	Contacts(context.Context, *messagingSvc.Actor, db.ChatTarget) ([]db.ChatContact, error)
	Inbox(context.Context, *messagingSvc.Actor, int) (db.ChatInbox, error)
	Thread(context.Context, *messagingSvc.Actor, int, int, int) (db.ChatThread, error)
	Start(context.Context, *messagingSvc.Actor, db.ChatTarget, string, string) (int, error)
	Send(context.Context, *messagingSvc.Actor, int, string, string) (int, error)
	Action(context.Context, *messagingSvc.Actor, int, string, int, string) error
	Reports(context.Context, *messagingSvc.Actor) ([]db.ChatReport, error)
	ResolveReport(context.Context, *messagingSvc.Actor, int) error
}

type AuthService interface {
	APIAuthMiddleware(next http.Handler) http.Handler
	AdminLogin(tmpl auth.TemplateRenderer) http.HandlerFunc
	AdminSetup(tmpl auth.TemplateRenderer) http.HandlerFunc
	AuthMiddleware(next http.Handler) http.Handler
	CSRFMiddleware(next http.Handler) http.Handler
	CSRFToken(r *http.Request) string
	Login(tmpl auth.TemplateRenderer) http.HandlerFunc
	Logout() http.HandlerFunc
	OptionalUser(next http.Handler) http.Handler
	PasswordResetConfirm(tmpl auth.TemplateRenderer, allowedRoles []string, title string, helper string, loginPath string) http.HandlerFunc
	PasswordResetRequest(tmpl auth.TemplateRenderer, allowedRoles []string, title string, helper string) http.HandlerFunc
	SignUp(tmpl auth.TemplateRenderer) http.HandlerFunc
	StaffSignUp(tmpl auth.TemplateRenderer) http.HandlerFunc
	CourierSignUp(tmpl auth.TemplateRenderer) http.HandlerFunc
	CourierLogin(tmpl auth.TemplateRenderer) http.HandlerFunc
	PasswordChange(tmpl auth.TemplateRenderer, action, backHref string) http.HandlerFunc
	WorkspaceLogin(tmpl auth.TemplateRenderer) http.HandlerFunc
}

type Dependencies struct {
	Readiness      ReadinessChecker
	RateLimitStore SharedRequestRateLimitStore
	Catalog        CatalogService
	Directory      DirectoryService
	Bookings       BookingService
	Account        AccountService
	Commerce       CommerceService
	Checkout       CheckoutService
	Payment        PaymentService
	Admin          AdminService
	Partner        PartnerService
	Logistics      LogisticsService
	Messaging      MessagingService
	Auth           AuthService
	Templates      *ui.Templates
	StaticFS       embed.FS
	// PublicBaseURL is the public origin used in payment return and callback
	// URLs. When empty, development derives it from the request.
	PublicBaseURL         string
	Production            bool
	PasswordResetDisabled bool
	DemoMode              bool
}

type App struct {
	Readiness             ReadinessChecker
	RateLimitStore        SharedRequestRateLimitStore
	Catalog               CatalogService
	Directory             DirectoryService
	Bookings              BookingService
	Account               AccountService
	Commerce              CommerceService
	Checkout              CheckoutService
	Payment               PaymentService
	Admin                 AdminService
	Partner               PartnerService
	Logistics             LogisticsService
	Messaging             MessagingService
	Auth                  AuthService
	Templates             *ui.Templates
	StaticFS              embed.FS
	PublicBaseURL         string
	Production            bool
	PasswordResetDisabled bool
	DemoMode              bool
	// Version is the build identifier, usually the git commit. Empty in tests.
	Version string
}

func New(deps Dependencies) *App {
	return &App{
		Readiness:             deps.Readiness,
		RateLimitStore:        deps.RateLimitStore,
		Catalog:               deps.Catalog,
		Directory:             deps.Directory,
		Bookings:              deps.Bookings,
		Account:               deps.Account,
		Commerce:              deps.Commerce,
		Checkout:              deps.Checkout,
		Payment:               deps.Payment,
		Admin:                 deps.Admin,
		Partner:               deps.Partner,
		Logistics:             deps.Logistics,
		Messaging:             deps.Messaging,
		Auth:                  deps.Auth,
		Templates:             deps.Templates,
		StaticFS:              deps.StaticFS,
		PublicBaseURL:         deps.PublicBaseURL,
		Production:            deps.Production,
		PasswordResetDisabled: deps.PasswordResetDisabled,
		DemoMode:              deps.DemoMode,
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
	for _, prefix := range []string{"admin.", "partner.", "courier."} {
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
		slog.Error("template execution error", "error", err)
		RenderErrorPage(w, nil, http.StatusInternalServerError, "Internal Server Error", "We could not render this page.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(buf.Bytes()); err != nil {
		slog.Error("response write error", "error", err)
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
		slog.Error("json encode error", "error", err)
	}
}
