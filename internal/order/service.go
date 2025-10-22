package order

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/order/discount"
	rediswrap "ms-ticketing/internal/order/redis"
	tickets "ms-ticketing/internal/tickets/service"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/google/uuid"

	// Import the logger package
	"ms-ticketing/internal/logger"
)

type DBLayer interface {
	CreateOrder(order models.Order) error
	GetOrderByID(id string) (*models.Order, error)
	GetOrderWithSeats(id string) (*models.OrderWithSeats, error)
	UpdateOrder(order models.Order) error
	CancelOrder(id string) error
	GetOrderBySeat(seatID string) (*models.Order, error)
	GetPendingOrdersBySeat(seatID string) ([]*models.Order, error)
	GetSeatsByOrder(orderID string) ([]string, error)
	GetSessionIdBySeat(seatID string) (string, error)
	GetOrdersWithTicketsByUserID(userID string) ([]models.OrderWithTickets, error)
	GetOrdersWithTicketsAndQRByUserID(userID string) ([]models.OrderWithTicketsAndQR, error)
}

type RedisLock interface {
	CheckSeatsAvailability(seatIDs []string) (bool, []string, error)
	LockSeats(seatIDs []string, orderID string) (bool, error)
	UnlockSeats(seatIDs []string, orderID string) error
}

type KafkaProducer interface {
	Publish(topic string, key string, value []byte) error
	Close() error
}

type OrderService struct {
	DB                   DBLayer
	Redis                RedisLock
	Kafka                KafkaProducer
	TicketService        *tickets.TicketService
	DiscountService      *discount.DiscountService
	client               *http.Client
	logger               *logger.Logger
	CheckoutEventEmitter CheckoutEventEmitter
}

// CheckoutEventEmitter is an interface for emitting checkout events
type CheckoutEventEmitter interface {
	EmitCheckoutEvent(order models.OrderWithTickets)
}

func NewOrderService(db DBLayer, redis RedisLock, kafka KafkaProducer, ticketService *tickets.TicketService, client *http.Client) *OrderService {
	return &OrderService{
		DB:              db,
		Redis:           redis,
		Kafka:           kafka,
		TicketService:   ticketService,
		DiscountService: discount.NewDiscountService(),
		client:          client,
		logger:          logger.NewLogger(), // Initialize logger
	}
}

// SetCheckoutEventEmitter sets the checkout event emitter for SSE notifications
func (s *OrderService) SetCheckoutEventEmitter(emitter CheckoutEventEmitter) {
	s.CheckoutEventEmitter = emitter
}

// ---------------- ORDERS ----------------

func (s *OrderService) GetOrderBySeat(seatID string) (*models.Order, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting order by seat: %s", seatID))
	return s.DB.GetOrderBySeat(seatID)
}

func (s *OrderService) GetOrder(id string) (*models.Order, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting order by ID: %s", id))
	return s.DB.GetOrderByID(id)
}

