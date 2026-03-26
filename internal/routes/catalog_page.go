package routes

import catalogsvc "github.com/kyambuthia/sokomoko/internal/service/catalog"

type IndexPageData struct {
	HasProducts bool
	Products    []catalogsvc.Product
}

type SearchPageData struct {
	Query     string
	Products  []catalogsvc.Product
	NoResults bool
}

func indexPage(products []catalogsvc.Product) IndexPageData {
	return IndexPageData{
		HasProducts: len(products) > 0,
		Products:    products,
	}
}

func productPage(product *catalogsvc.Product) ProductPageData {
	return ProductPageData{
		Title:   product.Name,
		Product: product,
	}
}

func searchPage(query string, products []catalogsvc.Product) SearchPageData {
	return SearchPageData{
		Query:     query,
		Products:  products,
		NoResults: len(products) == 0 && query != "",
	}
}
