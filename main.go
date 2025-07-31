package main

import (
	"context"
	"log"
	"net/http"

	"ms-ticketing/internal/order"
	"ms-ticketing/internal/order/api"
	_ "ms-ticketing/internal/order/db"
	"ms-ticketing/internal/order/kafka"
	"ms-ticketing/internal/order/redis"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

func main() {
	// PostgreSQL connection
	dsn := "postgres://postgres:postgres@localhost:5432/orderdb?sslmode=disable"
	pgConn := pgdriver.NewConnector(pgdriver.WithDSN(dsn))
	db := bun.NewDB(pgConn, pgdialect.New())

	// Redis connection
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Kafka Producer (stub for now)
	kafkaProd := &kafka.Producer{} // TODO: Add real implementation

	// Layer init
	dbLayer := &db.DB{Bun: db}
	redisLock := &redis.Redis{Client: redisClient}
	orderService := order.NewOrderService(dbLayer, redisLock, kafkaProd)
	handler := &api.Handler{OrderService: orderService}

	// Routes
	http.HandleFunc("/order", handler.CreateOrder)

	log.Println("Order Service running at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
