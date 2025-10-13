//go:build migrate
// +build migrate

package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	// Use environment variable if available, otherwise fallback to default
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://ticketly:ticketly@localhost:5432/order_service?sslmode=disable"
		fmt.Println("⚠️ Using default DSN. Set POSTGRES_DSN environment variable to override.")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("❌ Failed to open DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("❌ Failed to ping DB: %v", err)
	}
	fmt.Println("✅ Connected to DB")

	if err := migrate(db); err != nil {
		log.Fatalf("❌ Migration failed: %v", err)
	}
	fmt.Println("✅ Migration complete")

	if err := seed(db); err != nil {
		log.Fatalf("❌ Seeding failed: %v", err)
	}
	fmt.Println("✅ Seeding complete")

	if err := printData(db); err != nil {
		log.Fatalf("❌ Print failed: %v", err)
	}
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

	DROP TABLE IF EXISTS tickets;
	DROP TABLE IF EXISTS orders;

	CREATE TABLE orders (
		order_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id UUID NOT NULL,
		event_id UUID,
		organization_id UUID,
		session_id UUID NOT NULL,
		status   TEXT NOT NULL,
		subtotal NUMERIC(10,2) NOT NULL,
		discount_id UUID,
		discount_code TEXT,
		discount_amount NUMERIC(10,2) DEFAULT 0,
		price    NUMERIC(10,2) NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		payment_intent_id TEXT
	);

	CREATE TABLE tickets (
		ticket_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		order_id  UUID NOT NULL REFERENCES orders(order_id) ON DELETE CASCADE,
		seat_id   UUID NOT NULL,
		seat_label TEXT NOT NULL,
		colour     TEXT NOT NULL,
		tier_id    UUID NOT NULL,
		tier_name  TEXT NOT NULL,
		qr_code    BYTEA,
		price_at_purchase NUMERIC(10,2) NOT NULL,
		issued_at TIMESTAMP NOT NULL DEFAULT NOW(),
		checked_in BOOLEAN NOT NULL DEFAULT FALSE,
		checked_in_time TIMESTAMP
	);
	`
	_, err := db.Exec(schema)
	return err
}

func seed(db *sql.DB) error {
	now := time.Now()

	// Sample UUIDs
	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	eventID := "cccccccc-cccc-cccc-cccc-cccccccccccc" // Added eventID
	sessionID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	organizationID := "dddddddd-dddd-dddd-dddd-dddddddddddd" // Added organizationID
	seat1 := "11111111-1111-1111-1111-111111111111"
	seat2 := "22222222-2222-2222-2222-222222222222"
	tier1 := "33333333-3333-3333-3333-333333333333"
	tier2 := "44444444-4444-4444-4444-444444444444"
	discountID := "55555555-5555-5555-5555-555555555555"

	// Insert sample Order
	var orderID string
	err := db.QueryRow(
		`INSERT INTO orders (user_id, event_id, session_id, organization_id, status, subtotal, discount_id, discount_code, discount_amount, price, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING order_id`,
		userID,
		eventID,
		sessionID,
		organizationID,
		"completed",
		300.00,
		discountID,
		"WELCOME20",
		50.00,
		250.00,
		now,
	).Scan(&orderID)
	if err != nil {
		return err
	}

	// Insert sample Tickets
	_, err = db.Exec(
		`INSERT INTO tickets (order_id, seat_id, seat_label, colour, tier_id, tier_name, qr_code, price_at_purchase, issued_at) 
		 VALUES
		 ($1, $2, 'A1', 'Red', $3, 'VIP', decode('48656c6c6f20515221', 'hex'), 150.00, $4),
		 ($1, $5, 'A2', 'Blue', $6, 'Standard', decode('48656c6c6f20515221', 'hex'), 100.00, $4)`,
		orderID, // $1
		seat1,   // $2
		tier1,   // $3
		now,     // $4
		seat2,   // $5
		tier2,   // $6
	)
	return err
}

func printData(db *sql.DB) error {
	fmt.Println("\n📦 Orders:")
	rows, err := db.Query(`SELECT order_id, user_id, event_id, session_id, status, price, created_at FROM orders`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, userID, eventID, sessionID string
		var status string
		var price float64
		var created time.Time
		if err := rows.Scan(&id, &userID, &eventID, &sessionID, &organizationID, &status, &price, &created); err != nil {
			return err
		}
		fmt.Printf("- Order %s | User %s | Event %s | Session %s | Organization %s | Status: %s | Price: %.2f | Created: %s\n",
			id, userID, eventID, sessionID, organizationID, status, price, created)
	}

	fmt.Println("\n🎟 Tickets:")
	trows, err := db.Query(`SELECT ticket_id, order_id, seat_id, seat_label, colour, tier_name, price_at_purchase FROM tickets`)
	if err != nil {
		return err
	}
	defer trows.Close()

	for trows.Next() {
		var tid, oid, sid, label, colour, tier string
		var price float64
		if err := trows.Scan(&tid, &oid, &sid, &label, &colour, &tier, &price); err != nil {
			return err
		}
		fmt.Printf("- Ticket %s | Order: %s | Seat: %s (%s, %s) | Tier: %s | Price: %.2f\n",
			tid, oid, sid, label, colour, tier, price)
	}

	return nil
}
