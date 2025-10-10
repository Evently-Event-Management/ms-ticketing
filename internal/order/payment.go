package order

import (
	"fmt"

	"github.com/stripe/stripe-go/v74"
	"github.com/stripe/stripe-go/v74/paymentintent"
)

// CancelPaymentIntent cancels a Stripe payment intent associated with an order
func (s *OrderService) CancelPaymentIntent(paymentIntentID string) error {
	s.logger.Info("PAYMENT", fmt.Sprintf("Cancelling payment intent: %s", paymentIntentID))

	// Stripe API call to cancel the payment intent
	params := &stripe.PaymentIntentCancelParams{
		CancellationReason: stringPtr(string(stripe.PaymentIntentCancellationReasonAbandoned)),
	}

	_, err := paymentintent.Cancel(paymentIntentID, params)
	if err != nil {
		s.logger.Error("PAYMENT", fmt.Sprintf("Failed to cancel payment intent %s: %v", paymentIntentID, err))
		return fmt.Errorf("failed to cancel payment intent: %w", err)
	}

	s.logger.Info("PAYMENT", fmt.Sprintf("Successfully cancelled payment intent: %s", paymentIntentID))
	return nil
}

// Helper function to create a string pointer
func stringPtr(s string) *string {
	return &s
}
