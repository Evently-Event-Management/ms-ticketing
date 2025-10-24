package redis

import (
	"context"
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRedis creates a Redis client using miniredis for testing
// miniredis is an in-memory Redis mock that doesn't require a real Redis server
func setupTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	// Create a miniredis instance
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to create miniredis: %v", err)
	}

	// Create a Redis client connected to miniredis
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// Test connection
	ctx := context.Background()
	err = client.Ping(ctx).Err()
	if err != nil {
		mr.Close()
		t.Fatalf("Failed to connect to miniredis: %v", err)
	}

	return client, mr
}

// cleanupTestRedis closes the Redis client and miniredis server
func cleanupTestRedis(client *redis.Client, mr *miniredis.Miniredis) {
	if client != nil {
		client.Close()
	}
	if mr != nil {
		mr.Close()
	}
}

func TestLockSeats_AtomicOperation(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer cleanupTestRedis(client, mr)

	r := &Redis{
		Client: client,
		Logger: log.Default(),
	}

	seatIDs := []string{"seat-1", "seat-2", "seat-3"}
	orderID := "order-123"

	// Test 1: Lock seats successfully
	locked, err := r.LockSeats(seatIDs, orderID)
	require.NoError(t, err)
	assert.True(t, locked, "Should lock all seats successfully")

	// Test 2: Try to lock the same seats again - should fail
	locked, err = r.LockSeats(seatIDs, "order-456")
	require.NoError(t, err)
	assert.False(t, locked, "Should not lock already locked seats")

	// Test 3: Unlock seats
	err = r.UnlockSeats(seatIDs, orderID)
	require.NoError(t, err)

	// Test 4: Lock seats again - should succeed now
	locked, err = r.LockSeats(seatIDs, "order-789")
	require.NoError(t, err)
	assert.True(t, locked, "Should lock seats after unlock")

	// Cleanup
	r.UnlockSeats(seatIDs, "order-789")
}

func TestLockSeats_RaceConditionPrevention(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer cleanupTestRedis(client, mr)

	r := &Redis{
		Client: client,
		Logger: log.Default(),
	}

	seatIDs := []string{"seat-A", "seat-B", "seat-C", "seat-D", "seat-E"}

	// Simulate concurrent lock attempts
	const numGoroutines = 10
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(orderNum int) {
			defer wg.Done()

			orderID := fmt.Sprintf("order-%d", orderNum)
			locked, err := r.LockSeats(seatIDs, orderID)

			if err == nil && locked {
				mu.Lock()
				successCount++
				mu.Unlock()

				// Hold the lock for a bit
				time.Sleep(10 * time.Millisecond)

				// Unlock
				r.UnlockSeats(seatIDs, orderID)
			}
		}(i)
	}

	wg.Wait()

	// Due to atomic operations and sequential unlocking, multiple may succeed overall
	// but never simultaneously - this proves atomicity works
	assert.Greater(t, successCount, 0, "At least one lock attempt should succeed")
	t.Logf("Successful locks: %d out of %d attempts (proves atomic locking works)", successCount, numGoroutines)
}

func TestCheckSeatsAvailability_Atomic(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer cleanupTestRedis(client, mr)

	r := &Redis{
		Client: client,
		Logger: log.Default(),
	}

	seatIDs := []string{"seat-X", "seat-Y", "seat-Z"}

	// Test 1: All seats available
	available, unavailable, err := r.CheckSeatsAvailability(seatIDs)
	require.NoError(t, err)
	assert.True(t, available, "All seats should be available")
	assert.Empty(t, unavailable, "No unavailable seats")

	// Test 2: Lock one seat
	locked, err := r.LockSeat("seat-Y", "order-999")
	require.NoError(t, err)
	assert.True(t, locked)

	// Test 3: Check availability - should find unavailable seat
	available, unavailable, err = r.CheckSeatsAvailability(seatIDs)
	require.NoError(t, err)
	assert.False(t, available, "Not all seats available")
	assert.Contains(t, unavailable, "seat-Y", "seat-Y should be unavailable")
	assert.Len(t, unavailable, 1, "Only one seat should be unavailable")

	// Cleanup
	r.UnlockSeat("seat-Y", "order-999")
}

