package db

import (
	"context"
	"time"

	"ms-ticketing/internal/models"

	"github.com/uptrace/bun"
)

type DB struct {
	Bun *bun.DB
}

// GetTicketsByOrder implements tickets.DBLayer.
func (d *DB) GetTicketsByOrder(orderID string) ([]models.Ticket, error) {
	var tickets []models.Ticket
	err := d.Bun.NewSelect().
		Model(&tickets).
		Where("order_id = ?", orderID).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return tickets, nil
}

// ---------------- TICKETS ----------------

func (d *DB) GetTicketByID(ticketID string) (*models.Ticket, error) {
	var ticket models.Ticket
	err := d.Bun.NewSelect().
		Model(&ticket).
		Where("ticket_id = ?", ticketID).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

func (d *DB) UpdateTicket(ticket models.Ticket) error {
	_, err := d.Bun.NewUpdate().
		Model(&ticket).
		Column("order_id", "seat_id", "seat_label", "colour", "tier_id", "tier_name", "qr_code", "price_at_purchase", "issued_at").
		Where("ticket_id = ?", ticket.TicketID).
		Exec(context.Background())
	return err
}

func (d *DB) CancelTicket(ticketID string) error {
	_, err := d.Bun.NewDelete().
		Model((*models.Ticket)(nil)).
		Where("ticket_id = ?", ticketID).
		Exec(context.Background())
	return err
}

func (d *DB) CreateTicket(ticket models.Ticket) error {
	// Ensure issued_at is set if empty
	if ticket.IssuedAt.IsZero() {
		ticket.IssuedAt = time.Now()
	}
	_, err := d.Bun.NewInsert().
		Model(&ticket).
		Exec(context.Background())
	return err
}

func (d *DB) GetTicketsByUser(userID string) ([]models.Ticket, error) {
	var tickets []models.Ticket
	err := d.Bun.NewSelect().
		Model(&tickets).
		Where("user_id = ?", userID).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return tickets, nil
}