func (s *OrderService) CancelOrder(id string) error {
	s.logger.Info("ORDER", fmt.Sprintf("Cancelling order: %s", id))
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Order %s not found: %v", id, err))
		return fmt.Errorf("order %s not found: %w", id, err)
	}
	if order.Status != "pending" {
		s.logger.Warn("ORDER", fmt.Sprintf("Cannot cancel non-pending order: %s (status: %s)", id, order.Status))
		return errors.New("cannot cancel a non-pending order")
	}

	// Get seat IDs associated with this order
	seatIDs, err := s.DB.GetSeatsByOrder(id)
	if err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to get seat IDs for order %s: %v", id, err))
		return fmt.Errorf("failed to get seat IDs: %w", err)
	}

	// Cancel the associated payment intent if it exists
	if order.PaymentIntentID != "" {
		s.logger.Info("PAYMENT", fmt.Sprintf("Cancelling payment intent %s for order %s", order.PaymentIntentID, id))
		if err := s.CancelPaymentIntent(order.PaymentIntentID); err != nil {
			s.logger.Error("PAYMENT", fmt.Sprintf("Failed to cancel payment intent %s: %v", order.PaymentIntentID, err))
			// Continue with order cancellation even if payment intent cancellation fails
		}
		// Reset the payment intent ID
		order.PaymentIntentID = ""
	}

	order.Status = "cancelled"
	if err := s.DB.UpdateOrder(*order); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to cancel order %s: %v", id, err))
		return fmt.Errorf("failed to cancel order %s: %w", id, err)
	}

	// Unlock seats
	if err := s.Redis.UnlockSeats(seatIDs, order.OrderID); err != nil {
		s.logger.Error("REDIS", fmt.Sprintf("Failed to unlock seats for order %s: %v", id, err))
	} else {
		s.logger.Info("REDIS", fmt.Sprintf("Seats unlocked for cancelled order %s", id))
	}

	// Try to get order with tickets for denormalized event
	orderWithTickets, err := s.GetOrderWithTickets(id)
	if err != nil {
		// If we can't get the tickets, fall back to seats-only approach
		s.logger.Warn("ORDER", fmt.Sprintf("Could not get tickets for order %s: %v, falling back to seats-only approach", id, err))
		return fmt.Errorf("could not get tickets for order %s: %w", id, err)
	} else {
		// Use the denormalized order with tickets for better event payload
		// Publish order cancelled event with full ticket details
		if err := s.publishOrderCancelledWithTickets(*orderWithTickets, seatIDs); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order cancelled with tickets): %v", err))
		}

		// We still need to publish seats released event
		orderWithSeats := &models.OrderWithSeats{
			Order:   *order,
			SeatIDs: seatIDs,
		}
		if err := s.publishSeatsReleased(*orderWithSeats); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (seats released): %v", err))
		}
	}

	s.logger.Info("ORDER", fmt.Sprintf("Order %s cancelled successfully", id))
	return nil
}

func (s *OrderService) Checkout(id string) error {
	s.logger.Info("ORDER", fmt.Sprintf("Checking out order: %s", id))
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return fmt.Errorf("failed to get order: %v", err)
	}

	if order.Status != "pending" {
		return fmt.Errorf("order is not in pending status, current status: %s", order.Status)
	}

	// Ensure payment has been processed (payment intent should exist)
	if order.PaymentIntentID == "" {
		return fmt.Errorf("payment intent not found for order")
	}

	// First get the tickets which contain seat IDs
	orderWithTickets, err := s.GetOrderWithTickets(id)
	if err != nil {
		return fmt.Errorf("failed to get tickets for order: %v", err)
	} else {
		// Extract seat IDs from tickets
		var seatIDs []string
		for _, ticket := range orderWithTickets.Tickets {
			seatIDs = append(seatIDs, ticket.SeatID)
		}

		// Update order status
		order.Status = "completed"
		err = s.DB.UpdateOrder(*order)
		if err != nil {
			return fmt.Errorf("failed to update order status: %v", err)
		}

		// Update the order in orderWithTickets to reflect the status change
		orderWithTickets.Order.Status = "completed"

		// Create an OrderWithSeats for publishing seats booked event
		orderWithSeats := models.OrderWithSeats{
			Order:   *order,
			SeatIDs: seatIDs,
		}

		// Publish seats booked event
		err = s.publishSeatsBooked(orderWithSeats)
		if err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish seats booked event: %v", err))
			// Continue execution even if event publishing fails
		}

		// Use the denormalized order with tickets for better event payload
		err = s.publishOrderCompletedWithTickets(*orderWithTickets)
		if err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order completed event: %v", err))
			// Continue execution even if event publishing fails
		}

		// Emit SSE event for successful checkout if SSE handler is registered
		if s.CheckoutEventEmitter != nil {
			s.logger.Debug("SSE", fmt.Sprintf("Emitting checkout event for order: %s", id))
			s.CheckoutEventEmitter.EmitCheckoutEvent(*orderWithTickets)
		}
	}

	s.logger.Info("ORDER", fmt.Sprintf("Order %s checkout completed successfully", id))
	return nil
}

