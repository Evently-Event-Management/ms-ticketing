package db

import (
	"github.com/uptrace/bun"
	"time"
)

type Order struct {
	bun.BaseModel `bun:"table:orders"`

	ID              string    `bun:"id,pk" json:"id"`
	EventID         string    `bun:"event_id,notnull" json:"event_id"`
	UserID          string    `bun:"user_id,notnull" json:"user_id"`
	SeatIDs         []string  `bun:"seat_ids,array,notnull" json:"seat_ids"` // PostgreSQL array
	Status          string    `bun:"status,notnull" json:"status"`
	CreatedAt       time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt       time.Time `bun:"updated_at,nullzero" json:"updated_at"`
	PromoCode       string    `bun:"promo_code,nullzero" json:"promo_code"`
	DiscountApplied bool      `bun:"discount_applied,nullzero" json:"discount_applied"`
}
