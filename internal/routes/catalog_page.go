package routes

import catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"

type IndexPageData struct {
	Title       string
	HasProducts bool
	Products    []catalogsvc.Product
	CSRFToken   string
}

type SearchPageData struct {
	Title     string
	Query     string
	Products  []catalogsvc.Product
	NoResults bool
	CSRFToken string
}

func indexPage(products []catalogsvc.Product, csrfToken string) IndexPageData {
	return IndexPageData{
		Title:       "Home",
		HasProducts: len(products) > 0,
		Products:    products,
		CSRFToken:   csrfToken,
	}
}

func productPage(product *catalogsvc.Product, csrfToken string) ProductPageData {
	return ProductPageData{
		Title:     product.Name,
		Product:   product,
		CSRFToken: csrfToken,
	}
}

func searchPage(query string, products []catalogsvc.Product, csrfToken string) SearchPageData {
	return SearchPageData{
		Title:     "Search",
		Query:     query,
		Products:  products,
		NoResults: len(products) == 0 && query != "",
		CSRFToken: csrfToken,
	}
}
