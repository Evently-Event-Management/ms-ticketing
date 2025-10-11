package db_test

import (
	"context"
	"database/sql"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/tickets/db"
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
	_, err = bunDB.NewCreateTable().Model((*models.Ticket)(nil)).Exec(context.Background())
	if err != nil {
		t.Fatalf("Failed to create ticket table: %v", err)
	}

	_, err = bunDB.NewCreateTable().Model((*models.TicketCount)(nil)).Exec(context.Background())
	if err != nil {
		t.Fatalf("Failed to create ticket_count table: %v", err)
	}

	// Return test DB
	return &db.DB{Bun: bunDB}, bunDB
}

func TestCreateAndGetTicket(t *testing.T) {
	// Set up test DB
	ticketDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create a test ticket
	ticketID := uuid.New().String()
	orderID := uuid.New().String()
	testTicket := models.Ticket{
		TicketID:        ticketID,
		OrderID:         orderID,
		SeatID:          "seat1",
		SeatLabel:       "A1",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 50.0,
		IssuedAt:        time.Now(),
	}

	// Test case: Create ticket
	err := ticketDB.CreateTicket(testTicket)
	assert.NoError(t, err)

	// Test case: Get ticket by ID
	ticket, err := ticketDB.GetTicketByID(ticketID)
	assert.NoError(t, err)
	assert.NotNil(t, ticket)
	assert.Equal(t, ticketID, ticket.TicketID)
	assert.Equal(t, "seat1", ticket.SeatID)
	assert.Equal(t, "A1", ticket.SeatLabel)

	// Test case: Get non-existent ticket
	ticket, err = ticketDB.GetTicketByID("non-existent")
	assert.Error(t, err)
	assert.Nil(t, ticket)
}

func TestUpdateTicket(t *testing.T) {
	// Set up test DB
	ticketDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create a test ticket
	ticketID := uuid.New().String()
	orderID := uuid.New().String()
	testTicket := models.Ticket{
		TicketID:        ticketID,
		OrderID:         orderID,
		SeatID:          "seat1",
		SeatLabel:       "A1",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 50.0,
		IssuedAt:        time.Now(),
		CheckedIn:       false,
	}

	// Insert the test ticket
	err := ticketDB.CreateTicket(testTicket)
	assert.NoError(t, err)

	// Test case: Update ticket
	testTicket.CheckedIn = true
	err = ticketDB.UpdateTicket(testTicket)
	assert.NoError(t, err)

	// Verify the ticket was updated
	updatedTicket, err := ticketDB.GetTicketByID(ticketID)
	assert.NoError(t, err)
	
	// Make sure the CheckedIn field was properly updated
	assert.Equal(t, true, updatedTicket.CheckedIn)
}

func TestCancelTicket(t *testing.T) {
	// Set up test DB
	ticketDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create a test ticket
	ticketID := uuid.New().String()
	orderID := uuid.New().String()
	testTicket := models.Ticket{
		TicketID:        ticketID,
		OrderID:         orderID,
		SeatID:          "seat1",
		SeatLabel:       "A1",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 50.0,
		IssuedAt:        time.Now(),
	}

	// Insert the test ticket
	err := ticketDB.CreateTicket(testTicket)
	assert.NoError(t, err)

	// Test case: Cancel ticket
	err = ticketDB.CancelTicket(ticketID)
	assert.NoError(t, err)

	// Verify the ticket was deleted
	_, err = ticketDB.GetTicketByID(ticketID)
	assert.Error(t, err)
}

func TestGetTicketsByOrder(t *testing.T) {
	// Set up test DB
	ticketDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create test tickets for the same order
	orderID := uuid.New().String()
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

	// Insert the test tickets
	for _, ticket := range testTickets {
		err := ticketDB.CreateTicket(ticket)
		assert.NoError(t, err)
	}

	// Test case: Get tickets by order
	tickets, err := ticketDB.GetTicketsByOrder(orderID)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(tickets))
	assert.Equal(t, "seat1", tickets[0].SeatID)
	assert.Equal(t, "seat2", tickets[1].SeatID)

	// Test case: Get tickets for non-existent order
	tickets, err = ticketDB.GetTicketsByOrder("non-existent")
	assert.NoError(t, err) // Should return empty slice, not error
	assert.Equal(t, 0, len(tickets))
}

func TestGetTicketsByUser(t *testing.T) {
	// Set up test DB
	ticketDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Create test orders for a user
	userID := "user123"
	orderID1 := uuid.New().String()
	orderID2 := uuid.New().String()

	// We need to create the orders table for this test
	_, err := bunDB.NewCreateTable().Model((*models.Order)(nil)).Exec(context.Background())
	assert.NoError(t, err)

	// Insert test orders
	orders := []models.Order{
		{
			OrderID:   orderID1,
			UserID:    userID,
			Status:    "completed",
			CreatedAt: time.Now(),
		},
		{
			OrderID:   orderID2,
			UserID:    userID,
			Status:    "completed",
			CreatedAt: time.Now(),
		},
	}

	for _, order := range orders {
		_, err := bunDB.NewInsert().Model(&order).Exec(context.Background())
		assert.NoError(t, err)
	}

	// Create test tickets for the orders
	testTickets := []models.Ticket{
		{
			TicketID:        uuid.New().String(),
			OrderID:         orderID1,
			SeatID:          "seat1",
			SeatLabel:       "A1",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
		{
			TicketID:        uuid.New().String(),
			OrderID:         orderID2,
			SeatID:          "seat2",
			SeatLabel:       "A2",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
	}

	// Insert the test tickets
	for _, ticket := range testTickets {
		err := ticketDB.CreateTicket(ticket)
		assert.NoError(t, err)
	}

	// Test case: Get tickets by user
	tickets, err := ticketDB.GetTicketsByUser(userID)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(tickets))
}

func TestTicketCount(t *testing.T) {
	// Set up test DB
	ticketDB, bunDB := setupTestDB(t)
	defer bunDB.Close()

	// Test case: Increment ticket count
	eventID := "event123"
	sessionID := "session456"
	timestamp := time.Now().Truncate(24 * time.Hour)

	// Increment count
	err := ticketDB.IncrementTicketCount(eventID, sessionID, timestamp)
	assert.NoError(t, err)

	// Increment again to ensure counter increases
	err = ticketDB.IncrementTicketCount(eventID, sessionID, timestamp)
	assert.NoError(t, err)

	// Test case: Get ticket counts for event
	counts, err := ticketDB.GetTicketCountsForEvent(eventID)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(counts))
	assert.Equal(t, eventID, counts[0].EventID)
	assert.Equal(t, sessionID, counts[0].SessionID)
	assert.Equal(t, 2, counts[0].Count) // Should be 2 after two increments

	// Test case: Get ticket counts for session
	counts, err = ticketDB.GetTicketCountsForSession(sessionID)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(counts))
	assert.Equal(t, eventID, counts[0].EventID)
	assert.Equal(t, sessionID, counts[0].SessionID)
	assert.Equal(t, 2, counts[0].Count)
}