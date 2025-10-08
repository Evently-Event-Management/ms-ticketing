package models

import (
	"github.com/google/uuid"
)

// SeatStatus represents the status of a seat
type SeatStatus string

// Seat status constants matching the Java enum ReadModelSeatStatus
const (
	SeatStatusAvailable SeatStatus = "AVAILABLE"
	SeatStatusReserved  SeatStatus = "RESERVED"
	SeatStatusBooked    SeatStatus = "BOOKED"
	SeatStatusLocked    SeatStatus = "LOCKED"
)

// SeatStatusChangeEventDto matches the Java consumer's expected format
// This is used for publishing seat status events to Kafka
type SeatStatusChangeEventDto struct {
	SessionID uuid.UUID   `json:"session_id"`
	SeatIDs   []uuid.UUID `json:"seat_ids"`
	Status    SeatStatus  `json:"status"`
}

// NewSeatStatusChangeEventDto creates a new DTO for seat status events
// It ensures the UUIDs are properly formatted for the Java consumer
func NewSeatStatusChangeEventDto(sessionID string, seatIDs []string, status SeatStatus) (SeatStatusChangeEventDto, error) {
	// Parse the session ID as UUID
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return SeatStatusChangeEventDto{}, err
	}

	// Parse each seat ID as UUID
	seatUUIDs := make([]uuid.UUID, 0, len(seatIDs))
	for _, seatID := range seatIDs {
		seatUUID, err := uuid.Parse(seatID)
		if err != nil {
			return SeatStatusChangeEventDto{}, err
		}
		seatUUIDs = append(seatUUIDs, seatUUID)
	}

	return SeatStatusChangeEventDto{
		SessionID: sessionUUID,
		SeatIDs:   seatUUIDs,
		Status:    status,
	}, nil
}
