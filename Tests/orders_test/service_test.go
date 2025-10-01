package orders_test

import (
	"errors"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/order"
	tickets "ms-ticketing/internal/tickets/service"
	"net/http"
	"testing"
	"time"
)

// Mock implementations for testing

type MockOrderDB struct {
	orders       map[string]*models.Order
	seatOrders   map[string]string
	shouldFailOn string
	errorMsg     string
}

func NewMockOrderDB() *MockOrderDB {
	return &MockOrderDB{
		orders:     make(map[string]*models.Order),
		seatOrders: make(map[string]string),
	}
}

func (m *MockOrderDB) CreateOrder(order models.Order) error {
	if m.shouldFailOn == "CreateOrder" {
		return errors.New(m.errorMsg)
	}
	m.orders[order.OrderID] = &order
	for _, seatID := range order.SeatIDs {
		m.seatOrders[seatID] = order.OrderID
	}
	return nil
}

func (m *MockOrderDB) GetOrderByID(id string) (*models.Order, error) {
	if m.shouldFailOn == "GetOrderByID" {
		return nil, errors.New(m.errorMsg)
	}
	order, exists := m.orders[id]
	if !exists {
		return nil, errors.New("order not found")
	}
	return order, nil
}

func (m *MockOrderDB) UpdateOrder(order models.Order) error {
	if m.shouldFailOn == "UpdateOrder" {
		return errors.New(m.errorMsg)
	}
	_, exists := m.orders[order.OrderID]
	if !exists {
		return errors.New("order not found")
	}
	m.orders[order.OrderID] = &order
	return nil
}

func (m *MockOrderDB) CancelOrder(id string) error {
	if m.shouldFailOn == "CancelOrder" {
		return errors.New(m.errorMsg)
	}
	order, exists := m.orders[id]
	if !exists {
		return errors.New("order not found")
	}
	for _, seatID := range order.SeatIDs {
		delete(m.seatOrders, seatID)
	}
	delete(m.orders, id)
	return nil
}

func (m *MockOrderDB) GetOrderBySeat(seatID string) (*models.Order, error) {
	if m.shouldFailOn == "GetOrderBySeat" {
		return nil, errors.New(m.errorMsg)
	}
	orderID, exists := m.seatOrders[seatID]
	if !exists {
		return nil, errors.New("no order found for seat")
	}
	return m.orders[orderID], nil
}

func (m *MockOrderDB) GetSessionIdBySeat(seatID string) (string, error) {
	if m.shouldFailOn == "GetSessionIdBySeat" {
		return "", errors.New(m.errorMsg)
	}
	orderID, exists := m.seatOrders[seatID]
	if !exists {
		return "", errors.New("no order found for seat")
	}
	return m.orders[orderID].SessionID, nil
}

type MockRedisLock struct {
	lockedSeats     map[string]string
	shouldFailOn    string
	errorMsg        string
	lockingSucceeds bool
}

func NewMockRedisLock() *MockRedisLock {
	return &MockRedisLock{
		lockedSeats:     make(map[string]string),
		lockingSucceeds: true,
	}
}

func (m *MockRedisLock) LockSeats(seatIDs []string, orderID string) (bool, error) {
	if m.shouldFailOn == "LockSeats" {
		return false, errors.New(m.errorMsg)
	}

	if !m.lockingSucceeds {
		return false, nil
	}

	for _, seatID := range seatIDs {
		m.lockedSeats[seatID] = orderID
	}
	return true, nil
}

func (m *MockRedisLock) UnlockSeats(seatIDs []string, orderID string) error {
	if m.shouldFailOn == "UnlockSeats" {
		return errors.New(m.errorMsg)
	}

	for _, seatID := range seatIDs {
		if m.lockedSeats[seatID] == orderID {
			delete(m.lockedSeats, seatID)
		}
	}
	return nil
}

type MockKafkaProducer struct {
	messages     map[string][]string
	shouldFailOn string
	errorMsg     string
	closed       bool
}

func NewMockKafkaProducer() *MockKafkaProducer {
	return &MockKafkaProducer{
		messages: make(map[string][]string),
	}
}

func (m *MockKafkaProducer) Publish(topic string, key string, value []byte) error {
	if m.shouldFailOn == "Publish" {
		return errors.New(m.errorMsg)
	}

	if m.closed {
		return errors.New("producer is closed")
	}

	m.messages[topic] = append(m.messages[topic], string(value))
	return nil
}

func (m *MockKafkaProducer) Close() error {
	if m.shouldFailOn == "Close" {
		return errors.New(m.errorMsg)
	}

	m.closed = true
	return nil
}

type MockTicketService struct {
	tickets      map[string]*models.Ticket
	shouldFailOn string
	errorMsg     string
}

