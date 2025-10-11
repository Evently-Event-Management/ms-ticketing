package tickets

import (
	"ms-ticketing/internal/models"
	"time"
)

// GetTotalTicketsCount returns the total count of tickets
func (s *TicketService) GetTotalTicketsCount() (int, error) {
	return s.DB.GetTotalTicketsCount()
}

// TicketCountDBLayer represents the interface for ticket count database operations
type TicketCountDBLayer interface {
	IncrementTicketCount(eventID string, sessionID string, timestamp time.Time) error
	GetTicketCountsForEvent(eventID string) ([]models.TicketCount, error)
	GetTicketCountsForSession(sessionID string) ([]models.TicketCount, error)
}

// TicketCountService handles the ticket count operations
type TicketCountService struct {
	DB TicketCountDBLayer
}

// IncrementTicketCount increases the count of tickets for an event/session on a given date
func (s *TicketCountService) IncrementTicketCount(eventID string, sessionID string, timestamp time.Time) error {
	return s.DB.IncrementTicketCount(eventID, sessionID, timestamp)
}

// GetTicketCountsForEvent returns all ticket counts for a specific event
func (s *TicketCountService) GetTicketCountsForEvent(eventID string) ([]models.TicketCount, error) {
	return s.DB.GetTicketCountsForEvent(eventID)
}

// GetTicketCountsForSession returns all ticket counts for a specific session
func (s *TicketCountService) GetTicketCountsForSession(sessionID string) ([]models.TicketCount, error) {
	return s.DB.GetTicketCountsForSession(sessionID)
}
