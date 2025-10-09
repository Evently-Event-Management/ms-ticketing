package analytics

import (
	"context"
	"ms-ticketing/internal/models"
	"time"

	"github.com/uptrace/bun"
)

// DB handles analytics database operations
type DB struct {
	bun *bun.DB
}

// NewDB creates a new analytics DB handler
func NewDB(db *bun.DB) *DB {
	return &DB{bun: db}
}

// GetOrdersByEventID retrieves all orders associated with an event
func (db *DB) GetOrdersByEventID(ctx context.Context, eventID string) ([]models.Order, error) {
	var orders []models.Order
	err := db.bun.NewSelect().
		Model(&orders).
		Where("event_id = ?", eventID).
		Scan(ctx)

	return orders, err
}

// GetOrdersBySessionID retrieves all orders for a specific session
func (db *DB) GetOrdersBySessionID(ctx context.Context, sessionID string) ([]models.Order, error) {
	var orders []models.Order
	err := db.bun.NewSelect().
		Model(&orders).
		Where("session_id = ?", sessionID).
		Scan(ctx)

	return orders, err
}

// GetTicketCountByEventID counts tickets sold for an event
func (db *DB) GetTicketCountByEventID(ctx context.Context, eventID string) (int, error) {
	var count int
	err := db.bun.NewRaw("SELECT COUNT(*) FROM tickets t JOIN orders o ON t.order_id = o.order_id WHERE o.event_id = ?", eventID).
		Scan(ctx, &count)

	return count, err
}

// GetTicketCountBySessionID counts tickets sold for a session
func (db *DB) GetTicketCountBySessionID(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := db.bun.NewRaw("SELECT COUNT(*) FROM tickets t JOIN orders o ON t.order_id = o.order_id WHERE o.session_id = ?", sessionID).
		Scan(ctx, &count)

	return count, err
}

// DailySalesData represents raw daily sales metrics from the database
type DailySalesData struct {
	SalesDate     time.Time `bun:"sales_date"`
	DailyRevenue  float64   `bun:"daily_revenue"`
	DailyQuantity int       `bun:"daily_quantity"`
}

// GetDailySalesByEventID retrieves daily sales metrics for an event
func (db *DB) GetDailySalesByEventID(ctx context.Context, eventID string) ([]DailySalesData, error) {
	var dailySales []DailySalesData
	// Use raw SQL to count tickets per day rather than orders
	err := db.bun.NewRaw(`
		SELECT 
			DATE(o.created_at) AS sales_date,
			SUM(o.price) AS daily_revenue,
			COUNT(t.ticket_id) AS daily_quantity
		FROM 
			orders o
		JOIN 
			tickets t ON t.order_id = o.order_id
		WHERE 
			o.event_id = ?
		GROUP BY 
			DATE(o.created_at)
		ORDER BY 
			DATE(o.created_at)
	`, eventID).Scan(ctx, &dailySales)

	return dailySales, err
}

// GetDailySalesBySessionID retrieves daily sales metrics for a session
func (db *DB) GetDailySalesBySessionID(ctx context.Context, sessionID string) ([]DailySalesData, error) {
	var dailySales []DailySalesData
	// Use raw SQL to count tickets per day rather than orders
	err := db.bun.NewRaw(`
		SELECT 
			DATE(o.created_at) AS sales_date,
			SUM(o.price) AS daily_revenue,
			COUNT(t.ticket_id) AS daily_quantity
		FROM 
			orders o
		JOIN 
			tickets t ON t.order_id = o.order_id
		WHERE 
			o.session_id = ?
		GROUP BY 
			DATE(o.created_at)
		ORDER BY 
			DATE(o.created_at)
	`, sessionID).Scan(ctx, &dailySales)

	return dailySales, err
}

// DiscountUsageData represents raw discount usage metrics from the database
type DiscountUsageData struct {
	UsageDate         time.Time `bun:"usage_date"`
	DiscountCode      string    `bun:"discount_code"`
	CodeUsageCount    int       `bun:"code_usage_count"`
	DiscountAmountSum float64   `bun:"discount_amount_sum"`
}

// GetDiscountUsageByEventID retrieves discount usage metrics for an event
func (db *DB) GetDiscountUsageByEventID(ctx context.Context, eventID string) ([]DiscountUsageData, error) {
	var discountUsage []DiscountUsageData
	err := db.bun.NewSelect().
		ColumnExpr("DATE(orders.created_at) AS usage_date").
		ColumnExpr("orders.discount_code").
		ColumnExpr("COUNT(*) AS code_usage_count").
		ColumnExpr("SUM(orders.discount_amount) AS discount_amount_sum").
		TableExpr("orders").
		Where("orders.event_id = ? AND orders.discount_code IS NOT NULL AND orders.discount_code != ''", eventID).
		GroupExpr("DATE(orders.created_at), orders.discount_code").
		OrderExpr("DATE(orders.created_at), orders.discount_code").
		Scan(ctx, &discountUsage)

	return discountUsage, err
}
