package partner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/money"
	"github.com/kyambuthia/sokomoko/internal/slug"
)

const maxProductPrice money.Cents = 10_000_000 // 100,000 major units

const (
	MaxProductImageBytes  = db.MaxProductImageBytes
	MaxProductVideoBytes  = db.MaxProductVideoBytes
	maxProductStock       = 1_000_000
	maxProductName        = 200
	maxDescriptionLn      = 5000
	maxPartnerProductPage = 1_000_000
)

type Product struct {
	ID            int
	Name          string
	Slug          string
	Category      string
	PartnerName   string
	Price         money.Cents
	Currency      string
	StockQuantity int
	PartnerID     int
}

type Category struct {
	ID   int
	Name string
}

type PartnerOption struct {
	ID   int
	Name string
}

type ProductsData struct {
	Settings   StoreSettings
	Products   []Product
	Categories []Category
	Partners   []PartnerOption
}

type ProductImageUpload struct {
	ContentType string
	Data        []byte
}

type ProductVideoUpload struct {
	ContentType string
	Data        []byte
}

type CreateProductInput struct {
	Name        string
	Description string
	Price       string
	Stock       string
	CategoryID  string
	PartnerID   string
	ImageURL    string
	Image       *ProductImageUpload
	VideoURL    string
	Video       *ProductVideoUpload
	RemoveVideo bool
}

// ProductInput holds the editable fields of a product as typed in a form.
type ProductInput struct {
	Name        string
	Description string
	Price       string
	Stock       string
	CategoryID  string
	ImageURL    string
	Image       *ProductImageUpload
	VideoURL    string
	Video       *ProductVideoUpload
	// RemoveImage deletes blob-backed uploads when Image is nil. A new upload
	// replaces those uploads instead, and URL-only images stay either way.
	RemoveImage bool
	RemoveVideo bool
}

func (s *Service) ProductsForActor(ctx context.Context, actor *Actor) (ProductsData, error) {
	data, _, err := s.productsForActor(ctx, actor, 0, 0)
	return data, err
}

// ProductOptionsForActor loads the shared form options without reading the catalog.
func (s *Service) ProductOptionsForActor(ctx context.Context, actor *Actor) (ProductsData, error) {
	if !canUseWorkspace(actor) {
		return ProductsData{}, ErrForbiddenWorkspace
	}
	settings, err := s.requireStoreSettings(ctx)
	if err != nil {
		return ProductsData{}, err
	}
	data := ProductsData{Settings: *settings}
	partnerID := 0
	if actorIsPartner(actor) {
		partnerID, err = s.partnerIDForActor(ctx, actor)
		if err != nil {
			return ProductsData{}, err
		}
		if partnerID == 0 {
			return data, nil
		}
	}
	categories, err := s.store.GetAllCategories(ctx)
	if err != nil {
		return ProductsData{}, err
	}
	partners, err := s.store.ListPartners(ctx)
	if err != nil {
		return ProductsData{}, err
	}
	for _, category := range categories {
		data.Categories = append(data.Categories, Category{ID: category.ID, Name: category.Name})
	}
	for _, partner := range partners {
		if partner.Status == "active" && (!actorIsPartner(actor) || partner.ID == partnerID) {
			data.Partners = append(data.Partners, PartnerOption{ID: partner.ID, Name: partner.Name})
		}
	}
	return data, nil
}

func (s *Service) ProductsPageForActor(ctx context.Context, actor *Actor, page, limit int) (ProductsData, int, error) {
	if page < 1 {
		page = 1
	} else if page > maxPartnerProductPage {
		page = maxPartnerProductPage
	}
	if limit < 1 {
		limit = 50
	} else if limit > 50 {
		limit = 50
	}
	return s.productsForActor(ctx, actor, limit, (page-1)*limit)
}

func (s *Service) productsForActor(ctx context.Context, actor *Actor, limit, offset int) (ProductsData, int, error) {
	if !canUseWorkspace(actor) {
		return ProductsData{}, 0, ErrForbiddenWorkspace
	}
	settings, err := s.requireStoreSettings(ctx)
	if err != nil {
		return ProductsData{}, 0, err
	}
	filter := db.ProductFilter{Limit: limit, Offset: offset}
	if actorIsPartner(actor) {
		partnerID, err := s.partnerIDForActor(ctx, actor)
		if err != nil {
			return ProductsData{}, 0, err
		}
		if partnerID == 0 {
			return ProductsData{Settings: *settings}, 0, nil
		}
		filter.PartnerID = partnerID
	}
	products, total, err := s.store.ListProductsPage(ctx, filter)
	if err != nil {
		return ProductsData{}, 0, err
	}
	categories, err := s.store.GetAllCategories(ctx)
	if err != nil {
		return ProductsData{}, 0, err
	}
	partners, err := s.store.ListPartners(ctx)
	if err != nil {
		return ProductsData{}, 0, err
	}

	data := ProductsData{Settings: *settings}
	for _, p := range products {
		data.Products = append(data.Products, Product{
			ID: p.ID, Name: p.Name, Slug: p.Slug, Category: p.Category, PartnerName: p.PartnerName,
			Price: p.Price, Currency: p.Currency, StockQuantity: p.StockQuantity, PartnerID: int(p.PartnerID.Int64),
		})
	}
	for _, c := range categories {
		data.Categories = append(data.Categories, Category{ID: c.ID, Name: c.Name})
	}
	for _, p := range partners {
		if p.Status == "active" && (!actorIsPartner(actor) || p.ID == filter.PartnerID) {
			data.Partners = append(data.Partners, PartnerOption{ID: p.ID, Name: p.Name})
		}
	}
	return data, total, nil
}

