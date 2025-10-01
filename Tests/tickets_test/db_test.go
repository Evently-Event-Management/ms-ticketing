package tickets_test

import (
	"context"
	"database/sql"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/tickets/db"
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
	err = bunDB.ResetModel(context.Background(), (*models.Ticket)(nil))
	if err != nil {
		return nil, err
	}

	// Return a new DB instance
	return &db.DB{Bun: bunDB}, nil
}

func TestCreateAndGetTicket(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample ticket
	ticket := models.Ticket{
		TicketID:        "test-ticket-id",
		OrderID:         "test-order-id",
		SeatID:          "test-seat-id",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		QRCode:          []byte("test-qr-code"),
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
	}

	// Test CreateTicket
	err = db.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Test GetTicketByID
	retrievedTicket, err := db.GetTicketByID("test-ticket-id")
	if err != nil {
		t.Fatalf("Failed to retrieve ticket: %v", err)
	}

	// Verify retrieved ticket matches created ticket
	if retrievedTicket.TicketID != ticket.TicketID {
		t.Errorf("Expected ticket ID %s, got %s", ticket.TicketID, retrievedTicket.TicketID)
	}
	if retrievedTicket.OrderID != ticket.OrderID {
		t.Errorf("Expected order ID %s, got %s", ticket.OrderID, retrievedTicket.OrderID)
	}
	if retrievedTicket.SeatID != ticket.SeatID {
		t.Errorf("Expected seat ID %s, got %s", ticket.SeatID, retrievedTicket.SeatID)
	}
}

func TestUpdateTicket(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample ticket
	ticket := models.Ticket{
		TicketID:        "test-ticket-id",
		OrderID:         "test-order-id",
		SeatID:          "test-seat-id",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		QRCode:          []byte("test-qr-code"),
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
	}

	// Create the ticket
	err = db.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Update the ticket
	ticket.SeatLabel = "A1-updated"
	ticket.Colour = "green"
	ticket.PriceAtPurchase = 150.0

	err = db.UpdateTicket(ticket)
	if err != nil {
		t.Fatalf("Failed to update ticket: %v", err)
	}

	// Retrieve the updated ticket
	retrievedTicket, err := db.GetTicketByID("test-ticket-id")
	if err != nil {
		t.Fatalf("Failed to retrieve updated ticket: %v", err)
	}

	// Verify the updates were applied
	if retrievedTicket.SeatLabel != "A1-updated" {
		t.Errorf("Expected seat label %s, got %s", "A1-updated", retrievedTicket.SeatLabel)
	}
	if retrievedTicket.Colour != "green" {
		t.Errorf("Expected colour %s, got %s", "green", retrievedTicket.Colour)
	}
	if retrievedTicket.PriceAtPurchase != 150.0 {
		t.Errorf("Expected price %f, got %f", 150.0, retrievedTicket.PriceAtPurchase)
	}
}

func TestCancelTicket(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	// Create a sample ticket
	ticket := models.Ticket{
		TicketID:        "test-ticket-id",
		OrderID:         "test-order-id",
		SeatID:          "test-seat-id",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		QRCode:          []byte("test-qr-code"),
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
	}

	// Create the ticket
	err = db.CreateTicket(ticket)
	if err != nil {
		t.Fatalf("Failed to create ticket: %v", err)
	}

	// Cancel the ticket
	err = db.CancelTicket("test-ticket-id")
	if err != nil {
		t.Fatalf("Failed to cancel ticket: %v", err)
	}

	// Try to retrieve the cancelled ticket
	_, err = db.GetTicketByID("test-ticket-id")
	if err == nil {
		t.Error("Expected error when retrieving cancelled ticket, got nil")
	}
}

func TestGetTicketsByOrder(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatalf("Failed to set up test database: %v", err)
	}

	orderID := "test-order-id"

	// Create multiple tickets for the same order
	tickets := []models.Ticket{
		{
			TicketID:        "ticket1",
			OrderID:         orderID,
			SeatID:          "seat1",
			SeatLabel:       "A1",
			Colour:          "blue",
			TierID:          "tier1",
			TierName:        "VIP",
			QRCode:          []byte("qr1"),
			PriceAtPurchase: 100.0,
			IssuedAt:        time.Now(),
		},
		{
			TicketID:        "ticket2",
			OrderID:         orderID,
			SeatID:          "seat2",
			SeatLabel:       "A2",
			Colour:          "red",
			TierID:          "tier1",
			TierName:        "VIP",
			QRCode:          []byte("qr2"),
			PriceAtPurchase: 100.0,
			IssuedAt:        time.Now(),
		},
	}

	// Create the tickets
	for _, ticket := range tickets {
		err = db.CreateTicket(ticket)
		if err != nil {
			t.Fatalf("Failed to create ticket: %v", err)
		}
	}

	// Get tickets by order
	retrievedTickets, err := db.GetTicketsByOrder(orderID)
	if err != nil {
		t.Fatalf("Failed to retrieve tickets by order: %v", err)
	}

	// Verify the correct number of tickets were retrieved
	if len(retrievedTickets) != len(tickets) {
		t.Errorf("Expected %d tickets, got %d", len(tickets), len(retrievedTickets))
	}

	// Verify each ticket is in the result
	ticketIDs := make(map[string]bool)
	for _, ticket := range retrievedTickets {
		ticketIDs[ticket.TicketID] = true
	}

	for _, ticket := range tickets {
		if !ticketIDs[ticket.TicketID] {
			t.Errorf("Expected ticket %s to be in results, but it was not", ticket.TicketID)
		}
	}
}

func TestGetTicketsByUser(t *testing.T) {
	// This test requires the Ticket model to have a UserID field
	// If the model doesn't have this field, you would need to adapt this test
	// or add the field to the model

	t.Skip("Skipping test for GetTicketsByUser as it requires additional implementation")
}
