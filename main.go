package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/kafka"
	ticket_db "ms-ticketing/internal/tickets/db"
	tickets "ms-ticketing/internal/tickets/service"
	ticket_api "ms-ticketing/internal/tickets/ticket_api"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"ms-ticketing/internal/order"
	"ms-ticketing/internal/order/db"
	"ms-ticketing/internal/order/order_api"
	rediswrap "ms-ticketing/internal/order/redis"

	// Import the logger package
	"ms-ticketing/internal/logger"
)

type DB interface {
	GetSessionIdBySeat(seatID string) (string, error)
}

func subscribeSeatUnlocks(rdb *redis.Client, producer *kafka.Producer, db DB, logger *logger.Logger, kafkaBrokers []string) {
	pubsub := rdb.PSubscribe(context.Background(), "__keyevent@0__:expired")
	go func() {
		for msg := range pubsub.Channel() {
			if strings.HasPrefix(msg.Payload, "seat_lock:") {
				seatID := strings.TrimPrefix(msg.Payload, "seat_lock:")
				logger.Info("SEAT_UNLOCK", fmt.Sprintf("Seat lock expired for seat: %s", seatID))

				// Get session ID from database
				sessionID, err := db.GetSessionIdBySeat(seatID)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to get session ID for seat %s: %v", seatID, err))
					continue
				}

				// Create payload for Kafka with camelCase field names
				payload := map[string]interface{}{
					"sessionId": sessionID,
					"seatIds":   []string{seatID},
				}

				value, err := json.Marshal(payload)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to marshal seat unlock payload: %v", err))
					continue
				}

				// Use the generic Publish method
				err = producer.Publish("ticketly.seats.released", seatID, value)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to publish seat unlock event: %v", err))
					// Try to create the topic if it doesn't exist
					err = kafka.CreateTopicIfNotExists(kafkaBrokers, "ticketly.seats.released")
					if err != nil {
						logger.Error("KAFKA", fmt.Sprintf("Failed to create topic: %v", err))
					} else {
						// Try publishing again
						err = producer.Publish("ticketly.seats.released", seatID, value)
						if err != nil {
							logger.Error("SEAT_UNLOCK", fmt.Sprintf("Still failed to publish after topic creation: %v", err))
						} else {
							logger.Info("KAFKA", fmt.Sprintf("Published seat unlock event for seat: %s after retry", seatID))
						}
					}
				} else {
					logger.Info("KAFKA", fmt.Sprintf("Published seat unlock event for seat: %s", seatID))
				}
			}
		}
	}()
}

// Verify PostgreSQL + Redis connections
func verifyConnections(ctx context.Context, logger *logger.Logger) (*bun.DB, *redis.Client) {
	// PostgreSQL DSN (from env)
	// Example: postgres://postgres:1234@localhost:5432/appdb?sslmode=disable
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		logger.Fatal("CONFIG", "POSTGRES_DSN not set")
	}

	// Open PostgreSQL
	sqldb, err := sql.Open("postgres", dsn)
	if err != nil {
		logger.Fatal("DATABASE", fmt.Sprintf("Failed to open PostgreSQL: %v", err))
	}
	if err := sqldb.Ping(); err != nil {
		logger.Fatal("DATABASE", fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
	}
	logger.Info("DATABASE", "✅ PostgreSQL connection successful")

	// Wrap with Bun
	bunDB := bun.NewDB(sqldb, pgdialect.New())

	// Redis connection
	redisAddr := os.Getenv("REDIS_ADDR") // e.g. localhost:6379
	if redisAddr == "" {
		logger.Fatal("CONFIG", "REDIS_ADDR not set")
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("DATABASE", fmt.Sprintf("Redis connection error: %v", err))
	}
	logger.Info("DATABASE", "✅ Redis connection successful")

	return bunDB, redisClient
}

// Secure test handler
func SecureHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id") // injected from AuthMiddleware
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("🔒 Secure endpoint accessed by user: " + userID.(string)))
}

