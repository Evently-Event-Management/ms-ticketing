package analytics

import (
	"context"
	"ms-ticketing/internal/models"
	"strings"

	"github.com/uptrace/bun"
)

// OrderSortField defines the valid fields for sorting orders
type OrderSortField string

const (
	OrderSortByPrice     OrderSortField = "price"
	OrderSortByCreatedAt OrderSortField = "created_at"
)

// GetEventOrders returns orders for a specific event with optional filters
func (s *Service) GetEventOrders(ctx context.Context, eventID string, options EventOrderOptions) ([]models.OrderWithTickets, error) {
	// Start with base query for orders by event_id
	q := s.db.NewSelect().
		Model((*models.Order)(nil)).
		Where("event_id = ?", eventID)

	// Apply session filter if provided
	if options.SessionID != "" {
		q = q.Where("session_id = ?", options.SessionID)
	}

	// Apply status filter if provided
	if options.Status != "" {
		q = q.Where("status = ?", options.Status)
	}

	// Apply sorting
	if options.SortBy != "" {
		direction := "ASC"
		if options.SortDesc {
			direction = "DESC"
		}

		switch OrderSortField(strings.ToLower(options.SortBy)) {
		case OrderSortByPrice:
			q = q.Order("price " + direction)
		case OrderSortByCreatedAt:
			q = q.Order("created_at " + direction)
		default:
			// Default to created_at if invalid sort field
			q = q.Order("created_at " + direction)
		}
	} else {
		// Default sort by created_at descending (newest first)
		q = q.Order("created_at DESC")
	}

	// Apply pagination if provided
	if options.Limit > 0 {
		q = q.Limit(options.Limit)
	}

	if options.Offset > 0 {
		q = q.Offset(options.Offset)
	}

	// Execute the query
	var orders []models.Order
	err := q.Scan(ctx, &orders)
	if err != nil {
		return nil, err
	}

	if len(orders) == 0 {
		return []models.OrderWithTickets{}, nil
	}

	// Collect all order IDs to fetch tickets
	orderIDs := make([]string, len(orders))
	for i, order := range orders {
		orderIDs[i] = order.OrderID
	}

	// Fetch tickets for all orders in a single query
	var tickets []models.Ticket
	err = s.db.NewSelect().
		Model(&tickets).
		Where("order_id IN (?)", bun.In(orderIDs)).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	// Create a map to group tickets by order ID
	ticketsByOrderID := make(map[string][]models.TicketForStreaming)
	for _, ticket := range tickets {
		ticketsByOrderID[ticket.OrderID] = append(ticketsByOrderID[ticket.OrderID], ticket.ToStreamingTicket())
	}

	// Combine orders with their tickets
	result := make([]models.OrderWithTickets, len(orders))
	for i, order := range orders {
		orderWithTickets := models.OrderWithTickets{
			Order:   order,
			Tickets: ticketsByOrderID[order.OrderID],
		}
		result[i] = orderWithTickets
	}

	return result, nil
}

// EventOrderOptions contains options for filtering and sorting orders
type EventOrderOptions struct {
	SessionID string
	Status    string
	SortBy    string
	SortDesc  bool
	Limit     int
	Offset    int
}
