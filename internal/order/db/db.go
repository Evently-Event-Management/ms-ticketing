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
		Join("JOIN tickets t ON t.order_id = \"order\".order_id").
		Where("t.seat_id = ?", seatID).
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

// GetOrdersWithTicketsByUserID → fetch all orders with tickets for a given user_id
func (d *DB) GetOrdersWithTicketsByUserID(userID string) ([]models.OrderWithTickets, error) {
	// First get all orders for the user
	var orders []models.Order
	err := d.Bun.NewSelect().
		Model(&orders).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(context.Background())
	if err != nil {
		return nil, err
	}

	// If no orders found, return empty slice
	if len(orders) == 0 {
		return []models.OrderWithTickets{}, nil
	}

	// Build a slice of order IDs for ticket query
	orderIDs := make([]string, len(orders))
	for i, order := range orders {
		orderIDs[i] = order.OrderID
	}

	// Get all tickets for these orders
	var tickets []models.Ticket
	err = d.Bun.NewSelect().
		Model(&tickets).
		Where("order_id IN (?)", bun.In(orderIDs)).
		Order("order_id", "issued_at").
		Scan(context.Background())
	if err != nil {
		return nil, err
	}

	// Group tickets by order_id
	ticketsByOrder := make(map[string][]models.TicketForStreaming)
	for _, ticket := range tickets {
		streamingTicket := ticket.ToStreamingTicket()
		ticketsByOrder[ticket.OrderID] = append(ticketsByOrder[ticket.OrderID], streamingTicket)
	}

	// Build the result with orders and their tickets
	result := make([]models.OrderWithTickets, len(orders))
	for i, order := range orders {
		result[i] = models.OrderWithTickets{
			Order:   order,
			Tickets: ticketsByOrder[order.OrderID],
		}
		// If no tickets found for this order, initialize empty slice
		if result[i].Tickets == nil {
			result[i].Tickets = []models.TicketForStreaming{}
		}
	}

	return result, nil
}

// GetOrdersWithTicketsAndQRByUserID → fetch all orders with tickets including QR codes for a given user_id
func (d *DB) GetOrdersWithTicketsAndQRByUserID(userID string) ([]models.OrderWithTicketsAndQR, error) {
	// First get all orders for the user
	var orders []models.Order
	err := d.Bun.NewSelect().
		Model(&orders).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(context.Background())
	if err != nil {
		return nil, err
	}

	// If no orders found, return empty slice
	if len(orders) == 0 {
		return []models.OrderWithTicketsAndQR{}, nil
	}

	// Build a slice of order IDs for ticket query
	orderIDs := make([]string, len(orders))
	for i, order := range orders {
		orderIDs[i] = order.OrderID
	}

	// Get all tickets for these orders INCLUDING QR codes
	var tickets []models.Ticket
	err = d.Bun.NewSelect().
		Model(&tickets).
		Where("order_id IN (?)", bun.In(orderIDs)).
		Order("order_id", "issued_at").
		Scan(context.Background())
	if err != nil {
		return nil, err
	}

	// Group tickets by order_id
	ticketsByOrder := make(map[string][]models.TicketWithQRCode)
	for _, ticket := range tickets {
		ticketWithQR := ticket.ToTicketWithQRCode()
		ticketsByOrder[ticket.OrderID] = append(ticketsByOrder[ticket.OrderID], ticketWithQR)
	}

	// Build the result with orders and their tickets (including QR codes)
	result := make([]models.OrderWithTicketsAndQR, len(orders))
	for i, order := range orders {
		result[i] = models.OrderWithTicketsAndQR{
			Order:   order,
			Tickets: ticketsByOrder[order.OrderID],
		}
		// If no tickets found for this order, initialize empty slice
		if result[i].Tickets == nil {
			result[i].Tickets = []models.TicketWithQRCode{}
		}
	}

	return result, nil
}