func validateProductImage(image *ProductImageUpload) error {
	if image == nil {
		return nil
	}
	if len(image.Data) == 0 || len(image.Data) > db.MaxProductImageBytes || (image.ContentType != "image/jpeg" && image.ContentType != "image/png") {
		return ErrInvalidProductImage
	}
	return nil
}

func validateProductVideo(video *ProductVideoUpload) error {
	if video == nil {
		return nil
	}
	if len(video.Data) == 0 || len(video.Data) > db.MaxProductVideoBytes ||
		(video.ContentType != "video/mp4" && video.ContentType != "video/webm") {
		return ErrInvalidProductVideo
	}
	return nil
}

func (s *Service) CreateProductForActor(ctx context.Context, actor *Actor, input CreateProductInput) error {
	if !canUseWorkspace(actor) {
		return ErrForbiddenWorkspace
	}
	if err := validateProductImage(input.Image); err != nil {
		return err
	}
	if err := validateProductVideo(input.Video); err != nil {
		return err
	}
	if _, err := s.requireStoreSettings(ctx); err != nil {
		return err
	}
	commerceSettings, err := s.store.GetCommerceSettings(ctx)
	if err != nil {
		return fmt.Errorf("get commerce settings: %w", err)
	}

	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	priceRaw := strings.TrimSpace(input.Price)
	stockRaw := strings.TrimSpace(input.Stock)
	if name == "" || priceRaw == "" || stockRaw == "" {
		return ErrMissingProductFields
	}
	if len([]rune(name)) > maxProductName || len([]rune(description)) > maxDescriptionLn {
		return ErrMissingProductFields
	}

	price, err := money.ParseCurrency(priceRaw, commerceSettings.Currency)
	if err != nil || price > maxProductPrice {
		return ErrInvalidProductPrice
	}
	stock, err := strconv.Atoi(stockRaw)
	if err != nil || stock < 0 || stock > maxProductStock {
		return ErrInvalidProductStock
	}

	product := db.Product{
		Name:          name,
		Description:   description,
		Price:         price,
		Currency:      commerceSettings.Currency,
		StockQuantity: stock,
		CategoryID:    parseOptionalID(input.CategoryID),
		PartnerID:     parseOptionalID(input.PartnerID),
	}
	if input.Image != nil {
		product.Images = []db.ProductImage{{
			AltText: name, ContentType: input.Image.ContentType, Data: input.Image.Data,
		}}
	}
	if input.Video != nil {
		product.Video = &db.ProductVideo{ContentType: input.Video.ContentType, Data: input.Video.Data}
	}

	slugBase := slug.Make(name)
	if slugBase == "" {
		slugBase = "product"
	}
	for attempt := 0; attempt < 10; attempt++ {
		product.Slug = slugBase
		if attempt > 0 {
			product.Slug = fmt.Sprintf("%s-%d-%d", slugBase, time.Now().Unix(), attempt)
		}
		_, err := s.store.CreateProductForActor(ctx, product, actor.ID, actor.Role)
		if err == nil {
			return nil
		}
		if errors.Is(err, db.ErrProductCurrencyChanged) {
			return ErrInvalidProductPrice
		}
		if !errors.Is(err, db.ErrProductSlugConflict) {
			return fmt.Errorf("%w: %w", ErrUnableToCreateProduct, err)
		}
	}
	return ErrUnableToCreateProductSlug
}

// productForActor loads a product the actor may edit. A partner user may only
// edit their own partner's products; anything else reads as not found.
func (s *Service) productForActor(ctx context.Context, actor *Actor, idRaw string) (*db.Product, error) {
	if !canUseWorkspace(actor) {
		return nil, ErrProductNotFound
	}
	id, err := strconv.Atoi(strings.TrimSpace(idRaw))
	if err != nil || id <= 0 {
		return nil, ErrProductNotFound
	}
	product, err := s.store.GetProductByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if product == nil {
		return nil, ErrProductNotFound
	}
	if actorIsPartner(actor) {
		partnerID, err := s.store.PartnerIDForUser(ctx, actor.ID)
		if err != nil {
			return nil, err
		}
		if partnerID == 0 || !product.PartnerID.Valid || int(product.PartnerID.Int64) != partnerID {
			return nil, ErrProductNotFound
		}
	}
	return product, nil
}

