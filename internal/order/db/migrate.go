package db

import (
	"context"
	"log"
	"time"

	"github.com/uptrace/bun"
)

func Migrate(db *bun.DB) {
	ctx := context.Background()

	// Drop existing table if exists (be cautious in production!)
	_, err := db.NewDropTable().Model((*Order)(nil)).IfExists().Cascade().Exec(ctx)
	if err != nil {
		log.Fatalf("drop table failed: %v", err)
	}

	// Create orders table
	_, err = db.NewCreateTable().Model((*Order)(nil)).IfNotExists().Exec(ctx)
	if err != nil {
		log.Fatalf("create table failed: %v", err)
	}

	log.Println("✅ orders table created")

	// Insert sample order with timestamps
	sample := &Order{
		ID:        "order123",
		EventID:   "event001",
		UserID:    "user001",
		SeatID:    "A1",
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	_, err = db.NewInsert().Model(sample).Exec(ctx)
	if err != nil {
		log.Fatalf("seed insert failed: %v", err)
	}

	log.Println("✅ sample order seeded")
}
