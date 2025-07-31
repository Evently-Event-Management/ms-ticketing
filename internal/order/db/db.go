package db

import (
	"context"
	"github.com/uptrace/bun"
)

type DB struct {
	Bun *bun.DB
}

func (d *DB) GetOrderByID(id string) (*Order, error) {
	var order Order
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

func (d *DB) UpdateOrder(order Order) error {
	_, err := d.Bun.NewUpdate().
		Model(&order).
		Column("event_id", "user_id", "seat_id", "status", "updated_at", "promo_code", "discount_applied").
		Where("id = ?", order.ID).
		Exec(context.Background())
	return err
}

func (d *DB) CancelOrder(id string) error {
	_, err := d.Bun.NewDelete().
		Model((*Order)(nil)).
		Where("id = ?", id).
		Exec(context.Background())
	return err
}

func (d *DB) CreateOrder(order Order) error {

	_, err := d.Bun.NewInsert().Model(&order).Exec(context.Background())
	return err
}
