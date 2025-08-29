package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type Redis struct {
	Client *redis.Client
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{Client: client}
}

const lockTTL = 5 * time.Minute

// Lock a single seat
func (r *Redis) LockSeat(seatID, orderID string) (bool, error) {
	key := "seat_lock:" + seatID
	ok, err := r.Client.SetNX(context.Background(), key, orderID, lockTTL).Result()
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
	return nil // do not unlock if not owned by this order
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

// Define DB interface or import the correct type
