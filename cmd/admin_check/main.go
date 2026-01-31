package main

import (
	"fmt"

	"github.com/kyambuthia/sokomoko/internal/routes"
)

func main() {
	_ = routes.AdminDashboard
	_ = routes.AdminProducts
	_ = routes.AdminOrders
	_ = routes.AdminReports
	_ = routes.AdminDeliveries

	fmt.Println("Admin handlers wired")
}
