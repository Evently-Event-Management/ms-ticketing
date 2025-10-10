package order_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/order"
	"net/http"

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

	// Return client secret and payment intent ID to the client
	response := struct {
		ClientSecret    string `json:"clientSecret"`
		PaymentIntentID string `json:"paymentIntentId"`
	}{
		ClientSecret:    intent.ClientSecret,
		PaymentIntentID: intent.ID,
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
