package analytics

import (
	"context"
	"time"
)

// OrganizationAnalytics represents aggregated analytics data for all events in an organization
type OrganizationAnalytics struct {
	OrganizationID   string              `json:"organization_id"`
	TotalRevenue     float64             `json:"total_revenue"`
	TotalBeforeDisc  float64             `json:"total_before_discounts"`
	TotalTicketsSold int                 `json:"total_tickets_sold"`
	DailySales       []DailySalesMetrics `json:"daily_sales"`
	SalesByTier      []TierSalesMetrics  `json:"sales_by_tier"`
}

// GetOrganizationAnalytics returns revenue analytics for all events in an organization
func (s *Service) GetOrganizationAnalytics(ctx context.Context, organizationID string, status string) (*OrganizationAnalytics, error) {
	// Query orders by organization_id field and optionally by status
	var orders []struct {
		TotalRevenue    float64 `bun:"total_revenue"`
		TotalBeforeDisc float64 `bun:"total_before_disc"`
	}

	rawSQL := `
		SELECT 
			SUM(price) AS total_revenue,
			SUM(subtotal) AS total_before_disc
		FROM 
			orders
		WHERE 
			organization_id = ?`

	args := []interface{}{organizationID}

	if status != "" {
		rawSQL += " AND status = ?"
		args = append(args, status)
	}

	err := s.db.NewRaw(rawSQL, args...).Scan(ctx, &orders)
	if err != nil {
		return nil, err
	}

	// Count tickets for this organization
	var ticketCount int
	rawSQL = `
		SELECT 
			COUNT(*)
		FROM 
			tickets t
		JOIN 
			orders o ON t.order_id = o.order_id
		WHERE 
			o.organization_id = ?`

	ticketArgs := []interface{}{organizationID}

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
	rawSQL = `
		SELECT
			DATE(o.created_at) AS sales_date,
			SUM(o.price) AS daily_revenue,
			COALESCE(SUM(ticket_count), 0) AS tickets_sold_on_date
		FROM (
			SELECT
				order_id,
				price,
				status,
				created_at
			FROM orders
			WHERE
				organization_id = ?`

	dailyArgs := []interface{}{organizationID}

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
			o.organization_id = ?`

	tierArgs := []interface{}{organizationID}

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

	// Calculate total values from the orders query
	var totalRevenue float64
	var totalBeforeDisc float64

	if len(orders) > 0 {
		totalRevenue = orders[0].TotalRevenue
		totalBeforeDisc = orders[0].TotalBeforeDisc
	}

	// Format results
	result := &OrganizationAnalytics{
		OrganizationID:   organizationID,
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
