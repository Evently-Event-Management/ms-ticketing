package models

import (
	"time"

	"github.com/uptrace/bun"
)

type OrderRequest struct {
	SessionID string   `json:"session_id"`
	EventID   string   `json:"event_id"`
	SeatIDs   []string `json:"seat_ids"`
}

type Order struct {
	bun.BaseModel `bun:"table:orders"`

	OrderID   string    `bun:"order_id,pk"`
	UserID    string    `bun:"user_id"`
	SessionID string    `bun:"session_id"`
	SeatIDs   []string  `bun:"seat_ids,array"`
	Status    string    `bun:"status"`
	Price     float64   `bun:"price"`
	CreatedAt time.Time `bun:"created_at"`
}

type Tier struct {
	ID    string  `bun:"tier_id" json:"id"`
	Name  string  `bun:"tier_name" json:"name"`
	Price float64 `bun:"price" json:"price"`
	Color string  `bun:"color" json:"color"`
}

type SeatDetails struct {
	SeatID string `bun:"seat_id" json:"seatId"`
	Label  string `bun:"seat_label" json:"label"`
	Tier   Tier   `bun:"tier" json:"tier"`
}

type OrderResponse struct {
	OrderID   string   `json:"order_id"`
	SessionID string   `json:"session_id"`
	SeatIDs   []string `json:"seat_ids"`
	UserID    string   `json:"user_id"`
}
