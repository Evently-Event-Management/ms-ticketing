package order

import (
	"errors"
	"fmt"
	"ms-ticketing/internal/order/db"
)

type DBLayer interface {
	CreateOrder(order db.Order) error
	GetOrderByID(id string) (*db.Order, error)
	UpdateOrder(order db.Order) error
	CancelOrder(id string) error
	GetOrderBySeatAndEvent(seatID, eventID string) (*db.Order, error)
}

type RedisLock interface {
	LockSeats(seatIDs []string, orderID string) (bool, error)
	UnlockSeats(seatIDs []string, orderID string) error
}

type KafkaPublisher interface {
	PublishOrderCreated(order db.Order) error
}

type OrderService struct {
	DB    DBLayer
	Redis RedisLock
	Kafka KafkaPublisher
}

func NewOrderService(db DBLayer, redis RedisLock, kafka KafkaPublisher) *OrderService {
	return &OrderService{DB: db, Redis: redis, Kafka: kafka}
}

func (s *OrderService) GetOrderBySeatAndEvent(seatID, eventID string) (*db.Order, error) {
	return s.DB.GetOrderBySeatAndEvent(seatID, eventID)
}

func (s *OrderService) PlaceOrder(order db.Order) error {
	fmt.Printf("Placing order: %s for event: %s\n", order.ID, order.EventID)

	for _, seatID := range order.SeatIDs {
		existingOrder, err := s.DB.GetOrderBySeatAndEvent(seatID, order.EventID)
		fmt.Printf("Seat ID: %s, Existing Order: %+v, Error: %v\n", seatID, existingOrder, err)
		if err == nil && existingOrder != nil && existingOrder.Status == "completed" {
			fmt.Printf("Seat %s for event %s is already booked.\n", seatID, order.EventID)
			return fmt.Errorf("seat %s for event %s is already booked", seatID, order.EventID)
		}
	}

	ok, err := s.Redis.LockSeats(order.SeatIDs, order.ID)
	if err != nil {
		fmt.Printf("Error locking seats: %v\n", err)
		return fmt.Errorf("redis error: %w", err)
	}
	if !ok {
		fmt.Println("One or more seats are already locked. Aborting order.")
		return fmt.Errorf("one or more seats already locked")
	}

	order.Status = "pending"
	fmt.Println("Creating order in DB...")
	err = s.DB.CreateOrder(order)
	if err != nil {
		fmt.Printf("Failed to create order: %v. Rolling back seat locks.\n", err)
		s.Redis.UnlockSeats(order.SeatIDs, order.ID)
		return err
	}
	fmt.Println("Order created successfully, publishing to Kafka...")
	// Optionally publish to Kafka here
	return nil
}

func (s *OrderService) GetOrder(id string) (*db.Order, error) {
	return s.DB.GetOrderByID(id)
}

func (s *OrderService) UpdateOrder(id string, updateData db.Order) error {
	fmt.Printf("Updating order: %s\n", id)
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		fmt.Printf("Order not found: %s\n", id)
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		fmt.Println("Cannot update a non-pending order")
		return errors.New("cannot update a non-pending order")
	}

	fmt.Println("Updating order in DB...")
	return s.DB.UpdateOrder(updateData)
}

func (s *OrderService) CancelOrder(id string) error {
	fmt.Printf("Cancelling order: %s\n", id)
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		fmt.Printf("Order not found: %s\n", id)
		return fmt.Errorf("order %s not found: %w", id, err)
	}
	if order.Status != "pending" {
		fmt.Println("Cannot cancel a non-pending order")
		return errors.New("cannot cancel a non-pending order")
	}

	order.Status = "cancelled"
	if err := s.DB.UpdateOrder(*order); err != nil {
		fmt.Printf("Failed to update order status to cancelled: %v\n", err)
		return fmt.Errorf("failed to cancel order %s: %w", id, err)
	}

	for _, seatID := range order.SeatIDs {
		_ = s.Redis.UnlockSeats(order.SeatIDs, order.ID)
		fmt.Printf("Unlocked seat: %s\n", seatID)
	}
	return nil
}

func (s *OrderService) ApplyPromoCode(id string, code string) error {
	fmt.Printf("Applying promo code '%s' to order: %s\n", code, id)
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		fmt.Printf("Order not found: %s\n", id)
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		fmt.Println("Cannot apply promo to a non-pending order")
		return errors.New("cannot apply promo to a non-pending order")
	}

	order.PromoCode = code
	order.DiscountApplied = true

	fmt.Println("Updating order with promo code...")
	if err := s.DB.UpdateOrder(*order); err != nil {
		fmt.Printf("Failed to apply promo code: %v\n", err)
		return fmt.Errorf("failed to apply promo: %w", err)
	}

	return nil
}

func (s *OrderService) Checkout(id string) error {
	fmt.Printf("Checking out order: %s\n", id)
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		fmt.Printf("Order not found: %s\n", id)
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		fmt.Println("Order is not in a valid state for checkout")
		return errors.New("order is not in a valid state for checkout")
	}

	order.Status = "completed"
	fmt.Println("Updating order status to completed...")

	if err := s.DB.UpdateOrder(*order); err != nil {
		fmt.Printf("Failed to update status to completed: %v\n", err)
		return fmt.Errorf("failed to complete checkout: %w", err)
	}

	return nil
}
