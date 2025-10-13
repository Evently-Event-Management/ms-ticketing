package analytics

import (
	"context"
	"ms-ticketing/internal/models"
	"time"

	"github.com/uptrace/bun"
)

// Service handles analytics operations
type Service struct {
	db *bun.DB
}

// NewService creates a new analytics service
func NewService(db *bun.DB) *Service {
	return &Service{db: db}
}

// EventAnalytics represents aggregated analytics data for an event
type EventAnalytics struct {
	EventID          string              `json:"event_id"`
	TotalRevenue     float64             `json:"total_revenue"`
	TotalBeforeDisc  float64             `json:"total_before_discounts"`
	TotalTicketsSold int                 `json:"total_tickets_sold"`
	DailySales       []DailySalesMetrics `json:"daily_sales"`
	SalesByTier      []TierSalesMetrics  `json:"sales_by_tier"`
}

// EventDiscountAnalytics represents discount usage data for an event
type EventDiscountAnalytics struct {
	EventID       string          `json:"event_id"`
	DiscountUsage []DiscountUsage `json:"discount_usage"`
}

// TierSalesMetrics contains sales metrics for a specific tier
type TierSalesMetrics struct {
	TierID      string  `json:"tier_id"`
	TierName    string  `json:"tier_name"`
	TierColor   string  `json:"tier_color"`
	TicketsSold int     `json:"tickets_sold"`
	Revenue     float64 `json:"revenue"`
}

// SessionAnalytics represents aggregated analytics data for a session
type SessionAnalytics struct {
	EventID          string              `json:"event_id"`
	SessionID        string              `json:"session_id"`
	TotalRevenue     float64             `json:"total_revenue"`
	TotalBeforeDisc  float64             `json:"total_before_discounts"`
	TotalTicketsSold int                 `json:"total_tickets_sold"`
	DailySales       []DailySalesMetrics `json:"daily_sales"`
	SalesByTier      []TierSalesMetrics  `json:"sales_by_tier"`
}

// SessionSummary contains basic revenue information for a session
type SessionSummary struct {
	SessionID        string  `json:"session_id"`
	TotalRevenue     float64 `json:"total_revenue"`
	TotalBeforeDisc  float64 `json:"total_before_discounts"`
	TotalTicketsSold int     `json:"total_tickets_sold"`
}

// EventSessionsAnalytics represents all sessions summary for an event
type EventSessionsAnalytics struct {
	EventID  string           `json:"event_id"`
	Sessions []SessionSummary `json:"sessions"`
}

// DailySalesMetrics contains metrics for a single day
type DailySalesMetrics struct {
	Date        string  `json:"date"`
	Revenue     float64 `json:"revenue"`
	TicketsSold int     `json:"tickets_sold"`
}

// DiscountUsage tracks discount code usage by day
type DiscountUsage struct {
	Date          string  `json:"date"`
	DiscountCode  string  `json:"discount_code"`
	UsageCount    int     `json:"usage_count"`
	TotalDiscount float64 `json:"total_discount_amount"`
}

