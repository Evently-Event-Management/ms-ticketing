# ms-ticketing

A microservice-based ticketing system for event management, built with Go, Kafka, PostgreSQL, Redis, and Keycloak for authentication.

## Features
- Order management (create, update, cancel, checkout)
- Ticket management (create, update, delete, view, check-in)
- Seat validation and locking
- Kafka integration for event-driven workflows
- Redis for seat locking
- Keycloak for authentication (OIDC)
- QR code generation for tickets

## Architecture
- **internal/order/**: Order service logic
- **internal/tickets/**: Ticket service logic
- **internal/auth/**: Authentication and middleware
- **internal/kafka/**: Kafka producer and consumer
- **internal/models/**: Data models
- **db.go**: Database layer for tickets and orders
- **main.go**: Service entrypoint and router setup
- **migrate.go**: Database migration and sample data

## Setup
1. **Clone the repo:**
   ```sh
   git clone https://github.com/Evently-Event-Management/ms-ticketing.git
   cd ms-ticketing
   ```
2. **Configure environment variables:**
   - `POSTGRES_DSN`: PostgreSQL connection string
   - `REDIS_ADDR`: Redis address
   - `KEYCLOAK_URL`, `KEYCLOAK_REALM`, etc. for authentication
   - `QR_SECRET_KEY`: Secret for QR code encryption
   - `SEAT_SERVICE_URL`: Seat validation service URL
3. **Run migrations:**
   ```sh
   go run migrate.go
   ```
4. **Start the service:**
   ```sh
   go run main.go
   ```

## Kafka Consumers
To consume multiple topics (e.g., `payment_succefully`, `payment_unseecuufull`), use the following pattern:

```go
import (
    "context"
    "log"
    "github.com/segmentio/kafka-go"
)

func consumeTopic(brokers []string, topic string, groupID string) {
    r := kafka.NewReader(kafka.ReaderConfig{
        Brokers: brokers,
        Topic:   topic,
        GroupID: groupID,
    })
    defer r.Close()

    for {
        m, err := r.ReadMessage(context.Background())
        if err != nil {
            log.Printf("error reading message from %s: %v", topic, err)
            continue
        }
        log.Printf("message from %s: %s", topic, string(m.Value))
        // handle message
    }
}

func main() {
    brokers := []string{"localhost:9092"}
    go consumeTopic(brokers, "payment_succefully", "my-group")
    go consumeTopic(brokers, "payment_unseecuufull", "my-group")
    select {} // block forever
}
```

## Sample Data
The migration file creates a sample ticket with a QR code:
```sql
INSERT INTO tickets (ticket_id, order_id, seat_id, seat_label, colour, tier_id, tier_name, qr_code, price_at_purchase, issued_at, checked_in, checked_in_time)
VALUES (
    '11111111-1111-1111-1111-111111111111',
    '22222222-2222-2222-2222-222222222222',
    'A1',
    'A1',
    '#FFD700',
    '33333333-3333-3333-3333-333333333333',
    'VIP',
    decode('48656c6c6f20515221', 'hex'), -- 'Hello QR!'
    100.00,
    NOW(),
    FALSE,
    NULL
);
```

## API Endpoints
- `/order`: Place, update, cancel, and view orders
- `/ticket`: Create, update, delete, view, and check-in tickets
- `/secure`: Test endpoint for JWT authentication

## License
MIT
