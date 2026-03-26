package routes

import (
	"net/http"

	partnersvc "github.com/kyambuthia/sokomoko/internal/service/partner"
)

func partnerIdentity(r *http.Request) (role string, username string) {
	role = "staff"
	user := requestUserFromContext(r)
	if user != nil {
		role = user.Role
		username = user.Username
	}
	return role, username
}

func partnerActorFromContext(r *http.Request) *partnersvc.Actor {
	user := requestUserFromContext(r)
	if user == nil {
		return nil
	}
	return &partnersvc.Actor{
		ID:   user.ID,
		Role: user.Role,
	}
}

func partnerDashboardPage(view partnersvc.DashboardData, role, username string) PartnerDashboardData {
	return PartnerDashboardData{
		Title:            "Partner Dashboard",
		StoreName:        view.Settings.StoreName,
		StoreSlug:        view.Settings.StoreSlug,
		Description:      view.Settings.Description,
		ContactEmail:     view.Settings.ContactEmail,
		ProductCount:     view.ProductCount,
		NewOrders:        view.OrderSummary.NewCount,
		InProgressOrders: view.OrderSummary.InProgressCount,
		DispatchedOrders: view.OrderSummary.DispatchedCount,
		CompletedOrders:  view.OrderSummary.CompletedCount,
		OverdueOrders:    view.OrderSummary.OverdueCount,
		Role:             role,
		Username:         username,
	}
}

func partnerProductsPage(view partnersvc.ProductsData) PartnerProductsData {
	return PartnerProductsData{
		Title:      "Partner Products",
		StoreName:  view.Settings.StoreName,
		Products:   view.Products,
		Categories: view.Categories,
	}
}

func partnerProductNewPage(categories []partnersvc.Category) PartnerProductNewData {
	return PartnerProductNewData{
		Title:      "Add Product",
		Categories: categories,
	}
}

func partnerOrdersPage(view partnersvc.OrdersData) PartnerOrdersPageData {
	return PartnerOrdersPageData{
		Title:           "Partner Orders",
		Orders:          view.Orders,
		Filter:          view.Filter,
		NewCount:        view.Summary.NewCount,
		InProgressCount: view.Summary.InProgressCount,
		DispatchedCount: view.Summary.DispatchedCount,
		CompletedCount:  view.Summary.CompletedCount,
		OverdueCount:    view.Summary.OverdueCount,
	}
}
