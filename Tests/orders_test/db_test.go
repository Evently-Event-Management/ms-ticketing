package orders_test

import (
	"context"
	"database/sql"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/order/db"
	"testing"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB() (*db.DB, error) {
	// Create a new SQLite in-memory database
	sqldb, err := sql.Open(sqliteshim.ShimName, "file::memory:?cache=shared")
	if err != nil {
		return nil, err
	}

	// Create a new bun.DB instance
	bunDB := bun.NewDB(sqldb, sqlitedialect.New())

	// Create tables
	err = bunDB.ResetModel(context.Background(), (*models.Order)(nil))
	if err != nil {
		return nil, err
	}

	// Return a new DB instance
	return &db.DB{Bun: bunDB}, nil
}

func TestCreateAndGetOrder(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample order
	order := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now().Round(time.Second), // Round to avoid precision issues
	}

	// Test CreateOrder
	err = db.CreateOrder(order)
	if err != nil {
		t.Fatalf("Failed to create order: %v", err)
	}

	// Test GetOrderByID
	retrievedOrder, err := db.GetOrderByID("test-order-id")
	if err != nil {
		t.Fatalf("Failed to retrieve order: %v", err)
	}

	// Verify retrieved order matches created order
	if retrievedOrder.OrderID != order.OrderID {
		t.Errorf("Expected order ID %s, got %s", order.OrderID, retrievedOrder.OrderID)
	}
	if retrievedOrder.UserID != order.UserID {
		t.Errorf("Expected user ID %s, got %s", order.UserID, retrievedOrder.UserID)
	}
	if retrievedOrder.SessionID != order.SessionID {
		t.Errorf("Expected session ID %s, got %s", order.SessionID, retrievedOrder.SessionID)
	}
	if len(retrievedOrder.SeatIDs) != len(order.SeatIDs) {
		t.Errorf("Expected %d seats, got %d", len(order.SeatIDs), len(retrievedOrder.SeatIDs))
	} else {
		for i, seatID := range order.SeatIDs {
			if retrievedOrder.SeatIDs[i] != seatID {
				t.Errorf("Expected seat ID %s at position %d, got %s", seatID, i, retrievedOrder.SeatIDs[i])
			}
		}
	}
	if retrievedOrder.Status != order.Status {
		t.Errorf("Expected status %s, got %s", order.Status, retrievedOrder.Status)
	}
	if retrievedOrder.Price != order.Price {
		t.Errorf("Expected price %f, got %f", order.Price, retrievedOrder.Price)
	}
}

func TestUpdateOrder(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample order
	order := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now().Round(time.Second),
	}

	// Create the order
	err = db.CreateOrder(order)
	if err != nil {
		t.Fatalf("Failed to create order: %v", err)
	}

	// Update the order
	order.Status = "completed"
	order.Price = 250.0

	err = db.UpdateOrder(order)
	if err != nil {
		t.Fatalf("Failed to update order: %v", err)
	}

	// Retrieve the updated order
	retrievedOrder, err := db.GetOrderByID("test-order-id")
	if err != nil {
		t.Fatalf("Failed to retrieve updated order: %v", err)
	}

	// Verify the updates were applied
	if retrievedOrder.Status != "completed" {
		t.Errorf("Expected status %s, got %s", "completed", retrievedOrder.Status)
	}
	if retrievedOrder.Price != 250.0 {
		t.Errorf("Expected price %f, got %f", 250.0, retrievedOrder.Price)
	}
}

func TestCancelOrder(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample order
	order := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now().Round(time.Second),
	}

	// Create the order
	err = db.CreateOrder(order)
	if err != nil {
		t.Fatalf("Failed to create order: %v", err)
	}

	// Cancel the order
	err = db.CancelOrder("test-order-id")
	if err != nil {
		t.Fatalf("Failed to cancel order: %v", err)
	}

	// Try to retrieve the cancelled order
	_, err = db.GetOrderByID("test-order-id")
	if err == nil {
		t.Error("Expected error when retrieving cancelled order, got nil")
	}
}

func TestGetOrderBySeat(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample order with seats
	order := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now().Round(time.Second),
	}

	// Create the order
	err = db.CreateOrder(order)
	if err != nil {
		t.Fatalf("Failed to create order: %v", err)
	}

	// Get order by seat
	retrievedOrder, err := db.GetOrderBySeat("seat1")
	if err != nil {
		t.Fatalf("Failed to retrieve order by seat: %v", err)
	}

	// Verify the correct order was retrieved
	if retrievedOrder.OrderID != order.OrderID {
		t.Errorf("Expected order ID %s, got %s", order.OrderID, retrievedOrder.OrderID)
	}

	// Try to get order by non-existent seat
	_, err = db.GetOrderBySeat("non-existent")
	if err == nil {
		t.Error("Expected error when retrieving order by non-existent seat, got nil")
	}
}

func TestGetSessionIdBySeat(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample order with seats
	order := models.Order{
		OrderID:   "test-order-id",
		UserID:    "test-user-id",
		SessionID: "test-session-id",
		SeatIDs:   []string{"seat1", "seat2"},
		Status:    "confirmed",
		Price:     200.0,
		CreatedAt: time.Now().Round(time.Second),
	}

	// Create the order
	err = db.CreateOrder(order)
	if err != nil {
		t.Fatalf("Failed to create order: %v", err)
	}

	// Get session ID by seat
	sessionID, err := db.GetSessionIdBySeat("seat1")
	if err != nil {
		t.Fatalf("Failed to retrieve session ID by seat: %v", err)
	}

	// Verify the correct session ID was retrieved
	if sessionID != order.SessionID {
		t.Errorf("Expected session ID %s, got %s", order.SessionID, sessionID)
	}

	// Try to get session ID by non-existent seat
	_, err = db.GetSessionIdBySeat("non-existent")
	if err == nil {
		t.Error("Expected error when retrieving session ID by non-existent seat, got nil")
	}
}
