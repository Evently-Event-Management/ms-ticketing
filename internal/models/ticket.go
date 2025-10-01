package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Ticket struct {
	bun.BaseModel `bun:"table:tickets"`

	TicketID        string    `bun:"ticket_id,pk"`
	OrderID         string    `bun:"order_id"`
	SeatID          string    `bun:"seat_id"`
	SeatLabel       string    `bun:"seat_label"`
	Colour          string    `bun:"colour"`
	TierID          string    `bun:"tier_id"`
	TierName        string    `bun:"tier_name"`
	QRCode          []byte    `bun:"qr_code"`
	PriceAtPurchase float64   `bun:"price_at_purchase"`
	IssuedAt        time.Time `bun:"issued_at"`
	CheckedIn       bool      `bun:"checked_in"`
	CheckedInTime   time.Time `bun:"checked_in_time"`
}
