package storage

import (
	"ms-ticketing/internal/models"
)

type Store interface {
	// Payment operations
	SavePayment(payment *models.Payment) error
	GetPayment(id string) (*models.Payment, error)
	UpdatePayment(payment *models.Payment) error
	ListPayments(merchantID string, limit, offset int) ([]*models.Payment, error)
	GetPaymentByOrderID(OrderID string) (*models.Payment, error)

	// Health and maintenance
	Close() error
	HealthCheck() error
}
