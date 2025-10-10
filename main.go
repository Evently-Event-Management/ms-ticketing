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
	_ "github.com/lib/pq"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"ms-ticketing/internal/order"
	"ms-ticketing/internal/order/db"
	"ms-ticketing/internal/order/order_api"
	rediswrap "ms-ticketing/internal/order/redis"

	"ms-ticketing/internal/logger"
)

type DB interface {
	GetSessionIdBySeat(seatID string) (string, error)
	GetOrderBySeat(seatID string) (*models.Order, error)
	UpdateOrder(order models.Order) error
}

func subscribeSeatUnlocks(rdb *redis.Client, producer *kafka.Producer, db DB, logger *logger.Logger, kafkaBrokers []string) {
	ctx := context.Background()

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

				sessionID, err := db.GetSessionIdBySeat(seatID)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to get session ID for seat %s: %v", seatID, err))
					continue
				}

				order, err := db.GetOrderBySeat(seatID)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to get order for seat %s: %v", seatID, err))
				} else {
					logger.Info("SEAT_UNLOCK", fmt.Sprintf("Found order %s for seat %s with status: %s", order.OrderID, seatID, order.Status))
					if order.Status == "pending" {
						order.Status = "cancelled"
						err = db.UpdateOrder(*order)
						if err != nil {
							logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to update order %s to cancelled: %v", order.OrderID, err))
						} else {
							logger.Info("SEAT_UNLOCK", fmt.Sprintf("Order %s status updated to cancelled due to seat lock expiry", order.OrderID))
						}
					} else {
						logger.Info("SEAT_UNLOCK", fmt.Sprintf("Order %s status is '%s', not updating to cancelled (only pending orders are updated)", order.OrderID, order.Status))
					}
				}

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

				err = producer.Publish("ticketly.seats.status", seatID, value)
				if err != nil {
					logger.Error("SEAT_UNLOCK", fmt.Sprintf("Failed to publish seat unlock event: %v", err))
					err = kafka.CreateTopicIfNotExists(kafkaBrokers, "ticketly.seats.status")
					if err != nil {
						logger.Error("KAFKA", fmt.Sprintf("Failed to create topic: %v", err))
					} else {
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

func verifyConnections(ctx context.Context, logger *logger.Logger) (*bun.DB, *redis.Client) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		logger.Fatal("CONFIG", "POSTGRES_DSN not set")
	}

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
			break
		}

		logger.Error("DATABASE", fmt.Sprintf("Failed to connect to PostgreSQL: %v", err))
		if i < maxRetries-1 {
			time.Sleep(2 * time.Second)
		}
	}

	if err != nil {
		logger.Fatal("DATABASE", fmt.Sprintf("Failed to connect to PostgreSQL after %d attempts: %v", maxRetries, err))
	}

	logger.Info("DATABASE", "✅ PostgreSQL connection successful")

	bunDB := bun.NewDB(sqldb, pgdialect.New())

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		logger.Fatal("CONFIG", "REDIS_ADDR not set")
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("DATABASE", fmt.Sprintf("Redis connection error: %v", err))
	}

	_, err = redisClient.ConfigSet(ctx, "notify-keyspace-events", "Ex").Result()
	if err != nil {
		logger.Warn("REDIS", fmt.Sprintf("Failed to enable keyspace notifications: %v", err))
	} else {
		logger.Info("REDIS", "Keyspace notifications enabled for expired events")
	}

	logger.Info("DATABASE", fmt.Sprintf("✅ Redis connection successful to %s (DB: %d)", redisAddr, redisClient.Options().DB))
	return bunDB, redisClient
}

func SecureHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte("🔒 Secure endpoint accessed by user: " + userID.(string)))
	if err != nil {
		fmt.Printf("Error writing response: %v", err)
	}
}

func main() {
	logger := logger.NewLogger()
	defer logger.Close()

	logger.Info("APP", "Starting Order Service initialization")

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
	bunDB, redisClient := verifyConnections(ctx, logger)
	defer bunDB.Close()
	defer redisClient.Close()

	kafkaADDR := os.Getenv("KAFKA_ADDR")
	logger.Info("KAFKA", fmt.Sprintf("Using Kafka address from environment variable: %s", kafkaADDR))
	kafkaBrokers := []string{kafkaADDR}
	kafkaProducer := kafka.NewProducer(kafkaBrokers)
	logger.Info("KAFKA", "Kafka producer initialized successfully")

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

	ticketService := tickets.NewTicketService(&ticket_db.DB{Bun: bunDB})
	analyticsService := analytics.NewService(bunDB)

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

	logger.Info("HTTP", "Setting up router and middleware")
	r := chi.NewRouter()

	// --- Public Routes ---
	r.Get("/api/order/tickets/count", ticketHandler.GetTotalTicketsCount)
	logger.Info("ROUTER", "Public ticket count endpoint registered at /api/order/tickets/count")

	// --- Protected Routes ---
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware())
		logger.Info("AUTH", "JWT middleware applied to protected API routes")

		r.Route("/api", func(r chi.Router) {
			r.Get("/secure", SecureHandler)

			r.Route("/order", func(r chi.Router) {
				r.Post("/", handler.SeatValidationAndPlaceOrder)
				r.Get("/{orderId}", handler.GetOrder)
				r.Put("/{orderId}", handler.UpdateOrder)
				r.Delete("/{orderId}", handler.DeleteOrder)
			})
			logger.Info("ROUTER", "Order routes registered under /api/order")

			r.Route("/order/ticket", func(r chi.Router) {
				r.Get("/", ticketHandler.ListTicketsByOrder)
				r.Get("/{ticketId}", ticketHandler.ViewTicket)
				r.Post("/", ticketHandler.CreateTicket)
				r.Put("/{ticketId}", ticketHandler.UpdateTicket)
				r.Delete("/{ticketId}", ticketHandler.DeleteTicket)
			})
			logger.Info("ROUTER", "Ticket routes registered under /api/order/ticket")

			analyticsHandler.RegisterRoutes(r)
			logger.Info("ROUTER", "Analytics routes registered under /api/order/analytics")
		})
	})

	server := &http.Server{
		Addr:    ":8084",
		Handler: r,
	}

	logger.Info("REDIS", "Starting seat unlock subscription")
	subscribeSeatUnlocks(redisClient, kafkaProducer, &db.DB{Bun: bunDB}, logger, kafkaBrokers)

	go func() {
		logger.Info("HTTP", "🚀 Order Service running on :8084")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("HTTP", fmt.Sprintf("HTTP server error: %v", err))
		}
	}()

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
