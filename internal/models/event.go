package models

import (
	"time"
)

type Event struct {
	ID          string    `bun:"id,pk"`
	Name        string    `bun:"name,notnull"`
	Description string    `bun:"description"`
	StartDate   time.Time `bun:"start_date,notnull"`
	EndDate     time.Time `bun:"end_date,notnull"`
	CreatedAt   time.Time `bun:"created_at,notnull"`
}
