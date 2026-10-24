package ui

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

func TestServicePagesFormatSavedCurrencyAndMinorUnits(t *testing.T) {
	previous := Currency()
	SetCurrency("USD")
	t.Cleanup(func() { SetCurrency(previous) })
	templates, err := ParseTemplatesWithDemo(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range []struct {
		name    string
		tmpl    *template.Template
		preview bool
	}{
		{"directory", templates.Services, false},
		{"detail", templates.ServiceDetail, false},
		{"preview", templates.ServiceDetail, true},
		{"seller list", templates.PartnerServices, false},
	} {
		for _, price := range []struct {
			name, mode, currency, want string
			cents                      money.Cents
		}{
			{"fixed", "fixed", "KES", "KES 500.00", 50000},
			{"fractional", "fixed", "KES", "KES 1,250.75", 125075},
			{"starting price", "from", "USD", "From $25.50", 2550},
			{"free", "fixed", "KES", "KES 0.00", 0},
			{"quote", "quote", "KES", "Quote", 0},
		} {
			t.Run(page.name+"/"+price.name, func(t *testing.T) {
				listing := db.ServiceListing{ID: 1, Title: "Haircut", PriceMode: price.mode, Currency: price.currency, PriceCents: price.cents}
				data := map[string]any{
					"Preview": page.preview, "Listing": listing,
					"Listings": []db.ServiceListing{listing},
					"Filter":   map[string]any{}, "Pager": map[string]any{"Pages": 1},
				}
				var body bytes.Buffer
				if err := page.tmpl.ExecuteTemplate(&body, "main-area", data); err != nil {
					t.Fatal(err)
				}
				want := price.want
				if price.mode == "quote" && page.name != "seller list" {
					want = "Ask for a quote"
				}
				if !strings.Contains(body.String(), want) {
					t.Fatalf("missing price %q in %s", want, body.String())
				}
				if strings.Contains(body.String(), "$500") || (price.mode == "quote" && strings.Contains(body.String(), "KES 0.00")) {
					t.Fatalf("incorrect currency or quote amount: %s", body.String())
				}
			})
		}
	}
}
