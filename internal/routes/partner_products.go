package routes

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/app"
	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
	"github.com/kyambuthia/sokomoko/internal/ui"
)

const partnerProductsPageSize = 50

type PartnerProductsData struct {
	Role       string
	Title      string
	CSRFToken  string
	StoreName  string
	Products   []partnersvc.Product
	Pager      Pager
	Message    string
	Error      string
	Categories []partnersvc.Category
}

type PartnerProductNewData struct {
	Role           string
	Title          string
	Message        string
	Error          string
	Categories     []partnersvc.Category
	Partners       []partnersvc.PartnerOption
	Form           partnersvc.CreateProductInput
	DefaultName    string
	CSRFToken      string
	EditID         int  // set when the form edits an existing product
	CanRemoveImage bool // current primary image is a stored upload
	RemoveImage    bool // redisplay the explicit remove checkbox after a rejected save
	CanRemoveVideo bool // current product video is stored
	RemoveVideo    bool // redisplay the explicit remove checkbox after a rejected save
}

func PartnerProducts(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		requestedPage := pageParam(r)
		view, total, err := svc.ProductsPageForActor(r.Context(), partnerActorFromContext(r), requestedPage, partnerProductsPageSize)
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case err != nil:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if total == 0 && requestedPage > 1 {
			http.Redirect(w, r, pageHref(r, 1), http.StatusSeeOther)
			return
		}
		if redirectPastLastPage(w, r, requestedPage, partnerProductsPageSize, total) {
			return
		}
		page := partnerProductsPage(view)
		page.Pager = newPager(r, requestedPage, partnerProductsPageSize, total)
		page.Role, _ = partnerIdentity(r)
		page.CSRFToken = a.Auth.CSRFToken(r)
		if flash := popFlash(w, r); flash.Message != "" {
			if flash.Kind == "danger" {
				page.Error = flash.Message
			} else {
				page.Message = flash.Message
			}
		}
		a.Render(w, a.Templates.PartnerProducts, page)
	}
}

func PartnerProductNew(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		view, err := svc.ProductOptionsForActor(r.Context(), partnerActorFromContext(r))
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case err != nil:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := partnerProductNewPage(view.Categories, a.Auth.CSRFToken(r))
		data.Role, _ = partnerIdentity(r)
		data.Partners = view.Partners

		if r.Method == http.MethodGet {
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		imageUpload, videoUpload, mediaErr := parseProductMediaUpload(w, r)
		if errors.Is(mediaErr, errProductMediaBusy) {
			data.Error = productMediaBusyMessage
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusServiceUnavailable, data)
			return
		}
		input := partnersvc.CreateProductInput{
			Name:        r.FormValue("name"),
			Description: r.FormValue("description"),
			Price:       r.FormValue("price"),
			Stock:       r.FormValue("stock_quantity"),
			CategoryID:  r.FormValue("category_id"),
			PartnerID:   r.FormValue("partner_id"),
			Image:       imageUpload,
			Video:       videoUpload,
		}
		data.Form = input
		data.DefaultName = strings.TrimSpace(input.Name)
		if mediaErr != nil {
			data.Error = productMediaUploadMessage(mediaErr)
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		}

		err = svc.CreateProductForActor(r.Context(), partnerActorFromContext(r), input)
		if err == nil {
			redirectWithFlash(w, r, "/products", "success", "Product created")
			return
		}
		if errors.Is(err, partnersvc.ErrStoreNotConfigured) {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}
		if msg, ok := partnerProductValidationMessage(err); ok {
			data.Error = msg
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		}
		if errors.Is(err, partnersvc.ErrUnableToCreateProductSlug) {
			data.Error = "Unable to create product slug"
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		}
		slog.ErrorContext(r.Context(), "partner create product", "error", err)
		data.Error = "Unable to create product"
		a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusInternalServerError, data)
	}
}

