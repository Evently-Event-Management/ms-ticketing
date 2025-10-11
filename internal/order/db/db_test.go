package db_test

import (
	"context"
	"database/sql"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/order/db"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	_ "github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB(t *testing.T) (*db.DB, *bun.DB) {
	// Connect to an in-memory SQLite DB for testing
	sqldb, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to connect to in-memory database: %v", err)
	}

	// Create a Bun DB instance
	bunDB := bun.NewDB(sqldb, sqlitedialect.New())

	// Create required tables
	_, err = bunDB.NewCreateTable().Model((*models.Order)(nil)).Exec(context.Background())
	if err != nil {
		t.Fatalf("Failed to create order table: %v", err)
	}

	_, err = bunDB.NewCreateTable().Model((*models.Ticket)(nil)).Exec(context.Background())
	if err != nil {
		t.Fatalf("Failed to create ticket table: %v", err)
	}

	// Return test DB
	return &db.DB{Bun: bunDB}, bunDB
}

func TestGetOrderByID(t *testing.T) {
	// Set up test DB
	orderDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create a test order
	orderID := uuid.New().String()
	testOrder := models.Order{
		OrderID:   orderID,
		UserID:    "user123",
		EventID:   "event456",
		SessionID: "session789",
		Status:    "pending",
		Price:     100.0,
		CreatedAt: time.Now(),
	}

	// Insert test order into DB
	_, err := bunDB.NewInsert().Model(&testOrder).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Get existing order
	order, err := orderDB.GetOrderByID(orderID)
	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, orderID, order.OrderID)
	assert.Equal(t, "user123", order.UserID)
	assert.Equal(t, "pending", order.Status)

	// Test case: Get non-existent order
	order, err = orderDB.GetOrderByID("non-existent")
	assert.Error(t, err)
	assert.Nil(t, order)
}

func TestCreateAndUpdateOrder(t *testing.T) {
	// Set up test DB
	orderDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Test case: Create a new order
	orderID := uuid.New().String()
	newOrder := models.Order{
		OrderID:   orderID,
		UserID:    "user123",
		EventID:   "event456",
		SessionID: "session789",
		Status:    "pending",
		Price:     100.0,
		CreatedAt: time.Now(),
	}

	// Create the order
	err := orderDB.CreateOrder(newOrder)
	assert.NoError(t, err)

	// Verify the order was created
	var order models.Order
	err = bunDB.NewSelect().
		Model(&order).
		Where("order_id = ?", orderID).
		Limit(1).
		Scan(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, orderID, order.OrderID)
	assert.Equal(t, "pending", order.Status)

	// Test case: Update the order
	order.Status = "completed"
	order.PaymentIntentID = "pi_test123"

	// Update the order
	err = orderDB.UpdateOrder(order)
	assert.NoError(t, err)

	// Verify the order was updated
	var updatedOrder models.Order
	err = bunDB.NewSelect().
		Model(&updatedOrder).
		Where("order_id = ?", orderID).
		Limit(1).
		Scan(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "completed", updatedOrder.Status)
	assert.Equal(t, "pi_test123", updatedOrder.PaymentIntentID)
}

