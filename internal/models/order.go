package models

import (
	"time"

	"github.com/uptrace/bun"
)

type OrderRequest struct {
	SessionID  string   `json:"session_id"`
	EventID    string   `json:"event_id"`
	SeatIDs    []string `json:"seat_ids"`
	DiscountID string   `json:"discount_id"`
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

type DiscountType string

const (
	PERCENTAGE       DiscountType = "PERCENTAGE"
	FLAT_OFF         DiscountType = "FLAT_OFF"
	BUY_N_GET_N_FREE DiscountType = "BUY_N_GET_N_FREE"
)

type DiscountParameters struct {
	Type        DiscountType `json:"type"`
	Percentage  *float64     `json:"percentage,omitempty"`
	Amount      *float64     `json:"amount,omitempty"`
	Currency    *string      `json:"currency,omitempty"`
	BuyQuantity *int         `json:"buyQuantity,omitempty"`
	GetQuantity *int         `json:"getQuantity,omitempty"`
	MinSpend    *float64     `json:"minSpend,omitempty"`
	MaxDiscount *float64     `json:"maxDiscount,omitempty"`
}

type Discount struct {
	ID                   string             `json:"id"`
	Code                 string             `json:"code"`
	Parameters           DiscountParameters `json:"parameters"`
	ActiveFrom           time.Time          `json:"activeFrom"`
	ExpiresAt            time.Time          `json:"expiresAt"`
	MaxUsage             int                `json:"maxUsage"`
	CurrentUsage         int                `json:"currentUsage"`
	ApplicableTiers      []Tier             `json:"applicableTiers"`
	ApplicableSessionIds []string           `json:"applicableSessionIds"`
	Public               bool               `json:"public"`
	Active               bool               `json:"active"`
}

type OrderDetailsDTO struct {
	Seats    []SeatDetails `json:"seats"`
	Discount *Discount     `json:"discount,omitempty"`
}
