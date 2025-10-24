package tickets_test

import (
	"errors"
	"ms-ticketing/internal/models"
	tickets "ms-ticketing/internal/tickets/service"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTicketDBLayer is a mock implementation of the TicketDBLayer interface
type MockTicketDBLayer struct {
	mock.Mock
}

func (m *MockTicketDBLayer) CreateTicket(ticket models.Ticket) error {
	args := m.Called(ticket)
	return args.Error(0)
}

func (m *MockTicketDBLayer) GetTicketByID(ticketID string) (*models.Ticket, error) {
	args := m.Called(ticketID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Ticket), args.Error(1)
}

func (m *MockTicketDBLayer) UpdateTicket(ticket models.Ticket) error {
	args := m.Called(ticket)
	return args.Error(0)
}

func (m *MockTicketDBLayer) CancelTicket(ticketID string) error {
	args := m.Called(ticketID)
	return args.Error(0)
}

func (m *MockTicketDBLayer) GetTicketsByOrder(orderID string) ([]models.Ticket, error) {
	args := m.Called(orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Ticket), args.Error(1)
}

func (m *MockTicketDBLayer) GetTicketsByUser(userID string) ([]models.Ticket, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Ticket), args.Error(1)
}

func (m *MockTicketDBLayer) GetTotalTicketsCount() (int, error) {
	args := m.Called()
	return args.Int(0), args.Error(1)
}

// Tests start here
func TestCreateTicket(t *testing.T) {
	// Set up mock
	mockDB := new(MockTicketDBLayer)
	ticketSvc := &tickets.TicketService{
		DB: mockDB,
	}

	// Test case: Successfully create a ticket
	ticket := models.Ticket{
		TicketID:        uuid.New().String(),
		OrderID:         uuid.New().String(),
		SeatID:          "seat1",
		SeatLabel:       "A1",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 50.0,
		IssuedAt:        time.Now(),
	}

	// Set up expectation
	mockDB.On("CreateTicket", mock.MatchedBy(func(t models.Ticket) bool {
		return t.TicketID == ticket.TicketID
	})).Return(nil)

	// Execute test
	err := ticketSvc.PlaceTicket(ticket)

	// Assertions
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestGetTicket(t *testing.T) {
	// Set up mock
	mockDB := new(MockTicketDBLayer)
	ticketSvc := &tickets.TicketService{
		DB: mockDB,
	}

	// Test case 1: Ticket exists
	ticketID := uuid.New().String()
	testTicket := &models.Ticket{
		TicketID:        ticketID,
		OrderID:         uuid.New().String(),
		SeatID:          "seat1",
		SeatLabel:       "A1",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 50.0,
		IssuedAt:        time.Now(),
	}

	// Set up expectation
	mockDB.On("GetTicketByID", ticketID).Return(testTicket, nil)

	// Execute test
	result, err := ticketSvc.GetTicket(ticketID)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, testTicket.TicketID, result.TicketID)
	assert.Equal(t, testTicket.SeatID, result.SeatID)

	// Test case 2: Ticket doesn't exist
	mockDB.On("GetTicketByID", "non-existent").Return(nil, errors.New("ticket not found"))

	// Execute test
	result, err = ticketSvc.GetTicket("non-existent")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, result)

	mockDB.AssertExpectations(t)
}

func TestUpdateTicket(t *testing.T) {
	// Set up mock
	mockDB := new(MockTicketDBLayer)
	ticketSvc := &tickets.TicketService{
		DB: mockDB,
	}

	// Test case: Successfully update a ticket
	ticketID := uuid.New().String()
	ticket := models.Ticket{
		TicketID:        ticketID,
		OrderID:         uuid.New().String(),
		SeatID:          "seat1",
		SeatLabel:       "A1",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 50.0,
		IssuedAt:        time.Now(),
		CheckedIn:       true, // Updated field
	}

	// Set up expectations
	// First the service will look up the ticket
	mockDB.On("GetTicketByID", ticketID).Return(&ticket, nil)
	
	// Then it will update it
	mockDB.On("UpdateTicket", mock.MatchedBy(func(t models.Ticket) bool {
		return t.TicketID == ticket.TicketID && t.CheckedIn == true
	})).Return(nil)

	// Execute test
	err := ticketSvc.UpdateTicket(ticket.TicketID, ticket)

	// Assertions
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestCancelTicket(t *testing.T) {
	// Set up mock
	mockDB := new(MockTicketDBLayer)
	ticketSvc := &tickets.TicketService{
		DB: mockDB,
	}

	// Test case: Successfully cancel a ticket
	ticketID := uuid.New().String()
	
	// Set up expectations - first the service will try to get the ticket
	testTicket := &models.Ticket{
		TicketID: ticketID,
		OrderID:  "order123",
	}
	mockDB.On("GetTicketByID", ticketID).Return(testTicket, nil)
	
	// Then it will call cancel
	mockDB.On("CancelTicket", ticketID).Return(nil)

	// Execute test
	err := ticketSvc.CancelTicket(ticketID)

	// Assertions
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestGetTicketsByOrder(t *testing.T) {
	// Set up mock
	mockDB := new(MockTicketDBLayer)
	ticketSvc := &tickets.TicketService{
		DB: mockDB,
	}

	// Test case: Successfully get tickets by order
	orderID := uuid.New().String()
	tickets := []models.Ticket{
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

	// Set up expectation
	mockDB.On("GetTicketsByOrder", orderID).Return(tickets, nil)

	// Execute test
	result, err := ticketSvc.GetTicketsByOrder(orderID)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, tickets[0].TicketID, result[0].TicketID)
	assert.Equal(t, tickets[1].TicketID, result[1].TicketID)

	mockDB.AssertExpectations(t)
}

func TestGetTicketsByUser(t *testing.T) {
	// Set up mock
	mockDB := new(MockTicketDBLayer)
	ticketSvc := &tickets.TicketService{
		DB: mockDB,
	}

	// Test case: Successfully get tickets by user
	userID := "user123"
	tickets := []models.Ticket{
		{
			TicketID:        uuid.New().String(),
			OrderID:         uuid.New().String(),
			SeatID:          "seat1",
			SeatLabel:       "A1",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
		{
			TicketID:        uuid.New().String(),
			OrderID:         uuid.New().String(),
			SeatID:          "seat2",
			SeatLabel:       "A2",
			TierID:          "tier1",
			TierName:        "VIP",
			PriceAtPurchase: 50.0,
			IssuedAt:        time.Now(),
		},
	}

	// Set up expectation
	mockDB.On("GetTicketsByUser", userID).Return(tickets, nil)

	// Execute test
	result, err := ticketSvc.GetTicketsByUser(userID)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result))
	assert.Equal(t, tickets[0].TicketID, result[0].TicketID)
	assert.Equal(t, tickets[1].TicketID, result[1].TicketID)

	mockDB.AssertExpectations(t)
}

func TestGetTotalTicketsCount(t *testing.T) {
	// Set up mock
	mockDB := new(MockTicketDBLayer)
	ticketSvc := &tickets.TicketService{
		DB: mockDB,
	}

	// Test case: Successfully get total tickets count
	expectedCount := 100

	// Set up expectation
	mockDB.On("GetTotalTicketsCount").Return(expectedCount, nil)

	// Execute test
	count, err := ticketSvc.GetTotalTicketsCount()

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, expectedCount, count)

	mockDB.AssertExpectations(t)
}