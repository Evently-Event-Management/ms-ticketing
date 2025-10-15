package order_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/auth"
	"net/http"
	"os"
	"time"
)

// verifyOrganizationOwnership checks if the user is a member of the organization
func (h *SSEHandler) verifyOrganizationOwnership(organizationID string, userID string) (bool, error) {
	h.Logger.Debug("SSE", fmt.Sprintf("Verifying ownership for organization %s by user %s", organizationID, userID))

	// Get the M2M token
	config := getConfigFromEnv()

	// Use the Redis client if available
	token, err := auth.GetM2MToken(config, &http.Client{Timeout: 10 * time.Second}, h.RedisClient, h.Logger)
	if err != nil {
		h.Logger.Error("AUTH", fmt.Sprintf("Failed to get M2M token: %v", err))
		return false, err
	}

	// Create and execute HTTP request to verify organization ownership
	seatingServiceURL := os.Getenv("EVENT_SEATING_SERVICE_URL")
	if seatingServiceURL == "" {
		h.Logger.Error("CONFIG", "EVENT_SEATING_SERVICE_URL environment variable not set")
		return false, fmt.Errorf("EVENT_SEATING_SERVICE_URL not set")
	}

	requestURL := fmt.Sprintf("%s/internal/v1/organizations/verify-ownership?organizationId=%s&userId=%s",
		seatingServiceURL, organizationID, userID)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to create organization ownership verification request: %v", err))
		return false, err
	}

	// Add token to the request
	req.Header.Add("Authorization", "Bearer "+token)

	// Create a client for the request
	client := &http.Client{Timeout: 10 * time.Second}

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to execute organization ownership verification request: %v", err))
		return false, err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		h.Logger.Error("HTTP", fmt.Sprintf("Organization ownership verification failed with status: %s", resp.Status))
		return false, fmt.Errorf("organization ownership verification failed with status: %s", resp.Status)
	}

	// Parse the response body
	var isMember bool
	err = json.NewDecoder(resp.Body).Decode(&isMember)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to parse organization ownership verification response: %v", err))
		return false, err
	}

	h.Logger.Debug("SSE", fmt.Sprintf("User %s membership of organization %s: %v", userID, organizationID, isMember))
	return isMember, nil
}

// verifyEventOwnership checks if the user is the owner of the event
func (h *SSEHandler) verifyEventOwnership(eventID string, userID string) (bool, error) {
	h.Logger.Debug("SSE", fmt.Sprintf("Verifying ownership for event %s by user %s", eventID, userID))

	// Get the M2M token
	config := getConfigFromEnv()

	// Use the Redis client if available
	client := &http.Client{Timeout: 10 * time.Second}
	token, err := auth.GetM2MToken(config, client, h.RedisClient, h.Logger)
	if err != nil {
		h.Logger.Error("AUTH", fmt.Sprintf("Failed to get M2M token: %v", err))
		return false, err
	}

	// Create and execute HTTP request to verify ownership
	seatingServiceURL := os.Getenv("EVENT_SEATING_SERVICE_URL")
	if seatingServiceURL == "" {
		h.Logger.Error("CONFIG", "EVENT_SEATING_SERVICE_URL environment variable not set")
		return false, fmt.Errorf("EVENT_SEATING_SERVICE_URL not set")
	}

	requestURL := fmt.Sprintf("%s/internal/v1/events/verify-ownership?eventId=%s&userId=%s",
		seatingServiceURL, eventID, userID)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to create ownership verification request: %v", err))
		return false, err
	}

	// Add token to the request
	req.Header.Add("Authorization", "Bearer "+token)

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to execute ownership verification request: %v", err))
		return false, err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		h.Logger.Error("HTTP", fmt.Sprintf("Ownership verification failed with status: %s", resp.Status))
		return false, fmt.Errorf("ownership verification failed with status: %s", resp.Status)
	}

	// Parse the response body
	var isOwner bool
	err = json.NewDecoder(resp.Body).Decode(&isOwner)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to parse ownership verification response: %v", err))
		return false, err
	}

	h.Logger.Debug("SSE", fmt.Sprintf("User %s ownership of event %s: %v", userID, eventID, isOwner))
	return isOwner, nil
}
