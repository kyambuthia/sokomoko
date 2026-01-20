package main

import (
	"fmt"

	"github.com/kyambuthia/sokomoko/internal/routes"
)

func main() {
	fmt.Println("AdminDashboard:", routes.AdminDashboard != nil)
	fmt.Println("AdminProducts:", routes.AdminProducts != nil)
	fmt.Println("AdminOrders:", routes.AdminOrders != nil)
	fmt.Println("AdminReports:", routes.AdminReports != nil)
	fmt.Println("AdminDeliveries:", routes.AdminDeliveries != nil)
}
