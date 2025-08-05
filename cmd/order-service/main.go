package main

import (
	"context"
	"database/sql"
	"log"
	"ms-ticketing/internal/kafka"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"

	"ms-ticketing/internal/order"
	"ms-ticketing/internal/order/api"
	"ms-ticketing/internal/order/db"
	rediswrap "ms-ticketing/internal/order/redis"
)

func verifyConnections(ctx context.Context) (*bun.DB, *redis.Client) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("[Database] MYSQL_DSN not set")
	}
	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("[Database] Failed to open MySQL: %v", err)
	}
	if err := sqldb.Ping(); err != nil {
		log.Fatalf("[Database] Failed to connect to MySQL: %v", err)
	}
	log.Println("[Database] MySQL connection successful")

	bunDB := bun.NewDB(sqldb, mysqldialect.New())
	db.Migrate()

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("[Database] REDIS_ADDR not set")
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("[Database] Redis connection error: %v", err)
	}
	log.Println("[Database] Redis connection successful")

	return bunDB, redisClient
}

func main() {
	_ = godotenv.Load() // Loads .env file if present

	ctx := context.Background()
	bunDB, redisClient := verifyConnections(ctx)
	defer bunDB.Close()
	defer redisClient.Close()

	// Create Kafka producer
	kafkaProducer := kafka.NewProducer([]string{"localhost:9092"}, "order_created")

	// Pass to service
	service := order.NewOrderService(&db.DB{Bun: bunDB}, rediswrap.NewRedis(redisClient), kafkaProducer)

	handler := &api.Handler{OrderService: service}

	r := chi.NewRouter()
	r.Route("/order", func(r chi.Router) {
		r.Post("/", handler.CreateOrder)
		r.Get("/{orderId}", handler.GetOrder)
		r.Put("/{orderId}", handler.UpdateOrder)
		r.Delete("/{orderId}", handler.DeleteOrder)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Println("🚀 Order Service on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctxShutdown, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctxShutdown)
	log.Println("✅ Order service shutdown complete")
}