func (s *OrderService) SeatValidationAndPlaceOrder(r *http.Request, orderReq models.OrderRequest) (*models.OrderResponse, error) {
	s.logger.Info("ORDER", "Starting seat validation and order placement process")

	// Step 1: Check Redis seat availability FIRST before any other operations
	s.logger.Debug("REDIS", "Checking seat availability in Redis before proceeding")
	available, unavailableSeats, err := s.Redis.CheckSeatsAvailability(orderReq.SeatIDs)
	if err != nil {
		s.logger.Error("REDIS", fmt.Sprintf("Failed to check seat availability: %v", err))
		return nil, fmt.Errorf("failed to check seat availability: %w", err)
	}
	if !available {
		s.logger.Warn("REDIS", fmt.Sprintf("One or more seats are already locked: %v", unavailableSeats))
		return nil, fmt.Errorf("one or more seats are already locked: %v", unavailableSeats)
	}
	s.logger.Info("REDIS", "All seats are available in Redis, proceeding with validation")

	// Step 2: Extract JWT from the request
	user_token, err := auth.ExtractTokenFromRequest(r)
	if err != nil {
		s.logger.Error("AUTH", fmt.Sprintf("Failed to extract token from request: %v", err))
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	userID, err := auth.ExtractUserIDFromJWT(user_token)
	if err != nil {
		s.logger.Error("AUTH", fmt.Sprintf("Failed to extract user ID from JWT: %v", err))
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	s.logger.Debug("ORDER", fmt.Sprintf("Order request: %+v", orderReq))

	var config models.Config
	config.ClientID = os.Getenv("TICKET_CLIENT_ID")
	config.ClientSecret = os.Getenv("TICKET_CLIENT_SECRET")
	config.KeycloakURL = os.Getenv("KEYCLOAK_URL")
	config.KeycloakRealm = os.Getenv("KEYCLOAK_REALM")

	s.logger.Debug("AUTH", "Requesting M2M token for seat validation")

	// Get Redis client from the existing Redis object if available
	var redisClient *redis.Client
	if s.Redis != nil {
		// Try to get the Redis client from our Redis wrapper
		if redisWrapper, ok := s.Redis.(*rediswrap.Redis); ok && redisWrapper != nil {
			redisClient = redisWrapper.Client
		}
	}

	m2m_token, err := auth.GetM2MToken(config, s.client, redisClient, s.logger)
	if err != nil {
		s.logger.Error("AUTH", fmt.Sprintf("Failed to get M2M token: %v", err))
		return nil, fmt.Errorf("failed to get M2M token: %w", err)
	}

	// Step 3: Generate unique OrderID
	orderID := uuid.NewString()
	s.logger.Debug("ORDER", fmt.Sprintf("Generated order ID: %s", orderID))

	// Step 4: Call Pre-validation Service (first HTTP request)
	s.logger.Debug("PRE_VALIDATION", "Making first HTTP request to validate pre-order")
	eventQueryServiceURL := os.Getenv("EVENT_QUERY_SERVICE_URL") // e.g., http://localhost:8082/api/event-query
	if eventQueryServiceURL != "" && eventQueryServiceURL[len(eventQueryServiceURL)-1] == '/' {
		eventQueryServiceURL = eventQueryServiceURL[:len(eventQueryServiceURL)-1]
	}

	preValidateURL := fmt.Sprintf("%s/internal/v1/validate-pre-order", eventQueryServiceURL)

	// Prepare request body
	reqBody, err := json.Marshal(orderReq)
	if err != nil {
		s.logger.Error("PRE_VALIDATION", fmt.Sprintf("Failed to marshal request: %v", err))
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	s.logger.Debug("PRE_VALIDATION", fmt.Sprintf("Pre-validation URL: %s", preValidateURL))
	s.logger.Debug("PRE_VALIDATION", fmt.Sprintf("Request body: %s", string(reqBody)))

	req, err := http.NewRequest("POST", preValidateURL, bytes.NewBuffer(reqBody))
	if err != nil {
		s.logger.Error("PRE_VALIDATION", fmt.Sprintf("Failed to create pre-validation request: %v", err))
		return nil, fmt.Errorf("failed to create pre-validation request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m2m_token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Error("PRE_VALIDATION", fmt.Sprintf("Pre-validation service error: %v", err))
		return nil, fmt.Errorf("pre-validation service error: %w", err)
	}

	// Read and store the response body
	var orderDetailsDTO models.OrderDetailsDTO
	err = json.NewDecoder(resp.Body).Decode(&orderDetailsDTO)
	if err != nil {
		s.logger.Error("PRE_VALIDATION", fmt.Sprintf("Failed to decode pre-validation response: %v", err))
		return nil, fmt.Errorf("failed to decode pre-validation response: %w", err)
	}

	err = resp.Body.Close()
	if err != nil {
		s.logger.Error("PRE_VALIDATION", fmt.Sprintf("Failed to close pre-validation response body: %v", err))
		return nil, fmt.Errorf("failed to close pre-validation response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		s.logger.Error("PRE_VALIDATION", fmt.Sprintf("Pre-validation failed: status %d", resp.StatusCode))
		return nil, fmt.Errorf("pre-validation failed: status %d", resp.StatusCode)
	}

	s.logger.Info("PRE_VALIDATION", "Pre-validation successful, OrderDetailsDTO received")

	// Step 5: Lock seats in Redis
	s.logger.Debug("REDIS", "Attempting to lock seats in Redis")
	ok, err := s.Redis.LockSeats(orderReq.SeatIDs, orderID)
	if err != nil {
		s.logger.Error("REDIS", fmt.Sprintf("Failed to lock seats: %v", err))
		return nil, fmt.Errorf("failed to lock seats: %w", err)
	}
	if !ok {
		s.logger.Warn("REDIS", "One or more seats already locked")
		return nil, fmt.Errorf("one or more seats already locked")
	}
	s.logger.Info("REDIS", "Seats locked successfully")

	// Transaction rollback helper
	rollback := func() {
		s.logger.Warn("TXN", "Rolling back: unlocking seats")
		_ = s.Redis.UnlockSeats(orderReq.SeatIDs, orderID)
	}

	// Step 6: Make second HTTP request to validate seats after locking
	s.logger.Debug("SEAT_VALIDATION", "Making second HTTP request to validate seats after locking")
	seatServiceBase := os.Getenv("EVENT_SEATING_SERVICE_URL") // e.g., http://localhost:8081/api/event-seating
	if seatServiceBase != "" && seatServiceBase[len(seatServiceBase)-1] == '/' {
		seatServiceBase = seatServiceBase[:len(seatServiceBase)-1]
	}

	finalValidateURL := fmt.Sprintf("%s/internal/v1/validate-pre-order", seatServiceBase)

	reqFinal, err := http.NewRequest("POST", finalValidateURL, bytes.NewBuffer(reqBody))
	if err != nil {
		s.logger.Error("SEAT_VALIDATION", fmt.Sprintf("Failed to create seat validation request: %v", err))
		rollback()
		return nil, fmt.Errorf("failed to create seat validation request: %w", err)
	}
	reqFinal.Header.Set("Authorization", "Bearer "+m2m_token)
	reqFinal.Header.Set("Content-Type", "application/json")

	respFinal, err := s.client.Do(reqFinal)
	if err != nil {
		s.logger.Error("SEAT_VALIDATION", fmt.Sprintf("Seat validation service error: %v", err))
		rollback()
		return nil, fmt.Errorf("seat validation service error: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.logger.Error("SEAT_VALIDATION", fmt.Sprintf("Failed to close seat validation response body: %v", err))
		}
	}(respFinal.Body)

	if respFinal.StatusCode != http.StatusOK {
		s.logger.Error("SEAT_VALIDATION", fmt.Sprintf("Final seat validation failed: status %d", respFinal.StatusCode))
		rollback()
		return nil, fmt.Errorf("final seat validation failed: status %d", respFinal.StatusCode)
	}

	s.logger.Info("SEAT_VALIDATION", "Final seat validation successful")

	// Step 7: Calculate prices and apply discount if available
	var subtotal float64 = 0
	for _, seat := range orderDetailsDTO.Seats {
		subtotal += seat.Tier.Price
	}

	// Default values assuming no discount
	discountAmount := 0.0
	discountID := ""
	discountCode := ""
	finalPrice := subtotal

	// Process discount if provided in the OrderDetailsDTO
	if orderDetailsDTO.Discount != nil && orderDetailsDTO.Discount.ID != "" {
		s.logger.Debug("DISCOUNT", fmt.Sprintf("Processing discount from OrderDetailsDTO: %s", orderDetailsDTO.Discount.Code))

		// Validate and calculate discount
		discountResult, err := s.DiscountService.ValidateAndCalculateDiscount(
			orderDetailsDTO.Discount,
			orderDetailsDTO.Seats,
			orderReq.SessionID,
		)

		if err != nil {
			s.logger.Error("DISCOUNT", fmt.Sprintf("Error calculating discount: %v", err))
			rollback()
			return nil, fmt.Errorf("error calculating discount: %w", err)
		}

		if !discountResult.IsValid {
			s.logger.Warn("DISCOUNT", fmt.Sprintf("Discount not applicable: %s", discountResult.Reason))
			rollback()
			return nil, fmt.Errorf("discount not applicable: %s", discountResult.Reason)
		}

		// Apply discount
		discountAmount = discountResult.DiscountAmount
		discountID = orderDetailsDTO.Discount.ID
		discountCode = orderDetailsDTO.Discount.Code
		finalPrice = subtotal - discountAmount

		if finalPrice < 0 {
			finalPrice = 0
		}

		s.logger.Info("DISCOUNT", fmt.Sprintf("Applied discount: %.2f, final price: %.2f", discountAmount, finalPrice))
	} else {
		s.logger.Debug("DISCOUNT", "No discount applied to order")
	}

	// Build the order object, ensuring empty discount values are treated as NULL in the database
	order := models.Order{
		OrderID:        orderID,
		UserID:         userID,
		EventID:        orderReq.EventID,
		OrganizationID: orderReq.OrganizationID,
		SessionID:      orderReq.SessionID,
		Status:         "pending",
		SubTotal:       subtotal,
		DiscountAmount: discountAmount,
		Price:          finalPrice,
		CreatedAt:      time.Now(),
	}

	// Only set discount fields if they have values
	if discountID != "" {
		order.DiscountID = discountID
	}
	if discountCode != "" {
		order.DiscountCode = discountCode
	}

	// Publish seats locked event
	if err := s.publishSeatsLocked(orderReq); err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (seats locked): %v", err))
	}

	// Step 8: Save order to DB - skip locking since we already locked the seats
	if err := s.SaveOrder(order, orderReq.SeatIDs); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to place order: %v. Unlocking seats.", err))
		rollback()
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Step 9: Create tickets for each seat
	s.logger.Info("TICKET", "Creating tickets for each seat")
	var createdTickets []models.TicketForStreaming
	for _, seat := range orderDetailsDTO.Seats {
		ticket := models.Ticket{
			TicketID:        uuid.NewString(),
			OrderID:         orderID,
			SeatID:          seat.SeatID,
			SeatLabel:       seat.Label,
			TierID:          seat.Tier.ID,
			TierName:        seat.Tier.Name,
			Colour:          seat.Tier.Color,
			PriceAtPurchase: seat.Tier.Price,
			IssuedAt:        time.Now(),
			CheckedIn:       false,
		}

		// Save the ticket using TicketService
		if s.TicketService != nil {
			if err := s.TicketService.PlaceTicket(ticket); err != nil {
				s.logger.Warn("TICKET", fmt.Sprintf("Failed to create ticket for seat %s: %v", seat.SeatID, err))
				rollback()
				return nil, fmt.Errorf("failed to create ticket for seat %s: %w", seat.SeatID, err)
			} else {
				s.logger.Info("TICKET", fmt.Sprintf("Created ticket %s for seat %s", ticket.TicketID, ticket.SeatID))
				// Add ticket to our local collection for event publishing
				createdTickets = append(createdTickets, ticket.ToStreamingTicket())
			}
		} else {
			s.logger.Warn("TICKET", "TicketService not configured, skipping ticket creation")
			rollback()
			return nil, fmt.Errorf("ticket service not configured")
		}
	}

	// Step 10: Now that we have the order and all tickets created, publish the event with full ticket details
	if len(createdTickets) > 0 {
		orderWithTickets := models.OrderWithTickets{
			Order:   order,
			Tickets: createdTickets,
		}

		s.logger.Info("KAFKA", fmt.Sprintf("Publishing order created event with %d tickets", len(createdTickets)))
		if err := s.publishOrderCreatedWithTickets(orderWithTickets); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order created event: %v", err))
			// Continue anyway - don't fail the transaction if just the event publishing fails
		}
	} else {
		// Fallback to basic order event if somehow no tickets were created
		s.logger.Info("KAFKA", "Publishing basic order created event")
		if err := s.publishOrderCreated(order); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order created event: %v", err))
		}
	}

	// Step 11: Build response
	s.logger.Info("ORDER", fmt.Sprintf("Order %s completed successfully for user %s", orderID, userID))
	return &models.OrderResponse{
		OrderID:        orderID,
		SessionID:      orderReq.SessionID,
		OrganizationID: orderReq.OrganizationID,
		SeatIDs:        orderReq.SeatIDs,
		UserID:         userID,
	}, nil
}

func (s *OrderService) SaveOrder(order models.Order, seatIDs []string) error {
	s.logger.Info("ORDER", fmt.Sprintf("Placing order: %s for session: %s", order.OrderID, order.SessionID))

	s.logger.Debug("ORDER", "Creating order in DB...")
	if err := s.DB.CreateOrder(order); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to create order: %v. Rolling back seat locks.", err))
		if len(seatIDs) > 0 {
			_ = s.Redis.UnlockSeats(seatIDs, order.OrderID)
		}
		return err
	}

	s.logger.Info("ORDER", fmt.Sprintf("Order %s placed successfully", order.OrderID))
	return nil
}

// PublishOrderCreatedEvent publishes relevant events after order creation
// DEPRECATED: This method assumes tickets already exist, which is not always true at order creation time
// Use direct calls to publishOrderCreated or publishOrderCreatedWithTickets instead
func (s *OrderService) PublishOrderCreatedEvent(order models.Order, seatIDs []string) {
	s.logger.Info("KAFKA", "Order created successfully, publishing to Kafka...")

	// For orders with tickets, use the denormalized OrderWithTickets structure
	if s.TicketService != nil && len(seatIDs) > 0 {
		// Try to get the order with tickets
		orderWithTickets, err := s.GetOrderWithTickets(order.OrderID)
		if err != nil {
			s.logger.Warn("KAFKA", fmt.Sprintf("Could not get tickets for order %s: %v, falling back to basic event", order.OrderID, err))
			// Fall back to basic order event
			if err := s.publishOrderCreated(order); err != nil {
				s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order created): %v", err))
			}
			return
		}

		// Publish the denormalized order with tickets
		if err := s.publishOrderCreatedWithTickets(*orderWithTickets); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order created with tickets): %v", err))
		}
	} else {
		// Fall back to basic order event if no tickets or ticket service
		if err := s.publishOrderCreated(order); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order created): %v", err))
		}
	}
}

