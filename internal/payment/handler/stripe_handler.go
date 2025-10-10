package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"ms-ticketing/internal/kafka"
	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/payment/services"
	"ms-ticketing/internal/payment/storage"
	"ms-ticketing/internal/utils"

	"github.com/gin-gonic/gin"
)

// OrderService interface for updating order status
type OrderService interface {
	GetOrder(orderID string) (*models.Order, error)
	UpdateOrderStatus(orderID string, status string) error
}

type StripeHandler struct {
	stripeService *services.StripeService
	paymentStore  storage.Store
	producer      *kafka.Producer
	orderService  OrderService
	logger        *logger.Logger
}

func NewStripeHandler(stripeService *services.StripeService, paymentStore storage.Store, producer *kafka.Producer, orderService OrderService, logger *logger.Logger) *StripeHandler {
	return &StripeHandler{
		stripeService: stripeService,
		paymentStore:  paymentStore,
		producer:      producer,
		orderService:  orderService,
		logger:        logger,
	}
}

// ValidateCard validates credit card details without creating a charge
func (h *StripeHandler) ValidateCard(c *gin.Context) {
	var req models.StripeCardValidationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request payload", err.Error()))
		return
	}

	// SECURITY ENHANCEMENT: Verify the order exists in our database
	// This ensures we only validate cards for legitimate orders
	_, err := h.paymentStore.GetPaymentByOrderID(req.OrderID)
	if err != nil {
		log.Printf("No existing payment found for order %s during card validation", req.OrderID)
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request",
			"No payment record found for this order_id. Create a payment record first."))
		return
	}

	// Map StripeCardDetails to StripeCard
	card := &models.StripeCard{
		Number:   req.Card.Number,
		ExpMonth: req.Card.ExpMonth,
		ExpYear:  req.Card.ExpYear,
		CVC:      req.Card.CVC,
		Name:     req.Card.Name,
	}
	result, err := h.stripeService.ValidateCard(card)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.ErrorResponse("Card validation failed", err.Error()))
		return
	}

	c.JSON(http.StatusOK, utils.SuccessResponse("Card validation result", result))
}

