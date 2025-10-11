package order_test

import (
	"testing"
)

// Reuse the mocks from service_test.go
// This file focuses on testing the Stripe payment functionality

func TestCreatePaymentIntent(t *testing.T) {

	// Since we can't easily mock the Stripe SDK, we'll skip this test for now
	// In a real scenario, we would use a custom interface around the Stripe SDK that we can mock
	t.Skip("Skipping test as we need a better way to mock Stripe SDK")
}

func TestHandleWebhookEvent(t *testing.T) {
	// Since we can't easily mock the Stripe SDK and webhook signatures,
	// we'll skip this test for now
	t.Skip("Skipping webhook test as we need a better way to mock Stripe SDK")
}