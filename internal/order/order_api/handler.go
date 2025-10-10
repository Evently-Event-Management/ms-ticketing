package order_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/order"
	tickets "ms-ticketing/internal/tickets/service"
	"net/http"

	"github.com/go-chi/chi/v5"

	// Import the logger package
	"ms-ticketing/internal/logger"
)

type Handler struct {
	OrderService  *order.OrderService
	TicketService *tickets.TicketService
	Logger        *logger.Logger
}

func NewHandler(orderService *order.OrderService, ticketService *tickets.TicketService) *Handler {
	return &Handler{
		OrderService:  orderService,
		TicketService: ticketService,
		Logger:        logger.NewLogger(), // Initialize logger
	}
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	h.Logger.Info("API", fmt.Sprintf("GetOrder: orderId=%s", orderID))

	orderData, err := h.OrderService.GetOrder(orderID)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("GetOrder: order not found: %v", err))
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}
	h.Logger.Debug("API", fmt.Sprintf("GetOrder: found order: %+v", orderData))

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(orderData)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("GetOrder: failed to encode response: %v", err))
		return
	}
	h.Logger.Info("API", "GetOrder: response sent successfully")
}

func (h *Handler) DeleteOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	h.Logger.Info("API", fmt.Sprintf("DeleteOrder: orderId=%s", orderID))

	err := h.OrderService.CancelOrder(orderID)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("DeleteOrder: failed to cancel order: %v", err))
		http.Error(w, "Could not cancel order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.Logger.Info("API", "DeleteOrder: order cancelled successfully")

	w.WriteHeader(http.StatusNoContent)
	h.Logger.Info("API", "DeleteOrder: response sent successfully")
}

// func (h *Handler) ApplyPromo(w http.ResponseWriter, r *http.Request) {
// 	orderID := chi.URLParam(r, "orderId")
// 	h.logger.Info("API", fmt.Sprintf("ApplyPromo: orderId=%s", orderID))

// 	var promo struct {
// 		Code string `json:"code"`
// 	}
// 	if err := json.NewDecoder(r.Body).Decode(&promo); err != nil {
// 		h.logger.Error("API", fmt.Sprintf("ApplyPromo: failed to decode promo: %v", err))
// 		http.Error(w, "Invalid promo code JSON: "+err.Error(), http.StatusBadRequest)
// 		return
// 	}
// 	h.logger.Debug("API", fmt.Sprintf("ApplyPromo: promo code: %s", promo.Code))

// 	if promo.Code == "" {
// 		h.logger.Warn("API", "ApplyPromo: promo code is empty")
// 		http.Error(w, "Promo code cannot be empty", http.StatusBadRequest)
// 		return
// 	}

// 	if err := h.OrderService.ApplyPromoCode(orderID, promo.Code); err != nil {
// 		h.logger.Error("API", fmt.Sprintf("ApplyPromo: failed to apply promo: %v", err))
// 		http.Error(w, "Failed to apply promo: "+err.Error(), http.StatusBadRequest)
// 		return
// 	}
// 	h.logger.Info("API", "ApplyPromo: promo code applied successfully")

// 	w.Header().Set("Content-Type", "application/json")
// 	w.WriteHeader(http.StatusOK)
// 	w.Write([]byte(`{"message":"Promo code applied successfully"}`))
// }

func (h *Handler) SeatValidationAndPlaceOrder(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("API", "SeatValidationAndPlaceOrder: received request")

	// Parse the JSON request body
	var orderReq models.OrderRequest

	if err := json.NewDecoder(r.Body).Decode(&orderReq); err != nil {
		h.Logger.Error("API", fmt.Sprintf("SeatValidationAndPlaceOrder: failed to decode request body: %v", err))
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	h.Logger.Debug("API", fmt.Sprintf("SeatValidationAndPlaceOrder: SessionID: %s", orderReq.SessionID))
	h.Logger.Debug("API", fmt.Sprintf("SeatValidationAndPlaceOrder: SeatIDs: %v", orderReq.SeatIDs))

	// Call service
	response, err := h.OrderService.SeatValidationAndPlaceOrder(r, orderReq)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("SeatValidationAndPlaceOrder: seat validation failed: %v", err))
		http.Error(w, "Seat validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Success
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("SeatValidationAndPlaceOrder: failed to encode response: %v", err))
		return
	}
	h.Logger.Info("API", "SeatValidationAndPlaceOrder: order created successfully")
}

func (h *Handler) GetOrdersWithTicketsByUserID(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	h.Logger.Info("API", fmt.Sprintf("GetOrdersWithTicketsByUserID: userId=%s", userID))

	if userID == "" {
		h.Logger.Error("API", "GetOrdersWithTicketsByUserID: user ID is required")
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Use the new method that includes QR codes
	ordersWithTicketsAndQR, err := h.OrderService.GetOrdersWithTicketsAndQRByUserID(userID)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("GetOrdersWithTicketsByUserID: failed to get orders with tickets and QR codes: %v", err))
		http.Error(w, "Failed to retrieve orders: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.Logger.Debug("API", fmt.Sprintf("GetOrdersWithTicketsByUserID: found %d orders for user %s", len(ordersWithTicketsAndQR), userID))

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(ordersWithTicketsAndQR)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("GetOrdersWithTicketsByUserID: failed to encode response: %v", err))
		return
	}
	h.Logger.Info("API", fmt.Sprintf("GetOrdersWithTicketsByUserID: response sent successfully for user %s", userID))
}
