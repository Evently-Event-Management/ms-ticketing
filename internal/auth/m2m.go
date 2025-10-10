package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-redis/redis/v8"
)

// GetM2MToken retrieves a machine-to-machine token from Keycloak
// If redisClient is provided, it will try to get the token from Redis cache first
func GetM2MToken(cfg models.Config, client *http.Client, redisClient *redis.Client, logger *logger.Logger) (string, error) {
	// Use standard logger if custom logger is not provided
	logInfo := log.Printf
	logError := log.Printf
	if logger != nil {
		logInfo = func(format string, v ...interface{}) {
			logger.Info("AUTH", fmt.Sprintf(format, v...))
		}
		logError = func(format string, v ...interface{}) {
			logger.Error("AUTH", fmt.Sprintf(format, v...))
		}
	}

	// Try to get token from Redis cache if available
	if redisClient != nil {
		ctx := context.Background()
		tokenCache := NewRedisTokenCache(redisClient)
		cachedToken, err := tokenCache.GetToken(ctx)

		if err != nil {
			logError("Error retrieving token from cache: %v", err)
			// Continue to request a new token if cache retrieval fails
		} else if cachedToken != nil && cachedToken.IsValid() {
			logInfo("Using cached M2M token from Redis (expires at: %s)", cachedToken.ExpiresAt)
			return cachedToken.Token, nil
		} else {
			logInfo("No valid cached token found in Redis, requesting new token")
		}
	} else {
		logInfo("Redis client not provided, unable to use token caching")
	}

	// Proceed with requesting a new token
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", cfg.KeycloakURL, cfg.KeycloakRealm)
	logInfo("Requesting M2M token from: %s", tokenURL)

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", cfg.ClientID)
	data.Set("client_secret", cfg.ClientSecret)

	req, _ := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	logInfo("Sending POST request to Keycloak for token with client_id: %s", cfg.ClientID)
	resp, err := client.Do(req)
	if err != nil {
		logError("HTTP request to Keycloak failed: %v", err)
		return "", err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logError("Error closing response body: %v", cerr)
		}
	}()

	logInfo("Keycloak token response status: %s", resp.Status)
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logError("Keycloak token response body: %s", string(bodyBytes))
		return "", fmt.Errorf("failed to get token, status: %s", resp.Status)
	}

	var tokenResp models.M2MTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		logError("Error decoding token response: %v", err)
		return "", err
	}
	logInfo("Received new access token")

	// Store the token in Redis cache if Redis client is provided
	if redisClient != nil {
		ctx := context.Background()
		tokenCache := NewRedisTokenCache(redisClient)
		if err := tokenCache.SetToken(ctx, tokenResp.AccessToken, tokenResp.ExpiresIn); err != nil {
			logError("Failed to cache token in Redis: %v", err)
			// Continue even if caching fails
		} else {
			logInfo("Successfully cached M2M token in Redis (expires in %d seconds)", tokenResp.ExpiresIn)
		}
	}

	return tokenResp.AccessToken, nil
}
