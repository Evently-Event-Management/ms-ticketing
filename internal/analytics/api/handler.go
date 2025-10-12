package analytics_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/analytics"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-redis/redis/v8"
)

// Handler handles analytics HTTP endpoints
type Handler struct {
	Service     *analytics.Service
	Logger      *logger.Logger
	Client      *http.Client
	RedisClient *redis.Client
}

// NewHandler creates a new analytics handler
func NewHandler(service *analytics.Service, logger *logger.Logger) *Handler {
	return &Handler{
		Service: service,
		Logger:  logger,
		Client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// NewHandlerWithRedis creates a new analytics handler with Redis client for token caching
func NewHandlerWithRedis(service *analytics.Service, logger *logger.Logger, redisClient *redis.Client) *Handler {
	return &Handler{
		Service:     service,
		Logger:      logger,
		Client:      &http.Client{Timeout: 10 * time.Second},
		RedisClient: redisClient,
	}
}

// RegisterRoutes registers the analytics routes on a chi router
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/order/analytics", func(r chi.Router) {
		r.Get("/events/{eventId}", h.GetEventAnalytics)
		r.Get("/events/{eventId}/discounts", h.GetEventDiscountAnalytics)
		r.Get("/events/{eventId}/sessions", h.GetEventSessionsAnalytics)
		r.Get("/events/{eventId}/sessions/{sessionId}", h.GetSessionAnalytics)
		r.Get("/events/{eventId}/orders", h.GetEventOrders)
		r.Post("/events/batch", h.GetBatchEventAnalytics)
	})
}

// sendJSONResponse is a helper function to send JSON responses
func sendJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// If we can't encode the response, log the error
		// This is a server error after we've already started sending the response,
		// so we can't change the status code now
		// Just log it and return
	}
}

// verifyEventOwnership checks if the user is the owner of the event
func (h *Handler) verifyEventOwnership(eventID string, userID string) (bool, error) {
	h.Logger.Debug("ANALYTICS", fmt.Sprintf("Verifying ownership for event %s by user %s", eventID, userID))

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
	resp, err := h.Client.Do(req)
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

	h.Logger.Debug("ANALYTICS", fmt.Sprintf("User %s ownership of event %s: %v", userID, eventID, isOwner))
	return isOwner, nil
}

// verifyBatchEventOwnership checks if the user owns the specified events
// Returns a list of event IDs that the user owns
func (h *Handler) verifyBatchEventOwnership(eventIDs []string, userID string) ([]string, error) {
	h.Logger.Debug("ANALYTICS", fmt.Sprintf("Verifying batch ownership for %d events by user %s", len(eventIDs), userID))

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
		return nil, err
	}

	// Create and execute HTTP request to verify batch ownership
	seatingServiceURL := os.Getenv("EVENT_SEATING_SERVICE_URL")
	if seatingServiceURL == "" {
		h.Logger.Error("CONFIG", "EVENT_SEATING_SERVICE_URL environment variable not set")
		return nil, fmt.Errorf("EVENT_SEATING_SERVICE_URL not set")
	}

	requestURL := fmt.Sprintf("%s/internal/v1/events/verify-batch-ownership", seatingServiceURL)

	// Create request body
	requestBody := models.BatchOwnershipRequest{
		EventIDs: eventIDs,
		UserID:   userID,
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to marshal batch ownership request: %v", err))
		return nil, err
	}

	req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(requestJSON))
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to create batch ownership verification request: %v", err))
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)

	// Execute the request
	resp, err := h.Client.Do(req)
	if err != nil {
		h.Logger.Error("HTTP", fmt.Sprintf("Failed to execute batch ownership verification request: %v", err))
		return nil, err
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode == http.StatusOK {
		// If OK, assume all events are owned
		h.Logger.Debug("ANALYTICS", fmt.Sprintf("User %s has ownership of all %d events", userID, len(eventIDs)))
		return eventIDs, nil
	} else if resp.StatusCode == http.StatusForbidden {
		// If Forbidden, user doesn't own any of these events
		h.Logger.Debug("ANALYTICS", fmt.Sprintf("User %s doesn't own any of the %d events", userID, len(eventIDs)))
		return []string{}, nil
	} else {
		// For other status codes, treat as error
		h.Logger.Error("HTTP", fmt.Sprintf("Batch ownership verification failed with status: %s", resp.Status))
		return nil, fmt.Errorf("batch ownership verification failed with status: %s", resp.Status)
	}
}

