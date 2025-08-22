package main

import (
	"context"
	"database/sql"
	"log"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/kafka"
	ticket_db "ms-ticketing/internal/tickets/db"
	tickets "ms-ticketing/internal/tickets/service"
	"ms-ticketing/internal/tickets/ticket_api"
	"net/http"
	"os"
	"os/signal"
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
)

// Verify PostgreSQL + Redis connections
func verifyConnections(ctx context.Context) (*bun.DB, *redis.Client) {
	// PostgreSQL DSN (from env)
	// Example: postgres://postgres:1234@localhost:5432/appdb?sslmode=disable
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("[Database] POSTGRES_DSN not set")
	}

	// Open PostgreSQL
	sqldb, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("[Database] Failed to open PostgreSQL: %v", err)
	}
	if err := sqldb.Ping(); err != nil {
		log.Fatalf("[Database] Failed to connect to PostgreSQL: %v", err)
	}
	log.Println("[Database] ✅ PostgreSQL connection successful")

	// Wrap with Bun
	bunDB := bun.NewDB(sqldb, pgdialect.New())

	// Redis connection
	redisAddr := os.Getenv("REDIS_ADDR") // e.g. localhost:6379
	if redisAddr == "" {
		log.Fatal("[Database] REDIS_ADDR not set")
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("[Database] Redis connection error: %v", err)
	}
	log.Println("[Database] ✅ Redis connection successful")

	return bunDB, redisClient
}

// Secure test handler
func SecureHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id") // injected from AuthMiddleware
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("🔒 Secure endpoint accessed by user: " + userID.(string)))
}

func main() {
	// Load .env if present
	_ = godotenv.Load()

	client := &http.Client{
		Timeout: time.Second * 10,
	}
	ctx := context.Background()

	// Verify DB + Redis connections
	bunDB, redisClient := verifyConnections(ctx)
	defer bunDB.Close()
	defer redisClient.Close()

	// Kafka producer
	kafkaProducer := kafka.NewProducer([]string{"localhost:9092"}, "order_created")

	ticketService := tickets.NewTicketService(&ticket_db.DB{Bun: bunDB})
	// Service layer
	orderService := order.NewOrderService(
		&db.DB{Bun: bunDB},
		rediswrap.NewRedis(redisClient),
		kafkaProducer,
		ticketService,
		client,
	)

	handler := &order_api.Handler{OrderService: orderService}
	ticketHandler := &ticket_api.Handler{TicketService: ticketService}
	// Router setup
	r := chi.NewRouter()

	// Apply JWT middleware globally
	r.Use(auth.Middleware())

	// Secure test route
	r.Get("/secure", SecureHandler)

	// Order routes
	r.Route("/order", func(r chi.Router) {
		r.Post("/", handler.SeatValidationAndPlaceOrder)
		r.Get("/{orderId}", handler.GetOrder)
		r.Put("/{orderId}", handler.UpdateOrder)
		r.Delete("/{orderId}", handler.DeleteOrder)
	})

	r.Route("/ticket", func(r chi.Router) {
		r.Get("/", ticketHandler.ListTicketsByOrder)
		r.Get("/{ticketId}", ticketHandler.ViewTicket)
		r.Post("/", ticketHandler.CreateTicket)
		r.Put("/{ticketId}", ticketHandler.UpdateTicket)
		r.Delete("/{ticketId}", ticketHandler.DeleteTicket)
	})

	// HTTP Server
	server := &http.Server{
		Addr:    ":8083",
		Handler: r,
	}

	// Start server
	go func() {
		log.Println("🚀 Order Service running on :8083")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctxShutdown, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxShutdown); err != nil {
		log.Fatalf("❌ Server Shutdown Failed: %v", err)
	}
	log.Println("✅ Order Service shutdown complete")
}
