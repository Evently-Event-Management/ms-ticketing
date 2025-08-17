package order

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/models"
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
	DB     DBLayer
	Redis  RedisLock
	Kafka  KafkaPublisher
	client *http.Client
}

func NewOrderService(db DBLayer, redis RedisLock, kafka KafkaPublisher, client *http.Client) *OrderService {
	return &OrderService{DB: db, Redis: redis, Kafka: kafka, client: client}
}

// ---------------- ORDERS ----------------

func (s *OrderService) GetOrderBySeat(seatID string) (*models.Order, error) {
	return s.DB.GetOrderBySeat(seatID)
}

func (s *OrderService) PlaceOrder(order models.Order) error {
	fmt.Printf("Placing order: %s for session: %s\n", order.OrderID, order.SessionID)

	// Step 1: Check if seats are already booked
	for _, seatID := range order.SeatIDs {
		existingOrder, err := s.DB.GetOrderBySeat(seatID)
		if err != nil {
			return fmt.Errorf("failed to check seat %s: %w", seatID, err)
		}
		if existingOrder != nil && existingOrder.Status == "completed" {
			return fmt.Errorf("seat %s is already booked in this session", seatID)
		}
	}

	// Step 2: Lock seats in Redis
	ok, err := s.Redis.LockSeats(order.SeatIDs, order.OrderID)
	if err != nil {
		return fmt.Errorf("redis lock error: %w", err)
	}
	if !ok {
		return fmt.Errorf("one or more seats already locked")
	}

	// Step 3: Create pending order in DB
	order.Status = "pending"
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
	// Step 1: Extract JWT
	token, err := auth.getM2MToken(r)
	if err != nil {
		return nil, fmt.Errorf("unauthorized: %w", err)
	}

	userID, err := auth.ExtractUserIDFromJWT(token)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Step 2: Generate unique OrderID
	orderID := uuid.NewString()

	// Step 3: Call Seat Validation Service
	seatServiceBase := os.Getenv("SEAT_SERVICE_URL") // e.g. http://seating-service:8080
	validateURL := fmt.Sprintf("%s/api/seats/validate", seatServiceBase)

	reqBody, _ := json.Marshal(orderReq)
	req, err := http.NewRequest("POST", validateURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
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

	// Step 5: Build order model
	order := models.Order{
		OrderID:   orderID,
		UserID:    userID,
		SessionID: orderReq.SessionID,
		SeatIDs:   orderReq.SeatIDs,
		Status:    "pending",
		Price:     0.0, // TODO: price service
		CreatedAt: time.Now(),
	}

	// Step 6: Save order (DB + Kafka event)
	if err := s.PlaceOrder(order); err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	// Step 7: Build response
	return &models.OrderResponse{
		OrderID:   orderID,
		SessionID: orderReq.SessionID,
		SeatIDs:   orderReq.SeatIDs,
		UserID:    userID,
	}, nil
}
