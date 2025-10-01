# MS-Ticketing Unit Tests

This directory contains unit tests for the MS-Ticketing microservice. The tests are organized by component to make it easier to run specific test suites.

## Directory Structure

- `tickets/`: Tests for the ticket service, API handlers, and database operations
- `order/`: Tests for the order service and database operations
- `qr_generator/`: Tests for the QR code generation functionality

## Running the Tests

### Running All Tests

To run all tests, use the following command from the project root:

```bash
go test -v ./Tests/...
```

### Running Tests for a Specific Component

To run tests for a specific component, use one of the following commands:

```bash
# Run ticket service tests
go test -v ./Tests/tickets

# Run order service tests
go test -v ./Tests/order

# Run QR generator tests
go test -v ./Tests/qr_generator
```

### Running a Specific Test

To run a specific test, use the `-run` flag with the test name:

```bash
go test -v ./Tests/tickets -run TestPlaceTicket
```

## Mock Dependencies

The tests use mock implementations of dependencies to avoid external dependencies like databases and Redis. This makes the tests faster and more reliable.

### Common Mock Objects

- `MockTicketDB`: Mock implementation of the TicketDBLayer interface
- `MockOrderDB`: Mock implementation of the DBLayer interface for orders
- `MockRedisLock`: Mock implementation of the RedisLock interface
- `MockKafkaProducer`: Mock implementation of the KafkaProducer interface
- `MockTicketService`: Mock implementation of the TicketService

## Adding New Tests

When adding new tests:

1. Create a new test file in the appropriate directory
2. Follow the naming convention: `component_test.go` (e.g., `service_test.go`, `handler_test.go`)
3. Use existing mock implementations or create new ones as needed
4. Ensure each test function starts with `Test` followed by the name of the functionality being tested
5. Add appropriate assertions to verify the expected behavior

## Integration Tests

Some tests that require external dependencies like Redis or a database are marked with `t.Skip` to avoid failures when running in environments without these dependencies. To run these tests:

1. Ensure the required dependencies are available
2. Remove or comment out the `t.Skip` line in the test
3. Run the test as usual

## Code Coverage

To generate a code coverage report, run:

```bash
go test -v -coverprofile=coverage.out ./Tests/...
go tool cover -html=coverage.out -o coverage.html
```

This will create a `coverage.html` file that you can open in a web browser to see which lines of code are covered by the tests.
