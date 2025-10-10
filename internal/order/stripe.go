package order

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/stripe/stripe-go/v74"
	"github.com/stripe/stripe-go/v74/paymentintent"
	"github.com/stripe/stripe-go/v74/webhook"
)

// InitStripe initializes the Stripe API with the secret key
func InitStripe() {
	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")
}

// Use a map to store locks for payment intents - thread safe
var paymentIntentLocks = make(map[string]bool)
var paymentIntentMutex = &sync.Mutex{}

// CreatePaymentIntent creates a Stripe payment intent for an order
func (s *OrderService) CreatePaymentIntent(orderID string) (*stripe.PaymentIntent, error) {
	s.logger.Info("PAYMENT", fmt.Sprintf("Creating payment intent for order: %s", orderID))

	// Use mutex to lock this order ID to prevent race conditions
	paymentIntentMutex.Lock()
	if _, locked := paymentIntentLocks[orderID]; locked {
		// Order is already being processed by another request
		paymentIntentMutex.Unlock()
		s.logger.Warn("PAYMENT", fmt.Sprintf("Payment intent creation for order %s is already in progress", orderID))
		time.Sleep(500 * time.Millisecond)    // Wait briefly
		return s.CreatePaymentIntent(orderID) // Retry after waiting
	}

	// Mark this order as being processed
	paymentIntentLocks[orderID] = true
	paymentIntentMutex.Unlock()

	// Make sure we remove the lock when done
	defer func() {
		paymentIntentMutex.Lock()
		delete(paymentIntentLocks, orderID)
		paymentIntentMutex.Unlock()
	}()

	// Get the order to fetch the price
	order, err := s.DB.GetOrderByID(orderID)
	if err != nil {
		s.logger.Error("PAYMENT", fmt.Sprintf("Failed to find order %s: %v", orderID, err))
		return nil, err
	}

	if order.Status != "pending" {
		s.logger.Warn("PAYMENT", fmt.Sprintf("Cannot create payment intent for order %s with status %s", orderID, order.Status))
		return nil, errors.New("cannot create payment intent for an order that is not pending")
	}

	// Check if the order already has a payment intent ID
	if order.PaymentIntentID != "" {
		s.logger.Info("PAYMENT", fmt.Sprintf("Order %s already has a payment intent %s, retrieving it", orderID, order.PaymentIntentID))

		// Retrieve the existing payment intent
		intent, err := paymentintent.Get(order.PaymentIntentID, nil)
		if err != nil {
			s.logger.Error("PAYMENT", fmt.Sprintf("Failed to retrieve existing Stripe payment intent %s: %v", order.PaymentIntentID, err))
			// If we can't retrieve the existing intent, we'll create a new one
		} else {
			// Return the existing payment intent if it's still valid
			if intent.Status != "canceled" && intent.Status != "succeeded" {
				s.logger.Info("PAYMENT", fmt.Sprintf("Retrieved existing payment intent %s with status %s", intent.ID, intent.Status))
				return intent, nil
			}
			s.logger.Info("PAYMENT", fmt.Sprintf("Existing payment intent %s has status %s, creating a new one", intent.ID, intent.Status))
		}
	}

	// Convert to cents for Stripe
	amountInCents := int64(order.Price * 100)

	// Create payment intent parameters
	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountInCents),
		Currency: stripe.String("lkr"), // Sri Lankan Rupee
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}

	// Add metadata
	params.AddMetadata("order_id", orderID)

	// Create the payment intent
	intent, err := paymentintent.New(params)
	if err != nil {
		s.logger.Error("PAYMENT", fmt.Sprintf("Failed to create Stripe payment intent: %v", err))
		return nil, err
	}

	// Update order with payment intent ID
	order.PaymentIntentID = intent.ID
	err = s.DB.UpdateOrder(*order)
	if err != nil {
		s.logger.Error("PAYMENT", fmt.Sprintf("Failed to update order with payment intent ID: %v", err))
		return nil, err
	}

	s.logger.Info("PAYMENT", fmt.Sprintf("Created payment intent %s for order %s (LKR %0.2f)", intent.ID, orderID, order.Price))
	return intent, nil
}

// WebhookError represents an error that occurred during webhook processing
type WebhookError struct {
	Category      string // "configuration", "validation", "processing"
	StatusCode    int    // HTTP status code
	PublicError   string // Safe to expose to clients
	InternalError string // Detailed error for logs only
	OriginalErr   error  // Underlying error
}

func (e *WebhookError) Error() string {
	return e.InternalError
}

