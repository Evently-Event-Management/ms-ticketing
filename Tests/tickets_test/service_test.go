package tickets_test

import (
	"errors"
	"ms-ticketing/internal/models"
	tickets "ms-ticketing/internal/tickets/service"
	"os"
	"testing"
	"time"
)

// MockTicketDB is a mock implementation of the TicketDBLayer interface
type MockTicketDB struct {
	tickets       map[string]*models.Ticket
	userTickets   map[string][]models.Ticket
	orderTickets  map[string][]models.Ticket
	shouldFailOn  string
	errorToReturn error
}

func NewMockTicketDB() *MockTicketDB {
	return &MockTicketDB{
		tickets:      make(map[string]*models.Ticket),
		userTickets:  make(map[string][]models.Ticket),
		orderTickets: make(map[string][]models.Ticket),
	}
}

func (m *MockTicketDB) CreateTicket(ticket models.Ticket) error {
	if m.shouldFailOn == "CreateTicket" {
		return m.errorToReturn
	}
	m.tickets[ticket.TicketID] = &ticket

	// Add to order tickets map
	m.orderTickets[ticket.OrderID] = append(m.orderTickets[ticket.OrderID], ticket)

	return nil
}

func (m *MockTicketDB) GetTicketByID(ticketID string) (*models.Ticket, error) {
	if m.shouldFailOn == "GetTicketByID" {
		return nil, m.errorToReturn
	}
	ticket, exists := m.tickets[ticketID]
	if !exists {
		return nil, errors.New("ticket not found")
	}
	return ticket, nil
}

func (m *MockTicketDB) UpdateTicket(ticket models.Ticket) error {
	if m.shouldFailOn == "UpdateTicket" {
		return m.errorToReturn
	}
	_, exists := m.tickets[ticket.TicketID]
	if !exists {
		return errors.New("ticket not found")
	}
	m.tickets[ticket.TicketID] = &ticket
	return nil
}

func (m *MockTicketDB) CancelTicket(ticketID string) error {
	if m.shouldFailOn == "CancelTicket" {
		return m.errorToReturn
	}
	_, exists := m.tickets[ticketID]
	if !exists {
		return errors.New("ticket not found")
	}
	delete(m.tickets, ticketID)
	return nil
}

func (m *MockTicketDB) GetTicketsByOrder(orderID string) ([]models.Ticket, error) {
	if m.shouldFailOn == "GetTicketsByOrder" {
		return nil, m.errorToReturn
	}
	tickets, exists := m.orderTickets[orderID]
	if !exists || len(tickets) == 0 {
		return nil, errors.New("no tickets found for order")
	}
	return tickets, nil
}

func (m *MockTicketDB) GetTicketsByUser(userID string) ([]models.Ticket, error) {
	if m.shouldFailOn == "GetTicketsByUser" {
		return nil, m.errorToReturn
	}
	tickets, exists := m.userTickets[userID]
	if !exists || len(tickets) == 0 {
		return nil, errors.New("no tickets found for user")
	}
	return tickets, nil
}

func setupMockDB() *MockTicketDB {
	// Setup our mock DB with some test data
	mockDB := NewMockTicketDB()

	// Create a sample ticket
	ticket := models.Ticket{
		TicketID:        "ticket1",
		OrderID:         "order1",
		SeatID:          "seat1",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		QRCode:          []byte("qrcode"),
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
		CheckedIn:       false,
	}

	mockDB.tickets[ticket.TicketID] = &ticket
	mockDB.orderTickets[ticket.OrderID] = append(mockDB.orderTickets[ticket.OrderID], ticket)

	return mockDB
}

func setupService() (*tickets.TicketService, *MockTicketDB) {
	mockDB := setupMockDB()
	service := tickets.NewTicketService(mockDB)

	// Set QR_SECRET_KEY environment variable for testing
	os.Setenv("QR_SECRET_KEY", "test-secret-key")

	return service, mockDB
}