// GetOrderWithTickets retrieves an order with all its associated tickets (without QR codes)
func (s *OrderService) GetOrderWithTickets(orderID string) (*models.OrderWithTickets, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting order with ticketsByOrder for ID: %s", orderID))

	// Get the order
	order, err := s.DB.GetOrderByID(orderID)
	if err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to get order %s: %v", orderID, err))
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Get the ticketsByOrder if TicketService is available
	if s.TicketService == nil {
		s.logger.Error("ORDER", "TicketService is not configured")
		return nil, errors.New("ticket service not configured")
	}

	ticketsByOrder, err := s.TicketService.DB.GetTicketsByOrder(orderID)
	if err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to get ticketsByOrder for order %s: %v", orderID, err))
		return nil, fmt.Errorf("failed to get ticketsByOrder: %w", err)
	}

	// Convert ticketsByOrder to streaming format (without QR codes)
	streamingTickets := make([]models.TicketForStreaming, len(ticketsByOrder))
	for i, ticket := range ticketsByOrder {
		streamingTickets[i] = ticket.ToStreamingTicket()
	}

	// Create and return the OrderWithTickets
	return &models.OrderWithTickets{
		Order:   *order,
		Tickets: streamingTickets,
	}, nil
}

// GetOrdersWithTicketsByUserID retrieves all orders with their associated tickets for a given user
func (s *OrderService) GetOrdersWithTicketsByUserID(userID string) ([]models.OrderWithTickets, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting orders with tickets for user: %s", userID))

	ordersWithTickets, err := s.DB.GetOrdersWithTicketsByUserID(userID)
	if err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to get orders with tickets for user %s: %v", userID, err))
		return nil, fmt.Errorf("failed to get orders with tickets for user: %w", err)
	}

	s.logger.Info("ORDER", fmt.Sprintf("Retrieved %d orders with tickets for user %s", len(ordersWithTickets), userID))
	return ordersWithTickets, nil
}

