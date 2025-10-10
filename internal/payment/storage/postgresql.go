package storage

import (
	"database/sql"
	"fmt"
	"ms-ticketing/internal/config"
	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"

	_ "github.com/lib/pq"
)

type PostgreSQLStore struct {
	db  *sql.DB
	log *logger.Logger
}

// NewPostgreSQLStoreWithDB creates a new PostgreSQL store using an existing database connection
func NewPostgreSQLStoreWithDB(db *sql.DB, log *logger.Logger) (*PostgreSQLStore, error) {
	log.Info("DATABASE", "Creating payment storage with existing database connection")

	store := &PostgreSQLStore{
		db:  db,
		log: log,
	}

	// Initialize tables
	if err := store.initTables(); err != nil {
		log.Error("DATABASE", "Failed to initialize payment tables: "+err.Error())
		return nil, fmt.Errorf("failed to initialize payment tables: %w", err)
	}

	log.Info("DATABASE", "Payment storage initialized successfully with existing connection")
	return store, nil
}

func NewPostgreSQLStore(cfg config.DatabaseConfig, log *logger.Logger) (*PostgreSQLStore, error) {
	log.LogDatabase("CONNECT", "postgresql", fmt.Sprintf("Connecting to PostgreSQL at %s:%s", cfg.Host, cfg.Port))

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Error("DATABASE", "Failed to open PostgreSQL connection: "+err.Error())
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.MaxLifetime)

	// Test connection
	if err := db.Ping(); err != nil {
		log.Error("DATABASE", "Failed to ping PostgreSQL: "+err.Error())
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &PostgreSQLStore{
		db:  db,
		log: log,
	}

	// Initialize tables
	if err := store.initTables(); err != nil {
		log.Error("DATABASE", "Failed to initialize tables: "+err.Error())
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	log.LogDatabase("SUCCESS", "postgresql", "PostgreSQL connection established and tables initialized")
	return store, nil
}

func (s *PostgreSQLStore) initTables() error {
	s.log.LogDatabase("MIGRATE", "postgresql", "Creating payments table if not exists")

	// Create payments table with PostgreSQL syntax
	paymentsQuery := `
    CREATE TABLE IF NOT EXISTS payments (
        payment_id VARCHAR(36) PRIMARY KEY,
        order_id VARCHAR(36) NOT NULL,
        status VARCHAR(50) NOT NULL,
        price DECIMAL(10,2) NOT NULL,
        created_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        url VARCHAR(500)
    );
    `

	if _, err := s.db.Exec(paymentsQuery); err != nil {
		return fmt.Errorf("failed to create payments table: %w", err)
	}

	// Create indexes
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);",
		"CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);",
		"CREATE INDEX IF NOT EXISTS idx_payments_created_date ON payments(created_date);",
	}

	for _, indexQuery := range indexes {
		if _, err := s.db.Exec(indexQuery); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	s.log.LogDatabase("SUCCESS", "postgresql", "Payment tables and indexes ready")
	return nil
}

// SavePayment saves a payment to the database
func (s *PostgreSQLStore) SavePayment(payment *models.Payment) error {
	s.log.LogDatabase("INSERT", "postgresql", fmt.Sprintf("Saving payment %s", payment.PaymentID))

	query := `
    INSERT INTO payments (
        payment_id, order_id, status, price, created_date, url
    ) VALUES ($1, $2, $3, $4, $5, $6)
    `

	_, err := s.db.Exec(query,
		payment.PaymentID, payment.OrderID, payment.Status, payment.Price, payment.CreatedDate, payment.URL,
	)

	if err != nil {
		s.log.Error("DATABASE", fmt.Sprintf("Failed to save payment %s: %s", payment.PaymentID, err.Error()))
		return fmt.Errorf("failed to save payment: %w", err)
	}

	s.log.LogDatabase("SUCCESS", "postgresql", fmt.Sprintf("Payment %s saved successfully", payment.PaymentID))
	return nil
}

// GetPayment retrieves a payment by ID
func (s *PostgreSQLStore) GetPayment(id string) (*models.Payment, error) {
	s.log.LogDatabase("SELECT", "postgresql", fmt.Sprintf("Fetching payment %s", id))

	query := `
    SELECT payment_id, order_id, status, price, created_date, url
    FROM payments WHERE payment_id = $1
    `

	payment := &models.Payment{}
	err := s.db.QueryRow(query, id).Scan(
		&payment.PaymentID, &payment.OrderID, &payment.Status, &payment.Price, &payment.CreatedDate, &payment.URL,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			s.log.LogDatabase("NOT_FOUND", "postgresql", fmt.Sprintf("Payment %s not found", id))
			return nil, fmt.Errorf("payment not found")
		}
		s.log.Error("DATABASE", fmt.Sprintf("Failed to get payment %s: %s", id, err.Error()))
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	s.log.LogDatabase("SUCCESS", "postgresql", fmt.Sprintf("Payment %s fetched successfully", id))
	return payment, nil
}

