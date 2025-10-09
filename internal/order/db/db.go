package db

import (
	"context"
	"ms-ticketing/internal/models"

	"github.com/uptrace/bun"
)

type DB struct {
	Bun *bun.DB
}

// ---------------- ORDERS ----------------

// GetOrderByID → fetch one order by its ID
func (d *DB) GetOrderByID(id string) (*models.Order, error) {
	var order models.Order
	err := d.Bun.NewSelect().
		Model(&order).
		Where("order_id = ?", id).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// GetOrderWithSeats retrieves an order and its associated seat IDs
func (d *DB) GetOrderWithSeats(id string) (*models.OrderWithSeats, error) {
	// First get the order
	var order models.Order
	err := d.Bun.NewSelect().
		Model(&order).
		Where("order_id = ?", id).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}

	// Then get the seats from tickets table
	var seatIDs []string
	err = d.Bun.NewSelect().
		Column("seat_id").
		Table("tickets").
		Where("order_id = ?", id).
		Scan(context.Background(), &seatIDs)
	if err != nil {
		return nil, err
	}

	// Combine the results
	return &models.OrderWithSeats{
		Order:   order,
		SeatIDs: seatIDs,
	}, nil
}

// UpdateOrder → update allowed fields
func (d *DB) UpdateOrder(order models.Order) error {
	_, err := d.Bun.NewUpdate().
		Model(&order).
		Column("session_id", "event_id", "user_id", "status", "price", "created_at").
		Where("order_id = ?", order.OrderID).
		Exec(context.Background())
	return err
}

// CancelOrder → delete an order by ID
func (d *DB) CancelOrder(id string) error {
	_, err := d.Bun.NewDelete().
		Model((*models.Order)(nil)).
		Where("order_id = ?", id).
		Exec(context.Background())
	return err
}

// CreateOrder → insert new order
func (d *DB) CreateOrder(order models.Order) error {
	_, err := d.Bun.NewInsert().Model(&order).Exec(context.Background())
	return err
}

// ---------------- RELATION QUERIES ----------------

// GetOrderBySeat → find an order that contains a given seat ID
func (d *DB) GetOrderBySeat(seatID string) (*models.Order, error) {
	var order models.Order
	err := d.Bun.NewSelect().
		Model(&order).
		Join("JOIN tickets ON tickets.order_id = orders.order_id").
		Where("tickets.seat_id = ?", seatID).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// GetTicketsByOrder → fetch all tickets linked to an order
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

// GetSeatsByOrder → fetch all seat IDs linked to an order
func (d *DB) GetSeatsByOrder(orderID string) ([]string, error) {
	var seatIDs []string
	err := d.Bun.NewSelect().
		Column("seat_id").
		Table("tickets").
		Where("order_id = ?", orderID).
		Scan(context.Background(), &seatIDs)
	if err != nil {
		return nil, err
	}
	return seatIDs, nil
}

func (d *DB) GetSessionIdBySeat(seatID string) (string, error) {
	// Get session ID by joining orders and tickets tables
	var sessionID string
	err := d.Bun.NewSelect().
		Column("orders.session_id").
		Table("orders").
		Join("JOIN tickets ON tickets.order_id = orders.order_id").
		Where("tickets.seat_id = ?", seatID).
		Limit(1).
		Scan(context.Background(), &sessionID)

	if err != nil {
		return "", err
	}

	return sessionID, nil
}
