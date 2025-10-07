package order_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/order"
	tickets "ms-ticketing/internal/tickets/service"
	"net/http"
	"strings"

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

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("API", "CreateOrder: received request")

	var req models.Order
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.Logger.Error("API", fmt.Sprintf("CreateOrder: failed to decode body: %v", err))
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	h.Logger.Debug("API", fmt.Sprintf("CreateOrder: decoded order: %+v", req))

	err := h.OrderService.PlaceOrder(req, false) // false = do not skip locking
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("CreateOrder: failed to place order: %v", err))
		http.Error(w, "Could not place order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.Logger.Info("API", fmt.Sprintf("CreateOrder: order placed successfully, OrderID: %v", req.OrderID))

	resp := map[string]interface{}{
		"status":  "created",
		"orderId": req.OrderID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
	h.Logger.Info("API", "CreateOrder: response sent successfully")
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
	json.NewEncoder(w).Encode(orderData)
	h.Logger.Info("API", "GetOrder: response sent successfully")
}

func (h *Handler) UpdateOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	h.Logger.Info("API", fmt.Sprintf("UpdateOrder: orderId=%s", orderID))

	// Parse the raw request to better understand what's being sent
	var rawData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rawData); err != nil {
		h.Logger.Error("API", fmt.Sprintf("UpdateOrder: failed to decode body: %v", err))
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	h.Logger.Debug("API", fmt.Sprintf("UpdateOrder: raw request data: %+v", rawData))

	// Create an update data object
	updateData := models.Order{
		OrderID: orderID,
	}

	// Handle status field if present
	if status, ok := rawData["status"].(string); ok && status != "" {
		updateData.Status = status
	}

	// Handle user_id field if present
	if userID, ok := rawData["user_id"].(string); ok && userID != "" {
		updateData.UserID = userID
	}

	// Handle seat_ids or seat_id field - handle both array and comma-separated string
	seatIDs := []string{}

	// Try seat_ids first (array)
	if seatIDsRaw, ok := rawData["seat_ids"].([]interface{}); ok {
		for _, id := range seatIDsRaw {
			if idStr, ok := id.(string); ok {
				seatIDs = append(seatIDs, idStr)
			}
		}
	}

	// Try seat_id (singular, could be array or string)
	if len(seatIDs) == 0 {
		if seatIDArray, ok := rawData["seat_id"].([]interface{}); ok {
			// It's an array
			for _, id := range seatIDArray {
				if idStr, ok := id.(string); ok {
					// Check if it contains a comma
					if strings.Contains(idStr, ",") {
						// Split by comma
						parts := strings.Split(idStr, ",")
						for _, part := range parts {
							seatIDs = append(seatIDs, strings.TrimSpace(part))
						}
					} else {
						seatIDs = append(seatIDs, idStr)
					}
				}
			}
		} else if seatIDStr, ok := rawData["seat_id"].(string); ok {
			// It's a string - check if comma-separated
			if strings.Contains(seatIDStr, ",") {
				parts := strings.Split(seatIDStr, ",")
				for _, part := range parts {
					seatIDs = append(seatIDs, strings.TrimSpace(part))
				}
			} else {
				seatIDs = append(seatIDs, seatIDStr)
			}
		}
	}

	if len(seatIDs) > 0 {
		updateData.SeatIDs = seatIDs
	}

	h.Logger.Debug("API", fmt.Sprintf("UpdateOrder: processed update data: %+v", updateData))

	// Get current order to check if it exists
	currentOrder, err := h.OrderService.GetOrder(orderID)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("UpdateOrder: order not found: %v", err))
		http.Error(w, "Order not found: "+err.Error(), http.StatusNotFound)
		return
	}

	h.Logger.Debug("API", fmt.Sprintf("UpdateOrder: current order data: %+v", currentOrder))

	err = h.OrderService.UpdateOrder(orderID, updateData)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("UpdateOrder: failed to update order: %v", err))
		http.Error(w, "Could not update order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.Logger.Info("API", "UpdateOrder: order updated successfully")

	// Return the updated order
	updatedOrder, err := h.OrderService.GetOrder(orderID)
	if err != nil {
		h.Logger.Warn("API", fmt.Sprintf("UpdateOrder: could not fetch updated order: %v", err))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedOrder)
	h.Logger.Info("API", "UpdateOrder: response sent successfully")
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

func (h *Handler) CheckoutOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	h.Logger.Info("API", fmt.Sprintf("CheckoutOrder: orderId=%s", orderID))

	if err := h.OrderService.Checkout(orderID); err != nil {
		h.Logger.Error("API", fmt.Sprintf("CheckoutOrder: failed to checkout: %v", err))
		http.Error(w, "Failed to checkout order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.Logger.Info("API", "CheckoutOrder: order checked out successfully")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Order checked out successfully"}`))
	h.Logger.Info("API", "CheckoutOrder: response sent successfully")
}

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
	json.NewEncoder(w).Encode(response)
	h.Logger.Info("API", "SeatValidationAndPlaceOrder: order created successfully")
}
