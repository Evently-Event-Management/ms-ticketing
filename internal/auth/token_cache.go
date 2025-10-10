package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// M2MTokenKey is the key used to store the M2M token in Redis
	M2MTokenKey = "m2m_token"
	// TokenExpiryBuffer is the buffer time before actual token expiry to refresh it (in seconds)
	TokenExpiryBuffer = 60
)

// TokenCache represents a cached token with its expiry time
type TokenCache struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsValid checks if the token is still valid with a buffer time before expiry
func (tc *TokenCache) IsValid() bool {
	if tc == nil || tc.Token == "" {
		return false
	}
	// Consider the token invalid if it's within the buffer period of expiry
	return time.Now().Add(TokenExpiryBuffer * time.Second).Before(tc.ExpiresAt)
}

// RedisTokenCache implements token caching using Redis
type RedisTokenCache struct {
	Client *redis.Client
}

// NewRedisTokenCache creates a new Redis token cache
func NewRedisTokenCache(client *redis.Client) *RedisTokenCache {
	return &RedisTokenCache{
		Client: client,
	}
}

// GetToken retrieves a token from the cache
func (c *RedisTokenCache) GetToken(ctx context.Context) (*TokenCache, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("redis client not initialized")
	}

	// Try to get the cached token from Redis
	tokenJSON, err := c.Client.Get(ctx, M2MTokenKey).Result()
	if err == redis.Nil {
		// Key does not exist
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to get token from Redis: %w", err)
	}

	// Parse the JSON back into a TokenCache struct
	var tokenCache TokenCache
	if err := json.Unmarshal([]byte(tokenJSON), &tokenCache); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token cache: %w", err)
	}

	// Check if the token is still valid
	if !tokenCache.IsValid() {
		// Token exists but is expired
		return nil, nil
	}

	return &tokenCache, nil
}

// SetToken stores a token in the cache with its expiry time
func (c *RedisTokenCache) SetToken(ctx context.Context, token string, expiresIn int) error {
	if c.Client == nil {
		return fmt.Errorf("redis client not initialized")
	}

	// Calculate the expiry time
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	// Create a token cache object
	tokenCache := &TokenCache{
		Token:     token,
		ExpiresAt: expiresAt,
	}

	// Serialize the token cache to JSON
	tokenJSON, err := json.Marshal(tokenCache)
	if err != nil {
		return fmt.Errorf("failed to marshal token cache: %w", err)
	}

	// Store in Redis with expiry
	// Set the Redis TTL to the token expiry plus a small buffer for clock skew
	ttl := time.Duration(expiresIn+TokenExpiryBuffer) * time.Second
	if err := c.Client.Set(ctx, M2MTokenKey, tokenJSON, ttl).Err(); err != nil {
		return fmt.Errorf("failed to store token in Redis: %w", err)
	}

	return nil
}
