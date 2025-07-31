package main

import (
	"context"
	"database/sql"
	"github.com/go-redis/redis/v8"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"ms-ticketing/internal/order"
	"ms-ticketing/internal/order/api"
	"ms-ticketing/internal/order/db"
	"ms-ticketing/internal/order/kafka"
	rediswrap "ms-ticketing/internal/order/redis"
)

func main() {
	ctx := context.Background()

	// --- PostgreSQL Setup ---
	connector := pgdriver.NewConnector(pgdriver.WithDSN("postgres://eventuser:eventpass@localhost:5432/eventdb?sslmode=disable"))
	sqldb := sql.OpenDB(connector)
	defer sqldb.Close()

	if err := sqldb.Ping(); err != nil {
		log.Fatalf("❌ Failed to connect to Postgres: %v", err)
	}

	bunDB := bun.NewDB(sqldb, pgdialect.New())

	// Run migrations
	db.Migrate(bunDB)

	// --- Redis Setup ---
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	log.Println("🔗 Connecting to Redis...")

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}

	// --- Initialize Dependencies ---
	dbLayer := &db.DB{Bun: bunDB}
	redisLock := &rediswrap.Redis{Client: redisClient}
	kafkaProd := &kafka.Producer{} // NOTE: Stub, implement the actual logic
	log.Println("📦 Initializing Order Service...")
	service := order.NewOrderService(dbLayer, redisLock, kafkaProd)
	handler := &api.Handler{OrderService: service}

	// --- Setup Router ---
	r := chi.NewRouter()

	r.Post("/api/v1/orders", handler.CreateOrder)
	r.Get("/api/v1/orders/{orderId}", handler.GetOrder)
	r.Put("/api/v1/orders/{orderId}", handler.UpdateOrder)
	r.Delete("/api/v1/orders/{orderId}", handler.DeleteOrder)

	// --- Start HTTP Server ---
	server := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Println("🚀 Order Service running on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP server error: %v", err)
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("📦 Shutdown signal received. Cleaning up...")

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctxShutdown); err != nil {
		log.Fatalf("❌ Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server exited gracefully")
}
