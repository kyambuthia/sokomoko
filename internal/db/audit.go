package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) UpdateOrderFulfillment(orderID int, partnerStatus, deliveryStatus, deliveryNotice string) error {
	var currentPartnerStatus string
	var currentDeliveryStatus string
	err := s.DB.QueryRow(
		"SELECT partner_status, delivery_status FROM orders WHERE id = ?",
		orderID,
	).Scan(&currentPartnerStatus, &currentDeliveryStatus)
	if err == sql.ErrNoRows {
		return ErrOrderNotFound
	}
	if err != nil {
		return err
	}

	if !isAllowedPartnerTransition(currentPartnerStatus, partnerStatus) {
		return ErrInvalidPartnerTransition
	}
	if !isAllowedDeliveryTransition(currentDeliveryStatus, deliveryStatus) {
		return ErrInvalidDeliveryTransition
	}

	statusMap := map[string]string{
		"new":        "pending",
		"accepted":   "processing",
		"packing":    "processing",
		"dispatched": "shipped",
		"completed":  "delivered",
		"cancelled":  "cancelled",
	}
	nextStatus, ok := statusMap[partnerStatus]
	if !ok {
		return ErrInvalidPartnerStatus
	}

	validDelivery := map[string]bool{
		"queued":     true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
	}
	if !validDelivery[deliveryStatus] {
		return ErrInvalidDeliveryStatus
	}

	result, err := s.DB.Exec(
		`UPDATE orders
		 SET status = ?, partner_status = ?, delivery_status = ?, delivery_notice = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		nextStatus, partnerStatus, deliveryStatus, strings.TrimSpace(deliveryNotice), orderID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func isAllowedPartnerTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"new": {
			"new":       true,
			"accepted":  true,
			"cancelled": true,
		},
		"accepted": {
			"accepted":  true,
			"packing":   true,
			"cancelled": true,
		},
		"packing": {
			"packing":    true,
			"dispatched": true,
			"cancelled":  true,
		},
		"dispatched": {
			"dispatched": true,
			"completed":  true,
		},
		"completed": {
			"completed": true,
		},
		"cancelled": {
			"cancelled": true,
		},
	}
	return allowed[from][to]
}

func isAllowedDeliveryTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"queued": {
			"queued":     true,
			"processing": true,
			"shipped":    true,
			"delivered":  true,
		},
		"processing": {
			"processing": true,
			"shipped":    true,
			"delivered":  true,
		},
		"shipped": {
			"shipped":   true,
			"delivered": true,
		},
		"delivered": {
			"delivered": true,
		},
	}
	return allowed[from][to]
}

func (s *Store) GetPartnerOrderSummary() (PartnerOrderSummary, error) {
	summary := PartnerOrderSummary{}

	row := s.DB.QueryRow(
		`SELECT
		    SUM(CASE WHEN partner_status = 'new' THEN 1 ELSE 0 END),
		    SUM(CASE WHEN partner_status IN ('accepted', 'packing') THEN 1 ELSE 0 END),
		    SUM(CASE WHEN partner_status = 'dispatched' THEN 1 ELSE 0 END),
		    SUM(CASE WHEN partner_status = 'completed' THEN 1 ELSE 0 END)
		 FROM orders
		 WHERE status != 'cancelled'`,
	)
	var newCount, inProgress, dispatched, completed sql.NullInt64
	if err := row.Scan(&newCount, &inProgress, &dispatched, &completed); err != nil {
		return summary, err
	}

	if newCount.Valid {
		summary.NewCount = int(newCount.Int64)
	}
	if inProgress.Valid {
		summary.InProgressCount = int(inProgress.Int64)
	}
	if dispatched.Valid {
		summary.DispatchedCount = int(dispatched.Int64)
	}
	if completed.Valid {
		summary.CompletedCount = int(completed.Int64)
	}

	overdueRow := s.DB.QueryRow(
		`SELECT COUNT(*)
		 FROM orders
		 WHERE partner_status IN ('new', 'accepted')
		   AND status != 'cancelled'
		   AND created_at <= datetime('now', '-2 hours')`,
	)
	if err := overdueRow.Scan(&summary.OverdueCount); err != nil {
		return summary, err
	}

	return summary, nil
}

func (s *Store) ListAllOrders() ([]FulfillmentOrder, error) {
	rows, err := s.DB.Query(
		`SELECT o.id, o.user_id, u.username, o.status, o.partner_status, o.delivery_status, COALESCE(o.delivery_notice, ''), o.delivery_address, o.total_amount, o.created_at
		 FROM orders o
		 JOIN users u ON u.id = o.user_id
		 ORDER BY o.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := []FulfillmentOrder{}
	orderIDs := []int{}
	for rows.Next() {
		order := FulfillmentOrder{}
		if err := rows.Scan(
			&order.ID,
			&order.CustomerUserID,
			&order.CustomerName,
			&order.Status,
			&order.PartnerStatus,
			&order.DeliveryStatus,
			&order.DeliveryNotice,
			&order.DeliveryAddress,
			&order.TotalAmount,
			&order.CreatedAt,
		); err != nil {
			return nil, err
		}
		order.TotalAmount = RoundMoney(order.TotalAmount)
		orders = append(orders, order)
		orderIDs = append(orderIDs, order.ID)
	}

	itemsByOrder, err := s.listOrderItemsByOrderIDs(orderIDs)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}
	return orders, nil
}

