package db

import (
	"context"
	"ms-ticketing/internal/models"
)

// GetTotalTicketsCount returns the total count of tickets in the database
func (d *DB) GetTotalTicketsCount() (int, error) {
	count, err := d.Bun.NewSelect().
		Model((*models.Ticket)(nil)).
		Count(context.Background())

	return count, err
}