// HandleStripeWebhook processes Stripe webhook events with enhanced error handling
func (s *OrderService) HandleStripeWebhook(r *http.Request) error {
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if webhookSecret == "" {
		s.logger.Error("WEBHOOK", "Stripe webhook secret is not configured")
		return &WebhookError{
			Category:      "configuration",
			StatusCode:    http.StatusInternalServerError,
			PublicError:   "Webhook processing error",
			InternalError: "Stripe webhook secret is not configured",
		}
	}

	// Read the entire request body
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("WEBHOOK", fmt.Sprintf("Failed to read webhook payload: %v", err))
		return &WebhookError{
			Category:      "validation",
			StatusCode:    http.StatusBadRequest,
			PublicError:   "Invalid webhook payload",
			InternalError: fmt.Sprintf("Failed to read webhook payload: %v", err),
			OriginalErr:   err,
		}
	}

	// Verify signature with API version mismatch tolerance
	opts := webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true, // Allow API version mismatches
	}

	event, err := webhook.ConstructEventWithOptions(payload, r.Header.Get("Stripe-Signature"), webhookSecret, opts)
	if err != nil {
		// Classify signature errors
		var errorCategory, errorMessage string
		if stripeErr, ok := err.(*stripe.Error); ok {
			switch stripeErr.Code {
			case "signature_verification_failed":
				errorCategory = "validation"
				errorMessage = "Webhook signature verification failed"
			default:
				errorCategory = "processing"
				errorMessage = "Stripe API error"
			}
		} else {
			errorCategory = "validation"
			errorMessage = "Invalid webhook signature"
		}

		s.logger.Error("WEBHOOK", fmt.Sprintf("%s: %v", errorMessage, err))
		return &WebhookError{
			Category:      errorCategory,
			StatusCode:    http.StatusBadRequest,
			PublicError:   errorMessage,
			InternalError: fmt.Sprintf("%s: %v", errorMessage, err),
			OriginalErr:   err,
		}
	}

	s.logger.Info("WEBHOOK", fmt.Sprintf("Processing Stripe webhook event: %s", event.Type))

	// Handle the event
	switch event.Type {
	case "payment_intent.succeeded":
		// Payment was successful, extract payment intent from event
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			s.logger.Error("WEBHOOK", fmt.Sprintf("Failed to unmarshal payment intent: %v", err))
			return &WebhookError{
				Category:      "processing",
				StatusCode:    http.StatusBadRequest,
				PublicError:   "Invalid event data",
				InternalError: fmt.Sprintf("Failed to unmarshal payment intent: %v", err),
				OriginalErr:   err,
			}
		}

		// Get order ID from metadata
		orderID, exists := paymentIntent.Metadata["order_id"]
		if !exists {
			s.logger.Error("WEBHOOK", "Payment intent has no order_id in metadata")
			return &WebhookError{
				Category:      "processing",
				StatusCode:    http.StatusBadRequest,
				PublicError:   "Invalid payment intent data",
				InternalError: "Payment intent has no order_id in metadata",
			}
		}

		// Complete the order
		err = s.Checkout(orderID)
		if err != nil {
			s.logger.Error("WEBHOOK", fmt.Sprintf("Failed to checkout order %s: %v", orderID, err))
			return &WebhookError{
				Category:      "processing",
				StatusCode:    http.StatusInternalServerError,
				PublicError:   "Failed to process payment",
				InternalError: fmt.Sprintf("Failed to checkout order %s: %v", orderID, err),
				OriginalErr:   err,
			}
		}

		s.logger.Info("WEBHOOK", fmt.Sprintf("Successfully processed payment for order %s", orderID))

	case "payment_intent.payment_failed":
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			s.logger.Error("WEBHOOK", fmt.Sprintf("Failed to unmarshal payment intent: %v", err))
			return &WebhookError{
				Category:      "processing",
				StatusCode:    http.StatusBadRequest,
				PublicError:   "Invalid event data",
				InternalError: fmt.Sprintf("Failed to unmarshal payment intent: %v", err),
				OriginalErr:   err,
			}
		}

		orderID, exists := paymentIntent.Metadata["order_id"]
		if !exists {
			s.logger.Error("WEBHOOK", "Failed payment intent has no order_id in metadata")
			return &WebhookError{
				Category:      "processing",
				StatusCode:    http.StatusBadRequest,
				PublicError:   "Invalid payment intent data",
				InternalError: "Failed payment intent has no order_id in metadata",
			}
		}

		// Cancel the order
		err = s.CancelOrder(orderID)
		if err != nil {
			s.logger.Error("WEBHOOK", fmt.Sprintf("Failed to cancel order %s after payment failure: %v", orderID, err))
			return &WebhookError{
				Category:      "processing",
				StatusCode:    http.StatusInternalServerError,
				PublicError:   "Failed to cancel order after payment failure",
				InternalError: fmt.Sprintf("Failed to cancel order %s after payment failure: %v", orderID, err),
				OriginalErr:   err,
			}
		}

		s.logger.Info("WEBHOOK", fmt.Sprintf("Cancelled order %s due to payment failure", orderID))

	default:
		s.logger.Info("WEBHOOK", fmt.Sprintf("Unhandled event type: %s", event.Type))
	}

	return nil
}
