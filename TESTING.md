# Ticketly Microservices - Testing Guide

This document provides comprehensive information on the testing approach, methodology, and instructions for the Ticketly ticketing microservice.

## Testing Philosophy

The Ticketly ticketing microservice follows a comprehensive testing strategy to ensure reliable and robust code. Our testing approach is based on the following principles:

- **Test-Driven Development (TDD)**: We encourage writing tests before implementation
- **Comprehensive Coverage**: Aim for high code coverage (target: >80%)
- **Isolation**: Services are tested in isolation using mocks for external dependencies
- **Realistic Scenarios**: Tests are designed to model real-world use cases
- **Continuous Testing**: Tests run automatically in our CI/CD pipeline

## Testing Categories

The project implements several testing categories:

1. **Unit Tests**: Testing individual components in isolation
2. **Integration Tests**: Testing interaction between components
3. **Mock Tests**: Using mock implementations to simulate external dependencies
4. **Database Tests**: Testing database operations with in-memory SQLite 
5. **API Tests**: Testing HTTP endpoints

## Prerequisites

- Go 1.18 or later
- Access to project dependencies (either internet access for downloading or vendor directory)
- PostgreSQL client (for integration tests)
- Redis client (for integration tests)
- Docker (optional, for containerized testing)

## Running Tests

### Using the Test Script

For convenience, use the provided test script:

```bash
# Make script executable if needed
chmod +x run_tests.sh

# Run all tests with coverage reporting
./run_tests.sh
```

The test script provides verbose output with real-time test status:
- ✓ PASSED: [Test Name] - For successful tests
- ✗ FAILED: [Test Name] - For failed tests

At the end, you'll see a summary showing:
- Total number of tests
- Number of passed tests
- Number of failed tests (if any)

### Running All Tests Manually

To run all tests in the project:

```bash
go test ./...
```

### Running Tests for Specific Packages

To run tests for specific packages:

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
go test ./... -coverprofile=coverage/coverage.out
go tool cover -html=coverage/coverage.out -o coverage/coverage.html
```

Then open `coverage/coverage.html` in a browser to see coverage details.

### Running Tests in Docker

You can run tests in a containerized environment:

```bash
# Build and run tests in Docker
docker build -t ms-ticketing-tests -f Dockerfile.test .
docker run --rm ms-ticketing-tests
```

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

## Testing Libraries and Tools

The project leverages the following testing libraries:

- **Standard Go Testing**: Using the built-in `testing` package
- **Testify**: 
  - `github.com/stretchr/testify/assert` for assertions
  - `github.com/stretchr/testify/require` for fatal assertions
  - `github.com/stretchr/testify/mock` for mocking
- **SQLite**: For in-memory database testing
- **UUID**: `github.com/google/uuid` for generating test identifiers
- **Custom Mocks**: Handwritten mocks for various interfaces

## Mocking Strategy

The project uses interface-based design to facilitate mocking:

1. **Service Dependencies**: All service dependencies are defined as interfaces
2. **Mock Implementations**: Mock implementations are created using testify/mock
3. **Behavior Simulation**: Mocks are configured to simulate specific behaviors
4. **Verification**: Mock expectations verify that dependencies are called correctly

Example of a mock implementation:

```go
// MockDBLayer implements the OrderDBLayer interface for testing
type MockDBLayer struct {
    mock.Mock
}

func (m *MockDBLayer) GetOrderByID(id string) (*models.Order, error) {
    args := m.Called(id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*models.Order), args.Error(1)
}
```

## Test Environment Configuration

Tests can be configured using environment variables:

- `KAFKA_MOCK_MODE=true` - Use Kafka mocks instead of real connections
- `TEST_DB_DSN` - Database connection string for integration tests
- `TEST_REDIS_ADDR` - Redis address for integration tests

## Continuous Integration

Tests run automatically in our CI/CD pipeline:

1. **Pull Request Checks**: All tests run on PR creation and updates
2. **Main Branch Validation**: Tests must pass before merging to main
3. **Coverage Reports**: Code coverage is tracked and reported
4. **Performance Benchmarks**: Critical paths are benchmarked for performance regression

## Adding New Tests

When adding new functionality, please follow these guidelines:

1. Write unit tests for all new functions
2. Use mocks for external dependencies
3. Follow the existing naming pattern: `TestFunctionName`
4. For database tests, use the in-memory SQLite setup
5. Test error cases, not just happy paths
6. Add integration tests for API endpoints
7. Include benchmarks for performance-critical code

Example test structure:

```go
func TestCreateOrder_Success(t *testing.T) {
    // Setup mocks
    mockDB := new(MockDBLayer)
    mockLock := new(MockRedisLock)
    
    // Create service with mocks
    svc := &order.OrderService{
        DB: mockDB,
        Lock: mockLock,
    }
    
    // Set expectations
    mockDB.On("CreateOrder", mock.Anything).Return(nil)
    mockLock.On("Lock", mock.Anything).Return(true, nil)
    
    // Execute test
    result, err := svc.CreateOrder(testOrder)
    
    // Assert results
    assert.NoError(t, err)
    assert.Equal(t, expectedResult, result)
    mockDB.AssertExpectations(t)
    mockLock.AssertExpectations(t)
}
```

## Troubleshooting

### Common Issues

1. **Missing dependencies**

   If you see errors about missing packages:

   ```bash
   go get -u github.com/stretchr/testify
   go get -u github.com/google/uuid
   ```

2. **Database setup issues**

   For database tests, make sure the required tables are created in the test setup function.

3. **Test times out**

   Some tests may have implicit timeouts. Use `-timeout` flag to extend:

   ```bash
   go test ./... -timeout 5m
   ```

4. **Redis connection errors**

   For tests involving Redis locking, make sure the mock is properly configured.

5. **Flaky tests**

   If you encounter intermittent test failures:
   - Check for goroutine race conditions
   - Look for time-dependent logic
   - Ensure proper cleanup between tests

## Test Coverage Goals

- **Unit Test Coverage**: 80%+ for business logic
- **Integration Test Coverage**: Key API endpoints and workflows
- **Edge Cases**: Error handling, boundary conditions, invalid inputs

## Future Testing Improvements

- Implement property-based testing for complex algorithms
- Add end-to-end testing with real database instances
- Improve test parallelization for faster execution
- Add fuzzing tests for request handlers

For more assistance, please contact the development team.