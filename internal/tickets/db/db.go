package db

import (
	"context"
	"fmt"
	"github.com/uptrace/bun"
	"ms-ticketing/internal/models"
)

type DB struct {
	Bun *bun.DB
}

func (d *DB) GetTicketByID(id string) (*models.Ticket, error) {
	var ticket models.Ticket
	err := d.Bun.NewSelect().
		Model(&ticket).
		Where("id = ?", id).
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
		Column("event_id", "user_id", "seat_id", "status", "checked_in", "check_in_time", "updated_at").
		Where("id = ?", ticket.ID).
		Exec(context.Background())
	return err
}

func (d *DB) CancelTicket(id string) error {
	_, err := d.Bun.NewDelete().
		Model((*models.Ticket)(nil)).
		Where("id = ?", id).
		Exec(context.Background())
	return err
}

func (d *DB) CreateTicket(ticket models.Ticket) error {
	_, err := d.Bun.NewInsert().Model(&ticket).Exec(context.Background())
	return err
}

func (d *DB) GetTicketsByUser(userID string) (*models.Ticket, error) {
	var ticket models.Ticket
	err := d.Bun.NewSelect().
		Model(&ticket).
		Where("user_id = ?", userID).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// UserExists checks if a user with the given ID exists in the database
func (d *DB) UserExists(userID string) (bool, error) {
	fmt.Println(userID)
	err, _ := d.Bun.NewSelect().
		Model((*models.User)(nil)).
		Where("id = ?", userID).
		Exists(context.Background())
	if err != true {
		return false, nil
	}
	return true, nil
}

// EventExists checks if an event with the given ID exists in the database
func (d *DB) EventExists(eventID string) (bool, error) {
	err, _ := d.Bun.NewSelect().
		Model((*models.Event)(nil)). // Assuming you have an Event model
		Where("id = ?", eventID).
		Exists(context.Background())
	if err != true {
		return false, nil
	}
	return true, nil
}

// SeatExists checks if a seat with the given ID exists in the database
