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
- Stripe payment integration for processing payments
- Redis-based caching for machine-to-machine authentication tokens
- Database migrations using golang-migrate

## Token Caching
The service uses Redis to cache M2M (machine-to-machine) authentication tokens, which reduces the number of requests to the authentication server. This is particularly useful in high-traffic scenarios where multiple microservices are communicating with each other. Token caching provides:

- Reduced latency for API calls requiring authentication
- Lower load on the authentication server
- Improved overall system performance

The caching mechanism automatically refreshes tokens before they expire and falls back to direct token requests if Redis is unavailable.

## Architecture
- **internal/order/**: Order service logic
- **internal/tickets/**: Ticket service logic
- **internal/auth/**: Authentication and middleware
- **internal/kafka/**: Kafka producer and consumer
- **internal/models/**: Data models
- **internal/order/stripe.go**: Stripe payment integration
- **internal/database/migrations/**: Database migration system
- **migrations/**: SQL migration files
- **main.go**: Service entrypoint and router setup

## Database Migrations
The application uses [golang-migrate/migrate](https://github.com/golang-migrate/migrate) for database schema management. Migrations are automatically applied when the application starts (configurable via environment variables).

### Migration Files
- Migration files are stored in the `migrations` directory
- Files are named with a version number and description: `000001_init_schema.up.sql`
- Each migration has an "up" file for applying changes and a "down" file for rolling back

### Running Migrations Manually
Use the provided script:

```sh
./scripts/migrate.sh -a [action]
```

Available actions:
- `up`: Apply all pending migrations
- `down`: Rollback the most recent migration
- `version`: Print the current migration version
- `create`: Create a new migration (requires additional name argument)

Examples:
```sh
# Apply all migrations
./scripts/migrate.sh -a up

# Roll back one migration
./scripts/migrate.sh -a down

# Create a new migration
./scripts/migrate.sh -a create -n add_users_table
```

### Migration Configuration
The migration system can be configured using environment variables:
- `MIGRATIONS_DIR`: Directory containing migration files (default: `./migrations`)
- `AUTO_MIGRATE`: Whether to run migrations automatically on startup (default: `true`)
- `SEED_DATA`: Whether to include seed data migrations (default: `false`)

## Setup

### Local Development Setup
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
   - `STRIPE_SECRET_KEY`: Your Stripe API secret key
   - `SEAT_LOCK_TTL_MINUTES`: Duration in minutes for seat locks (default: 5)
   - `STRIPE_WEBHOOK_SECRET`: Your Stripe webhook signing secret
   - `REDIS_ADDR`: Redis address for seat locks and M2M token caching
3. **Run migrations:**
   ```sh
   # Using docker-compose with the migration profile
   docker-compose --profile migrate up migration
   
   # Or, if you prefer to run migrations directly
   go run migrate.go
   ```
4. **Start the service:**
   ```sh
   go run main.go
   ```

### Docker Setup
1. **Build and run with Docker Compose:**
   ```sh
   # Build all services
   docker-compose build

   # Start all services (Postgres, Redis, Kafka, Zookeeper, and ms-ticketing)
   docker-compose up -d
   
   # Run migrations (using the migrate profile)
   docker-compose --profile migrate up migration

   # To build or rebuild just the ms-ticketing service
   docker-compose build ms-ticketing
   
   # View logs
   docker-compose logs -f ms-ticketing
   ```

2. **Build Docker images individually:**
   ```sh
   # Build the main application
   docker build -t ms-ticketing .
   
   # Build the migration image
   docker build -t ms-ticketing-migrate -f Dockerfile.migrate .
   ```

3. **Running services independently:**
   ```sh
   # Run the main application (ensure dependencies are running)
   docker run --network host -e POSTGRES_DSN="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" ms-ticketing
   
   # Run migrations manually
   docker run --network host -e POSTGRES_DSN="postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" ms-ticketing-migrate
   ```

4. **Accessing the service:**
   - The service will be available at http://localhost:8084
   - Kafka UI is available at http://localhost:8081
   - PostgreSQL is available at localhost:5432
   - Redis is available at localhost:6379

5. **Troubleshooting Docker setup:**
   ```sh
   # Check container status
   docker-compose ps
   
   # View detailed logs
   docker-compose logs -f
   
   # Restart a specific service
   docker-compose restart ms-ticketing
   
   # Rebuild and restart a service
   docker-compose up -d --build ms-ticketing
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

## Payment Flow
1. Create an order via `/api/order` endpoint (initial status is "pending")
2. Obtain the order ID from the response
3. Create a payment intent via `/api/order/{orderId}/create-payment-intent`
4. Use the returned client secret with Stripe.js in your frontend to process the payment
5. Upon successful payment, Stripe will call the webhook endpoint which will update the order status to "completed"

## API Endpoints
- `/api/order`: Place, update, cancel, and view orders
- `/api/order/{orderId}/create-payment-intent`: Create a Stripe payment intent for an order
- `/api/webhook/stripe`: Webhook endpoint for Stripe event handling
- `/api/order/ticket`: Create, update, delete, view, and check-in tickets
- `/api/secure`: Test endpoint for JWT authentication

## License
MIT
