# Ticketly Microservices - Testing

This document provides information on how to run the tests for the Ticketly ticketing microservice.

## Prerequisites

- Go 1.18 or later
- Access to the project dependencies (either internet access for downloading or vendor directory)

## Running Tests

### Running All Tests

To run all tests in the project:

```bash
go test ./...
```

### Running Tests for Specific Packages

To run tests for a specific package:

```bash
# For order service tests
go test ./internal/order/...

# For ticket service tests
go test ./internal/tickets/...

# For a specific test file
go test ./internal/order/service_test.go
```

### Running Tests with Coverage

To run tests and get coverage information:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

Then open `coverage.html` in a browser to see coverage details.

## Test Structure

The test suite is organized as follows:

### Order Service Tests
- `internal/order/service_test.go` - Tests for the order service functionality
- `internal/order/stripe_test.go` - Tests for the payment processing functionality
- `internal/order/db/db_test.go` - Tests for the order database layer

### Ticket Service Tests
- `internal/tickets/service/service_test.go` - Tests for the ticket service functionality
- `internal/tickets/service/ticket_count_test.go` - Tests for the ticket counting service
- `internal/tickets/db/db_test.go` - Tests for the ticket database layer
- `internal/tickets/db/ticket_count_test.go` - Tests for the ticket count database layer

## Test Dependencies

The tests use the following libraries:
- `github.com/stretchr/testify` for assertions and mocking
- `github.com/google/uuid` for generating IDs
- In-memory SQLite database for database tests

## Adding New Tests

When adding new functionality, please follow these guidelines for adding tests:

1. Unit tests should be written for all new functions
2. Use mocks for external dependencies
3. Follow the existing naming pattern: `TestFunctionName`
4. For database tests, use the in-memory SQLite setup
5. Make sure to test error cases, not just happy paths

## Troubleshooting

### Common Issues

1. **Missing dependencies**

   If you see errors about missing packages:

   ```bash
   go get -u github.com/stretchr/testify
   ```

2. **Database setup issues**

   For database tests, make sure the required tables are created in the test setup function.

3. **Test times out**

   Some tests may have implicit timeouts. Use `-timeout` flag to extend:

   ```bash
   go test ./... -timeout 5m
   ```

For more assistance, please contact the development team.