// GetEventAnalytics returns revenue analytics for a specific event
func (s *Service) GetEventAnalytics(ctx context.Context, eventID string, status string) (*EventAnalytics, error) {
	// Query orders directly by event_id field and optionally by status
	var orders []models.Order
	query := s.db.NewSelect().
		Model(&orders).
		Where("event_id = ?", eventID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Scan(ctx)
	if err != nil {
		return nil, err
	}

	// Count tickets - using a simpler query approach
	var ticketCount int
	rawSQL := "SELECT COUNT(*) FROM tickets t JOIN orders o ON t.order_id = o.order_id WHERE o.event_id = ?"
	args := []interface{}{eventID}

	if status != "" {
		rawSQL += " AND o.status = ?"
		args = append(args, status)
	}

	err = s.db.NewRaw(rawSQL, args...).
		Scan(ctx, &ticketCount)
	if err != nil {
		return nil, err
	}

	// Get daily sales
	type dailySalesRaw struct {
		SalesDate     time.Time `bun:"sales_date"`
		DailyRevenue  float64   `bun:"daily_revenue"`
		DailyQuantity int       `bun:"tickets_sold_on_date"`
	}

	var dailySales []dailySalesRaw
	// Use raw SQL to count tickets per day with proper status filtering
	rawSQL = `
		SELECT
			DATE(o.created_at) AS sales_date,
			SUM(o.price) AS daily_revenue,
			COALESCE(SUM(ticket_count), 0) AS tickets_sold_on_date
		FROM (
			SELECT
				order_id,
				event_id,
				price,
				status,
				created_at
			FROM orders
			WHERE
				event_id = ?
	`
	args = []interface{}{eventID}

	if status != "" {
		rawSQL += " AND status = ?"
		args = append(args, status)
	}

	rawSQL += `
		) o
		LEFT JOIN (
			SELECT
				order_id,
				COUNT(ticket_id) AS ticket_count
			FROM tickets
			GROUP BY order_id
		) t ON t.order_id = o.order_id
		GROUP BY
			DATE(o.created_at)
		ORDER BY
			sales_date
	`

	err = s.db.NewRaw(rawSQL, args...).Scan(ctx, &dailySales)
	if err != nil {
		return nil, err
	}

	// Calculate total revenue and subtotal (before discounts)
	var totalRevenue float64
	var totalBeforeDisc float64
	for _, order := range orders {
		totalRevenue += order.Price
		totalBeforeDisc += order.SubTotal
	}

	// Get sales by tier
	type tierSalesRaw struct {
		TierID      string  `bun:"tier_id"`
		TierName    string  `bun:"tier_name"`
		TierColor   string  `bun:"tier_color"`
		TicketCount int     `bun:"ticket_count"`
		TierRevenue float64 `bun:"tier_revenue"`
	}

	var tierSales []tierSalesRaw
	rawSQL = `
		SELECT 
			t.tier_id,
			t.tier_name,
			t.colour AS tier_color,
			COUNT(t.ticket_id) AS ticket_count,
			SUM(t.price_at_purchase) AS tier_revenue
		FROM 
			tickets t
		JOIN 
			orders o ON t.order_id = o.order_id
		WHERE 
			o.event_id = ?
	`
	args = []interface{}{eventID}

	if status != "" {
		rawSQL += " AND o.status = ?"
		args = append(args, status)
	}

	rawSQL += `
		GROUP BY 
			t.tier_id, t.tier_name, t.colour
		ORDER BY 
			t.tier_name
	`

	err = s.db.NewRaw(rawSQL, args...).Scan(ctx, &tierSales)
	if err != nil {
		return nil, err
	}

	// Format results
	result := &EventAnalytics{
		EventID:          eventID,
		TotalRevenue:     totalRevenue,
		TotalBeforeDisc:  totalBeforeDisc,
		TotalTicketsSold: ticketCount,
		DailySales:       make([]DailySalesMetrics, 0, len(dailySales)),
		SalesByTier:      make([]TierSalesMetrics, 0, len(tierSales)),
	}

	for _, ds := range dailySales {
		result.DailySales = append(result.DailySales, DailySalesMetrics{
			Date:        ds.SalesDate.Format("2006-01-02"),
			Revenue:     ds.DailyRevenue,
			TicketsSold: ds.DailyQuantity,
		})
	}

	for _, ts := range tierSales {
		result.SalesByTier = append(result.SalesByTier, TierSalesMetrics{
			TierID:      ts.TierID,
			TierName:    ts.TierName,
			TierColor:   ts.TierColor,
			TicketsSold: ts.TicketCount,
			Revenue:     ts.TierRevenue,
		})
	}

	return result, nil
}

// GetEventDiscountAnalytics returns discount usage analytics for a specific event
func (s *Service) GetEventDiscountAnalytics(ctx context.Context, eventID string, status string) (*EventDiscountAnalytics, error) {
	// Query orders directly by event_id field
	// Get discount usage
	type discountUsageRaw struct {
		UsageDate         time.Time `bun:"usage_date"`
		DiscountCode      string    `bun:"discount_code"`
		CodeUsageCount    int       `bun:"code_usage_count"`
		DiscountAmountSum float64   `bun:"discount_amount_sum"`
	}

	var discountUsage []discountUsageRaw
	query := s.db.NewSelect().
		ColumnExpr("DATE(orders.created_at) AS usage_date").
		ColumnExpr("orders.discount_code").
		ColumnExpr("COUNT(*) AS code_usage_count").
		ColumnExpr("SUM(orders.discount_amount) AS discount_amount_sum").
		TableExpr("orders").
		Where("orders.event_id = ? AND orders.discount_code IS NOT NULL AND orders.discount_code != ''", eventID)

	if status != "" {
		query = query.Where("orders.status = ?", status)
	}

	err := query.
		GroupExpr("DATE(orders.created_at), orders.discount_code").
		OrderExpr("DATE(orders.created_at), orders.discount_code").
		Scan(ctx, &discountUsage)
	if err != nil {
		return nil, err
	}

	// Format results
	result := &EventDiscountAnalytics{
		EventID:       eventID,
		DiscountUsage: make([]DiscountUsage, 0, len(discountUsage)),
	}

	for _, du := range discountUsage {
		result.DiscountUsage = append(result.DiscountUsage, DiscountUsage{
			Date:          du.UsageDate.Format("2006-01-02"),
			DiscountCode:  du.DiscountCode,
			UsageCount:    du.CodeUsageCount,
			TotalDiscount: du.DiscountAmountSum,
		})
	}

	return result, nil
}

// GetEventSessionsAnalytics returns summary analytics for all sessions of an event
func (s *Service) GetEventSessionsAnalytics(ctx context.Context, eventID string, status string) (*EventSessionsAnalytics, error) {
	// sessionSummaryRaw is used to scan the raw SQL query result.
	type sessionSummaryRaw struct {
		SessionID        string  `bun:"session_id"`
		TotalRevenue     float64 `bun:"total_revenue"`
		TotalBeforeDisc  float64 `bun:"total_before_disc"`
		TotalTicketsSold int     `bun:"total_tickets_sold"`
	}

	var sessionSummaries []sessionSummaryRaw
	// The corrected query using Common Table Expressions (CTEs)
	rawSQL := `
        WITH OrderTotals AS (
            -- First, calculate the total revenue and subtotal from the orders table
            SELECT
                session_id,
                SUM(price) AS total_revenue,
                SUM(subtotal) AS total_before_disc
            FROM
                orders
            WHERE
                event_id = ?`

	args := []interface{}{eventID}

	if status != "" {
		rawSQL += " AND status = ?"
		args = append(args, status)
	}

	rawSQL += `
            GROUP BY
                session_id
        ),
        TicketCounts AS (
            -- Second, count the number of tickets sold
            SELECT
                o.session_id,
                COUNT(t.ticket_id) AS total_tickets_sold
            FROM
                tickets t
            JOIN
                orders o ON t.order_id = o.order_id
            WHERE
                o.event_id = ?`

	args = append(args, eventID)

	if status != "" {
		rawSQL += " AND o.status = ?"
		args = append(args, status)
	}

	rawSQL += `
            GROUP BY
                o.session_id
        )
        -- Finally, join the two results
        SELECT
            ot.session_id,
            ot.total_revenue,
            ot.total_before_disc,
            tc.total_tickets_sold
        FROM
            OrderTotals ot
        JOIN
            TicketCounts tc ON ot.session_id = tc.session_id
        ORDER BY
            ot.session_id;
    `

	err := s.db.NewRaw(rawSQL, args...).Scan(ctx, &sessionSummaries)

	if err != nil {
		return nil, err
	}

	// Format results into the final structure
	result := &EventSessionsAnalytics{
		EventID:  eventID,
		Sessions: make([]SessionSummary, 0, len(sessionSummaries)),
	}

	for _, ss := range sessionSummaries {
		result.Sessions = append(result.Sessions, SessionSummary(ss))
	}

	return result, nil
}

// GetSessionAnalytics returns analytics for a specific session
func (s *Service) GetSessionAnalytics(ctx context.Context, eventID, sessionID string, status string) (*SessionAnalytics, error) {
	// Get all orders for this session
	var orders []models.Order
	query := s.db.NewSelect().
		Model(&orders).
		Where("session_id = ?", sessionID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var err error
	err = query.Scan(ctx)
	if err != nil {
		return nil, err
	}

	// Count tickets - using a simpler query approach
	var ticketCount int
	rawSQL := "SELECT COUNT(*) FROM tickets t JOIN orders o ON t.order_id = o.order_id WHERE o.session_id = ?"
	args := []interface{}{sessionID}

	if status != "" {
		rawSQL += " AND o.status = ?"
		args = append(args, status)
	}

	err = s.db.NewRaw(rawSQL, args...).
		Scan(ctx, &ticketCount)
	if err != nil {
		return nil, err
	}

	// Get daily sales
	type dailySalesRaw struct {
		SalesDate     time.Time `bun:"sales_date"`
		DailyRevenue  float64   `bun:"daily_revenue"`
		DailyQuantity int       `bun:"tickets_sold_on_date"`
	}

	var dailySales []dailySalesRaw
	// Use raw SQL to count tickets per day with proper status filtering
	rawSQL = `
		SELECT
			DATE(o.created_at) AS sales_date,
			SUM(o.price) AS daily_revenue,
			COALESCE(SUM(ticket_count), 0) AS tickets_sold_on_date
		FROM (
			SELECT
				order_id,
				session_id,
				price,
				status,
				created_at
			FROM orders
			WHERE
				session_id = ?
	`
	args = []interface{}{sessionID}

	if status != "" {
		rawSQL += " AND status = ?"
		args = append(args, status)
	}

	rawSQL += `
		) o
		LEFT JOIN (
			SELECT
				order_id,
				COUNT(ticket_id) AS ticket_count
			FROM tickets
			GROUP BY order_id
		) t ON t.order_id = o.order_id
		GROUP BY
			DATE(o.created_at)
		ORDER BY
			sales_date
	`

	err = s.db.NewRaw(rawSQL, args...).Scan(ctx, &dailySales)
	if err != nil {
		return nil, err
	}

	// Calculate total revenue and subtotal (before discounts)
	var totalRevenue float64
	var totalBeforeDisc float64
	for _, order := range orders {
		totalRevenue += order.Price
		totalBeforeDisc += order.SubTotal
	}

	// Get sales by tier for this session
	type tierSalesRaw struct {
		TierID      string  `bun:"tier_id"`
		TierName    string  `bun:"tier_name"`
		TierColor   string  `bun:"tier_color"`
		TicketCount int     `bun:"ticket_count"`
		TierRevenue float64 `bun:"tier_revenue"`
	}

	var tierSales []tierSalesRaw
	rawSQL = `
		SELECT 
			t.tier_id,
			t.tier_name,
			t.colour AS tier_color,
			COUNT(t.ticket_id) AS ticket_count,
			SUM(t.price_at_purchase) AS tier_revenue
		FROM 
			tickets t
		JOIN 
			orders o ON t.order_id = o.order_id
		WHERE 
			o.session_id = ?
	`
	args = []interface{}{sessionID}

	if status != "" {
		rawSQL += " AND o.status = ?"
		args = append(args, status)
	}

	rawSQL += `
		GROUP BY 
			t.tier_id, t.tier_name, t.colour
		ORDER BY 
			t.tier_name
	`

	err = s.db.NewRaw(rawSQL, args...).Scan(ctx, &tierSales)
	if err != nil {
		return nil, err
	}

	// Format results
	result := &SessionAnalytics{
		EventID:          eventID,
		SessionID:        sessionID,
		TotalRevenue:     totalRevenue,
		TotalBeforeDisc:  totalBeforeDisc,
		TotalTicketsSold: ticketCount,
		DailySales:       make([]DailySalesMetrics, 0, len(dailySales)),
		SalesByTier:      make([]TierSalesMetrics, 0, len(tierSales)),
	}

	for _, ds := range dailySales {
		result.DailySales = append(result.DailySales, DailySalesMetrics{
			Date:        ds.SalesDate.Format("2006-01-02"),
			Revenue:     ds.DailyRevenue,
			TicketsSold: ds.DailyQuantity,
		})
	}

	for _, ts := range tierSales {
		result.SalesByTier = append(result.SalesByTier, TierSalesMetrics{
			TierID:      ts.TierID,
			TierName:    ts.TierName,
			TierColor:   ts.TierColor,
			TicketsSold: ts.TicketCount,
			Revenue:     ts.TierRevenue,
		})
	}

	return result, nil
}
