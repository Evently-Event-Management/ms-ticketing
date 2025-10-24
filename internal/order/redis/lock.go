package redis

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"log"
	kafka "ms-ticketing/internal/kafka"

	"github.com/go-redis/redis/v8"
)

type Redis struct {
	Client   *redis.Client
	Producer *kafka.Producer
	Logger   *log.Logger
}

func NewRedis(client *redis.Client, producer *kafka.Producer) *Redis {
	return &Redis{
		Client:   client,
		Producer: producer,
		Logger:   log.Default(),
	}
}

// getSeatLockDuration returns the seat lock duration from environment variables or the default value
func (r *Redis) getSeatLockDuration() time.Duration {
	// Default lock TTL is 5 minutes
	defaultDuration := 5 * time.Minute

	// Get the lock TTL from environment variable, default to 5 minutes if not set
	lockTTLStr := os.Getenv("SEAT_LOCK_TTL_MINUTES")
	if lockTTLStr == "" {
		r.Logger.Println("REDIS: SEAT_LOCK_TTL_MINUTES not set, using default 5 minutes")
		return defaultDuration
	}

	// Parse the string to an integer
	lockTTLMin, err := strconv.Atoi(lockTTLStr)
	if err != nil {
		r.Logger.Println("REDIS: Invalid SEAT_LOCK_TTL_MINUTES value '" + lockTTLStr + "', using default 5 minutes")
		return defaultDuration
	}

	// Log the duration we're using
	r.Logger.Printf("REDIS: Using seat lock duration of %d minutes from environment", lockTTLMin)
	return time.Duration(lockTTLMin) * time.Minute
}

// CheckSeatAvailability checks if a seat is available (not locked) without locking it
func (r *Redis) CheckSeatAvailability(seatID string) (bool, error) {
	key := "seat_lock:" + seatID
	_, err := r.Client.Get(context.Background(), key).Result()
	if err == redis.Nil {
		// Key doesn't exist, seat is available
		return true, nil
	}
	if err != nil {
		// Error checking the key
		return false, err
	}
	// Key exists, seat is locked
	return false, nil
}

// CheckSeatsAvailability checks if multiple seats are available atomically using Lua script
// This prevents race conditions by checking all seats in a single atomic operation
func (r *Redis) CheckSeatsAvailability(seatIDs []string) (bool, []string, error) {
	ctx := context.Background()

	// Lua script to atomically check multiple seats
	luaScript := `
		local unavailableSeats = {}
		
		for i = 1, #KEYS do
			local key = "seat_lock:" .. KEYS[i]
			if redis.call('EXISTS', key) == 1 then
				table.insert(unavailableSeats, KEYS[i])
			end
		end
		
		return unavailableSeats
	`

	// Execute Lua script with all seat IDs
	result, err := r.Client.Eval(ctx, luaScript, seatIDs).Result()
	if err != nil {
		r.Logger.Printf("REDIS: Failed to execute atomic availability check: %v", err)
		return false, nil, err
	}

	// Parse result
	unavailableSeatsInterface := result.([]interface{})
	unavailableSeats := make([]string, len(unavailableSeatsInterface))
	for i, v := range unavailableSeatsInterface {
		unavailableSeats[i] = v.(string)
	}

	if len(unavailableSeats) > 0 {
		r.Logger.Printf("REDIS: Found %d unavailable seats", len(unavailableSeats))
		return false, unavailableSeats, nil
	}

	return true, nil, nil
}

// Lock a single seat
func (r *Redis) LockSeat(seatID, orderID string) (bool, error) {
	key := "seat_lock:" + seatID
	lockDuration := r.getSeatLockDuration()
	ok, err := r.Client.SetNX(context.Background(), key, orderID, lockDuration).Result()
	return ok, err
}

// Unlock a single seat
func (r *Redis) UnlockSeat(seatID, orderID string) error {
	ctx := context.Background()
	key := fmt.Sprintf("seat_lock:%s", seatID)
	val, err := r.Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil // already unlocked
	}
	if err != nil {
		return err
	}
	if val == orderID {
		_, err := r.Client.Del(ctx, key).Result()
		return err
	}
	return nil
}

// Lock multiple seats atomically using Lua script
// This prevents race conditions by executing all lock operations atomically
func (r *Redis) LockSeats(seatIDs []string, orderID string) (bool, error) {
	ctx := context.Background()
	lockDuration := r.getSeatLockDuration()

	// Lua script to atomically lock multiple seats
	// Returns 1 if all seats were locked successfully, 0 if any seat is already locked
	luaScript := `
		local seats = ARGV
		local orderID = KEYS[1]
		local ttl = KEYS[2]
		
		-- First pass: check if all seats are available
		for i = 3, #KEYS do
			local key = "seat_lock:" .. KEYS[i]
			if redis.call('EXISTS', key) == 1 then
				-- Seat is already locked
				return 0
			end
		end
		
		-- Second pass: lock all seats atomically
		for i = 3, #KEYS do
			local key = "seat_lock:" .. KEYS[i]
			redis.call('SETEX', key, ttl, orderID)
		end
		
		return 1
	`

	// Prepare keys: [orderID, ttl, seatID1, seatID2, ...]
	keys := make([]string, 0, len(seatIDs)+2)
	keys = append(keys, orderID)
	keys = append(keys, fmt.Sprintf("%d", int(lockDuration.Seconds())))
	keys = append(keys, seatIDs...)

	// Execute Lua script
	result, err := r.Client.Eval(ctx, luaScript, keys).Result()
	if err != nil {
		r.Logger.Printf("REDIS: Failed to execute atomic lock script: %v", err)
		return false, err
	}

	// Check result
	if result.(int64) == 1 {
		r.Logger.Printf("REDIS: Successfully locked %d seats atomically", len(seatIDs))
		return true, nil
	}

	r.Logger.Println("REDIS: One or more seats are already locked")
	return false, nil
}

// Unlock multiple seats atomically using Lua script
// This ensures all seats are unlocked together and only if they belong to the same order
func (r *Redis) UnlockSeats(seatIDs []string, orderID string) error {
	ctx := context.Background()

	// Lua script to atomically unlock multiple seats
	// Only unlocks seats that belong to the specified orderID
	luaScript := `
		local orderID = KEYS[1]
		local unlockedCount = 0
		
		-- Unlock all seats that belong to this order
		for i = 2, #KEYS do
			local key = "seat_lock:" .. KEYS[i]
			local currentOwner = redis.call('GET', key)
			
			-- Only unlock if the seat is locked by this order
			if currentOwner == orderID then
				redis.call('DEL', key)
				unlockedCount = unlockedCount + 1
			end
		end
		
		return unlockedCount
	`

	// Prepare keys: [orderID, seatID1, seatID2, ...]
	keys := make([]string, 0, len(seatIDs)+1)
	keys = append(keys, orderID)
	keys = append(keys, seatIDs...)

	// Execute Lua script
	result, err := r.Client.Eval(ctx, luaScript, keys).Result()
	if err != nil {
		r.Logger.Printf("REDIS: Failed to execute atomic unlock script: %v", err)
		return err
	}

	unlockedCount := result.(int64)
	r.Logger.Printf("REDIS: Atomically unlocked %d out of %d seats for order %s", unlockedCount, len(seatIDs), orderID)

	return nil
}