// ProcessPayment processes a payment through Stripe
func (h *StripeHandler) ProcessPayment(c *gin.Context) {
	var req models.StripePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request payload", err.Error()))
		return
	}

	// Validate order_id is provided
	if req.OrderID == "" {
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request payload", "order_id is required"))
		return
	}

	// Set default currency if not provided
	if req.Currency == "" {
		req.Currency = "lkr"
	}

	// Validate token or card is provided
	if req.Token == "" && req.Card == nil {
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request payload", "Either token or card must be provided"))
		return
	}

	// This prevents the frontend from specifying the amount, which could be a security risk
	existingPayment, err := h.paymentStore.GetPaymentByOrderID(req.OrderID)
	if err != nil {
		log.Printf("No existing payment found for order %s", req.OrderID)
		// Check if this is for a new order with no payment record yet
		// In that case, we should return an error as we can't proceed without knowing the amount
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request",
			"No payment record found for this order_id. Create a payment record first."))
		return
	}

	// Override any amount provided in the request with the amount from the database
	req.Amount = existingPayment.Price
	log.Printf("Using price %.2f from database for order %s", req.Amount, req.OrderID)

	// Also get the payment ID if available and add it to the request
	if existingPayment.PaymentID != "" {
		req.PaymentID = existingPayment.PaymentID
		log.Printf("Using existing payment ID %s for order %s", req.PaymentID, req.OrderID)
	}

	// Process payment through Stripe
	result, err := h.stripeService.ProcessPayment(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.ErrorResponse("Payment processing failed", err.Error()))
		return
	}

	// Update existing payment record with results from Stripe
	if result.Status == models.StatusSuccess || result.Status == models.StatusPending {
		// We already have the existing payment from earlier database lookup
		paymentReq := &models.PaymentRequest{
			OrderID: result.OrderID,
			Status:  result.Status,
			// Price already set in the database, no need to update it
			URL:    result.ReceiptURL, // Use receipt URL if available
			Source: "stripe",          // Mark this as a Stripe payment to skip OTP
		}

		// If receipt URL is empty, use a default URL
		if paymentReq.URL == "" {
			paymentReq.URL = fmt.Sprintf("https://payment.gateway.com/checkout/%s", result.OrderID)
		}

		// If we already have a payment ID, include it in the request
		if req.PaymentID != "" {
			paymentReq.PaymentID = req.PaymentID
		}

		// Update the existing payment record
		existingPayment.Status = result.Status
		if result.ReceiptURL != "" {
			existingPayment.URL = result.ReceiptURL
		}

		err = h.paymentStore.UpdatePayment(existingPayment)
		if err != nil {
			// Log the error but continue since the Stripe payment was successful
			log.Printf("Failed to update payment record: %v", err)
		}

		paymentRecord := existingPayment

		// Return both Stripe result and our payment record
		response := map[string]interface{}{
			"stripe_result":  result,
			"payment_record": paymentRecord,
		}

		// Also stream the payment event to Kafka if payment was successful
		switch result.Status {
		case models.StatusSuccess:
			event := &models.PaymentEvent{
				Type:      "payment.success",
				PaymentID: paymentRecord.PaymentID,
				Payment:   paymentRecord,
				Timestamp: time.Now(),
			}

			eventData, _ := json.Marshal(event)
			if err := h.producer.Publish("payment_succefully", paymentRecord.PaymentID, eventData); err != nil {
				log.Printf("Warning: Failed to publish success event to Kafka: %v", err)
			} else {
				log.Printf("Payment success event published to Kafka for payment %s", paymentRecord.PaymentID)
			}
		case models.StatusFailed:
			event := &models.PaymentEvent{
				Type:      "payment.failed",
				PaymentID: paymentRecord.PaymentID,
				Payment:   paymentRecord,
				Timestamp: time.Now(),
			}

			eventData, _ := json.Marshal(event)
			if err := h.producer.Publish("payment_unseecuufull", paymentRecord.PaymentID, eventData); err != nil {
				log.Printf("Warning: Failed to publish failure event to Kafka: %v", err)
			} else {
				log.Printf("Payment failure event published to Kafka for payment %s", paymentRecord.PaymentID)
			}
		}

		c.JSON(http.StatusOK, utils.SuccessResponse("Payment processed", response))
		return
	}

	// When we don't have a payment record, still publish to Kafka based on the Stripe result
	event := &models.PaymentEvent{
		Type:      "payment." + string(result.Status),
		PaymentID: result.TransactionID, // Use transaction ID as payment ID in this case
		Payment:   nil,                  // No payment record available
		Timestamp: time.Now(),
	}

	eventData, _ := json.Marshal(event)
	topic := "payment_unseecuufull"
	if result.Status == models.StatusSuccess {
		topic = "payment_succefully"
	}

	if err := h.producer.Publish(topic, result.TransactionID, eventData); err != nil {
		log.Printf("Warning: Failed to publish event to Kafka: %v", err)
	} else {
		log.Printf("Payment event published to Kafka for transaction %s with status %s",
			result.TransactionID, result.Status)
	}

	c.JSON(http.StatusOK, utils.SuccessResponse("Payment processed", result))
}

// StreamPaymentToKafka streams payment events to Kafka
func (h *StripeHandler) StreamPaymentToKafka(c *gin.Context) {
	var req models.PaymentStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request payload", err.Error()))
		return
	}

	// Validate payment_id is provided
	if req.PaymentID == "" {
		c.JSON(http.StatusBadRequest, utils.ErrorResponse("Invalid request payload", "payment_id is required"))
		return
	}

	// Get payment details from our database
	payment, err := h.paymentStore.GetPayment(req.PaymentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.ErrorResponse("Failed to retrieve payment details", err.Error()))
		return
	}

	// Create payment event
	eventType := "payment.event"
	if req.Status == "success" || payment.Status == models.StatusSuccess {
		eventType = "payment.success"
	} else if req.Status == "failed" || payment.Status == models.StatusFailed {
		eventType = "payment.failed"
	} else if req.Status == "refunded" || payment.Status == models.StatusRefunded {
		eventType = "payment.refunded"
	}

	event := &models.PaymentEvent{
		Type:      eventType,
		PaymentID: payment.PaymentID,
		Payment:   payment,
		Timestamp: time.Now(),
	}

	// Publish event to Kafka
	eventData, _ := json.Marshal(event)
	topic := "payment_unseecuufull"
	if payment.Status == models.StatusSuccess {
		topic = "payment_succefully"
	}

	if err := h.producer.Publish(topic, payment.PaymentID, eventData); err != nil {
		log.Printf("Failed to publish payment event to Kafka: %v", err)
		c.JSON(http.StatusInternalServerError, utils.ErrorResponse("Failed to stream payment event", err.Error()))
		return
	}

	c.JSON(http.StatusOK, utils.SuccessResponse("Payment event streamed successfully", map[string]interface{}{
		"event_type": eventType,
		"payment_id": payment.PaymentID,
		"status":     payment.Status,
	}))
}

