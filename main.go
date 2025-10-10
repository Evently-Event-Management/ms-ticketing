package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/analytics"
	analytics_api "ms-ticketing/internal/analytics/api"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/kafka"
	"ms-ticketing/internal/models"
	ticket_db "ms-ticketing/internal/tickets/db"
	tickets "ms-ticketing/internal/tickets/service"
	"ms-ticketing/internal/tickets/ticket_api"
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
	ctx := context.Background()

	// Ensure keyspace notifications are enabled
	val, err := rdb.ConfigGet(ctx, "notify-keyspace-events").Result()
	if err != nil {
		logger.Error("REDIS", fmt.Sprintf("Failed to get keyspace config: %v", err))
	} else {
		logger.Info("REDIS", fmt.Sprintf("Current keyspace notifications setting: %v", val))
		if len(val) < 2 || !strings.Contains(val[1].(string), "x") || !strings.Contains(val[1].(string), "E") {
			logger.Warn("REDIS", "Keyspace notifications not properly configured for expiry events!")
		}
	}

	pubsub := rdb.PSubscribe(ctx, "__keyevent@0__:expired")
	logger.Info("REDIS", fmt.Sprintf("Subscribed to Redis keyevent expired notifications (DB %d)", rdb.Options().DB))

	go func() {
		for msg := range pubsub.Channel() {
			logger.Info("REDIS", fmt.Sprintf("Received expired key event: %s", msg.Payload))
			if strings.HasPrefix(msg.Payload, "seat_lock:") {
				seatID := strings.TrimPrefix(msg.Payload, "seat_lock:")
				logger.Info("SEAT_UNLOCK", fmt.Sprintf("Seat lock expired for seat: %s", seatID))

				// Get session ID from database
				sessionID, err := db.GetSessionIdBySeat(seatID)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to get session ID for seat %s: %v", seatID, err))
					continue
				}

				// Create SeatStatusChangeEventDto using the proper model
				seatEvent, err := models.NewSeatStatusChangeEventDto(sessionID, []string{seatID}, models.SeatStatusAvailable)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to create seat status event DTO: %v", err))
					continue
				}

				value, err := json.Marshal(seatEvent)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to marshal seat unlock payload: %v", err))
					continue
				}

				// Use the generic Publish method
				err = producer.Publish("ticketly.seats.status", seatID, value)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to publish seat unlock event: %v", err))
					// Try to create the topic if it doesn't exist
					err = kafka.CreateTopicIfNotExists(kafkaBrokers, "ticketly.seats.status")
					if err != nil {
						logger.Error("KAFKA", fmt.Sprintf("Failed to create topic: %v", err))
					} else {
						// Try publishing again
						err = producer.Publish("ticketly.seats.status", seatID, value)
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
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		logger.Fatal("CONFIG", "POSTGRES_DSN not set")
	}

	// Open PostgreSQL with retry logic
	var sqldb *sql.DB
	var err error
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		logger.Info("DATABASE", fmt.Sprintf("Attempting to connect to PostgreSQL (attempt %d/%d)", i+1, maxRetries))
		sqldb, err = sql.Open("postgres", dsn)
		if err != nil {
			logger.Error("DATABASE", fmt.Sprintf("Failed to open PostgreSQL: %v", err))
			time.Sleep(2 * time.Second)
			continue
		}

		err = sqldb.Ping()
		if err == nil {
			break // Connection successful
		}

		logger.Error("DATABASE", fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
		if i < maxRetries-1 {
			time.Sleep(2 * time.Second)
		}
	}

	// Final check after all retries
	if err != nil {
		logger.Fatal("DATABASE", fmt.Sprintf("Failed to connect to PostgreSQL after %d attempts: %v", maxRetries, err))
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
	// Configure Redis keyspace notifications
	_, err = redisClient.ConfigSet(ctx, "notify-keyspace-events", "Ex").Result()
	if err != nil {
		logger.Warn("REDIS", fmt.Sprintf("Failed to enable keyspace notifications: %v", err))
	} else {
		logger.Info("REDIS", "Keyspace notifications enabled for expired events")
	}
	logger.Info("DATABASE", fmt.Sprintf("✅ Redis connection successful to %s (DB: %d)", redisAddr, redisClient.Options().DB))

	return bunDB, redisClient
}

// Secure test handler
func SecureHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id") // injected from AuthMiddleware
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("🔒 Secure endpoint accessed by user: " + userID.(string)))
	if err != nil {
		fmt.Printf("Error writing response: %v", err)
		return
	}
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
		err := bunDB.Close()
		if err != nil {
			logger.Error("DATABASE", fmt.Sprintf("Error closing PostgreSQL connection: %v", err))
			return
		}
		logger.Info("DATABASE", "PostgreSQL connection closed")
	}()
	defer func() {
		err := redisClient.Close()
		if err != nil {
			logger.Error("DATABASE", fmt.Sprintf("Error closing Redis connection: %v", err))
			return
		}
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
		"ticketly.seats.status",
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

	logger.Info("SERVICE", "Initializing analytics service")
	analyticsService := analytics.NewService(bunDB)

	logger.Info("SERVICE", "Initializing order service")
	// Service layer
	orderService := order.NewOrderService(
		&db.DB{Bun: bunDB},
		rediswrap.NewRedis(redisClient, kafkaProducer),
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

	analyticsHandler := analytics_api.NewHandler(analyticsService, logger)

	// main.go

	logger.Info("HTTP", "Setting up router and middleware")
	// Router setup
	r := chi.NewRouter()

	// --- Public Routes ---
	// Define any routes that DO NOT require authentication first.
	// We are getting rid of the separate publicRouter for simplicity.
	r.Get("/api/order/tickets/count", ticketHandler.GetTotalTicketsCount)
	logger.Info("ROUTER", "Public ticket count endpoint registered at /api/orders/tickets/count")

	// --- Protected Routes ---
	// Use a Group to apply middleware to a specific set of routes.
	r.Group(func(r chi.Router) {
		// Apply the JWT middleware ONLY to this group.
		r.Use(auth.Middleware())
		logger.Info("AUTH", "JWT middleware applied to protected API routes")

		// All your protected API routes go inside this group.
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

			// Register analytics routes
			analyticsHandler.RegisterRoutes(r)
			logger.Info("ROUTER", "Analytics routes registered under /api/order/analytics")
		})
	})

	// HTTP Server
	// ... (the rest of your main function remains the same)

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
