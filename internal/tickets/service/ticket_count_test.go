package tickets_test

import (
	"errors"
	"ms-ticketing/internal/models"
	tickets "ms-ticketing/internal/tickets/service"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockTicketCountDB is a mock implementation of the db.TicketCountDB interface
type MockTicketCountDB struct {
	mock.Mock
}

func (m *MockTicketCountDB) IncrementTicketCount(eventID string, sessionID string, timestamp time.Time) error {
	args := m.Called(eventID, sessionID, timestamp)
	return args.Error(0)
}

func (m *MockTicketCountDB) GetTicketCountsForEvent(eventID string) ([]models.TicketCount, error) {
	args := m.Called(eventID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TicketCount), args.Error(1)
}

func (m *MockTicketCountDB) GetTicketCountsForSession(sessionID string) ([]models.TicketCount, error) {
	args := m.Called(sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.TicketCount), args.Error(1)
}

func TestIncrementTicketCount(t *testing.T) {
	// Set up mocks
	mockDB := new(MockTicketCountDB)

	// Create service with mocks
	ticketCountSvc := &tickets.TicketCountService{
		DB: mockDB,
	}

	// Test case: Successfully increment ticket count
	eventID := "event123"
	sessionID := "session456"
	timestamp := time.Now()

	// Set up expectation
	mockDB.On("IncrementTicketCount", eventID, sessionID, mock.Anything).Return(nil)

	// Execute test
	err := ticketCountSvc.IncrementTicketCount(eventID, sessionID, timestamp)

	// Assertions
	assert.NoError(t, err)
	mockDB.AssertExpectations(t)
}

func TestGetTicketCountsForEvent(t *testing.T) {
	// Set up mocks
	mockDB := new(MockTicketCountDB)

	// Create service with mocks
	ticketCountSvc := &tickets.TicketCountService{
		DB: mockDB,
	}

	// Test case: Successfully get ticket counts for event
	eventID := "event123"
	expectedCounts := []models.TicketCount{
		{
			EventID:   eventID,
			SessionID: "session1",
			Count:     10,
			Date:      time.Now().Truncate(24 * time.Hour),
		},
		{
			EventID:   eventID,
			SessionID: "session2",
			Count:     20,
			Date:      time.Now().Truncate(24 * time.Hour),
		},
	}

	// Set up expectation
	mockDB.On("GetTicketCountsForEvent", eventID).Return(expectedCounts, nil)

	// Execute test
	counts, err := ticketCountSvc.GetTicketCountsForEvent(eventID)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, 2, len(counts))
	assert.Equal(t, expectedCounts[0].SessionID, counts[0].SessionID)
	assert.Equal(t, expectedCounts[0].Count, counts[0].Count)
	assert.Equal(t, expectedCounts[1].SessionID, counts[1].SessionID)
	assert.Equal(t, expectedCounts[1].Count, counts[1].Count)

	mockDB.AssertExpectations(t)

	// Test case: Error getting ticket counts
	mockDB.On("GetTicketCountsForEvent", "non-existent").Return(nil, errors.New("event not found"))

	// Execute test
	counts, err = ticketCountSvc.GetTicketCountsForEvent("non-existent")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, counts)

	mockDB.AssertExpectations(t)
}

func TestGetTicketCountsForSession(t *testing.T) {
	// Set up mocks
	mockDB := new(MockTicketCountDB)

	// Create service with mocks
	ticketCountSvc := &tickets.TicketCountService{
		DB: mockDB,
	}

	// Test case: Successfully get ticket counts for session
	sessionID := "session123"
	expectedCounts := []models.TicketCount{
		{
			EventID:   "event1",
			SessionID: sessionID,
			Count:     10,
			Date:      time.Now().Truncate(24 * time.Hour),
		},
		{
			EventID:   "event1",
			SessionID: sessionID,
			Count:     15,
			Date:      time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour),
		},
	}

	// Set up expectation
	mockDB.On("GetTicketCountsForSession", sessionID).Return(expectedCounts, nil)

	// Execute test
	counts, err := ticketCountSvc.GetTicketCountsForSession(sessionID)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, 2, len(counts))
	assert.Equal(t, expectedCounts[0].Count, counts[0].Count)
	assert.Equal(t, expectedCounts[1].Count, counts[1].Count)

	mockDB.AssertExpectations(t)

	// Test case: Error getting ticket counts
	mockDB.On("GetTicketCountsForSession", "non-existent").Return(nil, errors.New("session not found"))

	// Execute test
	counts, err = ticketCountSvc.GetTicketCountsForSession("non-existent")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, counts)

	mockDB.AssertExpectations(t)
}