func (s *Store) UpdateOrderByAdmin(orderID int, status, partnerStatus, deliveryStatus, deliveryNotice string) error {
	validStatus := map[string]bool{
		"pending":    true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
		"cancelled":  true,
	}
	validPartner := map[string]bool{
		"new":        true,
		"accepted":   true,
		"packing":    true,
		"dispatched": true,
		"completed":  true,
		"cancelled":  true,
	}
	validDelivery := map[string]bool{
		"queued":     true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
	}
	if !validStatus[status] || !validPartner[partnerStatus] || !validDelivery[deliveryStatus] {
		return ErrInvalidOrderState
	}

	result, err := s.DB.Exec(
		`UPDATE orders
		 SET status = ?, partner_status = ?, delivery_status = ?, delivery_notice = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		status, partnerStatus, deliveryStatus, strings.TrimSpace(deliveryNotice), orderID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func (s *Store) GetOrderStatusCounts() (map[string]int, error) {
	rows, err := s.DB.Query("SELECT status, COUNT(*) FROM orders GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{
		"pending":    0,
		"processing": 0,
		"shipped":    0,
		"delivered":  0,
		"cancelled":  0,
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, nil
}

func (s *Store) SumOrderRevenue() (float64, error) {
	row := s.DB.QueryRow("SELECT COALESCE(SUM(total_amount), 0) FROM orders WHERE status != 'cancelled'")
	var total float64
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return RoundMoney(total), nil
}

func (s *Store) CreateAuditLog(actorUserID int, action, targetType string, targetID int, details string) error {
	if strings.TrimSpace(action) == "" || strings.TrimSpace(targetType) == "" {
		return fmt.Errorf("action and target_type are required")
	}

	var actor sql.NullInt64
	if actorUserID > 0 {
		actor = sql.NullInt64{Int64: int64(actorUserID), Valid: true}
	}
	var target sql.NullInt64
	if targetID > 0 {
		target = sql.NullInt64{Int64: int64(targetID), Valid: true}
	}

	_, err := s.DB.Exec(
		"INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, details) VALUES (?, ?, ?, ?, ?)",
		actor, strings.TrimSpace(action), strings.TrimSpace(targetType), target, strings.TrimSpace(details),
	)
	return err
}

func (s *Store) ListAuditLogs(limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.DB.Query(
		"SELECT id, actor_user_id, action, target_type, target_id, COALESCE(details, ''), created_at FROM audit_logs ORDER BY created_at DESC, id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := []AuditLog{}
	for rows.Next() {
		entry := AuditLog{}
		if err := rows.Scan(
			&entry.ID,
			&entry.ActorUserID,
			&entry.Action,
			&entry.TargetType,
			&entry.TargetID,
			&entry.Details,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	return logs, nil
}