// ProductForEdit returns the current values of a product for its edit form.
func (s *Service) ProductForEdit(ctx context.Context, actor *Actor, idRaw string) (int, ProductInput, error) {
	product, err := s.productForActor(ctx, actor, idRaw)
	if err != nil {
		return 0, ProductInput{}, err
	}
	input := ProductInput{
		Name:        product.Name,
		Description: product.Description,
		Price:       money.InputString(product.Price, product.Currency),
		Stock:       strconv.Itoa(product.StockQuantity),
		ImageURL:    product.PrimaryImage(),
		VideoURL:    product.VideoURL,
	}
	if product.CategoryID.Valid {
		input.CategoryID = strconv.FormatInt(product.CategoryID.Int64, 10)
	}
	return product.ID, input, nil
}

// UpdateProductForActor changes a product's details and sets its available
// stock. The slug stays the same so product links keep working. With no new
// upload and RemoveImage false, existing images stay. RemoveImage deletes
// only blob-backed uploads. A new upload replaces those uploads instead.
// Product videos stay unchanged unless a replacement is uploaded or removed.
func (s *Service) UpdateProductForActor(ctx context.Context, actor *Actor, idRaw string, input ProductInput) error {
	product, err := s.productForActor(ctx, actor, idRaw)
	if err != nil {
		return err
	}
	if err := validateProductImage(input.Image); err != nil {
		return err
	}
	if err := validateProductVideo(input.Video); err != nil {
		return err
	}
	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	priceRaw := strings.TrimSpace(input.Price)
	stockRaw := strings.TrimSpace(input.Stock)
	if name == "" || priceRaw == "" || stockRaw == "" {
		return ErrMissingProductFields
	}
	if len([]rune(name)) > maxProductName || len([]rune(description)) > maxDescriptionLn {
		return ErrMissingProductFields
	}
	price, err := parseEditableProductPrice(priceRaw, product)
	if err != nil || price > maxProductPrice {
		return ErrInvalidProductPrice
	}
	stock, err := strconv.Atoi(stockRaw)
	if err != nil || stock < 0 || stock > maxProductStock {
		return ErrInvalidProductStock
	}
	before := fmt.Sprintf("price=%s stock=%d", product.Price, product.StockQuantity)
	product.Name, product.Description, product.Price, product.StockQuantity = name, description, price, stock
	product.CategoryID = parseOptionalID(input.CategoryID)
	if input.Image != nil {
		product.Images = append([]db.ProductImage{{
			AltText: name, ContentType: input.Image.ContentType, Data: input.Image.Data,
		}}, product.Images...)
		for i := range product.Images {
			product.Images[i].DisplayOrder = i
		}
	} else if input.RemoveImage {
		product.RemoveUploadedImages = true
	}
	if input.Video != nil {
		product.Video = &db.ProductVideo{ContentType: input.Video.ContentType, Data: input.Video.Data}
	} else if input.RemoveVideo {
		product.RemoveUploadedVideo = true
	}
	if err := s.store.UpdateProductForActor(ctx, *product, actor.ID, actor.Role); err != nil {
		if errors.Is(err, db.ErrProductNotFound) {
			return ErrProductNotFound
		}
		if errors.Is(err, db.ErrProductCurrencyChanged) {
			return ErrInvalidProductPrice
		}
		return fmt.Errorf("update product: %w", err)
	}
	details := fmt.Sprintf("%s -> price=%s stock=%d", before, price, stock)
	if input.Image != nil {
		details += " image updated"
	} else if input.RemoveImage {
		details += " uploaded image removed"
	}
	if input.Video != nil {
		details += " video updated"
	} else if input.RemoveVideo {
		details += " video removed"
	}
	s.logAudit(ctx, actor.ID, "product.update", "product", product.ID, details)
	return nil
}

func parseEditableProductPrice(raw string, product *db.Product) (money.Cents, error) {
	if _, ok := money.LookupCurrency(product.Currency); !ok {
		return money.Parse(raw)
	}
	price, err := money.ParseCurrency(raw, product.Currency)
	if err == nil {
		return price, nil
	}
	currency, _ := money.LookupCurrency(product.Currency)
	if currency.MinorDigits == 0 {
		legacyPrice, legacyErr := money.Parse(raw)
		if legacyErr == nil && legacyPrice == product.Price {
			return legacyPrice, nil
		}
	}
	return 0, err
}

// DeleteProductForActor soft-deletes a product the actor may manage and clears its public data.
func (s *Service) DeleteProductForActor(ctx context.Context, actor *Actor, idRaw string) error {
	product, err := s.productForActor(ctx, actor, idRaw)
	if err != nil {
		return err
	}
	if err := s.store.DeleteProductForActor(ctx, product.ID, actor.ID, actor.Role); err != nil {
		if errors.Is(err, db.ErrProductNotFound) {
			return ErrProductNotFound
		}
		return fmt.Errorf("delete product: %w", err)
	}
	s.logAudit(ctx, actor.ID, "product.delete", "product", product.ID, product.Name)
	return nil
}

func parseOptionalID(raw string) sql.NullInt64 {
	id, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || id <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(id), Valid: true}
}
