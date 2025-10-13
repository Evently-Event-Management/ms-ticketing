package analytics_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/models"
	"net/http"
	"os"
)

// verifyOrganizationOwnership checks if the user is a member of the organization
func (h *Handler) verifyOrganizationOwnership(organizationID string, userID string) (bool, error) {
	h.Logger.Debug("ANALYTICS", fmt.Sprintf("Verifying ownership for organization %s by user %s", organizationID, userID))

	// Get the M2M token
	config := models.Config{
		KeycloakURL:   os.Getenv("KEYCLOAK_URL"),
		KeycloakRealm: os.Getenv("KEYCLOAK_REALM"),
		ClientID:      os.Getenv("TICKET_CLIENT_ID"),
		ClientSecret:  os.Getenv("TICKET_CLIENT_SECRET"),
	}

	// Use the Redis client if available
	token, err := auth.GetM2MToken(config, h.Client, h.RedisClient, h.Logger)
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

	// Execute the request
	resp, err := h.Client.Do(req)
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

	h.Logger.Debug("ANALYTICS", fmt.Sprintf("User %s membership of organization %s: %v", userID, organizationID, isMember))
	return isMember, nil
}
