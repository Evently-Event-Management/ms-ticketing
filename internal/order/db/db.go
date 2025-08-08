package db

import (
	"context"
	"github.com/uptrace/bun"
	"ms-ticketing/internal/models"
)

type DB struct {
	Bun *bun.DB
}

func (d *DB) GetOrderByID(id string) (*models.Order, error) {
	var order models.Order
	err := d.Bun.NewSelect().
		Model(&order).
		Where("id = ?", id).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return &order, nil
}

func (d *DB) UpdateOrder(order models.Order) error {
	_, err := d.Bun.NewUpdate().
		Model(&order).
		Column("event_id", "user_id", "seat_ids", "status", "updated_at", "promo_code", "discount_applied").
		Where("id = ?", order.ID).
		Exec(context.Background())
	return err
}

func (d *DB) CancelOrder(id string) error {
	_, err := d.Bun.NewDelete().
		Model((*models.Order)(nil)).
		Where("id = ?", id).
		Exec(context.Background())
	return err
}

func (d *DB) CreateOrder(order models.Order) error {

	_, err := d.Bun.NewInsert().Model(&order).Exec(context.Background())
	return err
}

func (d *DB) GetOrderBySeatAndEvent(seatID, eventID string) (*models.Order, error) {
	var order models.Order
	err := d.Bun.NewSelect().
		Model(&order).
		Where("event_id = ?", eventID).
		Where("? = ANY(seat_ids)", seatID).
		Limit(1).
		Scan(context.Background())
	if err != nil {
		return nil, err
	}
	return &order, nil
}
