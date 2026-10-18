package app

import (
	"embed"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/payments"
	"github.com/kyambuthia/sokomoko/internal/secret"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
	checkoutsvc "github.com/kyambuthia/sokomoko/internal/service/checkout"
	commerceSvc "github.com/kyambuthia/sokomoko/internal/service/commerce"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
	paymentsvc "github.com/kyambuthia/sokomoko/internal/service/payment"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

type ComposeOptions struct {
	Store     *db.Store
	Templates *ui.Templates
	StaticFS  embed.FS
	Auth      auth.Config
	// Secrets seals payment provider credentials. Nil means credentials
	// cannot be saved.
	Secrets       *secret.Box
	Payments      payments.Options
	PublicBaseURL string
	Production    bool
}

func Compose(opts ComposeOptions) *App {
	authService := auth.NewService(opts.Store, opts.Auth)
	paymentService := paymentsvc.New(opts.Store, payments.NewRegistry(opts.Payments), opts.Secrets)
	checkoutService := checkoutsvc.New(opts.Store, paymentService)

	return New(Dependencies{
		Readiness:     opts.Store,
		Catalog:       catalogsvc.New(opts.Store),
		Account:       accountsvc.New(opts.Store),
		Commerce:      commerceSvc.New(opts.Store),
		Checkout:      checkoutService,
		Payment:       paymentService,
		Admin:         adminsvc.New(opts.Store),
		Partner:       partnersvc.New(opts.Store),
		Auth:          authService,
		Templates:     opts.Templates,
		StaticFS:      opts.StaticFS,
		PublicBaseURL: opts.PublicBaseURL,
		Production:    opts.Production,
	})
}
