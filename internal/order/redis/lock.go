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
	r.Logger.Println(fmt.Sprintf("REDIS: Using seat lock duration of %d minutes from environment", lockTTLMin))
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

// CheckSeatsAvailability checks if multiple seats are available without locking them
func (r *Redis) CheckSeatsAvailability(seatIDs []string) (bool, []string, error) {
	unavailableSeats := []string{}
	for _, seatID := range seatIDs {
		available, err := r.CheckSeatAvailability(seatID)
		if err != nil {
			return false, nil, err
		}
		if !available {
			unavailableSeats = append(unavailableSeats, seatID)
		}
	}
	if len(unavailableSeats) > 0 {
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

// Lock multiple seats atomically
func (r *Redis) LockSeats(seatIDs []string, orderID string) (bool, error) {
	locked := []string{}
	for _, seatID := range seatIDs {
		ok, err := r.LockSeat(seatID, orderID)
		if err != nil {
			// Unlock all previously locked seats
			for _, l := range locked {
				_ = r.UnlockSeat(l, orderID)
			}
			return false, err
		}
		if !ok {
			// Unlock all previously locked seats
			for _, l := range locked {
				_ = r.UnlockSeat(l, orderID)
			}
			return false, nil
		}
		locked = append(locked, seatID)
	}
	return true, nil
}

// Unlock multiple seats
func (r *Redis) UnlockSeats(seatIDs []string, orderID string) error {
	var firstErr error
	for _, seatID := range seatIDs {
		err := r.UnlockSeat(seatID, orderID)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
