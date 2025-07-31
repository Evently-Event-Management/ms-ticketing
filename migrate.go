package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// --- Models ---

type User struct {
	bun.BaseModel `bun:"table:users"`
	ID            string    `bun:"id,pk"`
	Email         string    `bun:"email,unique,notnull"`
	FullName      string    `bun:"full_name,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp"`
}

type Event struct {
	bun.BaseModel `bun:"table:events"`
	ID            string    `bun:"id,pk"`
	Name          string    `bun:"name,notnull"`
	Description   string    `bun:"description,nullzero"`
	StartDate     time.Time `bun:"start_date,notnull"`
	EndDate       time.Time `bun:"end_date,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp"`
}

type Seat struct {
	bun.BaseModel `bun:"table:seats"`
	ID            string `bun:"id,pk"`
	EventID       string `bun:"event_id,notnull"`
	Label         string `bun:"label,notnull"` // e.g., "A1", "B10"
	Event         *Event `bun:"rel:belongs-to,join:event_id=id"`
}

type Promo struct {
	bun.BaseModel `bun:"table:promos"`
	ID            string    `bun:"id,pk"`
	Code          string    `bun:"code,unique,notnull"`
	Description   string    `bun:"description,nullzero"`
	Discount      float64   `bun:"discount,notnull"`
	ValidFrom     time.Time `bun:"valid_from,notnull"`
	ValidUntil    time.Time `bun:"valid_until,notnull"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp"`
}

type Order struct {
	bun.BaseModel   `bun:"table:orders"`
	ID              string    `bun:"id,pk"`
	EventID         string    `bun:"event_id,notnull"`
	UserID          string    `bun:"user_id,notnull"`
	SeatID          string    `bun:"seat_id,notnull"`
	Status          string    `bun:"status,notnull"` // pending, completed, cancelled
	PromoCode       string    `bun:"promo_code,nullzero"`
	CreatedAt       time.Time `bun:"created_at,notnull,default:current_timestamp"`
	UpdatedAt       time.Time `bun:"updated_at,nullzero"`
	DiscountApplied bool      `bun:"discount_applied,nullzero"`

	// Relations
	Event *Event `bun:"rel:belongs-to,join:event_id=id"`
	User  *User  `bun:"rel:belongs-to,join:user_id=id"`
	Seat  *Seat  `bun:"rel:belongs-to,join:seat_id=id"`
	Promo *Promo `bun:"rel:belongs-to,join:promo_code=code"`
}

type Ticket struct {
	bun.BaseModel `bun:"table:tickets"`
	ID            string    `bun:"id,pk"`
	OrderID       string    `bun:"order_id,notnull"`
	EventID       string    `bun:"event_id,notnull"`
	SeatID        string    `bun:"seat_id,notnull"`
	UserID        string    `bun:"user_id,notnull"`
	IssuedAt      time.Time `bun:"issued_at,notnull,default:current_timestamp"`

	Order *Order `bun:"rel:belongs-to,join:order_id=id"`
	Seat  *Seat  `bun:"rel:belongs-to,join:seat_id=id"`
	Event *Event `bun:"rel:belongs-to,join:event_id=id"`
	User  *User  `bun:"rel:belongs-to,join:user_id=id"`
}

// --- Main ---

func main() {
	ctx := context.Background()

	dsn := "postgres://eventuser:eventpass@localhost:5432/eventdb?sslmode=disable"
	connector := pgdriver.NewConnector(pgdriver.WithDSN(dsn))
	sqldb := sql.OpenDB(connector)
	defer sqldb.Close()

	if err := sqldb.PingContext(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	db := bun.NewDB(sqldb, pgdialect.New())

	// Drop tables in reverse dependency order
	log.Println("Dropping tables...")
	_ = dropTables(ctx, db)

	// Create tables
	log.Println("Creating tables...")
	_ = createTables(ctx, db)

	// Seed sample data
	log.Println("Seeding sample data...")
	_ = seedData(ctx, db)

	log.Println("✅ Done.")
}

// --- Helper Functions ---

func dropTables(ctx context.Context, db *bun.DB) error {
	tables := []interface{}{(*Ticket)(nil), (*Order)(nil), (*Promo)(nil), (*Seat)(nil), (*User)(nil), (*Event)(nil)}
	for _, m := range tables {
		_, _ = db.NewDropTable().Model(m).IfExists().Cascade().Exec(ctx)
	}
	return nil
}

func createTables(ctx context.Context, db *bun.DB) error {
	tables := []interface{}{(*User)(nil), (*Event)(nil), (*Seat)(nil), (*Promo)(nil), (*Order)(nil), (*Ticket)(nil)}
	for _, m := range tables {
		_, err := db.NewCreateTable().Model(m).IfNotExists().Exec(ctx)
		if err != nil {
			log.Fatalf("❌ Failed to create table for %T: %v", m, err)
		}
	}
	return nil
}

func seedData(ctx context.Context, db *bun.DB) error {
	// Users
	users := []User{
		{ID: "user001", Email: "alice@example.com", FullName: "Alice Wonderland", CreatedAt: time.Now()},
		{ID: "user002", Email: "bob@example.com", FullName: "Bob Builder", CreatedAt: time.Now()},
	}
	_, _ = db.NewInsert().Model(&users).Exec(ctx)

	// Event
	event := Event{
		ID:          "event001",
		Name:        "Summer Fest 2025",
		Description: "Annual summer music festival.",
		StartDate:   time.Now().AddDate(0, 1, 0),
		EndDate:     time.Now().AddDate(0, 1, 3),
		CreatedAt:   time.Now(),
	}
	_, _ = db.NewInsert().Model(&event).Exec(ctx)

	// Seats
	seats := []Seat{
		{ID: "seatA1", EventID: "event001", Label: "A1"},
		{ID: "seatA2", EventID: "event001", Label: "A2"},
	}
	_, _ = db.NewInsert().Model(&seats).Exec(ctx)

	// Promo
	promo := Promo{
		ID:          "promo001",
		Code:        "SUMMER20",
		Description: "20% off summer tickets",
		Discount:    20.0,
		ValidFrom:   time.Now(),
		ValidUntil:  time.Now().AddDate(0, 2, 0),
		CreatedAt:   time.Now(),
	}
	_, _ = db.NewInsert().Model(&promo).Exec(ctx)

	// Order
	order := Order{
		ID:              "order123",
		EventID:         "event001",
		UserID:          "user001",
		SeatID:          "seatA1",
		Status:          "completed",
		CreatedAt:       time.Now(),
		DiscountApplied: true,
		PromoCode:       "SUMMER20",
	}
	_, _ = db.NewInsert().Model(&order).Exec(ctx)

	// Ticket
	ticket := Ticket{
		ID:       "ticket123",
		OrderID:  "order123",
		EventID:  "event001",
		SeatID:   "seatA1",
		UserID:   "user001",
		IssuedAt: time.Now(),
	}
	_, _ = db.NewInsert().Model(&ticket).Exec(ctx)

	return nil
}
