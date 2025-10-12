package db

import (
	"context"
	"ms-ticketing/internal/models"
	"time"
)

// GetTotalTicketsCount returns the total count of tickets in the database
func (d *DB) GetTotalTicketsCount() (int, error) {
	count, err := d.Bun.NewSelect().
		Model((*models.Ticket)(nil)).
		Count(context.Background())

	return count, err
}

// IncrementTicketCount increments the ticket count for a given event, session, and date
func (d *DB) IncrementTicketCount(eventID string, sessionID string, timestamp time.Time) error {
	// Truncate timestamp to day precision
	date := timestamp.Truncate(24 * time.Hour)

	// Check if there's an existing count for this event/session/date
	var existingCount models.TicketCount
	err := d.Bun.NewSelect().
		Model(&existingCount).
		Where("event_id = ?", eventID).
		Where("session_id = ?", sessionID).
		Where("date = ?", date).
		Limit(1).
		Scan(context.Background())

	if err != nil {
		// If not found, create a new count record with count=1
		newCount := models.TicketCount{
			EventID:   eventID,
			SessionID: sessionID,
			Count:     1,
			Date:      date,
		}
		_, err = d.Bun.NewInsert().Model(&newCount).Exec(context.Background())
		return err
	}

	// If found, increment the count
	existingCount.Count++
	_, err = d.Bun.NewUpdate().
		Model(&existingCount).
		Column("count").
		Where("event_id = ?", eventID).
		Where("session_id = ?", sessionID).
		Where("date = ?", date).
		Exec(context.Background())

	return err
}

// GetTicketCountsForEvent returns all ticket counts for a specific event
func (d *DB) GetTicketCountsForEvent(eventID string) ([]models.TicketCount, error) {
	var counts []models.TicketCount
	err := d.Bun.NewSelect().
		Model(&counts).
		Where("event_id = ?", eventID).
		Order("session_id", "date").
		Scan(context.Background())

	return counts, err
}

// GetTicketCountsForSession returns all ticket counts for a specific session
func (d *DB) GetTicketCountsForSession(sessionID string) ([]models.TicketCount, error) {
	var counts []models.TicketCount
	err := d.Bun.NewSelect().
		Model(&counts).
		Where("session_id = ?", sessionID).
		Order("event_id", "date").
		Scan(context.Background())

	return counts, err
}
