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
)

type DBLayer interface {
	CreateOrder(order models.Order) error
	GetOrderByID(id string) (*models.Order, error)
	UpdateOrder(order models.Order) error
	CancelOrder(id string) error
	GetOrderBySeat(seatID string) (*models.Order, error)
}

type RedisLock interface {
	LockSeats(seatIDs []string, orderID string) (bool, error)
	UnlockSeats(seatIDs []string, orderID string) error
}

type KafkaPublisher interface {
	PublishOrderCreated(order models.Order) error
	PublishOrderUpdated(order models.Order) error
	PublishOrderCancelled(order models.Order) error
}

type OrderService struct {
	DB            DBLayer
	Redis         RedisLock
	Kafka         KafkaPublisher
	TicketService *tickets.TicketService
	client        *http.Client
}

func NewOrderService(db DBLayer, redis RedisLock, kafka KafkaPublisher, ticketService *tickets.TicketService, client *http.Client) *OrderService {
	return &OrderService{DB: db, Redis: redis, Kafka: kafka, TicketService: ticketService, client: client}
}

// ---------------- ORDERS ----------------

func (s *OrderService) GetOrderBySeat(seatID string) (*models.Order, error) {
	return s.DB.GetOrderBySeat(seatID)
}

func (s *OrderService) PlaceOrder(order models.Order) error {
	fmt.Printf("Placing order: %s for session: %s\n", order.OrderID, order.SessionID)

	fmt.Println("Creating order in DB...")
	if err := s.DB.CreateOrder(order); err != nil {
		fmt.Printf("Failed to create order: %v. Rolling back seat locks.\n", err)
		_ = s.Redis.UnlockSeats(order.SeatIDs, order.OrderID)
		return err
	}

	// Step 4: Publish Kafka event
	fmt.Println("Order created successfully, publishing to Kafka...")
	if err := s.Kafka.PublishOrderCreated(order); err != nil {
		fmt.Printf("Kafka publish error (order created): %v\n", err)
	}

	return nil
}

func (s *OrderService) GetOrder(id string) (*models.Order, error) {
	return s.DB.GetOrderByID(id)
}

func (s *OrderService) UpdateOrder(id string, updateData models.Order) error {
	fmt.Printf("Updating order: %s\n", id)
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		return errors.New("cannot update a non-pending order")
	}

	// Ensure ID consistency
	updateData.OrderID = id

	if err := s.DB.UpdateOrder(updateData); err != nil {
		return fmt.Errorf("failed to update order: %w", err)
	}

	if err := s.Kafka.PublishOrderUpdated(updateData); err != nil {
		fmt.Printf("Kafka publish error (order updated): %v\n", err)
	}

	return nil
}

func (s *OrderService) CancelOrder(id string) error {
	fmt.Printf("Cancelling order: %s\n", id)
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return fmt.Errorf("order %s not found: %w", id, err)
	}
	if order.Status != "pending" {
		return errors.New("cannot cancel a non-pending order")
	}

	order.Status = "cancelled"
	if err := s.DB.UpdateOrder(*order); err != nil {
		return fmt.Errorf("failed to cancel order %s: %w", id, err)
	}

	// Unlock seats
	if err := s.Redis.UnlockSeats(order.SeatIDs, order.OrderID); err != nil {
		fmt.Printf("Failed to unlock seats for order %s: %v\n", id, err)
	}

	if err := s.Kafka.PublishOrderCancelled(*order); err != nil {
		fmt.Printf("Kafka publish error (order cancelled): %v\n", err)
	}

	return nil
}

// func (s *OrderService) ApplyPromoCode(id string, code string) error {
// 	fmt.Printf("Applying promo code '%s' to order: %s\n", code, id)
// 	order, err := s.DB.GetOrderByID(id)
// 	if err != nil {
// 		return fmt.Errorf("order %s not found: %w", id, err)
// 	}

// 	if order.Status != "pending" {
// 		return errors.New("cannot apply promo to a non-pending order")
// 	}

// 	order.PromoCode = code
// 	order.DiscountApplied = true

// 	if err := s.DB.UpdateOrder(*order); err != nil {
// 		return fmt.Errorf("failed to apply promo: %w", err)
// 	}

// 	return nil
// }