// UpdatePayment updates a payment in the database
func (s *PostgreSQLStore) UpdatePayment(payment *models.Payment) error {
	s.log.LogDatabase("UPDATE", "postgresql", fmt.Sprintf("Updating payment %s", payment.PaymentID))

	query := `
    UPDATE payments SET
        order_id = $1, status = $2, price = $3, url = $4
    WHERE payment_id = $5
    `

	_, err := s.db.Exec(query,
		payment.OrderID, payment.Status, payment.Price, payment.URL, payment.PaymentID,
	)

	if err != nil {
		s.log.Error("DATABASE", fmt.Sprintf("Failed to update payment %s: %s", payment.PaymentID, err.Error()))
		return fmt.Errorf("failed to update payment: %w", err)
	}

	s.log.LogDatabase("SUCCESS", "postgresql", fmt.Sprintf("Payment %s updated successfully", payment.PaymentID))
	return nil
}

// ListPayments retrieves payments for a specific order
func (s *PostgreSQLStore) ListPayments(merchantID string, limit, offset int) ([]*models.Payment, error) {
	s.log.LogDatabase("SELECT", "postgresql", fmt.Sprintf("Listing payments for order %s (limit: %d, offset: %d)", merchantID, limit, offset))

	query := `
    SELECT payment_id, order_id, status, price, created_date, url
    FROM payments 
    WHERE order_id = $1 
    ORDER BY created_date DESC 
    LIMIT $2 OFFSET $3
    `

	rows, err := s.db.Query(query, merchantID, limit, offset)
	if err != nil {
		s.log.Error("DATABASE", fmt.Sprintf("Failed to list payments: %s", err.Error()))
		return nil, fmt.Errorf("failed to list payments: %w", err)
	}
	defer rows.Close()

	var payments []*models.Payment
	for rows.Next() {
		payment := &models.Payment{}
		err := rows.Scan(
			&payment.PaymentID, &payment.OrderID, &payment.Status, &payment.Price, &payment.CreatedDate, &payment.URL,
		)

		if err != nil {
			s.log.Error("DATABASE", fmt.Sprintf("Failed to scan payment row: %s", err.Error()))
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}

		payments = append(payments, payment)
	}

	if err = rows.Err(); err != nil {
		s.log.Error("DATABASE", fmt.Sprintf("Row iteration error: %s", err.Error()))
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	s.log.LogDatabase("SUCCESS", "postgresql", fmt.Sprintf("Listed %d payments for order %s", len(payments), merchantID))
	return payments, nil
}

func (s *PostgreSQLStore) Close() error {
	s.log.LogDatabase("CLOSE", "postgresql", "Closing PostgreSQL connection")
	return s.db.Close()
}

func (s *PostgreSQLStore) HealthCheck() error {
	return s.db.Ping()
}

// GetPaymentByOrderID retrieves a payment by order ID
func (s *PostgreSQLStore) GetPaymentByOrderID(orderID string) (*models.Payment, error) {
	s.log.LogDatabase("SELECT", "postgresql", fmt.Sprintf("Fetching payment for OrderID %s", orderID))

	query := `
    SELECT payment_id, order_id, status, price, created_date, url
    FROM payments WHERE order_id = $1
    `

	payment := &models.Payment{}
	err := s.db.QueryRow(query, orderID).Scan(
		&payment.PaymentID, &payment.OrderID, &payment.Status, &payment.Price, &payment.CreatedDate, &payment.URL,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			s.log.LogDatabase("NOT_FOUND", "postgresql", fmt.Sprintf("Payment not found for OrderID %s", orderID))
			return nil, fmt.Errorf("payment not found")
		}
		s.log.Error("DATABASE", fmt.Sprintf("Failed to get payment %s: %s", orderID, err.Error()))
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	s.log.LogDatabase("SUCCESS", "postgresql", fmt.Sprintf("Payment %s fetched successfully for OrderID %s", payment.PaymentID, orderID))
	return payment, nil
}
