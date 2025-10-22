package order_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/order"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// CreatePaymentIntent creates a payment intent for an order
func (h *Handler) CreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderId")
	h.Logger.Info("API", fmt.Sprintf("CreatePaymentIntent: orderId=%s", orderID))

	if orderID == "" {
		h.Logger.Error("API", "CreatePaymentIntent: order ID is required")
		http.Error(w, "Order ID is required", http.StatusBadRequest)
		return
	}

	// Create payment intent
	intent, err := h.OrderService.CreatePaymentIntent(orderID)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("CreatePaymentIntent: failed to create payment intent: %v", err))
		http.Error(w, "Failed to create payment intent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get order with tickets for comprehensive response
	orderWithTickets, err := h.OrderService.GetOrderWithTickets(orderID)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("CreatePaymentIntent: failed to get order with tickets: %v", err))
		// Continue anyway as we have the payment intent
	}

	// Get seat lock duration from environment variables
	totalLockDurationMinutes := 5 // Default to 5 minutes
	lockTTLStr := os.Getenv("SEAT_LOCK_TTL_MINUTES")
	if lockTTLStr != "" {
		if duration, err := strconv.Atoi(lockTTLStr); err == nil {
			totalLockDurationMinutes = duration
			h.Logger.Debug("API", fmt.Sprintf("Using seat lock duration of %d minutes from environment", totalLockDurationMinutes))
		} else {
			h.Logger.Warn("API", fmt.Sprintf("Invalid SEAT_LOCK_TTL_MINUTES value: %s, using default 5 minutes", lockTTLStr))
		}
	} else {
		h.Logger.Debug("API", "SEAT_LOCK_TTL_MINUTES not set, using default 5 minutes")
	}

	// Calculate remaining time based on order's CreatedAt
	order, err := h.OrderService.GetOrder(orderID)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("CreatePaymentIntent: failed to get order for remaining time calculation: %v", err))
		// Fallback to total duration if we can't get the order
		http.Error(w, "Failed to calculate remaining time", http.StatusInternalServerError)
		return
	}

	// Calculate elapsed time since order creation
	elapsedTime := time.Since(order.CreatedAt)
	elapsedMinutes := int(elapsedTime.Minutes())

	// Calculate remaining time
	remainingMinutes := totalLockDurationMinutes - elapsedMinutes
	if remainingMinutes < 0 {
		remainingMinutes = 0 // Seat lock has expired
		h.Logger.Warn("API", fmt.Sprintf("Order %s seat lock has expired (elapsed: %d mins, limit: %d mins)", orderID, elapsedMinutes, totalLockDurationMinutes))
	}

	h.Logger.Info("API", fmt.Sprintf("Order %s: elapsed=%d mins, remaining=%d mins", orderID, elapsedMinutes, remainingMinutes))

	// Return client secret, payment intent ID, remaining seat lock time, and order details to the client
	response := struct {
		ClientSecret         string                   `json:"clientSecret"`
		PaymentIntentID      string                   `json:"paymentIntentId"`
		SeatLockDurationMins int                      `json:"seatLockDurationMins"`
		Order                *models.OrderWithTickets `json:"order,omitempty"`
	}{
		ClientSecret:         intent.ClientSecret,
		PaymentIntentID:      intent.ID,
		SeatLockDurationMins: remainingMinutes,
		Order:                orderWithTickets,
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("CreatePaymentIntent: failed to encode response: %v", err))
		return
	}
	h.Logger.Info("API", fmt.Sprintf("CreatePaymentIntent: created payment intent for order %s", orderID))
}

// StripeWebhook handles webhook events from Stripe
func (h *Handler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	h.Logger.Info("API", "StripeWebhook: received webhook event")

	// Process the webhook
	err := h.OrderService.HandleStripeWebhook(r)
	if err != nil {
		h.Logger.Error("API", fmt.Sprintf("StripeWebhook: failed to process webhook: %v", err))

		// Check if it's a WebhookError with detailed information
		if webhookErr, ok := err.(*order.WebhookError); ok {
			// Return appropriate status code and error message
			h.Logger.Info("API", fmt.Sprintf("StripeWebhook: handling webhook error category=%s, status=%d",
				webhookErr.Category, webhookErr.StatusCode))

			// Return the public error message
			http.Error(w, webhookErr.PublicError, webhookErr.StatusCode)
			return
		}

		// Default error handling
		http.Error(w, "Webhook processing error", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	h.Logger.Info("API", "StripeWebhook: successfully processed webhook event")
}
