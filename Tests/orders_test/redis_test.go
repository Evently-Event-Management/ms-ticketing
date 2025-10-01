package orders_test

import (
	"context"
	orderredis "ms-ticketing/internal/order/redis"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// MockRedisClient is a mock for redis operations
type MockRedisClient struct {
	mock.Mock
	lockMap map[string]string
}

// NewMockRedisClient creates a new mock client
func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		lockMap: make(map[string]string),
	}
}

// SetNX mocks SetNX operation
func (m *MockRedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	// Create a real command with context
	cmd := new(redis.BoolCmd)

	// If key doesn't exist, set it and return true
	if _, exists := m.lockMap[key]; !exists {
		m.lockMap[key] = value.(string)
		cmd.SetVal(true)
	} else {
		cmd.SetVal(false)
	}

	return cmd
}

// Get mocks Get operation
func (m *MockRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := new(redis.StringCmd)

	if val, exists := m.lockMap[key]; exists {
		cmd.SetVal(val)
	} else {
		cmd.SetErr(redis.Nil)
	}

	return cmd
}

// Del mocks Del operation
func (m *MockRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	cmd := new(redis.IntCmd)
	count := int64(0)

	for _, key := range keys {
		if _, exists := m.lockMap[key]; exists {
			delete(m.lockMap, key)
			count++
		}
	}

	cmd.SetVal(count)
	return cmd
}

// TestRedisIntegration tests the Redis lock with a real Redis container
func TestRedisIntegration(t *testing.T) {
	// Skip if short test mode
	if testing.Short() {
		t.Skip("Skipping Redis integration test in short mode")
	}

	// Start a Redis container
	ctx := context.Background()
	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:latest",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})

	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}

	defer redisContainer.Terminate(ctx)

	// Get Redis host and port
	host, err := redisContainer.Host(ctx)
	require.NoError(t, err)

	port, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err)

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:     host + ":" + port.Port(),
		Password: "",
		DB:       0,
	})

	// Create Redis lock manager
	redisLock := orderredis.NewRedis(client, nil)

	// Test locking seats
	seatIDs := []string{"seat1", "seat2", "seat3"}
	orderID := "test-order-id"

	// Lock seats
	locked, err := redisLock.LockSeats(seatIDs, orderID)
	require.NoError(t, err)
	assert.True(t, locked, "Expected seats to be lockable")

	// Try to lock again (should fail)
	locked, err = redisLock.LockSeats(seatIDs, "another-order-id")
	require.NoError(t, err)
	assert.False(t, locked, "Expected seats to be already locked")

	// Unlock seats
	err = redisLock.UnlockSeats(seatIDs, orderID)
	require.NoError(t, err)

	// Lock seats again (should succeed now)
	locked, err = redisLock.LockSeats(seatIDs, orderID)
	require.NoError(t, err)
	assert.True(t, locked, "Expected seats to be lockable after unlock")
}

// TestRedisMock tests the Redis lock with a mock client
func TestRedisMock(t *testing.T) {
	// This test is simplified since we need a more complex mock implementation
	// to properly test the Redis lock with a mock client
	t.Skip("Skipping Redis mock test - use integration test instead")
}
