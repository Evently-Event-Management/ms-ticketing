package template

import (
	"bytes"
	"fmt"
	"image/png"
	"ms-ticketing/internal/models"
	"strings"

	"github.com/signintech/gopdf"
)

type TicketPDFGenerator struct{}

func NewTicketPDFGenerator() *TicketPDFGenerator {
	return &TicketPDFGenerator{}
}

func (g *TicketPDFGenerator) Generate(ticket models.Ticket, qrCode []byte) ([]byte, error) {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4}) // A4 size
	pdf.AddPage()

	err := pdf.AddTTFFont("dejavu", "./fonts/DejaVuSans.ttf")
	if err != nil {
		return nil, fmt.Errorf("failed to load font: %w", err)
	}

	err = pdf.SetFont("dejavu", "", 14)
	if err != nil {
		return nil, fmt.Errorf("failed to set font: %w", err)
	}

	// Header
	addHeader(pdf)

	// Ticket Info
	pdf.SetY(60)
	addTicketInfo(pdf, ticket)

	// QR Code
	if len(qrCode) > 0 {
		pdf.SetY(pdf.GetY() + 20)
		addQRCode(pdf, qrCode)
	}

	// Footer
	pdf.SetY(260)
	addFooter(pdf)

	// Output
	var buf bytes.Buffer
	err = pdf.Write(&buf)
	if err != nil {
		return nil, fmt.Errorf("failed to write PDF: %w", err)
	}

	return buf.Bytes(), nil
}

func addHeader(pdf *gopdf.GoPdf) {
	pdf.SetX(40)
	pdf.SetY(30)
	pdf.Cell(nil, "🎟️ EVENT TICKET")
}

func addTicketInfo(pdf *gopdf.GoPdf, ticket models.Ticket) {
	info := []struct {
		Label string
		Value string
	}{
		{"Ticket ID", ticket.ID},
		{"Event ID", ticket.EventID},
		{"User ID", ticket.UserID},
		{"Seat ID", ticket.SeatID},
		{"Status", strings.Title(ticket.Status)},
		{"Created At", ticket.CreatedAt.Format("2006-01-02 15:04")},
		{"Checked In", fmt.Sprintf("%v", ticket.CheckedIn)},
	}

	for _, item := range info {
		pdf.Cell(nil, item.Label+": "+item.Value)
		pdf.Br(20)
	}
}

func addQRCode(pdf *gopdf.GoPdf, qrCode []byte) {
	img, err := png.Decode(bytes.NewReader(qrCode))
	if err != nil {
		pdf.Cell(nil, "Failed to load QR code")
		return
	}

	rect := &gopdf.Rect{W: 100, H: 100}
	err = pdf.ImageFrom(img, 100, pdf.GetY(), rect)
	if err != nil {
		pdf.Cell(nil, "Failed to draw QR code")
	}
}

func addFooter(pdf *gopdf.GoPdf) {
	pdf.SetX(50)
	pdf.Cell(nil, "Thank you for using our ticketing system.")
}
