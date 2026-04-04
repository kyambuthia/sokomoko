package app

import (
	"embed"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	accountsvc "github.com/kyambuthia/sokomoko/internal/service/account"
	adminsvc "github.com/kyambuthia/sokomoko/internal/service/admin"
	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
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
}

func Compose(opts ComposeOptions) *App {
	authService := auth.NewService(opts.Store, opts.Auth)
	paymentService := paymentsvc.New()

	return New(Dependencies{
		Readiness: opts.Store,
		Catalog:   catalogsvc.New(opts.Store),
		Account:   accountsvc.New(opts.Store),
		Commerce:  commerceSvc.New(opts.Store, paymentService),
		Payment:   paymentService,
		Admin:     adminsvc.New(opts.Store),
		Partner:   partnersvc.New(opts.Store),
		Auth:      authService,
		Templates: opts.Templates,
		StaticFS:  opts.StaticFS,
	})
}