// GetEventAnalytics handles revenue analytics request for an event
func (h *Handler) GetEventAnalytics(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	if eventID == "" {
		h.Logger.Error("ANALYTICS", "event_id is required")
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "event_id is required"})
		return
	}

	// Extract user ID from context (injected by auth middleware)
	userID := auth.UserID(r.Context())
	if userID == "" {
		h.Logger.Error("ANALYTICS", "User ID not found in context")
		sendJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		return
	}

	// Verify ownership before proceeding
	isOwner, err := h.verifyEventOwnership(eventID, userID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error verifying event ownership: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify event ownership"})
		return
	}

	if !isOwner {
		h.Logger.Warn("ANALYTICS", fmt.Sprintf("User %s attempted to access analytics for event %s without ownership", userID, eventID))
		sendJSONResponse(w, http.StatusForbidden, map[string]string{"error": "You do not have permission to access these analytics"})
		return
	}

	// Only consider orders with status "completed"
	analytics, err := h.Service.GetEventAnalytics(r.Context(), eventID, "completed")
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting event analytics: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get analytics"})
		return
	}

	sendJSONResponse(w, http.StatusOK, analytics)
}

// GetEventDiscountAnalytics handles discount analytics request for an event
func (h *Handler) GetEventDiscountAnalytics(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	if eventID == "" {
		h.Logger.Error("ANALYTICS", "event_id is required")
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "event_id is required"})
		return
	}

	// Extract user ID from context (injected by auth middleware)
	userID := auth.UserID(r.Context())
	if userID == "" {
		h.Logger.Error("ANALYTICS", "User ID not found in context")
		sendJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		return
	}

	// Verify ownership before proceeding
	isOwner, err := h.verifyEventOwnership(eventID, userID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error verifying event ownership: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify event ownership"})
		return
	}

	if !isOwner {
		h.Logger.Warn("ANALYTICS", fmt.Sprintf("User %s attempted to access discount analytics for event %s without ownership", userID, eventID))
		sendJSONResponse(w, http.StatusForbidden, map[string]string{"error": "You do not have permission to access these analytics"})
		return
	}

	// Only consider orders with status "completed"
	discountAnalytics, err := h.Service.GetEventDiscountAnalytics(r.Context(), eventID, "completed")
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting discount analytics: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get discount analytics"})
		return
	}

	sendJSONResponse(w, http.StatusOK, discountAnalytics)
}

// GetEventSessionsAnalytics handles analytics request for all sessions of an event
func (h *Handler) GetEventSessionsAnalytics(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	if eventID == "" {
		h.Logger.Error("ANALYTICS", "event_id is required")
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "event_id is required"})
		return
	}

	// Extract user ID from context (injected by auth middleware)
	userID := auth.UserID(r.Context())
	if userID == "" {
		h.Logger.Error("ANALYTICS", "User ID not found in context")
		sendJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		return
	}

	// Verify ownership before proceeding
	isOwner, err := h.verifyEventOwnership(eventID, userID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error verifying event ownership: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify event ownership"})
		return
	}

	if !isOwner {
		h.Logger.Warn("ANALYTICS", fmt.Sprintf("User %s attempted to access sessions analytics for event %s without ownership", userID, eventID))
		sendJSONResponse(w, http.StatusForbidden, map[string]string{"error": "You do not have permission to access these analytics"})
		return
	}

	// Only consider orders with status "completed"
	sessionsAnalytics, err := h.Service.GetEventSessionsAnalytics(r.Context(), eventID, "completed")
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting sessions analytics: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get sessions analytics"})
		return
	}

	sendJSONResponse(w, http.StatusOK, sessionsAnalytics)
}

