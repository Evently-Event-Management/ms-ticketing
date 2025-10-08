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
	tickets "ms-ticketing/internal/tickets/service"
	"net/http"
	"os"
	"time"

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
	GetSeatsByOrder(orderID string) ([]string, error)
	GetSessionIdBySeat(seatID string) (string, error)
}

type RedisLock interface {
	LockSeats(seatIDs []string, orderID string) (bool, error)
	UnlockSeats(seatIDs []string, orderID string) error
}

type KafkaProducer interface {
	Publish(topic string, key string, value []byte) error
	Close() error
}

type OrderService struct {
	DB              DBLayer
	Redis           RedisLock
	Kafka           KafkaProducer
	TicketService   *tickets.TicketService
	DiscountService *discount.DiscountService
	client          *http.Client
	logger          *logger.Logger
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

// ---------------- ORDERS ----------------

func (s *OrderService) GetOrderBySeat(seatID string) (*models.Order, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting order by seat: %s", seatID))
	return s.DB.GetOrderBySeat(seatID)
}

func (s *OrderService) GetOrder(id string) (*models.Order, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting order by ID: %s", id))
	return s.DB.GetOrderByID(id)
}

func (s *OrderService) UpdateOrder(id string, updateData models.Order) error {
	s.logger.Info("ORDER", fmt.Sprintf("Updating order: %s", id))
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Order %s not found: %v", id, err))
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	// Allow updates regardless of status (for testing purposes)
	// In a real-world application, you might want to restrict this
	// to specific status transitions

	// Create a merged order with original values preserved for empty fields
	mergedOrder := *order // Start with original order

	// Only update specific fields if they are provided in updateData
	if updateData.Status != "" {
		mergedOrder.Status = updateData.Status
	}
	if updateData.Price > 0 {
		mergedOrder.Price = updateData.Price
	}

	// Always ensure ID consistency
	mergedOrder.OrderID = id

	s.logger.Debug("ORDER", fmt.Sprintf("Merged order data: %+v", mergedOrder))

	if err := s.DB.UpdateOrder(mergedOrder); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to update order %s: %v", id, err))
		return fmt.Errorf("failed to update order: %w", err)
	}

	if err := s.publishOrderUpdated(mergedOrder); err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order updated): %v", err))
	}

	s.logger.Info("ORDER", fmt.Sprintf("Order %s updated successfully", id))
	return nil
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

		// Create an OrderWithSeats for event publishing as fallback
		orderWithSeats := &models.OrderWithSeats{
			Order:   *order,
			SeatIDs: seatIDs,
		}

		// Publish order cancelled event
		if err := s.publishOrderCancelled(*orderWithSeats); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order cancelled): %v", err))
		}

		// Publish seats released event
		if err := s.publishSeatsReleased(*orderWithSeats); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (seats released): %v", err))
		}
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
		s.logger.Error("ORDER", fmt.Sprintf("Order %s not found: %v", id, err))
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		s.logger.Warn("ORDER", fmt.Sprintf("Order %s is not in a valid state for checkout (status: %s)", id, order.Status))
		return errors.New("order is not in a valid state for checkout")
	}

	order.Status = "completed"

	if err := s.DB.UpdateOrder(*order); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to complete checkout for order %s: %v", id, err))
		return fmt.Errorf("failed to complete checkout: %w", err)
	}

	// Publish order completed event with ticket details
	orderWithTickets, err := s.GetOrderWithTickets(id)
	if err != nil {
		s.logger.Warn("ORDER", fmt.Sprintf("Could not get tickets for completed order %s: %v", id, err))

		// Fall back to basic order update event
		if err := s.publishOrderUpdated(*order); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order completed): %v", err))
		}
	} else {
		// Use the denormalized order with tickets for better event payload
		if err := s.publishOrderCompletedWithTickets(*orderWithTickets); err != nil {
			s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order completed with tickets): %v", err))
		}
	}

	s.logger.Info("ORDER", fmt.Sprintf("Order %s checkout completed successfully", id))
	return nil
}

func (s *OrderService) SeatValidationAndPlaceOrder(r *http.Request, orderReq models.OrderRequest) (*models.OrderResponse, error) {
	s.logger.Info("ORDER", "Starting seat validation and order placement process")

	// Step 1: Extract JWT from the request
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
	m2m_token, err := auth.GetM2MToken(config, s.client)
	if err != nil {
		s.logger.Error("AUTH", fmt.Sprintf("Failed to get M2M token: %v", err))
		return nil, fmt.Errorf("failed to get M2M token: %w", err)
	}

	// Step 2: Generate unique OrderID
	orderID := uuid.NewString()
	s.logger.Debug("ORDER", fmt.Sprintf("Generated order ID: %s", orderID))

	// Step 3: Call Pre-validation Service (first HTTP request)
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

	// Step 4: Lock seats in Redis
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

	// Step 5: Make second HTTP request to validate seats after locking
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

	// Step 6: Calculate prices and apply discount if available
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

	// Step 7: Save order to DB and publish events - skip locking since we already locked the seats
	if err := s.SaveOrderWithEvents(order, orderReq.SeatIDs); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to place order: %v. Unlocking seats.", err))
		rollback()
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Step 8: Create tickets for each seat
	s.logger.Info("TICKET", "Creating tickets for each seat")
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
			}
		} else {
			s.logger.Warn("TICKET", "TicketService not configured, skipping ticket creation")
			rollback()
			return nil, fmt.Errorf("ticket service not configured")
		}
	}

	// Step 9: Build response
	s.logger.Info("ORDER", fmt.Sprintf("Order %s completed successfully for user %s", orderID, userID))
	return &models.OrderResponse{
		OrderID:   orderID,
		SessionID: orderReq.SessionID,
		SeatIDs:   orderReq.SeatIDs,
		UserID:    userID,
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

// SaveOrderWithEvents saves an order to the database and publishes related events
func (s *OrderService) SaveOrderWithEvents(order models.Order, seatIDs []string) error {
	// First save to database
	if err := s.SaveOrder(order, seatIDs); err != nil {
		return err
	}

	// Then publish events
	s.PublishOrderCreatedEvent(order, seatIDs)

	return nil
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

func (s *OrderService) publishOrderCancelled(orderWithSeats models.OrderWithSeats) error {
	payload, err := json.Marshal(orderWithSeats)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order with seats: %v", err))
		return fmt.Errorf("failed to marshal order with seats: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.canceled", orderWithSeats.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order cancelled event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order cancelled event for order: %s", orderWithSeats.OrderID))
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

func (s *OrderService) publishOrderCreatedWithSeats(orderWithSeats models.OrderWithSeats) error {
	payload, err := json.Marshal(orderWithSeats)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order with seats: %v", err))
		return fmt.Errorf("failed to marshal order with seats: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.created", orderWithSeats.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order created event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order created event for order: %s with %d seats", orderWithSeats.OrderID, len(orderWithSeats.SeatIDs)))
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