// PartnerProductEdit shows and saves the edit form for one product.
func PartnerProductEdit(a *app.App) http.HandlerFunc {
	svc := a.Partner

	return func(w http.ResponseWriter, r *http.Request) {
		actor := partnerActorFromContext(r)
		view, err := svc.ProductOptionsForActor(r.Context(), actor)
		switch {
		case errors.Is(err, partnersvc.ErrStoreNotConfigured):
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		case err != nil:
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		idRaw := r.PathValue("id")
		id, current, err := svc.ProductForEdit(r.Context(), actor, idRaw)
		switch {
		case errors.Is(err, partnersvc.ErrProductNotFound):
			http.NotFound(w, r)
			return
		case err != nil:
			slog.ErrorContext(r.Context(), "partner load product", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		data := partnerProductNewPage(view.Categories, a.Auth.CSRFToken(r))
		data.Role, _ = partnerIdentity(r)
		data.Title = "Edit Product"
		data.EditID = id
		imageURL := ui.ProductImageURL(current.ImageURL, current.Name, current.Name, "")
		data.CanRemoveImage = strings.HasPrefix(current.ImageURL, "/media/product-images/")
		data.CanRemoveVideo = current.VideoURL != ""
		data.Form = partnersvc.CreateProductInput{Name: current.Name, Description: current.Description,
			Price: current.Price, Stock: current.Stock, CategoryID: current.CategoryID, ImageURL: imageURL,
			VideoURL: current.VideoURL}
		data.DefaultName = current.Name

		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			a.Render(w, a.Templates.PartnerProductNew, data)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		imageUpload, videoUpload, mediaErr := parseProductMediaUpload(w, r)
		if errors.Is(mediaErr, errProductMediaBusy) {
			data.Error = productMediaBusyMessage
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusServiceUnavailable, data)
			return
		}
		input := partnersvc.ProductInput{
			Name:        r.FormValue("name"),
			Description: r.FormValue("description"),
			Price:       r.FormValue("price"),
			Stock:       r.FormValue("stock_quantity"),
			CategoryID:  r.FormValue("category_id"),
			ImageURL:    imageURL,
			Image:       imageUpload,
			VideoURL:    current.VideoURL,
			Video:       videoUpload,
			RemoveImage: r.FormValue("remove_image") == "1",
			RemoveVideo: r.FormValue("remove_video") == "1",
		}
		data.Form = partnersvc.CreateProductInput{Name: input.Name, Description: input.Description,
			Price: input.Price, Stock: input.Stock, CategoryID: input.CategoryID, ImageURL: imageURL,
			VideoURL: current.VideoURL, RemoveVideo: input.RemoveVideo}
		data.DefaultName = strings.TrimSpace(input.Name)
		data.RemoveImage = input.RemoveImage
		data.RemoveVideo = input.RemoveVideo
		if mediaErr != nil {
			data.Error = productMediaUploadMessage(mediaErr)
			a.RenderStatus(w, a.Templates.PartnerProductNew, http.StatusBadRequest, data)
			return
		}

		err = svc.UpdateProductForActor(r.Context(), actor, idRaw, input)
		if err == nil {
			redirectWithFlash(w, r, "/products", "success", "Product updated")
			return
		}
		if errors.Is(err, partnersvc.ErrProductNotFound) {
			http.NotFound(w, r)
			return
		}
		status := http.StatusBadRequest
		if msg, ok := partnerProductValidationMessage(err); ok {
			data.Error = msg
		} else {
			slog.ErrorContext(r.Context(), "partner update product", "error", err)
			data.Error = "Unable to update product"
			status = http.StatusInternalServerError
		}
		a.RenderStatus(w, a.Templates.PartnerProductNew, status, data)
	}
}

// PartnerProductDelete soft-deletes a product after the product service verifies ownership.
func PartnerProductDelete(a *app.App) http.HandlerFunc {
	svc := a.Partner
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		err := svc.DeleteProductForActor(r.Context(), partnerActorFromContext(r), r.PathValue("id"))
		switch {
		case errors.Is(err, partnersvc.ErrProductNotFound):
			http.NotFound(w, r)
			return
		case err != nil:
			slog.ErrorContext(r.Context(), "partner delete product", "error", err)
			http.Error(w, "Unable to delete product", http.StatusInternalServerError)
			return
		}
		redirectWithFlash(w, r, "/products", "success", "Product deleted")
	}
}

func partnerProductValidationMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, partnersvc.ErrMissingProductFields):
		return "Name, price and stock are required", true
	case errors.Is(err, partnersvc.ErrInvalidProductPrice):
		return "Price must be valid for the store currency; UGX and RWF use whole units (up to 100000)", true
	case errors.Is(err, partnersvc.ErrInvalidProductStock):
		return "Stock must be a non-negative integer", true
	case errors.Is(err, partnersvc.ErrInvalidProductImage):
		return invalidProductImageMessage, true
	case errors.Is(err, partnersvc.ErrInvalidProductVideo):
		return invalidProductVideoMessage, true
	default:
		return "", false
	}
}