func TestPlaceTicket(t *testing.T) {
	service, _ := setupService()

	newTicket := models.Ticket{
		TicketID:        "ticket2",
		OrderID:         "order1",
		SeatID:          "seat2",
		SeatLabel:       "A2",
		Colour:          "red",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 100.0,
	}

	err := service.PlaceTicket(newTicket)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify ticket was created
	retrievedTicket, err := service.GetTicket("ticket2")
	if err != nil {
		t.Errorf("Expected to retrieve ticket, got error: %v", err)
	}

	if retrievedTicket.TicketID != "ticket2" {
		t.Errorf("Expected ticket ID to be 'ticket2', got %s", retrievedTicket.TicketID)
	}

	// Verify QR code was generated
	if len(retrievedTicket.QRCode) == 0 {
		t.Error("Expected QR code to be generated")
	}
}

func TestGetTicket(t *testing.T) {
	service, _ := setupService()

	// Test retrieving an existing ticket
	ticket, err := service.GetTicket("ticket1")
	if err != nil {
		t.Errorf("Expected to retrieve ticket, got error: %v", err)
	}

	if ticket.TicketID != "ticket1" {
		t.Errorf("Expected ticket ID to be 'ticket1', got %s", ticket.TicketID)
	}

	// Test retrieving a non-existent ticket
	_, err = service.GetTicket("nonexistent")
	if err == nil {
		t.Error("Expected error when retrieving non-existent ticket, got nil")
	}
}

func TestUpdateTicketService(t *testing.T) {
	service, mockDB := setupService()

	// Update ticket
	updateData := models.Ticket{
		SeatID:          "seat1-updated",
		SeatLabel:       "A1-updated",
		Colour:          "green",
		TierID:          "tier2",
		TierName:        "Regular",
		PriceAtPurchase: 50.0,
	}

	err := service.UpdateTicket("ticket1", updateData)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify ticket was updated
	updatedTicket, _ := mockDB.GetTicketByID("ticket1")

	if updatedTicket.SeatID != "seat1-updated" {
		t.Errorf("Expected seat ID to be updated to 'seat1-updated', got %s", updatedTicket.SeatID)
	}

	if updatedTicket.SeatLabel != "A1-updated" {
		t.Errorf("Expected seat label to be updated to 'A1-updated', got %s", updatedTicket.SeatLabel)
	}

	if updatedTicket.Colour != "green" {
		t.Errorf("Expected colour to be updated to 'green', got %s", updatedTicket.Colour)
	}

	// Test updating a non-existent ticket
	err = service.UpdateTicket("nonexistent", updateData)
	if err == nil {
		t.Error("Expected error when updating non-existent ticket, got nil")
	}
}

func TestCancelTicketService(t *testing.T) {
	service, mockDB := setupService()

	// Cancel a ticket
	err := service.CancelTicket("ticket1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify ticket was deleted
	_, err = mockDB.GetTicketByID("ticket1")
	if err == nil {
		t.Error("Expected ticket to be deleted, but it still exists")
	}

	// Test cancelling a non-existent ticket
	err = service.CancelTicket("nonexistent")
	if err == nil {
		t.Error("Expected error when cancelling non-existent ticket, got nil")
	}
}

func TestCheckin(t *testing.T) {
	service, mockDB := setupService()

	// Perform checkin
	success, err := service.Checkin("ticket1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if !success {
		t.Error("Expected checkin to be successful, got false")
	}

	// Verify ticket was checked in
	ticket, _ := mockDB.GetTicketByID("ticket1")
	if !ticket.CheckedIn {
		t.Error("Expected ticket to be checked in, but it was not")
	}

	// Test checking in a non-existent ticket
	_, err = service.Checkin("nonexistent")
	if err == nil {
		t.Error("Expected error when checking in non-existent ticket, got nil")
	}
}

func TestGetTicketsByOrderService(t *testing.T) {
	service, _ := setupService()

	// Test retrieving tickets for an existing order
	tickets, err := service.GetTicketsByOrder("order1")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(tickets) == 0 {
		t.Error("Expected to retrieve tickets, got empty slice")
	}

	// Test retrieving tickets for a non-existent order
	_, err = service.GetTicketsByOrder("nonexistent")
	if err == nil {
		t.Error("Expected error when retrieving tickets for non-existent order, got nil")
	}
}