func NewMockTicketService() *MockTicketService {
	return &MockTicketService{
		tickets: make(map[string]*models.Ticket),
	}
}

func (m *MockTicketService) PlaceTicket(ticket models.Ticket) error {
	if m.shouldFailOn == "PlaceTicket" {
		return errors.New(m.errorMsg)
	}

	m.tickets[ticket.TicketID] = &ticket
	return nil
}

func setupMocks() (*MockOrderDB, *MockRedisLock, *MockKafkaProducer, *MockTicketService) {
	db := NewMockOrderDB()
	redis := NewMockRedisLock()
	kafka := NewMockKafkaProducer()
	ticketService := NewMockTicketService()

	return db, redis, kafka, ticketService
}

func TestPlaceOrder(t *testing.T) {
	db, redis, kafka, _ := setupMocks()
	httpClient := &http.Client{}

	// Create order service with mocks
	orderService := order.NewOrderService(db, redis, kafka, &tickets.TicketService{}, httpClient)

	// Create an order
	testOrder := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now(),
	}

	// Test placing an order
	err := orderService.PlaceOrder(testOrder)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify order was created in the database
	order, err := db.GetOrderByID("test-order-id")
	if err != nil {
		t.Errorf("Failed to retrieve order: %v", err)
	}

	if order.OrderID != testOrder.OrderID {
		t.Errorf("Expected order ID %s, got %s", testOrder.OrderID, order.OrderID)
	}

	if order.UserID != testOrder.UserID {
		t.Errorf("Expected user ID %s, got %s", testOrder.UserID, order.UserID)
	}

	// Verify seats were locked
	for _, seatID := range testOrder.SeatIDs {
		if redis.lockedSeats[seatID] != testOrder.OrderID {
			t.Errorf("Expected seat %s to be locked for order %s", seatID, testOrder.OrderID)
		}
	}

	// Test with DB failure
	db.shouldFailOn = "CreateOrder"
	db.errorMsg = "db error"

	err = orderService.PlaceOrder(testOrder)
	if err == nil {
		t.Error("Expected error when DB fails, got nil")
	}
}

func TestGetOrder(t *testing.T) {
	db, redis, kafka, _ := setupMocks()
	httpClient := &http.Client{}

	// Create order service with mocks
	orderService := order.NewOrderService(db, redis, kafka, &tickets.TicketService{}, httpClient)

	// Add an order to the mock DB
	testOrder := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now(),
	}

	db.CreateOrder(testOrder)

	// Test getting an order
	order, err := orderService.GetOrder("test-order-id")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if order.OrderID != testOrder.OrderID {
		t.Errorf("Expected order ID %s, got %s", testOrder.OrderID, order.OrderID)
	}

	// Test getting a non-existent order
	_, err = orderService.GetOrder("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent order, got nil")
	}
}

func TestUpdateOrderService(t *testing.T) {
	db, redis, kafka, _ := setupMocks()
	httpClient := &http.Client{}

	// Create order service with mocks
	orderService := order.NewOrderService(db, redis, kafka, &tickets.TicketService{}, httpClient)

	// Add an order to the mock DB
	testOrder := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now(),
	}

	db.CreateOrder(testOrder)

	// Create update data
	updateData := models.Order{
		Status: "completed",
		Price:  250.0,
	}

	// Test updating an order
	err := orderService.UpdateOrder("test-order-id", updateData)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify order was updated
	order, _ := db.GetOrderByID("test-order-id")

	if order.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", order.Status)
	}

	if order.Price != 250.0 {
		t.Errorf("Expected price 250.0, got %f", order.Price)
	}

	// Test updating a non-existent order
	err = orderService.UpdateOrder("non-existent", updateData)
	if err == nil {
		t.Error("Expected error when updating non-existent order, got nil")
	}
}

func TestGetOrderBySeatService(t *testing.T) {
	db, redis, kafka, _ := setupMocks()
	httpClient := &http.Client{}

	// Create order service with mocks
	orderService := order.NewOrderService(db, redis, kafka, &tickets.TicketService{}, httpClient)

	// Add an order to the mock DB
	testOrder := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now(),
	}

	db.CreateOrder(testOrder)

	// Test getting an order by seat
	order, err := orderService.GetOrderBySeat("seat1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if order.OrderID != testOrder.OrderID {
		t.Errorf("Expected order ID %s, got %s", testOrder.OrderID, order.OrderID)
	}

	// Test getting an order by non-existent seat
	_, err = orderService.GetOrderBySeat("non-existent")
	if err == nil {
		t.Error("Expected error when getting order by non-existent seat, got nil")
	}
}