// ProcessPaymentChi is a Chi-compatible version of ProcessPayment
func (h *StripeHandler) ProcessPaymentChi(w http.ResponseWriter, r *http.Request) {
	var req models.StripePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErrorResponse(w, "Invalid request payload", err.Error(), http.StatusBadRequest)
		return
	}

	// Validate order_id is provided
	if req.OrderID == "" {
		h.writeErrorResponse(w, "Invalid request payload", "order_id is required", http.StatusBadRequest)
		return
	}

	// Set default currency if not provided
	if req.Currency == "" {
		req.Currency = "lkr"
	}

	// Validate token or card is provided
	if req.Token == "" && req.Card == nil {
		h.writeErrorResponse(w, "Invalid request payload", "Either token or card must be provided", http.StatusBadRequest)
		return
	}

	// This prevents the frontend from specifying the amount, which could be a security risk
	existingPayment, err := h.paymentStore.GetPaymentByOrderID(req.OrderID)
	if err != nil {
		h.logger.Info("PAYMENT", fmt.Sprintf("No existing payment found for order %s", req.OrderID))
		// Check if this is for a new order with no payment record yet
		// Get order details to create payment record
		order, orderErr := h.orderService.GetOrder(req.OrderID)
		if orderErr != nil {
			h.writeErrorResponse(w, "Invalid request", "No payment or order record found for this order_id", http.StatusBadRequest)
			return
		}

		h.logger.Info("PAYMENT", fmt.Sprintf("Retrieved order %s with price: %.2f", order.OrderID, order.Price))

		// Validate that order has a valid price (allow 0 for free orders)
		if order.Price < 0 {
			h.writeErrorResponse(w, "Invalid order", fmt.Sprintf("Order %s has invalid price: %.2f", req.OrderID, order.Price), http.StatusBadRequest)
			return
		}

		// Create payment record based on order
		existingPayment = &models.Payment{
			PaymentID:     fmt.Sprintf("pay_%d", time.Now().UnixNano()),
			OrderID:       order.OrderID,
			Price:         order.Price,
			Status:        models.StatusPending,
			CreatedDate:   time.Now(),
			TransactionID: "",
			URL:           "",
		}

		err = h.paymentStore.SavePayment(existingPayment)
		if err != nil {
			h.writeErrorResponse(w, "Payment creation failed", err.Error(), http.StatusInternalServerError)
			return
		}
		h.logger.Info("PAYMENT", fmt.Sprintf("Created new payment record for order %s with price %.2f", req.OrderID, existingPayment.Price))
	}

	// Override any amount provided in the request with the amount from the database
	req.Amount = existingPayment.Price
	h.logger.Info("PAYMENT", fmt.Sprintf("Using price %.2f from database for order %s", req.Amount, req.OrderID))

	// Validate that we have a valid amount (allow 0 for free orders)
	if req.Amount < 0 {
		h.writeErrorResponse(w, "Invalid payment amount", fmt.Sprintf("Payment amount cannot be negative, got: %.2f", req.Amount), http.StatusBadRequest)
		return
	}

	// Also get the payment ID if available and add it to the request
	if existingPayment.PaymentID != "" {
		req.PaymentID = existingPayment.PaymentID
		h.logger.Info("PAYMENT", fmt.Sprintf("Using existing payment ID %s for order %s", req.PaymentID, req.OrderID))
	}

	var result *models.StripePaymentResponse

	// Handle zero-amount payments (free orders)
	if req.Amount == 0 {
		h.logger.Info("PAYMENT", fmt.Sprintf("Processing free order (amount: 0.00) for order %s", req.OrderID))
		// Create a mock successful result for free orders
		result = &models.StripePaymentResponse{
			PaymentID:     existingPayment.PaymentID,
			OrderID:       req.OrderID,
			Status:        models.StatusSuccess,
			Amount:        0.00,
			Currency:      req.Currency,
			TransactionID: fmt.Sprintf("free_%d", time.Now().UnixNano()),
			PaymentMethod: "free",
			ReceiptURL:    "", // Leave empty initially, will be set below if needed
			Created:       time.Now().Unix(),
		}
	} else {
		// Process payment through Stripe for non-zero amounts
		result, err = h.stripeService.ProcessPayment(r.Context(), &req)
		if err != nil {
			h.writeErrorResponse(w, "Payment processing failed", err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Update existing payment record with results from Stripe
	if result.Status == models.StatusSuccess || result.Status == models.StatusPending {
		// Update payment record
		existingPayment.Status = result.Status
		if result.ReceiptURL != "" {
			// Use the original receipt URL from Stripe
			existingPayment.URL = result.ReceiptURL
		} else if req.Amount == 0 {
			// Only use free receipt URL for zero-amount payments when no Stripe URL exists
			existingPayment.URL = fmt.Sprintf("https://payment.gateway.com/free-receipt/%s", result.OrderID)
		} else {
			// Use default checkout URL for non-zero payments when no Stripe URL exists
			existingPayment.URL = fmt.Sprintf("https://payment.gateway.com/checkout/%s", result.OrderID)
		}

		if result.TransactionID != "" {
			existingPayment.TransactionID = result.TransactionID
		}

		err = h.paymentStore.UpdatePayment(existingPayment)
		if err != nil {
			h.logger.Error("PAYMENT", fmt.Sprintf("Failed to update payment record: %v", err))
			h.writeErrorResponse(w, "Payment record update failed", err.Error(), http.StatusInternalServerError)
			return
		}

		// Update order status to completed if payment successful
		if result.Status == models.StatusSuccess {
			err = h.orderService.UpdateOrderStatus(req.OrderID, "completed")
			if err != nil {
				h.logger.Error("ORDER", fmt.Sprintf("Failed to update order status to completed: %v", err))
				h.writeErrorResponse(w, "Order status update failed", err.Error(), http.StatusInternalServerError)
				return
			} else {
				h.logger.Info("ORDER", fmt.Sprintf("Order %s status updated to completed", req.OrderID))
			}
		}

		// Return both Stripe result and our payment record
		response := map[string]interface{}{
			"stripe_result":  result,
			"payment_record": existingPayment,
		}

		// Also stream the payment event to Kafka if payment was successful
		if result.Status == models.StatusSuccess {
			event := &models.PaymentEvent{
				Type:      "payment.success",
				PaymentID: existingPayment.PaymentID,
				Payment:   existingPayment,
				Timestamp: time.Now(),
			}

			eventData, _ := json.Marshal(event)
			if err := h.producer.Publish("payment_succefully", existingPayment.PaymentID, eventData); err != nil {
				h.logger.Error("KAFKA", fmt.Sprintf("Failed to publish success event: %v", err))
			} else {
				h.logger.Info("KAFKA", fmt.Sprintf("Payment success event published for payment %s", existingPayment.PaymentID))
			}
		} else if result.Status == models.StatusFailed {
			event := &models.PaymentEvent{
				Type:      "payment.failed",
				PaymentID: existingPayment.PaymentID,
				Payment:   existingPayment,
				Timestamp: time.Now(),
			}

			eventData, _ := json.Marshal(event)
			if err := h.producer.Publish("payment_unseecuufull", existingPayment.PaymentID, eventData); err != nil {
				h.logger.Error("KAFKA", fmt.Sprintf("Failed to publish failure event: %v", err))
			} else {
				h.logger.Info("KAFKA", fmt.Sprintf("Payment failure event published for payment %s", existingPayment.PaymentID))
			}
		}

		h.writeSuccessResponse(w, "Payment processed", response)
		return
	}

	// When we don't have a payment record, still publish to Kafka based on the Stripe result
	event := &models.PaymentEvent{
		Type:      "payment." + string(result.Status),
		PaymentID: result.TransactionID, // Use transaction ID as payment ID in this case
		Payment:   nil,                  // No payment record available
		Timestamp: time.Now(),
	}

	eventData, _ := json.Marshal(event)
	topic := "payment_unseecuufull"
	if result.Status == models.StatusSuccess {
		topic = "payment_succefully"
	}

	if err := h.producer.Publish(topic, result.TransactionID, eventData); err != nil {
		h.logger.Error("KAFKA", fmt.Sprintf("Failed to publish event: %v", err))
	} else {
		h.logger.Info("KAFKA", fmt.Sprintf("Payment event published for transaction %s with status %s", result.TransactionID, result.Status))
	}

	h.writeSuccessResponse(w, "Payment processed", result)
}

// Helper methods for consistent response formatting
func (h *StripeHandler) writeErrorResponse(w http.ResponseWriter, message, details string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := map[string]interface{}{
		"status":  "error",
		"message": message,
		"details": details,
	}
	json.NewEncoder(w).Encode(response)
}

func (h *StripeHandler) writeSuccessResponse(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"status":  "success",
		"message": message,
		"data":    data,
	}
	json.NewEncoder(w).Encode(response)
}
