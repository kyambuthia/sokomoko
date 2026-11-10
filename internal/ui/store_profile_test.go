package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
)

func TestStorefrontNavRailAndTabs(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := templates.Index.ExecuteTemplate(&body, "tabbar", map[string]any{"Tab": "services", "Page": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	railStart := strings.Index(html, `<nav class="rail"`)
	tabStart := strings.Index(html, `<nav class="tabbar"`)
	if railStart < 0 || tabStart < 0 || railStart > tabStart {
		t.Fatalf("rail and tab bar: %s", html)
	}
	rail := html[railStart:tabStart]
	tabs := html[tabStart:]
	for _, label := range []string{"Shop", "Search", "Services", "Cart", "Messages", "Account"} {
		if !strings.Contains(rail, ">"+label+"<") && !strings.Contains(rail, ">"+label+"</span>") {
			t.Fatalf("rail missing %s: %s", label, rail)
		}
	}
	if !strings.Contains(rail, `href="/services"`) || !strings.Contains(rail, `aria-current="page"`) {
		t.Fatalf("services is not current: %s", rail)
	}
	for _, label := range []string{"Shop</a>", "Search</a>", ">Cart", "Account</a>"} {
		if !strings.Contains(tabs, label) {
			t.Fatalf("tab bar missing %s: %s", label, tabs)
		}
	}
	if strings.Contains(tabs, "Messages") || strings.Contains(tabs, "Services") {
		t.Fatalf("phone tab bar gained a desktop destination: %s", tabs)
	}
	body.Reset()
	if err := templates.Index.ExecuteTemplate(&body, "storefront-header", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	header := body.String()
	if !strings.Contains(header, "bar--storefront") || !strings.Contains(header, `href="/messages"`) {
		t.Fatalf("phone header: %s", header)
	}
	if strings.Contains(header, "Services") || strings.Contains(header, `href="/cart"`) {
		t.Fatalf("phone header still carries desktop nav: %s", header)
	}
	body.Reset()
	if err := templates.Index.ExecuteTemplate(&body, "main-area", map[string]any{"HasProducts": false}); err != nil {
		t.Fatal(err)
	}
	main := body.String()
	if !strings.Contains(main, `class="btn btn-secondary btn-sm mobile-only" href="/services"`) {
		t.Fatalf("shop is missing the phone services link: %s", main)
	}
	if !strings.Contains(main, "desktop-only") || !strings.Contains(main, `action="/search"`) {
		t.Fatalf("shop is missing the wide search field: %s", main)
	}
}

func TestStorefrontFooterIsAColophon(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if err := templates.Index.ExecuteTemplate(&body, "storefront-footer", struct{}{}); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	if !strings.Contains(html, `class="brand footer__brand"`) || !strings.Contains(html, `aria-label="sokomoko"`) {
		t.Fatalf("footer missing the wordmark: %s", html)
	}
	for _, gone := range []string{"Prices in", "Sign in", "Sign up", `href="/login"`, `href="/signup"`, `href="/search"`, `href="/services"`, `href="/cart"`, `href="/account"`, "desktop-only"} {
		if strings.Contains(html, gone) {
			t.Fatalf("footer still carries %q: %s", gone, html)
		}
	}
}

func TestStoreProfileRendersSendMessage(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{
		"Name": "Urban Goods", "Slug": "urban_goods", "PartnerID": 3, "Initials": "UG",
		"CanMessage": true, "ProductTotal": 0, "ServiceTotal": 1, "CSRFToken": "tok",
		"Services": []db.ServiceListing{{
			Slug: "cut", Title: "Cut", Category: "hair", PublicArea: "Westlands",
			DeliveryMode: "premises", DurationMinutes: 30, PriceMode: "fixed",
			PriceCents: 10000, Currency: "KES",
		}},
	}
	var body bytes.Buffer
	if err := templates.Store.ExecuteTemplate(&body, "main-area", data); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	for _, want := range []string{"Urban Goods", "@urban_goods", "UG", `href="/messages/new?partner_id=3"`, "Send message", `href="/services/cut"`, "No products listed yet."} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %s", want, html)
		}
	}
	data["CanMessage"] = false
	body.Reset()
	if err := templates.Store.ExecuteTemplate(&body, "main-area", data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body.String(), "Send message") {
		t.Fatalf("own shop still offers a message: %s", body.String())
	}
}

type storeProductStub struct {
	ID                      int
	Name, Slug, PartnerName string
	PartnerSlug, ImageURL   string
	ImageAlt, Category      string
	Description, VideoURL   string
	Price                   money.Cents
	StockQuantity           int
}

func (p storeProductStub) InStock() bool { return p.StockQuantity > 0 }

func TestSoldByLinksToStore(t *testing.T) {
	templates, err := ParseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	product := storeProductStub{
		ID: 5, Name: "Leather Backpack", Slug: "leather-backpack",
		PartnerName: "Urban Goods", PartnerSlug: "urban_goods",
		ImageURL: "/static/a.png", ImageAlt: "bag", Price: 450000, StockQuantity: 4,
	}
	var body bytes.Buffer
	if err := templates.Product.ExecuteTemplate(&body, "main-area", map[string]any{"Product": product, "CSRFToken": "tok"}); err != nil {
		t.Fatal(err)
	}
	html := body.String()
	if !strings.Contains(html, `href="/stores/urban_goods"`) || !strings.Contains(html, "Sold by") {
		t.Fatalf("sold-by link missing: %s", html)
	}
	if !strings.Contains(html, `href="/messages/new?product_id=5"`) {
		t.Fatalf("product message missing: %s", html)
	}
}
