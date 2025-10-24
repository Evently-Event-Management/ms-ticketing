package tickets

import (
	"fmt"
	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"
	qr_genrator "ms-ticketing/internal/tickets/qr_genrator"
	"os"
	"time"
)

type TicketDBLayer interface {
	CreateTicket(ticket models.Ticket) error
	GetTicketByID(ticketID string) (*models.Ticket, error)
	UpdateTicket(ticket models.Ticket) error
	CancelTicket(ticketID string) error
	GetTicketsByOrder(orderID string) ([]models.Ticket, error)
	GetTicketsByUser(userID string) ([]models.Ticket, error)
	GetTotalTicketsCount() (int, error)
	CheckinTicket(ticketID string, checkedIn bool, checkedInTime time.Time) error
	GetCheckedInCountBySession(sessionID string) (int, error)
}

type TicketService struct {
	DB TicketDBLayer
}

type Handler struct {
	TicketService *TicketService
	Logger        *logger.Logger
}

func NewTicketService(db TicketDBLayer) *TicketService {
	return &TicketService{DB: db}
}

func (s *TicketService) PlaceTicket(ticket models.Ticket) error {
	fmt.Printf("Placing ticket: %s for order: %s\n", ticket.TicketID, ticket.OrderID)
	secretKey := os.Getenv("QR_SECRET_KEY")
	qrGen := qr_genrator.NewQRGenerator(secretKey)

	qrBytes, err := qrGen.GenerateEncryptedQR(ticket)
	if err != nil {
		return fmt.Errorf("failed to generate QR: %w", err)
	}
	// Ensure IssuedAt is set
	if ticket.IssuedAt.IsZero() {
		ticket.IssuedAt = time.Now()
	}
	ticket.QRCode = qrBytes

	if err := s.DB.CreateTicket(ticket); err != nil {
		fmt.Printf("❌ Failed to create ticket: %v\n", err)
		return err
	}

	fmt.Println("✅ Ticket placed successfully.")
	return nil
}

func (s *TicketService) GetTicket(ticketID string) (*models.Ticket, error) {
	ticket, err := s.DB.GetTicketByID(ticketID)
	if err != nil {
		fmt.Printf("❌ Ticket not found: %s\n", ticketID)
		return nil, fmt.Errorf("ticket %s not found: %w", ticketID, err)
	}
	return ticket, nil
}

func (s *TicketService) UpdateTicket(ticketID string, updateData models.Ticket) error {
	ticket, err := s.DB.GetTicketByID(ticketID)
	if err != nil {
		return fmt.Errorf("ticket %s not found: %w", ticketID, err)
	}

	// Update allowed fields only
	ticket.SeatID = updateData.SeatID
	ticket.SeatLabel = updateData.SeatLabel
	ticket.Colour = updateData.Colour
	ticket.TierID = updateData.TierID
	ticket.TierName = updateData.TierName
	ticket.PriceAtPurchase = updateData.PriceAtPurchase
	ticket.QRCode = updateData.QRCode

	if err := s.DB.UpdateTicket(*ticket); err != nil {
		return fmt.Errorf("failed to update ticket: %w", err)
	}

	fmt.Println("✅ Ticket updated.")
	return nil
}

func (s *TicketService) CancelTicket(ticketID string) error {
	_, err := s.DB.GetTicketByID(ticketID)
	if err != nil {
		return fmt.Errorf("ticket %s not found: %w", ticketID, err)
	}

	// In this model, cancellation can just remove it
	if err := s.DB.CancelTicket(ticketID); err != nil {
		return fmt.Errorf("failed to cancel ticket: %w", err)
	}

	fmt.Println("✅ Ticket cancelled.")
	return nil
}

// Checkin marks a ticket as checked in
func (s *TicketService) Checkin(ticketID string) (bool, error) {
	// First verify the ticket exists
	ticket, err := s.DB.GetTicketByID(ticketID)
	if err != nil {
		return false, fmt.Errorf("ticket %s not found: %w", ticketID, err)
	}

	fmt.Printf("Found ticket: %s, currently checked in: %t\n", ticket.TicketID, ticket.CheckedIn)

	// Check if already checked in
	if ticket.CheckedIn {
		return false, fmt.Errorf("ticket %s is already checked in", ticketID)
	}

	// Use the dedicated checkin method for atomic update
	checkinTime := time.Now()
	if err := s.DB.CheckinTicket(ticketID, true, checkinTime); err != nil {
		return false, fmt.Errorf("failed to checkin ticket: %w", err)
	}

	fmt.Printf("✅ Ticket %s checked in successfully at %v\n", ticketID, checkinTime)
	return true, nil
}

// GetTicketsByOrder returns tickets for a given order
func (s *TicketService) GetTicketsByOrder(orderID string) ([]models.Ticket, error) {
	tickets, err := s.DB.GetTicketsByOrder(orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tickets for order %s: %w", orderID, err)
	}

	// Optionally handle empty result
	if len(tickets) == 0 {
		return nil, fmt.Errorf("no tickets found for order %s", orderID)
	}

	return tickets, nil
}

func (s *TicketService) GetTicketsByUser(userID string) ([]models.Ticket, error) {
	tickets, err := s.DB.GetTicketsByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tickets for user %s: %w", userID, err)
	}

	// Optionally handle empty result
	if len(tickets) == 0 {
		return nil, fmt.Errorf("no tickets found for user %s", userID)
	}

	return tickets, nil
}

// GetCheckedInCountBySession returns the count of checked-in tickets for a given session
func (s *TicketService) GetCheckedInCountBySession(sessionID string) (int, error) {
	count, err := s.DB.GetCheckedInCountBySession(sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get checked-in count for session %s: %w", sessionID, err)
	}
	return count, nil
}
