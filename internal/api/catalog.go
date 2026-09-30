package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"
)

type catalogListDTO struct {
	Items []productDTO `json:"items"`
	Page  int          `json:"page"`
	Limit int          `json:"limit"`
	Total int          `json:"total"`
}

func (h *Handler) catalogProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	products, err := h.app.Catalog.AllProducts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "catalog_unavailable", "Unable to load products")
		return
	}
	page, limit := pageParams(r)
	start := (page - 1) * limit
	if start > len(products) {
		start = len(products)
	}
	end := start + limit
	if end > len(products) {
		end = len(products)
	}
	items := make([]productDTO, 0, end-start)
	for _, product := range products[start:end] {
		items = append(items, mapProduct(product))
	}

	w.Header().Set("Cache-Control", "public, max-age=30")
	writeData(w, http.StatusOK, catalogListDTO{Items: items, Page: page, Limit: limit, Total: len(products)})
}

func (h *Handler) catalogSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeData(w, http.StatusOK, catalogListDTO{Items: []productDTO{}, Page: 1, Limit: defaultPageSize, Total: 0})
		return
	}
	products, err := h.app.Catalog.Search(query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search_unavailable", "Unable to search products")
		return
	}
	page, limit := pageParams(r)
	start := (page - 1) * limit
	if start > len(products) {
		start = len(products)
	}
	end := start + limit
	if end > len(products) {
		end = len(products)
	}
	items := make([]productDTO, 0, end-start)
	for _, product := range products[start:end] {
		items = append(items, mapProduct(product))
	}

	w.Header().Set("Cache-Control", "public, max-age=15")
	writeData(w, http.StatusOK, catalogListDTO{Items: items, Page: page, Limit: limit, Total: len(products)})
}

func (h *Handler) catalogProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	slug := pathTail(r.URL.Path, apiPrefix+"/catalog/products/")
	if slug == "" {
		writeError(w, http.StatusNotFound, "product_not_found", "Product not found")
		return
	}
	product, err := h.app.Catalog.ProductBySlug(slug)
	if errors.Is(err, catalogsvc.ErrInvalidProductSlug) {
		writeError(w, http.StatusBadRequest, "invalid_product_slug", "Invalid product slug")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "catalog_unavailable", "Unable to load product")
		return
	}
	if product == nil {
		writeError(w, http.StatusNotFound, "product_not_found", "Product not found")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeData(w, http.StatusOK, mapProduct(*product))
}

func pageParams(r *http.Request) (int, int) {
	page := parseQueryInt(r, "page", 1)
	limit := parseQueryInt(r, "limit", defaultPageSize)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultPageSize
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	return page, limit
}

func parseQueryInt(r *http.Request, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil {
		return fallback
	}
	return value
}