// GetOrdersWithTicketsAndQRByUserID retrieves all orders with their associated tickets including QR codes for a given user
func (s *OrderService) GetOrdersWithTicketsAndQRByUserID(userID string) ([]models.OrderWithTicketsAndQR, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting orders with tickets and QR codes for user: %s", userID))

	ordersWithTicketsAndQR, err := s.DB.GetOrdersWithTicketsAndQRByUserID(userID)
	if err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to get orders with tickets and QR codes for user %s: %v", userID, err))
		return nil, fmt.Errorf("failed to get orders with tickets and QR codes for user: %w", err)
	}

	s.logger.Info("ORDER", fmt.Sprintf("Retrieved %d orders with tickets and QR codes for user %s", len(ordersWithTicketsAndQR), userID))
	return ordersWithTicketsAndQR, nil
}

// Helper methods for Kafka publishing
func (s *OrderService) publishOrderCreated(order models.Order) error {
	payload, err := json.Marshal(order)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order: %v", err))
		return fmt.Errorf("failed to marshal order: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.created", order.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order created event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order created event for order: %s", order.OrderID))
	}
	return err
}

func (s *OrderService) publishOrderUpdated(order models.Order) error {
	payload, err := json.Marshal(order)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order: %v", err))
		return fmt.Errorf("failed to marshal order: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.updated", order.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order updated event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order updated event for order: %s", order.OrderID))
	}
	return err
}

