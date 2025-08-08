package tickets

import (
	"errors"
	"fmt"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/tickets/qr_genrator"
	"ms-ticketing/internal/tickets/template"
	"os"
	"time"
)

type DBLayer interface {
	CreateTicket(ticket models.Ticket) error
	GetTicketByID(id string) (*models.Ticket, error)
	UpdateTicket(ticket models.Ticket) error
	CancelTicket(id string) error
	GetTicketsByUser(userID string) (*models.Ticket, error)
	UserExists(userID string) (bool, error)
	EventExists(eventID string) (bool, error)
}

type TicketService struct {
	DB DBLayer
}

func NewTicketService(db DBLayer) *TicketService {
	return &TicketService{DB: db}
}

func (s *TicketService) PlaceTicket(ticket models.Ticket) error {
	fmt.Printf("Placing ticket: %s for event: %s\n", ticket.ID, ticket.EventID)

	// Validate referenced data exists
	userExists, err := s.DB.UserExists(ticket.UserID)
	if err != nil {
		return fmt.Errorf("failed to validate user: %w", err)
	}
	if !userExists {
		return errors.New("user does not exist")
	}

	eventExists, err := s.DB.EventExists(ticket.EventID)
	if err != nil {
		return fmt.Errorf("failed to validate event: %w", err)
	}
	if !eventExists {
		return errors.New("event does not exist")
	}

	ticket.Status = "pending"
	ticket.CreatedAt = time.Now()
	ticket.UpdatedAt = time.Now()

	if err := s.DB.CreateTicket(ticket); err != nil {
		fmt.Printf("❌ Failed to create ticket: %v\n", err)
		return err
	}

	fmt.Println("✅ Ticket placed successfully.")
	return nil
}

func (s *TicketService) GetTicket(id string) (*models.Ticket, error) {
	ticket, err := s.DB.GetTicketByID(id)
	if err != nil {
		fmt.Printf("❌ Ticket not found: %s\n", id)
		return nil, fmt.Errorf("ticket %s not found: %w", id, err)
	}
	return ticket, nil
}

func (s *TicketService) UpdateTicket(id string, updateData models.Ticket) error {
	ticket, err := s.DB.GetTicketByID(id)
	if err != nil {
		return fmt.Errorf("ticket %s not found: %w", id, err)
	}

	if ticket.Status != "pending" {
		return errors.New("❌ only pending tickets can be updated")
	}

	// You can customize how much to allow updating
	ticket.SeatID = updateData.SeatID
	ticket.UpdatedAt = time.Now()

	if err := s.DB.UpdateTicket(*ticket); err != nil {
		return fmt.Errorf("failed to update ticket: %w", err)
	}

	fmt.Println("✅ Ticket updated.")
	return nil
}

func (s *TicketService) CancelTicket(id string) error {
	ticket, err := s.DB.GetTicketByID(id)
	if err != nil {
		return fmt.Errorf("ticket %s not found: %w", id, err)
	}

	if ticket.Status != "pending" {
		return errors.New("❌ only pending tickets can be cancelled")
	}

	ticket.Status = "cancelled"
	ticket.UpdatedAt = time.Now()

	if err := s.DB.UpdateTicket(*ticket); err != nil {
		return fmt.Errorf("failed to cancel ticket: %w", err)
	}

	fmt.Println("✅ Ticket cancelled.")
	return nil
}

func (s *TicketService) CheckInTicket(id string) error {
	ticket, err := s.DB.GetTicketByID(id)
	if err != nil {
		return fmt.Errorf("ticket %s not found: %w", id, err)
	}

	if ticket.Status != "completed" {
		return errors.New("❌ only completed tickets can be checked in")
	}

	ticket.CheckedIn = true
	ticket.CheckInTime = time.Now()
	ticket.UpdatedAt = time.Now()

	if err := s.DB.UpdateTicket(*ticket); err != nil {
		return fmt.Errorf("failed to check in ticket: %w", err)
	}

	fmt.Println("✅ Ticket checked in.")
	return nil
}

func (s *TicketService) Checkout(id string) ([]byte, error) {
	// Get the current ticket state
	ticket, err := s.DB.GetTicketByID(id)
	if err != nil {
		return nil, fmt.Errorf("ticket %s not found: %w", id, err)
	}

	if ticket.Status != "pending" {
		return nil, errors.New("❌ only pending tickets can be checked out")
	}

	// Validate all referenced data still exists before checkout
	userExists, err := s.DB.UserExists(ticket.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate user: %w", err)
	}
	if !userExists {
		return nil, errors.New("user does not exist")
	}

	eventExists, err := s.DB.EventExists(ticket.EventID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate event: %w", err)
	}
	if !eventExists {
		return nil, errors.New("event does not exist")
	}

	// Save the original status in case we need to rollback
	originalStatus := ticket.Status

	// Update ticket status to completed
	ticket.Status = "completed"
	ticket.UpdatedAt = time.Now()

	// Create a function to handle rollback
	rollback := func() error {
		ticket.Status = originalStatus
		ticket.UpdatedAt = time.Now()
		if err := s.DB.UpdateTicket(*ticket); err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}
		return nil
	}

	// Update the ticket status first
	if err := s.DB.UpdateTicket(*ticket); err != nil {
		return nil, fmt.Errorf("checkout failed: %w", err)
	}

	fmt.Println("✅ Checkout complete.")

	// Step 1: Generate QR with encryption
	secretKey := os.Getenv("QR_SECRET_KEY")
	qrGen := qr.NewQRGenerator(secretKey)

	qrBytes, err := qrGen.GenerateEncryptedQR(*ticket)
	if err != nil {
		// Rollback the ticket status if QR generation fails
		if rbErr := rollback(); rbErr != nil {
			return nil, fmt.Errorf("failed to generate QR and rollback also failed: %w (rollback error: %v)", err, rbErr)
		}
		return nil, fmt.Errorf("failed to generate QR: %w (status rolled back)", err)
	}

	// Step 2: Generate PDF with QR embedded
	generator := template.NewTicketPDFGenerator()
	pdfBytes, err := generator.Generate(*ticket, qrBytes)
	if err != nil {
		// Rollback the ticket status if PDF generation fails
		if rbErr := rollback(); rbErr != nil {
			return nil, fmt.Errorf("failed to generate PDF and rollback also failed: %w (rollback error: %v)", err, rbErr)
		}
		return nil, fmt.Errorf("failed to generate PDF: %w (status rolled back)", err)
	}

	return pdfBytes, nil
}

func (s *TicketService) UserExists(userID string) (bool, error) {
	exists, err := s.DB.UserExists(userID)
	fmt.Println(userID, exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}
	return exists, nil
}

func (s *TicketService) EventExists(eventID string) (bool, error) {
	exists, err := s.DB.EventExists(eventID)
	if err != nil {
		return false, nil
	}
	return exists, nil
}
