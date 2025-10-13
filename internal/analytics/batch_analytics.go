package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BatchEventAnalytics represents aggregated analytics data for multiple events
type BatchEventAnalytics struct {
	EventIDs         []string            `json:"event_ids"`
	TotalRevenue     float64             `json:"total_revenue"`
	TotalBeforeDisc  float64             `json:"total_before_discounts"`
	TotalTicketsSold int                 `json:"total_tickets_sold"`
	DailySales       []DailySalesMetrics `json:"daily_sales"`
	SalesByTier      []TierSalesMetrics  `json:"sales_by_tier"`
}

// GetBatchEventAnalytics returns aggregated analytics data for multiple events
func (s *Service) GetBatchEventAnalytics(ctx context.Context, eventIDs []string, status string) (*BatchEventAnalytics, error) {
	if len(eventIDs) == 0 {
		return &BatchEventAnalytics{EventIDs: []string{}}, nil
	}

	// Create placeholder for SQL IN clause
	placeholders := make([]string, len(eventIDs))
	args := make([]interface{}, len(eventIDs))
	for i, id := range eventIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	inClause := strings.Join(placeholders, ", ")

	// Query orders for all events in the array
	var orders []struct {
		TotalRevenue    float64 `bun:"total_revenue"`
		TotalBeforeDisc float64 `bun:"total_before_disc"`
	}
	rawSQL := fmt.Sprintf(`
		SELECT 
			SUM(price) AS total_revenue,
			SUM(subtotal) AS total_before_disc
		FROM 
			orders
		WHERE 
			event_id IN (%s)`, inClause)

	if status != "" {
		rawSQL += " AND status = ?"
		args = append(args, status)
	}

	err := s.db.NewRaw(rawSQL, args...).Scan(ctx, &orders)
	if err != nil {
		return nil, err
	}

	// Get total tickets count
	var ticketCount int
	rawSQL = fmt.Sprintf(`
		SELECT 
			COUNT(*)
		FROM 
			tickets t
		JOIN 
			orders o ON t.order_id = o.order_id
		WHERE 
			o.event_id IN (%s)`, inClause)

	ticketArgs := make([]interface{}, len(args))
	copy(ticketArgs, args)

	if status != "" {
		rawSQL += " AND o.status = ?"
		ticketArgs = append(ticketArgs, status)
	}

	err = s.db.NewRaw(rawSQL, ticketArgs...).Scan(ctx, &ticketCount)
	if err != nil {
		return nil, err
	}

	// Get daily sales metrics
	type dailySalesRaw struct {
		SalesDate     time.Time `bun:"sales_date"`
		DailyRevenue  float64   `bun:"daily_revenue"`
		DailyQuantity int       `bun:"tickets_sold_on_date"`
	}

	var dailySales []dailySalesRaw
	rawSQL = fmt.Sprintf(`
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
				event_id IN (%s)`, inClause)

	dailyArgs := make([]interface{}, len(eventIDs))
	copy(dailyArgs, args[:len(eventIDs)])

	if status != "" {
		rawSQL += " AND status = ?"
		dailyArgs = append(dailyArgs, status)
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

	err = s.db.NewRaw(rawSQL, dailyArgs...).Scan(ctx, &dailySales)
	if err != nil {
		return nil, err
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
	rawSQL = fmt.Sprintf(`
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
			o.event_id IN (%s)`, inClause)

	tierArgs := make([]interface{}, len(args))
	copy(tierArgs, args)

	if status != "" {
		rawSQL += " AND o.status = ?"
		tierArgs = append(tierArgs, status)
	}

	rawSQL += `
		GROUP BY 
			t.tier_id, t.tier_name, t.colour
		ORDER BY 
			t.tier_name
	`

	err = s.db.NewRaw(rawSQL, tierArgs...).Scan(ctx, &tierSales)
	if err != nil {
		return nil, err
	}

	// Format and prepare result
	var totalRevenue float64
	var totalBeforeDisc float64

	// If we got any orders, take the aggregated values
	if len(orders) > 0 {
		totalRevenue = orders[0].TotalRevenue
		totalBeforeDisc = orders[0].TotalBeforeDisc
	}

	// Format results
	result := &BatchEventAnalytics{
		EventIDs:         eventIDs,
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

// BatchEventAnalyticsMap represents analytics data for individual events in a map
type BatchEventAnalyticsMap struct {
	EventAnalytics map[string]*EventAnalytics `json:"eventAnalytics"`
}

// GetBatchEventAnalyticsMap returns individual analytics for each event in a batch
func (s *Service) GetBatchEventAnalyticsMap(ctx context.Context, eventIDs []string, status string) (*BatchEventAnalyticsMap, error) {
	// Initialize result map
	result := &BatchEventAnalyticsMap{
		EventAnalytics: make(map[string]*EventAnalytics),
	}

	// If no events provided, return empty map
	if len(eventIDs) == 0 {
		return result, nil
	}

	// Fetch analytics for each event individually
	for _, eventID := range eventIDs {
		analytics, err := s.GetEventAnalytics(ctx, eventID, status)
		if err != nil {
			// Log the error but continue with other events
			continue
		}
		result.EventAnalytics[eventID] = analytics
	}

	return result, nil
}
