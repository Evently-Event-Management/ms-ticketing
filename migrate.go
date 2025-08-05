package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := "appuser:secretpass@tcp(localhost:3307)/appdb?parseTime=true"

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("❌ Failed to open DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("❌ Failed to ping DB: %v", err)
	}

	fmt.Println("✅ Connected to MySQL!")

	// Run migrations
	if err := migrate(db); err != nil {
		log.Fatalf("❌ Migration failed: %v", err)
	}

	// Seed sample data
	if err := seed(db); err != nil {
		log.Fatalf("❌ Seeding failed: %v", err)
	}

	fmt.Println("✅ Migration and seeding completed!")
}

func migrate(db *sql.DB) error {
	queries := []string{
		`DROP TABLE IF EXISTS tickets`,
		`DROP TABLE IF EXISTS orders`,
		`DROP TABLE IF EXISTS promos`,
		`DROP TABLE IF EXISTS seats`,
		`DROP TABLE IF EXISTS users`,
		`DROP TABLE IF EXISTS events`,

		`CREATE TABLE users (
			id VARCHAR(50) PRIMARY KEY,
			email VARCHAR(100) NOT NULL UNIQUE,
			full_name VARCHAR(100) NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE events (
			id VARCHAR(50) PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description TEXT,
			start_date DATETIME NOT NULL,
			end_date DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE seats (
			id VARCHAR(50) PRIMARY KEY,
			event_id VARCHAR(50) NOT NULL,
			label VARCHAR(20) NOT NULL,
			FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE CASCADE
		)`,

		`CREATE TABLE promos (
			id VARCHAR(50) PRIMARY KEY,
			code VARCHAR(50) NOT NULL UNIQUE,
			description TEXT,
			discount FLOAT NOT NULL,
			valid_from DATETIME NOT NULL,
			valid_until DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE orders (
			id VARCHAR(50) PRIMARY KEY,
			event_id VARCHAR(50) NOT NULL,
			user_id VARCHAR(50) NOT NULL,
			seat_id VARCHAR(50) NOT NULL,
			status VARCHAR(20) NOT NULL,
			promo_code VARCHAR(50),
			discount_applied BOOLEAN DEFAULT FALSE,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NULL,
			FOREIGN KEY (event_id) REFERENCES events(id),
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (seat_id) REFERENCES seats(id),
			FOREIGN KEY (promo_code) REFERENCES promos(code)
		)`,

		`CREATE TABLE tickets (
			id VARCHAR(50) PRIMARY KEY,
			order_id VARCHAR(50) NOT NULL,
			event_id VARCHAR(50) NOT NULL,
			seat_id VARCHAR(50) NOT NULL,
			user_id VARCHAR(50) NOT NULL,
			issued_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (order_id) REFERENCES orders(id),
			FOREIGN KEY (event_id) REFERENCES events(id),
			FOREIGN KEY (seat_id) REFERENCES seats(id),
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("query failed: %v, error: %w", q, err)
		}
	}

	return nil
}

func seed(db *sql.DB) error {
	now := time.Now().Format("2006-01-02 15:04:05")

	// Insert Users
	users := []struct {
		id       string
		email    string
		fullName string
	}{
		{"user001", "alice@example.com", "Alice Wonderland"},
		{"user002", "bob@example.com", "Bob Builder"},
	}
	for _, u := range users {
		_, err := db.Exec(`INSERT INTO users (id, email, full_name, created_at) VALUES (?, ?, ?, ?)`,
			u.id, u.email, u.fullName, now)
		if err != nil {
			return err
		}
	}

	// Insert Event
	_, err := db.Exec(`INSERT INTO events (id, name, description, start_date, end_date, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"event001",
		"Summer Fest 2025",
		"Annual summer music festival.",
		time.Now().AddDate(0, 1, 0).Format("2006-01-02 15:04:05"),
		time.Now().AddDate(0, 1, 3).Format("2006-01-02 15:04:05"),
		now)
	if err != nil {
		return err
	}

	// Insert Seats
	seats := []struct {
		id      string
		eventID string
		label   string
	}{
		{"seatA1", "event001", "A1"},
		{"seatA2", "event001", "A2"},
	}
	for _, s := range seats {
		_, err := db.Exec(`INSERT INTO seats (id, event_id, label) VALUES (?, ?, ?)`, s.id, s.eventID, s.label)
		if err != nil {
			return err
		}
	}

	// Insert Promo
	_, err = db.Exec(`INSERT INTO promos (id, code, description, discount, valid_from, valid_until, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"promo001",
		"SUMMER20",
		"20% off summer tickets",
		20.0,
		now,
		time.Now().AddDate(0, 2, 0).Format("2006-01-02 15:04:05"),
		now)
	if err != nil {
		return err
	}

	// Insert Order
	_, err = db.Exec(`INSERT INTO orders (id, event_id, user_id, seat_id, status, promo_code, discount_applied, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"order123",
		"event001",
		"user001",
		"seatA1",
		"completed",
		"SUMMER20",
		true,
		now)
	if err != nil {
		return err
	}

	// Insert Ticket
	_, err = db.Exec(`INSERT INTO tickets (id, order_id, event_id, seat_id, user_id, issued_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"ticket123",
		"order123",
		"event001",
		"seatA1",
		"user001",
		now)
	if err != nil {
		return err
	}

	return nil
}
