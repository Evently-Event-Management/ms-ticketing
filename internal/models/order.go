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

	OrderID        string    `bun:"order_id,pk"`
	UserID         string    `bun:"user_id"`
	EventID        string    `bun:"event_id"` // Added event_id field
	SessionID      string    `bun:"session_id"`
	Status         string    `bun:"status"`
	SubTotal       float64   `bun:"subtotal"`               // Price before discount
	DiscountID     string    `bun:"discount_id,nullzero"`   // ID of applied discount code
	DiscountCode   string    `bun:"discount_code,nullzero"` // Code of applied discount
	DiscountAmount float64   `bun:"discount_amount"`        // Amount of discount applied
	Price          float64   `bun:"price"`                  // Final price after discount
	CreatedAt      time.Time `bun:"created_at"`
}

// OrderWithSeats extends the Order model with seat information
// This is not stored in the database but used for API responses
// DEPRECATED: Use OrderWithTickets instead for streaming events
type OrderWithSeats struct {
	Order
	SeatIDs []string `json:"seat_ids"`
}

// TicketForStreaming is a lightweight version of Ticket without QR code
// Used for event streaming to reduce payload size
type TicketForStreaming struct {
	TicketID        string    `json:"ticket_id"`
	OrderID         string    `json:"order_id"`
	SeatID          string    `json:"seat_id"`
	SeatLabel       string    `json:"seat_label"`
	Colour          string    `json:"colour"`
	TierID          string    `json:"tier_id"`
	TierName        string    `json:"tier_name"`
	PriceAtPurchase float64   `json:"price_at_purchase"`
	IssuedAt        time.Time `json:"issued_at"`
	CheckedIn       bool      `json:"checked_in"`
	CheckedInTime   time.Time `json:"checked_in_time,omitempty"`
}

// TicketWithQRCode includes the QR code for complete ticket information
// Used when QR code data is needed (e.g., for user ticket retrieval)
type TicketWithQRCode struct {
	TicketID        string    `json:"ticket_id"`
	OrderID         string    `json:"order_id"`
	SeatID          string    `json:"seat_id"`
	SeatLabel       string    `json:"seat_label"`
	Colour          string    `json:"colour"`
	TierID          string    `json:"tier_id"`
	TierName        string    `json:"tier_name"`
	QRCode          []byte    `json:"qr_code"`
	PriceAtPurchase float64   `json:"price_at_purchase"`
	IssuedAt        time.Time `json:"issued_at"`
	CheckedIn       bool      `json:"checked_in"`
	CheckedInTime   time.Time `json:"checked_in_time,omitempty"`
}

// OrderWithTickets extends the Order model with full ticket information
// This denormalized structure is used for streaming order events
type OrderWithTickets struct {
	Order
	Tickets []TicketForStreaming `json:"tickets"`
}

// OrderWithTicketsAndQR extends the Order model with complete ticket information including QR codes
// Used when full ticket details including QR codes are needed
type OrderWithTicketsAndQR struct {
	Order
	Tickets []TicketWithQRCode `json:"tickets"`
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
	ActiveFrom           *time.Time         `json:"activeFrom"`
	ExpiresAt            *time.Time         `json:"expiresAt"`
	MaxUsage             *int               `json:"maxUsage"`
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
