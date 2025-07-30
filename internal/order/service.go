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
}

type RedisLock interface {
	AcquireLock(key string) bool
	ReleaseLock(key string)
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

func (s *OrderService) PlaceOrder(order db.Order) error {
	if !s.Redis.AcquireLock(order.ID) {
		return fmt.Errorf("could not acquire lock for order %s", order.ID)
	}
	defer s.Redis.ReleaseLock(order.ID)

	order.Status = "pending"

	err := s.DB.CreateOrder(order)
	if err != nil {
		return err
	}

	return s.Kafka.PublishOrderCreated(order)
}

func (s *OrderService) GetOrder(id string) (*db.Order, error) {
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get order %s: %w", id, err)
	}
	return order, nil
}

func (s *OrderService) UpdateOrder(id string, updateData db.Order) error {
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		return errors.New("cannot update a non-pending order")
	}

	return s.DB.UpdateOrder(updateData)
}

func (s *OrderService) CancelOrder(id string) error {
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

	return nil
}

func (s *OrderService) ApplyPromoCode(id string, code string) error {
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		return errors.New("cannot apply promo to a non-pending order")
	}

	order.PromoCode = code
	order.DiscountApplied = true

	if err := s.DB.UpdateOrder(*order); err != nil {
		return fmt.Errorf("failed to apply promo: %w", err)
	}

	return nil
}

func (s *OrderService) Checkout(id string) error {
	order, err := s.DB.GetOrderByID(id)
	if err != nil {
		return fmt.Errorf("order %s not found: %w", id, err)
	}

	if order.Status != "pending" {
		return errors.New("order is not in a valid state for checkout")
	}

	// Simulate payment success
	order.Status = "completed"

	if err := s.DB.UpdateOrder(*order); err != nil {
		return fmt.Errorf("failed to complete checkout: %w", err)
	}

	// Optionally publish an event like "OrderCompleted"
	return nil
}
