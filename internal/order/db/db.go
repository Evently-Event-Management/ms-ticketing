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

// UpdateOrder → update allowed fields
func (d *DB) UpdateOrder(order models.Order) error {
	_, err := d.Bun.NewUpdate().
		Model(&order).
		Column("session_id", "user_id", "seat_ids", "status", "price", "created_at").
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
		Where("? = ANY(seat_ids)", seatID).
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