// GetSessionAnalytics handles analytics request for a session
func (h *Handler) GetSessionAnalytics(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	sessionID := chi.URLParam(r, "sessionId")

	if eventID == "" || sessionID == "" {
		h.Logger.Error("ANALYTICS", "event_id and session_id are required")
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "event_id and session_id are required"})
		return
	}

	// Extract user ID from context (injected by auth middleware)
	userID := auth.UserID(r.Context())
	if userID == "" {
		h.Logger.Error("ANALYTICS", "User ID not found in context")
		sendJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		return
	}

	// Verify ownership before proceeding
	isOwner, err := h.verifyEventOwnership(eventID, userID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error verifying event ownership: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify event ownership"})
		return
	}

	if !isOwner {
		h.Logger.Warn("ANALYTICS", fmt.Sprintf("User %s attempted to access analytics for session %s of event %s without ownership",
			userID, sessionID, eventID))
		sendJSONResponse(w, http.StatusForbidden, map[string]string{"error": "You do not have permission to access these analytics"})
		return
	}

	// Only consider orders with status "completed"
	analytics, err := h.Service.GetSessionAnalytics(r.Context(), eventID, sessionID, "completed")
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting session analytics: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get analytics"})
		return
	}

	sendJSONResponse(w, http.StatusOK, analytics)
}

// GetBatchEventAnalytics handles analytics requests for multiple events
func (h *Handler) GetBatchEventAnalytics(w http.ResponseWriter, r *http.Request) {
	// Parse request body to get event IDs
	var request struct {
		EventIDs []string `json:"eventIds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.Logger.Error("ANALYTICS", "Failed to parse request body: "+err.Error())
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid request format"})
		return
	}

	if len(request.EventIDs) == 0 {
		h.Logger.Error("ANALYTICS", "No event IDs provided")
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "No event IDs provided"})
		return
	}

	// Extract user ID from context (injected by auth middleware)
	userID := auth.UserID(r.Context())
	if userID == "" {
		h.Logger.Error("ANALYTICS", "User ID not found in context")
		sendJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		return
	}

	// Verify ownership of all events
	ownedEvents, err := h.verifyBatchEventOwnership(request.EventIDs, userID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error verifying batch event ownership: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify event ownership"})
		return
	}

	// If user doesn't own any of the requested events
	if len(ownedEvents) == 0 {
		h.Logger.Warn("ANALYTICS", fmt.Sprintf("User %s attempted to access batch analytics without ownership of any events", userID))
		sendJSONResponse(w, http.StatusForbidden, map[string]string{"error": "You do not have permission to access analytics for any of the requested events"})
		return
	}

	// Only consider orders with status "completed" and only for owned events
	analytics, err := h.Service.GetBatchEventAnalytics(r.Context(), ownedEvents, "completed")
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting batch event analytics: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get analytics"})
		return
	}

	// Log how many events were processed
	h.Logger.Info("ANALYTICS", fmt.Sprintf("Returning aggregated analytics for %d events", len(analytics.EventIDs)))
	sendJSONResponse(w, http.StatusOK, analytics)
}

// GetEventOrders handles request to get orders for an event with optional filters and sorting
func (h *Handler) GetEventOrders(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventId")
	if eventID == "" {
		h.Logger.Error("ANALYTICS", "event_id is required")
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "event_id is required"})
		return
	}

	// Extract user ID from context (injected by auth middleware)
	userID := auth.UserID(r.Context())
	if userID == "" {
		h.Logger.Error("ANALYTICS", "User ID not found in context")
		sendJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		return
	}

	// Verify ownership before proceeding
	isOwner, err := h.verifyEventOwnership(eventID, userID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error verifying event ownership: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify event ownership"})
		return
	}

	if !isOwner {
		h.Logger.Warn("ANALYTICS", fmt.Sprintf("User %s attempted to access orders for event %s without ownership", userID, eventID))
		sendJSONResponse(w, http.StatusForbidden, map[string]string{"error": "You do not have permission to access these orders"})
		return
	}

	// Parse query parameters
	options := analytics.EventOrderOptions{
		SessionID: r.URL.Query().Get("sessionId"),
		Status:    r.URL.Query().Get("status"),
		SortBy:    r.URL.Query().Get("sort"),
		SortDesc:  r.URL.Query().Get("order") == "desc",
	}

	// Parse pagination parameters
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var limit int
		_, err := fmt.Sscanf(limitStr, "%d", &limit)
		if err == nil && limit > 0 {
			options.Limit = limit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		var offset int
		_, err := fmt.Sscanf(offsetStr, "%d", &offset)
		if err == nil && offset >= 0 {
			options.Offset = offset
		}
	}

	orders, err := h.Service.GetEventOrders(r.Context(), eventID, options)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting event orders: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get orders"})
		return
	}

	sendJSONResponse(w, http.StatusOK, orders)
}