func TestCancelOrder(t *testing.T) {
	// Set up test DB
	orderDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create a test order
	orderID := uuid.New().String()
	testOrder := models.Order{
		OrderID:   orderID,
		UserID:    "user123",
		EventID:   "event456",
		SessionID: "session789",
		Status:    "pending",
		Price:     100.0,
		CreatedAt: time.Now(),
	}

	// Insert test order into DB
	_, err := bunDB.NewInsert().Model(&testOrder).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Cancel the order
	err = orderDB.CancelOrder(orderID)
	assert.NoError(t, err)

	// Verify the order was deleted
	var count int
	count, err = bunDB.NewSelect().
		Model((*models.Order)(nil)).
		Where("order_id = ?", orderID).
		Count(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestGetOrderWithSeats(t *testing.T) {
	// Set up test DB
	orderDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create a test order
	orderID := uuid.New().String()
	testOrder := models.Order{
		OrderID:   orderID,
		UserID:    "user123",
		EventID:   "event456",
		SessionID: "session789",
		Status:    "pending",
		Price:     100.0,
		CreatedAt: time.Now(),
	}

	// Create test tickets
	testTickets := []models.Ticket{
		{
			TicketID:        uuid.New().String(),
			OrderID:         orderID,
			SeatID:          "seat1",
			SeatLabel:       "A1",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
		{
			TicketID:        uuid.New().String(),
			OrderID:         orderID,
			SeatID:          "seat2",
			SeatLabel:       "A2",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
	}

	// Insert test order and tickets into DB
	_, err := bunDB.NewInsert().Model(&testOrder).Exec(context.Background())
	assert.NoError(t, err)

	_, err = bunDB.NewInsert().Model(&testTickets).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Get order with seats
	orderWithSeats, err := orderDB.GetOrderWithSeats(orderID)
	assert.NoError(t, err)
	assert.NotNil(t, orderWithSeats)
	assert.Equal(t, orderID, orderWithSeats.OrderID)
	assert.Equal(t, 2, len(orderWithSeats.SeatIDs))
	assert.Contains(t, orderWithSeats.SeatIDs, "seat1")
	assert.Contains(t, orderWithSeats.SeatIDs, "seat2")
}

func TestGetOrderBySeat(t *testing.T) {
	// Set up test DB
	orderDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create a test order
	orderID := uuid.New().String()
	testOrder := models.Order{
		OrderID:   orderID,
		UserID:    "user123",
		EventID:   "event456",
		SessionID: "session789",
		Status:    "pending",
		Price:     100.0,
		CreatedAt: time.Now(),
	}

	// Create test ticket
	testTicket := models.Ticket{
		TicketID:        uuid.New().String(),
		OrderID:         orderID,
		SeatID:          "seat1",
		SeatLabel:       "A1",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 50.0,
		IssuedAt:        time.Now(),
	}

	// Insert test order and ticket into DB
	_, err := bunDB.NewInsert().Model(&testOrder).Exec(context.Background())
	assert.NoError(t, err)

	_, err = bunDB.NewInsert().Model(&testTicket).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Get order by seat
	order, err := orderDB.GetOrderBySeat("seat1")
	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, orderID, order.OrderID)

	// Test case: Get order by non-existent seat
	order, err = orderDB.GetOrderBySeat("non-existent")
	assert.Error(t, err)
	assert.Nil(t, order)
}

func TestGetPendingOrdersBySeat(t *testing.T) {
	// Set up test DB
	orderDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create test orders with different statuses
	pendingOrderID := uuid.New().String()
	completedOrderID := uuid.New().String()
	
	testOrders := []models.Order{
		{
			OrderID:   pendingOrderID,
			UserID:    "user123",
			EventID:   "event456",
			SessionID: "session789",
			Status:    "pending",
			Price:     100.0,
			CreatedAt: time.Now(),
		},
		{
			OrderID:   completedOrderID,
			UserID:    "user123",
			EventID:   "event456",
			SessionID: "session789",
			Status:    "completed",
			Price:     100.0,
			CreatedAt: time.Now(),
		},
	}

	// Create test tickets
	testTickets := []models.Ticket{
		{
			TicketID:        uuid.New().String(),
			OrderID:         pendingOrderID,
			SeatID:          "seat1",
			SeatLabel:       "A1",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
		{
			TicketID:        uuid.New().String(),
			OrderID:         completedOrderID,
			SeatID:          "seat1",
			SeatLabel:       "A1",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
	}

	// Insert test orders and tickets into DB
	_, err := bunDB.NewInsert().Model(&testOrders).Exec(context.Background())
	assert.NoError(t, err)

	_, err = bunDB.NewInsert().Model(&testTickets).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Get pending orders by seat
	orders, err := orderDB.GetPendingOrdersBySeat("seat1")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(orders))
	assert.Equal(t, pendingOrderID, orders[0].OrderID)
	assert.Equal(t, "pending", orders[0].Status)
}

func TestGetOrdersWithTicketsByUserID(t *testing.T) {
	// Set up test DB
	orderDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create test user
	userID := "user123"
	
	// Create test orders
	order1ID := uuid.New().String()
	order2ID := uuid.New().String()
	
	testOrders := []models.Order{
		{
			OrderID:   order1ID,
			UserID:    userID,
			EventID:   "event456",
			SessionID: "session789",
			Status:    "completed",
			Price:     100.0,
			CreatedAt: time.Now(),
		},
		{
			OrderID:   order2ID,
			UserID:    userID,
			EventID:   "event456",
			SessionID: "session789",
			Status:    "pending",
			Price:     150.0,
			CreatedAt: time.Now(),
		},
	}

	// Create test tickets
	testTickets := []models.Ticket{
		{
			TicketID:        uuid.New().String(),
			OrderID:         order1ID,
			SeatID:          "seat1",
			SeatLabel:       "A1",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
		{
			TicketID:        uuid.New().String(),
			OrderID:         order1ID,
			SeatID:          "seat2",
			SeatLabel:       "A2",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
		{
			TicketID:        uuid.New().String(),
			OrderID:         order2ID,
			SeatID:          "seat3",
			SeatLabel:       "B1",
			TierID:          "tier2",
			TierName:        "Standard",
			PriceAtPurchase: 30.0,
			IssuedAt:        time.Now(),
		},
	}

	// Insert test orders and tickets into DB
	_, err := bunDB.NewInsert().Model(&testOrders).Exec(context.Background())
	assert.NoError(t, err)

	_, err = bunDB.NewInsert().Model(&testTickets).Exec(context.Background())
	assert.NoError(t, err)

	// Test case: Get orders with tickets by user ID
	ordersWithTickets, err := orderDB.GetOrdersWithTicketsByUserID(userID)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(ordersWithTickets))
	
	// We should have 2 orders, but the exact order might be affected by 
	// the timing of the test, so we don't assert specific order
	
	// Check ticket counts
	var order1, order2 models.OrderWithTickets
	if ordersWithTickets[0].OrderID == order1ID {
		order1 = ordersWithTickets[0]
		order2 = ordersWithTickets[1]
	} else {
		order1 = ordersWithTickets[1]
		order2 = ordersWithTickets[0]
	}
	
	assert.Equal(t, 2, len(order1.Tickets))
	assert.Equal(t, 1, len(order2.Tickets))
}