package order_api

import (
	"encoding/json"
	"fmt"
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
	fmt.Println("CreateOrder: received request")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("CreateOrder: failed to decode body: %v\n", err)
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Printf("CreateOrder: decoded order: %+v\n", req)

	err := h.OrderService.PlaceOrder(req)
	if err != nil {
		fmt.Printf("CreateOrder: failed to place order: %v\n", err)
		http.Error(w, "Could not place order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Printf("CreateOrder: order placed successfully, ID: %v\n", req.ID)

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
	fmt.Printf("GetOrder: orderId=%s\n", orderID)

	order, err := h.OrderService.GetOrder(orderID)
	if err != nil {
		fmt.Printf("GetOrder: order not found: %v\n", err)
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}
	fmt.Printf("GetOrder: found order: %+v\n", order)

	json.NewEncoder(w).Encode(order)
}

func (h *Handler) UpdateOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	fmt.Printf("UpdateOrder: orderId=%s\n", orderID)

	var updateData db.Order
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		fmt.Printf("UpdateOrder: failed to decode body: %v\n", err)
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Printf("UpdateOrder: decoded update data: %+v\n", updateData)

	updateData.ID = orderID

	err := h.OrderService.UpdateOrder(orderID, updateData)
	if err != nil {
		fmt.Printf("UpdateOrder: failed to update order: %v\n", err)
		http.Error(w, "Could not update order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Println("UpdateOrder: order updated successfully")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Order updated successfully"))
}

func (h *Handler) DeleteOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	fmt.Printf("DeleteOrder: orderId=%s\n", orderID)

	err := h.OrderService.CancelOrder(orderID)
	if err != nil {
		fmt.Printf("DeleteOrder: failed to delete order: %v\n", err)
		http.Error(w, "Could not delete order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Println("DeleteOrder: order deleted successfully")

	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte("Order deleted successfully"))
}

func (h *Handler) ApplyPromo(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	fmt.Printf("ApplyPromo: orderId=%s\n", orderID)

	var promo struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&promo); err != nil {
		fmt.Printf("ApplyPromo: failed to decode promo: %v\n", err)
		http.Error(w, "Invalid promo code JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Printf("ApplyPromo: promo code: %s\n", promo.Code)

	if promo.Code == "" {
		fmt.Println("ApplyPromo: promo code is empty")
		http.Error(w, "Promo code cannot be empty", http.StatusBadRequest)
		return
	}

	if err := h.OrderService.ApplyPromoCode(orderID, promo.Code); err != nil {
		fmt.Printf("ApplyPromo: failed to apply promo: %v\n", err)
		http.Error(w, "Failed to apply promo: "+err.Error(), http.StatusBadRequest)
		return
	}
	fmt.Println("ApplyPromo: promo code applied successfully")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Promo code applied successfully"}`))
}

func (h *Handler) CheckoutOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	fmt.Printf("CheckoutOrder: orderId=%s\n", orderID)

	if err := h.OrderService.Checkout(orderID); err != nil {
		fmt.Printf("CheckoutOrder: failed to checkout: %v\n", err)
		http.Error(w, "Failed to checkout order: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Println("CheckoutOrder: order checked out successfully")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"message":"Order checked out successfully"}`))
}
