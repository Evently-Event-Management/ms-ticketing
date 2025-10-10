package auth

import (
	"context"
	"ms-ticketing/internal/logger"
	"time"

	"github.com/go-redis/redis/v8"
)

// InitializeTokenCache sets up Redis for token caching and tests the connection
func InitializeTokenCache(redisAddr string, customLogger *logger.Logger) (*redis.Client, error) {
	var logInfo, logError func(format string, v ...interface{})

	if customLogger != nil {
		logInfo = func(format string, v ...interface{}) {
			customLogger.Info("AUTH", format)
		}
		logError = func(format string, v ...interface{}) {
			customLogger.Error("AUTH", format)
		}
	} else {
		// No-op if no logger provided
		logInfo = func(format string, v ...interface{}) {}
		logError = func(format string, v ...interface{}) {}
	}

	// Create Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "", // no password
		DB:       0,  // use default DB
		PoolSize: 10, // connection pool size
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		logError("Failed to connect to Redis at %s: %v", redisAddr, err)
		return nil, err
	}

	logInfo("Successfully connected to Redis at %s for token caching", redisAddr)

	// Test writing to the M2M token key
	testKey := M2MTokenKey + ":test"
	if err := redisClient.Set(ctx, testKey, "test", 5*time.Second).Err(); err != nil {
		logError("Failed to write test value to Redis: %v", err)
		return nil, err
	}

	logInfo("Redis token cache is ready for use")
	return redisClient, nil
}