// publishOrderCompletedWithTickets publishes an order completed event with full ticket details
func (s *OrderService) publishOrderCompletedWithTickets(orderWithTickets models.OrderWithTickets) error {
	payload, err := json.Marshal(orderWithTickets)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order completed event: %v", err))
		return fmt.Errorf("failed to marshal order completed event: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.updated", orderWithTickets.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order completed with tickets event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order completed event for order: %s with %d tickets",
			orderWithTickets.OrderID, len(orderWithTickets.Tickets)))
	}
	return err
}

// publishOrderCancelledWithTickets publishes an order cancelled event with full ticket details
func (s *OrderService) publishOrderCancelledWithTickets(orderWithTickets models.OrderWithTickets, seatIDs []string) error {
	// Add SeatIDs field to the event payload for backward compatibility
	type OrderCancelledEvent struct {
		models.OrderWithTickets
		SeatIDs []string `json:"seat_ids"`
	}

	event := OrderCancelledEvent{
		OrderWithTickets: orderWithTickets,
		SeatIDs:          seatIDs,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order cancelled event: %v", err))
		return fmt.Errorf("failed to marshal order cancelled event: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.canceled", orderWithTickets.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order cancelled with tickets event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order cancelled event for order: %s with %d tickets",
			orderWithTickets.OrderID, len(orderWithTickets.Tickets)))
	}
	return err
}

