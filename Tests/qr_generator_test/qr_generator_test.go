package qr_generator_test

import (
	"encoding/json"
	"ms-ticketing/internal/models"
	qr_generator "ms-ticketing/internal/tickets/qr_genrator"
	"testing"
	"time"
)

func TestQRGenerator(t *testing.T) {
	// Create a new QR generator with a test secret
	secret := "test-secret-key"
	qrGen := qr_generator.NewQRGenerator(secret)

	// Create a sample ticket
	ticket := models.Ticket{
		TicketID:        "test-ticket-id",
		OrderID:         "test-order-id",
		SeatID:          "test-seat-id",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
	}

	// Generate QR code
	qrBytes, err := qrGen.GenerateEncryptedQR(ticket)
	if err != nil {
		t.Fatalf("Failed to generate QR code: %v", err)
	}

	// Verify QR code is not empty
	if len(qrBytes) == 0 {
		t.Error("Generated QR code is empty")
	}
}

func TestQRGeneratorWithDifferentTickets(t *testing.T) {
	// Create a new QR generator with a test secret
	secret := "test-secret-key"
	qrGen := qr_generator.NewQRGenerator(secret)

	// Create two different tickets
	ticket1 := models.Ticket{
		TicketID:        "ticket1",
		OrderID:         "order1",
		SeatID:          "seat1",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
	}

	ticket2 := models.Ticket{
		TicketID:        "ticket2",
		OrderID:         "order1",
		SeatID:          "seat2",
		SeatLabel:       "A2",
		Colour:          "red",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
	}

	// Generate QR codes for both tickets
	qrBytes1, err := qrGen.GenerateEncryptedQR(ticket1)
	if err != nil {
		t.Fatalf("Failed to generate QR code for ticket1: %v", err)
	}

	qrBytes2, err := qrGen.GenerateEncryptedQR(ticket2)
	if err != nil {
		t.Fatalf("Failed to generate QR code for ticket2: %v", err)
	}

	// Verify QR codes are different for different tickets
	if string(qrBytes1) == string(qrBytes2) {
		t.Error("QR codes for different tickets should be different")
	}
}

func TestQRGeneratorConsistency(t *testing.T) {
	// Create a new QR generator with a test secret
	secret := "test-secret-key"
	qrGen := qr_generator.NewQRGenerator(secret)

	// Create a sample ticket with a fixed time to ensure consistency
	fixedTime := time.Date(2025, 9, 13, 12, 0, 0, 0, time.UTC)
	ticket := models.Ticket{
		TicketID:        "consistency-test",
		OrderID:         "order1",
		SeatID:          "seat1",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 100.0,
		IssuedAt:        fixedTime,
	}

	// Generate QR code twice
	qrBytes1, err := qrGen.GenerateEncryptedQR(ticket)
	if err != nil {
		t.Fatalf("Failed to generate first QR code: %v", err)
	}

	// Verify the ticket can be serialized to JSON (which is what happens inside the QR generator)
	_, err = json.Marshal(ticket)
	if err != nil {
		t.Fatalf("Failed to marshal ticket to JSON: %v", err)
	}

	// The QR codes should be different each time due to the random IV used in AES encryption
	qrBytes2, err := qrGen.GenerateEncryptedQR(ticket)
	if err != nil {
		t.Fatalf("Failed to generate second QR code: %v", err)
	}

	// Due to the random IV used in AES encryption, each generated QR code should be different
	// even for the same ticket
	if string(qrBytes1) == string(qrBytes2) {
		t.Error("QR codes should be different due to random IV in encryption")
	}
}

func TestQRGeneratorWithDifferentSecrets(t *testing.T) {
	// Create two QR generators with different secrets
	secret1 := "test-secret-key-1"
	secret2 := "test-secret-key-2"

	qrGen1 := qr_generator.NewQRGenerator(secret1)
	qrGen2 := qr_generator.NewQRGenerator(secret2)

	// Create a sample ticket
	ticket := models.Ticket{
		TicketID:        "test-ticket-id",
		OrderID:         "test-order-id",
		SeatID:          "test-seat-id",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
	}

	// Generate QR codes using both generators
	qrBytes1, err := qrGen1.GenerateEncryptedQR(ticket)
	if err != nil {
		t.Fatalf("Failed to generate QR code with first secret: %v", err)
	}

	qrBytes2, err := qrGen2.GenerateEncryptedQR(ticket)
	if err != nil {
		t.Fatalf("Failed to generate QR code with second secret: %v", err)
	}

	// QR codes generated with different secrets should be different
	if string(qrBytes1) == string(qrBytes2) {
		t.Error("QR codes generated with different secrets should be different")
	}
}