func main() {
	// Initialize logger first
	logger := logger.NewLogger()
	defer logger.Close()

	logger.Info("APP", "Starting Order Service initialization")

	// Load .env if present
	if err := godotenv.Load(); err != nil {
		logger.Warn("CONFIG", ".env file not found, using environment variables")
	} else {
		logger.Info("CONFIG", "Loaded environment variables from .env file")
	}

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	ctx := context.Background()

	logger.Info("APP", "Verifying database connections")
	// Verify DB + Redis connections
	bunDB, redisClient := verifyConnections(ctx, logger)
	defer func() {
		bunDB.Close()
		logger.Info("DATABASE", "PostgreSQL connection closed")
	}()
	defer func() {
		redisClient.Close()
		logger.Info("DATABASE", "Redis connection closed")
	}()

	logger.Info("KAFKA", "Initializing Kafka producer")
	// Kafka producer
	kafkaADDR := os.Getenv("KAFKA_ADDR")
	logger.Info("KAFKA", fmt.Sprintf("Using Kafka address from environment variable: %s", kafkaADDR))
	kafkaBrokers := []string{kafkaADDR}
	logger.Info("KAFKA", fmt.Sprintf("Kafka brokers configured: %v", kafkaBrokers))
	kafkaProducer := kafka.NewProducer(kafkaBrokers)
	logger.Info("KAFKA", "Kafka producer initialized successfully")

	// Ensure Kafka topics exist
	logger.Info("KAFKA", "Ensuring required topics exist")
	requiredTopics := []string{
		"ticketly.order.created",
		"ticketly.order.updated",
		"ticketly.order.canceled",
		"ticketly.seats.locked",
		"ticketly.seats.released",
		"payment_succefully",
		"payment_unseecuufull",
	}
	if err := kafka.EnsureTopicsExist(kafkaBrokers, requiredTopics); err != nil {
		logger.Warn("KAFKA", fmt.Sprintf("Topic creation might have failed: %v", err))
	} else {
		logger.Info("KAFKA", "Required topics ensured successfully")
	}

	logger.Info("SERVICE", "Initializing ticket service")
	ticketService := tickets.NewTicketService(&ticket_db.DB{Bun: bunDB})

	logger.Info("SERVICE", "Initializing order service")
	// Service layer
	orderService := order.NewOrderService(
		&db.DB{Bun: bunDB},
		rediswrap.NewRedis(redisClient),
		kafkaProducer,
		ticketService,
		client,
	)

	handler := &order_api.Handler{
		OrderService: orderService,
		Logger:       logger,
	}

	ticketHandler := &ticket_api.Handler{
		TicketService: ticketService,
	}

	logger.Info("HTTP", "Setting up router and middleware")
	// Router setup
	r := chi.NewRouter()

	// Apply JWT middleware globally
	r.Use(auth.Middleware())
	logger.Info("AUTH", "JWT middleware applied globally")

	// API Routes with /api prefix
	r.Route("/api", func(r chi.Router) {
		// Secure test route
		r.Get("/secure", SecureHandler)

		// Order routes with /order prefix
		r.Route("/order", func(r chi.Router) {
			r.Post("/", handler.SeatValidationAndPlaceOrder)
			r.Get("/{orderId}", handler.GetOrder)
			r.Put("/{orderId}", handler.UpdateOrder)
			r.Delete("/{orderId}", handler.DeleteOrder)
		})
		logger.Info("ROUTER", "Order routes registered under /api/order")

		// Ticket routes under order service
		r.Route("/order/ticket", func(r chi.Router) {
			r.Get("/", ticketHandler.ListTicketsByOrder)
			r.Get("/{ticketId}", ticketHandler.ViewTicket)
			r.Post("/", ticketHandler.CreateTicket)
			r.Put("/{ticketId}", ticketHandler.UpdateTicket)
			r.Delete("/{ticketId}", ticketHandler.DeleteTicket)
		})
		logger.Info("ROUTER", "Ticket routes registered under /api/order/ticket")
	})

	// HTTP Server
	server := &http.Server{
		Addr:    ":8084",
		Handler: r,
	}

	// Start seat unlock subscription
	logger.Info("REDIS", "Starting seat unlock subscription")
	subscribeSeatUnlocks(redisClient, kafkaProducer, &db.DB{Bun: bunDB}, logger, kafkaBrokers)

	// Start server
	go func() {
		logger.Info("HTTP", "🚀 Order Service running on :8084")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP", fmt.Sprintf("HTTP server error: %v", err))
		}
	}()

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	logger.Info("APP", "Service started successfully, waiting for shutdown signal")
	<-stop

	logger.Info("APP", "Shutdown signal received, initiating graceful shutdown")
	ctxShutdown, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Error("HTTP", fmt.Sprintf("Server Shutdown Failed: %v", err))
	} else {
		logger.Info("HTTP", "✅ Order Service shutdown complete")
	}
}
