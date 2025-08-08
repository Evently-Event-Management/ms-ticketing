package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"

	"ms-ticketing/internal/tickets"
	"ms-ticketing/internal/tickets/db"
	"ms-ticketing/internal/tickets/ticket_api"
)

func verifyConnections(ctx context.Context) *bun.DB {
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

	return bunDB
}

func main() {
	_ = godotenv.Load() // Loads .env file if present

	ctx := context.Background()
	bunDB := verifyConnections(ctx)
	defer bunDB.Close()

	service := tickets.NewTicketService(&db.DB{Bun: bunDB})
	handler := &ticket_api.Handler{TicketService: service}

	r := chi.NewRouter()
	r.Route("/ticket", func(r chi.Router) {
		r.Get("/checkout/{ticketID}", handler.CheckoutTicket)

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
