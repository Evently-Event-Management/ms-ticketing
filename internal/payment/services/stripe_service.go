package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/client"
)

var (
	ErrStripeAPIError         = errors.New("stripe API error")
	ErrStripeClientInitFailed = errors.New("failed to initialize Stripe client")
	ErrCardValidationFailed   = errors.New("card validation failed")
)

// StripeService handles integration with Stripe payment gateway
type StripeService struct {
	client *client.API
	log    *logger.Logger
}

// parseStringToInt64 safely converts a string to int64, returns 0 if conversion fails
func parseStringToInt64(s string) int64 {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// NewStripeService creates a new instance of StripeService
func NewStripeService(log *logger.Logger) (*StripeService, error) {
	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeKey == "" {
		log.Error("STRIPE", "STRIPE_SECRET_KEY environment variable not set")
		return nil, ErrStripeClientInitFailed
	}

	sc := client.New(stripeKey, nil)
	if sc == nil {
		log.Error("STRIPE", "Failed to initialize Stripe client")
		return nil, ErrStripeClientInitFailed
	}

	log.Info("STRIPE", "Stripe client initialized successfully")
	return &StripeService{
		client: sc,
		log:    log,
	}, nil
}

// ValidateCard validates the provided card details using Stripe
func (s *StripeService) ValidateCard(card *models.StripeCard) (*models.StripeCardValidationResponse, error) {
	// Create a payment method to validate the card
	params := &stripe.PaymentMethodParams{
		Type: stripe.String("card"),
		Card: &stripe.PaymentMethodCardParams{
			Number:   stripe.String(card.Number),
			ExpMonth: stripe.Int64(parseStringToInt64(card.ExpMonth)),
			ExpYear:  stripe.Int64(parseStringToInt64(card.ExpYear)),
			CVC:      stripe.String(card.CVC),
		},
	}

	pm, err := s.client.PaymentMethods.New(params)
	if err != nil {
		s.log.Error("STRIPE", fmt.Sprintf("Card validation failed: %v", err))

		return &models.StripeCardValidationResponse{
			Valid:   false,
			Message: err.Error(),
		}, nil
	}

	// If we get here, the card is valid
	response := &models.StripeCardValidationResponse{
		Valid:    true,
		Message:  "Card is valid",
		CardType: string(pm.Card.Brand),
		Last4:    pm.Card.Last4,
	}

	s.log.Info("VALIDATE", fmt.Sprintf("Card validation successful: %s ending in %s", response.CardType, response.Last4))

	// Clean up the payment method since we don't need it anymore
	_, err = s.client.PaymentMethods.Detach(pm.ID, &stripe.PaymentMethodDetachParams{})
	if err != nil {
		s.log.Warn("STRIPE", fmt.Sprintf("Failed to detach payment method: %v", err))
	}

	return response, nil
}

// ProcessPayment processes a payment through Stripe
func (s *StripeService) ProcessPayment(ctx context.Context, req *models.StripePaymentRequest) (*models.StripePaymentResponse, error) {
	paymentIdentifier := req.PaymentID
	if paymentIdentifier == "" {
		paymentIdentifier = "new"
	}

	s.log.Info("PROCESS", fmt.Sprintf("Processing Stripe payment for order %s, amount: %.2f %s (paymentIdentifier: %s)",
		req.OrderID, req.Amount, req.Currency, paymentIdentifier))

	// Validate that we have an amount to charge
	if req.Amount <= 0 {
		s.log.Error("ERROR", fmt.Sprintf("Invalid amount for order %s: %.2f (paymentIdentifier: %s)", req.OrderID, req.Amount, paymentIdentifier))
		return nil, fmt.Errorf("invalid payment amount: %.2f", req.Amount)
	}

	var paymentMethod string
	if req.Token != "" {
		paymentMethod = req.Token
		s.log.Info("STRIPE", fmt.Sprintf("Using provided token/payment method ID (paymentIdentifier: %s)", paymentIdentifier))
	} else if req.Card != nil {
		// Legacy/test: create payment method from card
		pmParams := &stripe.PaymentMethodParams{
			Type: stripe.String("card"),
			Card: &stripe.PaymentMethodCardParams{
				Number:   stripe.String(req.Card.Number),
				ExpMonth: stripe.Int64(parseStringToInt64(req.Card.ExpMonth)),
				ExpYear:  stripe.Int64(parseStringToInt64(req.Card.ExpYear)),
				CVC:      stripe.String(req.Card.CVC),
			},
		}
		if req.Card.Name != "" {
			pmParams.BillingDetails = &stripe.PaymentMethodBillingDetailsParams{
				Name: stripe.String(req.Card.Name),
			}
			if req.Card.Address != nil {
				pmParams.BillingDetails.Address = &stripe.AddressParams{
					Line1:      stripe.String(req.Card.Address.Line1),
					Line2:      stripe.String(req.Card.Address.Line2),
					City:       stripe.String(req.Card.Address.City),
					State:      stripe.String(req.Card.Address.State),
					PostalCode: stripe.String(req.Card.Address.PostalCode),
					Country:    stripe.String(req.Card.Address.Country),
				}
			}
		}
		s.log.Info("STRIPE", fmt.Sprintf("Creating payment method from card (paymentID: %s)", req.PaymentID))
		pm, err := s.client.PaymentMethods.New(pmParams)
		if err != nil {
			s.log.Error("STRIPE", fmt.Sprintf("Failed to create payment method: %v", err))
			return nil, fmt.Errorf("%w: %v", ErrStripeAPIError, err)
		}
		paymentMethod = pm.ID
		s.log.Info("STRIPE", fmt.Sprintf("Payment method created: %s (paymentID: %s)", pm.ID, req.PaymentID))
	} else {
		return nil, fmt.Errorf("%w: no payment method provided", ErrStripeAPIError)
	}

	// Convert amount to cents (Stripe uses smallest currency unit)
	amountInCents := int64(req.Amount * 100)
	metadata := make(map[string]string)
	metadata["payment_id"] = req.PaymentID
	metadata["order_id"] = req.OrderID

	// Add any additional metadata from the request
	for k, v := range req.Metadata {
		metadata[k] = v
	}

	// Create a payment intent
	piParams := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(amountInCents),
		Currency:           stripe.String(req.Currency),
		PaymentMethod:      stripe.String(paymentMethod),
		Description:        stripe.String(req.Description),
		Metadata:           metadata,
		ConfirmationMethod: stripe.String("manual"),
		Confirm:            stripe.Bool(true),
		PaymentMethodTypes: []*string{stripe.String("card")},
	}

	s.log.Info("STRIPE", fmt.Sprintf("Creating payment intent (paymentID: %s)", req.PaymentID))
	pi, err := s.client.PaymentIntents.New(piParams)
	if err != nil {
		s.log.Error("STRIPE", fmt.Sprintf("Failed to create payment intent: %v", err))
		return nil, fmt.Errorf("%w: %v", ErrStripeAPIError, err)
	}
	s.log.Info("STRIPE", fmt.Sprintf("Payment intent created: %s (paymentID: %s)", pi.ID, req.PaymentID))

	// Handle payment intent status
	var status models.PaymentStatus
	switch pi.Status {
	case stripe.PaymentIntentStatusSucceeded:
		status = models.StatusSuccess
		s.log.Info("STRIPE", fmt.Sprintf("Payment succeeded (paymentID: %s)", req.PaymentID))
	case stripe.PaymentIntentStatusProcessing:
		status = models.StatusPending
		s.log.Info("STRIPE", fmt.Sprintf("Payment is processing (paymentID: %s)", req.PaymentID))
	case stripe.PaymentIntentStatusRequiresAction:
		status = models.StatusPending
		s.log.Info("STRIPE", fmt.Sprintf("Payment requires further action (paymentID: %s)", req.PaymentID))
	default:
		status = models.StatusFailed
		s.log.Error("STRIPE", fmt.Sprintf("Payment failed with status: %s (paymentID: %s)", pi.Status, req.PaymentID))
	}

	// Create response
	response := &models.StripePaymentResponse{
		PaymentID:     req.PaymentID,
		OrderID:       req.OrderID,
		Status:        status,
		Amount:        float64(pi.Amount) / 100.0, // Convert back from cents
		Currency:      string(pi.Currency),
		TransactionID: pi.ID,
		PaymentMethod: paymentMethod,
		Created:       pi.Created,
	}

	if pi.LatestCharge != nil && pi.LatestCharge.ID != "" {
		charge, err := s.client.Charges.Get(pi.LatestCharge.ID, nil)
		if err == nil && charge.ReceiptURL != "" {
			response.ReceiptURL = charge.ReceiptURL
		}
	}

	return response, nil
}
