package models

import (
	"github.com/uptrace/bun"
	"time"
)

type Ticket struct {
	bun.BaseModel `bun:"table:tickets"`

	ID          string    `bun:"id,pk" json:"id"`
	EventID     string    `bun:"event_id,notnull" json:"event_id"`
	UserID      string    `bun:"user_id,notnull" json:"user_id"`
	SeatID      string    `bun:"seat_id,array,notnull" json:"seat_id"`
	Status      string    `bun:"status,notnull" json:"status"`
	CheckedIn   bool      `bun:"checked_in,nullzero" json:"checked_in"`
	CheckInTime time.Time `bun:"check_in_time,nullzero" json:"check_in_time"`
	CreatedAt   time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt   time.Time `bun:"updated_at,nullzero" json:"updated_at"`
}