// publishOrderCreatedWithTickets publishes a denormalized order with all ticket details
func (s *OrderService) publishOrderCreatedWithTickets(orderWithTickets models.OrderWithTickets) error {
	payload, err := json.Marshal(orderWithTickets)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order with tickets: %v", err))
		return fmt.Errorf("failed to marshal order with tickets: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.created", orderWithTickets.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order created event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order created event for order: %s with %d tickets", orderWithTickets.OrderID, len(orderWithTickets.Tickets)))
	}
	return err
}

func (s *OrderService) publishSeatsLocked(orderReq models.OrderRequest) error {
	seatEvent, err := models.NewSeatStatusChangeEventDto(orderReq.SessionID, orderReq.SeatIDs, models.SeatStatusLocked)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to create seat status event DTO: %v", err))
		return fmt.Errorf("failed to create seat status event DTO: %w", err)
	}

	payload, err := json.Marshal(seatEvent)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal seat status event: %v", err))
		return fmt.Errorf("failed to marshal seat status event: %w", err)
	}

	err = s.Kafka.Publish("ticketly.seats.status", orderReq.SessionID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish seat status event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published seat status (LOCKED) event for %d seats", len(orderReq.SeatIDs)))
	}
	return err
}

func (s *OrderService) publishSeatsReleased(orderWithSeats models.OrderWithSeats) error {
	seatEvent, err := models.NewSeatStatusChangeEventDto(orderWithSeats.SessionID, orderWithSeats.SeatIDs, models.SeatStatusAvailable)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to create seat status event DTO: %v", err))
		return fmt.Errorf("failed to create seat status event DTO: %w", err)
	}

	payload, err := json.Marshal(seatEvent)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal seat status event: %v", err))
		return fmt.Errorf("failed to marshal seat status event: %w", err)
	}

	err = s.Kafka.Publish("ticketly.seats.status", orderWithSeats.SessionID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish seat status event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published seat status (AVAILABLE) event for %d seats", len(orderWithSeats.SeatIDs)))
	}
	return err
}

func (s *OrderService) publishSeatsBooked(orderWithSeats models.OrderWithSeats) error {
	seatEvent, err := models.NewSeatStatusChangeEventDto(orderWithSeats.SessionID, orderWithSeats.SeatIDs, models.SeatStatusBooked)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to create seat status event DTO: %v", err))
		return fmt.Errorf("failed to create seat status event DTO: %w", err)
	}

	payload, err := json.Marshal(seatEvent)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal seat status event: %v", err))
		return fmt.Errorf("failed to marshal seat status event: %w", err)
	}

	err = s.Kafka.Publish("ticketly.seats.status", orderWithSeats.SessionID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish seat status event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published seat status (BOOKED) event for %d seats", len(orderWithSeats.SeatIDs)))
	}
	return err
}