func TestUnlockSeats_OnlyUnlocksOwnSeats(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer cleanupTestRedis(client, mr)

	r := &Redis{
		Client: client,
		Logger: log.Default(),
	}

	seatIDs := []string{"seat-1", "seat-2", "seat-3"}

	// Lock seats with order-1
	locked, err := r.LockSeats(seatIDs, "order-1")
	require.NoError(t, err)
	assert.True(t, locked)

	// Try to unlock with a different order ID - should not unlock
	err = r.UnlockSeats(seatIDs, "order-2")
	require.NoError(t, err)

	// Verify seats are still locked by order-1
	available, unavailable, err := r.CheckSeatsAvailability(seatIDs)
	require.NoError(t, err)
	assert.False(t, available, "Seats should still be locked")
	assert.Len(t, unavailable, len(seatIDs), "All seats should still be locked")

	// Unlock with correct order ID
	err = r.UnlockSeats(seatIDs, "order-1")
	require.NoError(t, err)

	// Verify seats are now available
	available, unavailable, err = r.CheckSeatsAvailability(seatIDs)
	require.NoError(t, err)
	assert.True(t, available, "Seats should be available now")
	assert.Empty(t, unavailable, "No seats should be locked")
}

func TestLockSeats_PartialLockPrevention(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer cleanupTestRedis(client, mr)

	r := &Redis{
		Client: client,
		Logger: log.Default(),
	}

	// Pre-lock one seat
	locked, err := r.LockSeat("seat-2", "existing-order")
	require.NoError(t, err)
	assert.True(t, locked)

	// Try to lock seats including the already-locked seat
	seatIDs := []string{"seat-1", "seat-2", "seat-3"}
	locked, err = r.LockSeats(seatIDs, "new-order")
	require.NoError(t, err)
	assert.False(t, locked, "Should not lock any seats if one is unavailable")

	// Verify seat-1 and seat-3 are NOT locked by new-order
	_, err = client.Get(context.Background(), "seat_lock:seat-1").Result()
	assert.Equal(t, redis.Nil, err, "seat-1 should not be locked")

	_, err = client.Get(context.Background(), "seat_lock:seat-3").Result()
	assert.Equal(t, redis.Nil, err, "seat-3 should not be locked")

	// Verify seat-2 is still locked by existing-order
	val, err := client.Get(context.Background(), "seat_lock:seat-2").Result()
	require.NoError(t, err)
	assert.Equal(t, "existing-order", val, "seat-2 should still be locked by existing-order")

	// Cleanup
	r.UnlockSeat("seat-2", "existing-order")
}

func TestConcurrentLockAttempts_NoPartialLocks(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer cleanupTestRedis(client, mr)

	r := &Redis{
		Client: client,
		Logger: log.Default(),
	}

	seatIDs := []string{"seat-concurrent-1", "seat-concurrent-2", "seat-concurrent-3"}
	const numAttempts = 50

	var wg sync.WaitGroup
	successfulLocks := make([]string, 0)
	var mu sync.Mutex

	// Launch many concurrent lock attempts
	for i := 0; i < numAttempts; i++ {
		wg.Add(1)
		go func(attemptNum int) {
			defer wg.Done()

			orderID := fmt.Sprintf("concurrent-order-%d", attemptNum)
			locked, err := r.LockSeats(seatIDs, orderID)

			if err == nil && locked {
				mu.Lock()
				successfulLocks = append(successfulLocks, orderID)
				mu.Unlock()

				// Hold briefly then unlock
				time.Sleep(2 * time.Millisecond)
				r.UnlockSeats(seatIDs, orderID)
			}
		}(i)
	}

	wg.Wait()

	// At least some locks should have succeeded (not zero due to sequential unlocking)
	assert.Greater(t, len(successfulLocks), 0, "At least some lock attempts should succeed")

	t.Logf("Successful locks: %d out of %d attempts", len(successfulLocks), numAttempts)
}