func (s *OrderService) Checkout(id string) error {
	fmt.Printf("Checking out order: %s\n", id)
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		return errors.New("order is not in a valid state for checkout")
	}

	order.Status = "completed"

	if err := s.DB.UpdateOrder(*order); err != nil {
		return fmt.Errorf("failed to complete checkout: %w", err)
	}

	return nil
}

func (s *OrderService) SeatValidationAndPlaceOrder(r *http.Request, orderReq models.OrderRequest) (*models.OrderResponse, error) {
	// Step 1: Extract JWT from the request
	user_token, err := auth.ExtractTokenFromRequest(r)
	if err != nil {
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	userID, err := auth.ExtractUserIDFromJWT(user_token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	fmt.Println(orderReq)
	var config models.Config
	config.ClientID = os.Getenv("TICKET_CLIENT_ID")
	config.ClientSecret = os.Getenv("TICKET_CLIENT_SECRET")
	config.KeycloakURL = os.Getenv("KEYCLOAK_URL")
	config.KeycloakRealm = os.Getenv("KEYCLOAK_REALM")
	m2m_token, err := auth.GetM2MToken(config, s.client)
	if err != nil {
		return nil, fmt.Errorf("failed to get M2M token: %w", err)
	}
	// Step 2: Generate unique OrderID
	orderID := uuid.NewString()

	// Step 3: Call Seat Validation Service
	seatServiceBase := os.Getenv("SEAT_SERVICE_URL") // e.g. http://seating-service:8080
	// Ensure no trailing slash in base URL to prevent double slashes
	if seatServiceBase != "" && seatServiceBase[len(seatServiceBase)-1] == '/' {
		seatServiceBase = seatServiceBase[:len(seatServiceBase)-1]
	}
	fmt.Println("Session ID:", orderReq.SessionID)
	validateURL := fmt.Sprintf("%s/internal/v1/sessions/%s/seats/validate", seatServiceBase, orderReq.SessionID)
	var seatIds = map[string]interface{}{
		"seatIds": orderReq.SeatIDs,
	}
	fmt.Println("Validating url:", validateURL)
	reqBody, _ := json.Marshal(seatIds)
	fmt.Println("Validating seats with request body:", string(reqBody))
	req, err := http.NewRequest("POST", validateURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m2m_token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("seat validation service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seat validation failed: status %d", resp.StatusCode)
	}

	// Step 4: Lock seats in Redis
	ok, err := s.Redis.LockSeats(orderReq.SeatIDs, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to lock seats: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("one or more seats already locked")
	}

	// Step 5: Get seat details for ticket creation
	validateURLDetails := fmt.Sprintf("%s/internal/v1/sessions/%s/seats/details", seatServiceBase, orderReq.SessionID)
	reqBody, _ = json.Marshal(seatIds)
	reqDetails, err := http.NewRequest("POST", validateURLDetails, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create seat details request: %w", err)
	}
	reqDetails.Header.Set("Authorization", "Bearer "+m2m_token)
	reqDetails.Header.Set("Content-Type", "application/json")

	respDetails, err := s.client.Do(reqDetails)
	if err != nil {
		return nil, fmt.Errorf("seat detail collection service error: %w", err)
	}
	defer respDetails.Body.Close()

	if respDetails.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seat detail collection failed: status %d", respDetails.StatusCode)
	}

	// Parse the seat details response as a slice
	var seatDetails []models.SeatDetails
	if err := json.NewDecoder(respDetails.Body).Decode(&seatDetails); err != nil {
		return nil, fmt.Errorf("failed to decode seat details: %w", err)
	}

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
		_ = s.Redis.UnlockSeats(orderReq.SeatIDs, orderID)
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Step 8: Create tickets for each seat
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
				fmt.Printf("Warning: Failed to create ticket for seat %s: %v\n", seat.SeatID, err)
			} else {
				fmt.Printf("Created ticket %s for seat %s\n", ticket.TicketID, ticket.SeatID)
			}
		} else {
			fmt.Println("Warning: TicketService not configured, skipping ticket creation")
		}
	}

	// Update order with total price
	if totalPrice > 0 {
		order.Price = totalPrice
		if err := s.DB.UpdateOrder(order); err != nil {
			fmt.Printf("Warning: Failed to update order price: %v\n", err)
		}
	}

	// Step 9: Build response
	return &models.OrderResponse{
		OrderID:   orderID,
		SessionID: orderReq.SessionID,
		SeatIDs:   orderReq.SeatIDs,
		UserID:    userID,
	}, nil
}
