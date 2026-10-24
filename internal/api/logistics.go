package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	logisticsvc "github.com/kyambuthia/sokomoko/internal/service/logistics"
)

type courierAssignmentInput struct {
	OrderID   int `json:"order_id"`
	CourierID int `json:"courier_id"`
}

type courierUpdateInput struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

type courierOptionDTO struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type courierItemDTO struct {
	ProductName string `json:"product_name"`
	Quantity    int    `json:"quantity"`
}

type courierDeliveryDTO struct {
	ID              int              `json:"id,omitempty"`
	OrderID         int              `json:"order_id"`
	CourierUserID   int              `json:"courier_user_id,omitempty"`
	CourierName     string           `json:"courier_username,omitempty"`
	CustomerName    string           `json:"customer_username"`
	DeliveryAddress string           `json:"delivery_address"`
	OrderStatus     string           `json:"order_status"`
	PartnerStatus   string           `json:"partner_status"`
	DeliveryStatus  string           `json:"delivery_status"`
	Status          string           `json:"status,omitempty"`
	FailureNote     string           `json:"failure_note,omitempty"`
	CreatedAt       time.Time        `json:"order_created_at"`
	AssignedAt      time.Time        `json:"assigned_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	Items           []courierItemDTO `json:"items"`
}

func (h *Handler) adminCourierDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	board, err := h.app.Logistics.DispatchBoard(r.Context(), logisticsActor(r))
	if err != nil {
		writeLogisticsError(w, err)
		return
	}
	couriers := make([]courierOptionDTO, 0, len(board.Couriers))
	for _, courier := range board.Couriers {
		couriers = append(couriers, courierOptionDTO{ID: courier.ID, Username: courier.Username})
	}
	orders := make([]courierDeliveryDTO, 0, len(board.Orders))
	for _, delivery := range board.Orders {
		orders = append(orders, mapCourierDelivery(delivery))
	}
	writeData(w, http.StatusOK, struct {
		Couriers []courierOptionDTO   `json:"couriers"`
		Orders   []courierDeliveryDTO `json:"orders"`
	}{Couriers: couriers, Orders: orders})
}

func (h *Handler) adminCourierAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input courierAssignmentInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	if err := h.app.Logistics.Assign(r.Context(), logisticsActor(r), strconv.Itoa(input.OrderID), strconv.Itoa(input.CourierID)); err != nil {
		writeLogisticsError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "Courier assignment saved"})
}

func (h *Handler) courierDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	deliveries, err := h.app.Logistics.Jobs(r.Context(), logisticsActor(r))
	if err != nil {
		writeLogisticsError(w, err)
		return
	}
	items := make([]courierDeliveryDTO, 0, len(deliveries))
	for _, delivery := range deliveries {
		items = append(items, mapCourierDelivery(delivery))
	}
	writeData(w, http.StatusOK, items)
}

func (h *Handler) courierDeliveryUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, apiPrefix+"/courier/deliveries/"), "/")
	if _, ok := parsePositiveID(id); !ok {
		writeError(w, http.StatusBadRequest, "invalid_delivery_id", "Delivery id must be a positive integer")
		return
	}
	var input courierUpdateInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	if err := h.app.Logistics.Update(r.Context(), logisticsActor(r), id, input.Status, input.Note); err != nil {
		writeLogisticsError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]string{"message": "Delivery status updated"})
}

func mapCourierDelivery(delivery db.CourierDelivery) courierDeliveryDTO {
	items := make([]courierItemDTO, 0, len(delivery.Items))
	for _, item := range delivery.Items {
		items = append(items, courierItemDTO{ProductName: item.ProductName, Quantity: item.Quantity})
	}
	return courierDeliveryDTO{
		ID: delivery.ID, OrderID: delivery.OrderID, CourierUserID: delivery.CourierUserID,
		CourierName: delivery.CourierName, CustomerName: delivery.CustomerName,
		DeliveryAddress: delivery.DeliveryAddress, OrderStatus: delivery.OrderStatus,
		PartnerStatus: delivery.PartnerStatus, DeliveryStatus: delivery.DeliveryStatus,
		Status: delivery.Status, FailureNote: delivery.FailureNote,
		CreatedAt: delivery.CreatedAt, AssignedAt: delivery.AssignedAt,
		UpdatedAt: delivery.UpdatedAt, Items: items,
	}
}

func logisticsActor(r *http.Request) *logisticsvc.Actor {
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		return nil
	}
	return &logisticsvc.Actor{ID: user.ID, Role: user.Role}
}

func writeLogisticsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, logisticsvc.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have access to this delivery")
	case errors.Is(err, logisticsvc.ErrInvalidOrderID), errors.Is(err, logisticsvc.ErrInvalidCourierID), errors.Is(err, logisticsvc.ErrInvalidDeliveryID):
		writeError(w, http.StatusBadRequest, "invalid_delivery", "Choose a valid order, courier, or delivery id")
	case errors.Is(err, logisticsvc.ErrCourierNotFound):
		writeError(w, http.StatusNotFound, "courier_not_found", "Courier account was not found")
	case errors.Is(err, logisticsvc.ErrDeliveryNotFound):
		writeError(w, http.StatusNotFound, "delivery_not_found", "Delivery was not found")
	case errors.Is(err, logisticsvc.ErrFailureNoteRequired):
		writeError(w, http.StatusBadRequest, "failure_reason_required", "Choose a reason when delivery fails")
	case errors.Is(err, logisticsvc.ErrOrderNotReady):
		writeError(w, http.StatusConflict, "order_not_ready", "Order is not ready for courier assignment")
	case errors.Is(err, logisticsvc.ErrDeliveryStarted):
		writeError(w, http.StatusConflict, "delivery_started", "A picked up delivery cannot be reassigned")
	case errors.Is(err, logisticsvc.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_delivery_transition", "That delivery update is not allowed")
	default:
		writeError(w, http.StatusInternalServerError, "logistics_unavailable", "Unable to process delivery request")
	}
}
