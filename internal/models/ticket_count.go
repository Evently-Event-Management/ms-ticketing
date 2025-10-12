package models

import (
	"time"

	"github.com/uptrace/bun"
)

// TicketCount represents a daily count of tickets issued for a specific event/session
type TicketCount struct {
	bun.BaseModel `bun:"table:ticket_counts"`

	ID        int64     `bun:"id,pk,autoincrement"`
	EventID   string    `bun:"event_id"`
	SessionID string    `bun:"session_id"`
	Count     int       `bun:"count"`
	Date      time.Time `bun:"date"`
}