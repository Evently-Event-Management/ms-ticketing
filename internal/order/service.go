package order

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/models"
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
	UpdateOrder(order models.Order) error
	CancelOrder(id string) error
	GetOrderBySeat(seatID string) (*models.Order, error)
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
	DB            DBLayer
	Redis         RedisLock
	Kafka         KafkaProducer
	TicketService *tickets.TicketService
	client        *http.Client
	logger        *logger.Logger
}

func NewOrderService(db DBLayer, redis RedisLock, kafka KafkaProducer, ticketService *tickets.TicketService, client *http.Client) *OrderService {
	return &OrderService{
		DB:            db,
		Redis:         redis,
		Kafka:         kafka,
		TicketService: ticketService,
		client:        client,
		logger:        logger.NewLogger(), // Initialize logger
	}
}

// ---------------- ORDERS ----------------

func (s *OrderService) GetOrderBySeat(seatID string) (*models.Order, error) {
	s.logger.Debug("ORDER", fmt.Sprintf("Getting order by seat: %s", seatID))
	return s.DB.GetOrderBySeat(seatID)
}

func (s *OrderService) PlaceOrder(order models.Order) error {
	s.logger.Info("ORDER", fmt.Sprintf("Placing order: %s for session: %s", order.OrderID, order.SessionID))

	s.logger.Debug("ORDER", "Creating order in DB...")
	if err := s.DB.CreateOrder(order); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to create order: %v. Rolling back seat locks.", err))
		_ = s.Redis.UnlockSeats(order.SeatIDs, order.OrderID)
		return err
	}

	// Step 4: Publish Kafka event
	s.logger.Info("KAFKA", "Order created successfully, publishing to Kafka...")
	if err := s.publishOrderCreated(order); err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order created): %v", err))
	}

	s.logger.Info("ORDER", fmt.Sprintf("Order %s placed successfully", order.OrderID))
	return nil
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

	if order.Status != "pending" {
		s.logger.Warn("ORDER", fmt.Sprintf("Cannot update non-pending order: %s (status: %s)", id, order.Status))
		return errors.New("cannot update a non-pending order")
	}

	// Create a merged order with original values preserved for empty fields
	mergedOrder := *order // Start with original order

	// Only update specific fields if they are provided in updateData
	if updateData.Status != "" {
		mergedOrder.Status = updateData.Status
	}
	if updateData.Price > 0 {
		mergedOrder.Price = updateData.Price
	}
	if len(updateData.SeatIDs) > 0 {
		mergedOrder.SeatIDs = updateData.SeatIDs
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

	order.Status = "cancelled"
	if err := s.DB.UpdateOrder(*order); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to cancel order %s: %v", id, err))
		return fmt.Errorf("failed to cancel order %s: %w", id, err)
	}

	// Unlock seats
	if err := s.Redis.UnlockSeats(order.SeatIDs, order.OrderID); err != nil {
		s.logger.Error("REDIS", fmt.Sprintf("Failed to unlock seats for order %s: %v", id, err))
	} else {
		s.logger.Info("REDIS", fmt.Sprintf("Seats unlocked for cancelled order %s", id))
	}

	if err := s.publishOrderCancelled(*order); err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (order cancelled): %v", err))
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

	// Step 3: Call Seat Validation Service
	seatServiceBase := os.Getenv("SEAT_SERVICE_URL") // e.g. http://seating-service:8080
	// Ensure no trailing slash in base URL to prevent double slashes
	if seatServiceBase != "" && seatServiceBase[len(seatServiceBase)-1] == '/' {
		seatServiceBase = seatServiceBase[:len(seatServiceBase)-1]
	}

	s.logger.Debug("SEAT_VALIDATION", fmt.Sprintf("Session ID: %s", orderReq.SessionID))
	validateURL := fmt.Sprintf("%s/internal/v1/sessions/%s/seats/validate", seatServiceBase, orderReq.SessionID)
	var seatIds = map[string]interface{}{
		"seatIds": orderReq.SeatIDs,
	}

	s.logger.Debug("SEAT_VALIDATION", fmt.Sprintf("Validation URL: %s", validateURL))
	reqBody, _ := json.Marshal(seatIds)
	s.logger.Debug("SEAT_VALIDATION", fmt.Sprintf("Request body: %s", string(reqBody)))

	req, err := http.NewRequest("POST", validateURL, bytes.NewBuffer(reqBody))
	if err != nil {
		s.logger.Error("SEAT_VALIDATION", fmt.Sprintf("Failed to create request: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m2m_token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.Error("SEAT_VALIDATION", fmt.Sprintf("Seat validation service error: %v", err))
		return nil, fmt.Errorf("seat validation service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.logger.Error("SEAT_VALIDATION", fmt.Sprintf("Seat validation failed: status %d", resp.StatusCode))
		return nil, fmt.Errorf("seat validation failed: status %d", resp.StatusCode)
	}

	s.logger.Info("SEAT_VALIDATION", "Seat validation successful")

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

	if err := s.publishSeatsLocked(orderReq); err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Kafka publish error (seats locked): %v", err))
	}

	// Step 5: Get seat details for ticket creation
	s.logger.Debug("SEAT_DETAILS", "Requesting seat details for ticket creation")
	validateURLDetails := fmt.Sprintf("%s/internal/v1/sessions/%s/seats/details", seatServiceBase, orderReq.SessionID)
	reqBody, _ = json.Marshal(seatIds)
	reqDetails, err := http.NewRequest("POST", validateURLDetails, bytes.NewBuffer(reqBody))
	if err != nil {
		s.logger.Error("SEAT_DETAILS", fmt.Sprintf("Failed to create seat details request: %v", err))
		return nil, fmt.Errorf("failed to create seat details request: %w", err)
	}
	reqDetails.Header.Set("Authorization", "Bearer "+m2m_token)
	reqDetails.Header.Set("Content-Type", "application/json")

	respDetails, err := s.client.Do(reqDetails)
	if err != nil {
		s.logger.Error("SEAT_DETAILS", fmt.Sprintf("Seat detail collection service error: %v", err))
		return nil, fmt.Errorf("seat detail collection service error: %w", err)
	}
	defer respDetails.Body.Close()

	if respDetails.StatusCode != http.StatusOK {
		s.logger.Error("SEAT_DETAILS", fmt.Sprintf("Seat detail collection failed: status %d", respDetails.StatusCode))
		return nil, fmt.Errorf("seat detail collection failed: status %d", respDetails.StatusCode)
	}

	// Parse the seat details response as a slice
	var seatDetails []models.SeatDetails
	if err := json.NewDecoder(respDetails.Body).Decode(&seatDetails); err != nil {
		s.logger.Error("SEAT_DETAILS", fmt.Sprintf("Failed to decode seat details: %v", err))
		return nil, fmt.Errorf("failed to decode seat details: %w", err)
	}

	s.logger.Info("SEAT_DETAILS", fmt.Sprintf("Retrieved details for %d seats", len(seatDetails)))

	// Step 6: Build order model
	order := models.Order{
		OrderID:   orderID,
		UserID:    userID,
		SessionID: orderReq.SessionID,
		SeatIDs:   orderReq.SeatIDs,
		Status:    "pending",
		Price:     0.0, // Will be updated based on seat prices
		CreatedAt: time.Now(),
	}

	// Step 7: Save order (DB + Kafka event)
	if err := s.PlaceOrder(order); err != nil {
		s.logger.Error("ORDER", fmt.Sprintf("Failed to place order: %v. Unlocking seats.", err))
		_ = s.Redis.UnlockSeats(orderReq.SeatIDs, orderID)
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Step 8: Create tickets for each seat
	s.logger.Info("TICKET", "Creating tickets for each seat")
	var totalPrice float64 = 0
	for _, seat := range seatDetails {
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
		// Add ticket price to total order price
		totalPrice += seat.Tier.Price
		// Save the ticket using TicketService
		if s.TicketService != nil {
			if err := s.TicketService.PlaceTicket(ticket); err != nil {
				s.logger.Warn("TICKET", fmt.Sprintf("Failed to create ticket for seat %s: %v", seat.SeatID, err))
			} else {
				s.logger.Info("TICKET", fmt.Sprintf("Created ticket %s for seat %s", ticket.TicketID, ticket.SeatID))
			}
		} else {
			s.logger.Warn("TICKET", "TicketService not configured, skipping ticket creation")
		}
	}

	// Update order with total price
	if totalPrice > 0 {
		order.Price = totalPrice
		if err := s.DB.UpdateOrder(order); err != nil {
			s.logger.Warn("ORDER", fmt.Sprintf("Failed to update order price: %v", err))
		} else {
			s.logger.Info("ORDER", fmt.Sprintf("Updated order price to: $%.2f", totalPrice))
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

func (s *OrderService) publishOrderCancelled(order models.Order) error {
	payload, err := json.Marshal(order)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order: %v", err))
		return fmt.Errorf("failed to marshal order: %w", err)
	}

	err = s.Kafka.Publish("ticketly.order.canceled", order.OrderID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish order cancelled event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published order cancelled event for order: %s", order.OrderID))
	}
	return err
}

func (s *OrderService) publishSeatsLocked(orderReq models.OrderRequest) error {
	payload, err := json.Marshal(orderReq)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to marshal order request: %v", err))
		return fmt.Errorf("failed to marshal order request: %w", err)
	}

	// Use the first seat ID as key, or generate a unique key if needed
	//key := "seats_locked"
	//if len(orderReq.SeatIDs) > 0 {
	//	key = orderReq.SeatIDs[0]
	//}

	err = s.Kafka.Publish("ticketly.seats.locked", orderReq.SessionID, payload)
	if err != nil {
		s.logger.Error("KAFKA", fmt.Sprintf("Failed to publish seats locked event: %v", err))
	} else {
		s.logger.Info("KAFKA", fmt.Sprintf("Published seats locked event for %d seats", len(orderReq.SeatIDs)))
	}
	return err
}
