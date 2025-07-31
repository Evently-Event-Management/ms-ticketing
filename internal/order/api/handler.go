package api

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"ms-ticketing/internal/order"
	"ms-ticketing/internal/order/db"
	"net/http"
)

type Handler struct {
	OrderService *order.OrderService
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req db.Order

	// Decode the incoming JSON body into an Order struct
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Place the order using service logic
	err := h.OrderService.PlaceOrder(req)
	if err != nil {
		http.Error(w, "Could not place order: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond with status and created order ID
	resp := map[string]interface{}{
		"status":  "created",
		"orderId": req.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	order, err := h.OrderService.GetOrder(orderID)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(order)
}

func (h *Handler) UpdateOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")

	var updateData db.Order
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	updateData.ID = orderID // Ensure the ID is set

	err := h.OrderService.UpdateOrder(orderID, updateData)
	if err != nil {
		http.Error(w, "Could not update order: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Order updated successfully")) // ✅ Write response
}

func (h *Handler) DeleteOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")

	// Call the service to delete the order
	err := h.OrderService.CancelOrder(orderID)
	if err != nil {
		http.Error(w, "Could not delete order: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent) // No content response for successful deletion
	w.Write([]byte("Order deleted successfully"))
}

func (h *Handler) ApplyPromo(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")

	var promo struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&promo); err != nil {
		http.Error(w, "Invalid promo code JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if promo.Code == "" {
		http.Error(w, "Promo code cannot be empty", http.StatusBadRequest)
		return
	}

	if err := h.OrderService.ApplyPromoCode(orderID, promo.Code); err != nil {
		http.Error(w, "Failed to apply promo: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Promo code applied successfully"}`))
}

func (h *Handler) CheckoutOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")

	if err := h.OrderService.Checkout(orderID); err != nil {
		http.Error(w, "Failed to checkout order: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Order checked out successfully"}`))
}
