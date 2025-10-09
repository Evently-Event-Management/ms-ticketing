package analytics_api

import (
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
)

// Handler handles analytics HTTP endpoints
type Handler struct {
	Service *analytics.Service
	Logger  *logger.Logger
	Client  *http.Client
}

// NewHandler creates a new analytics handler
func NewHandler(service *analytics.Service, logger *logger.Logger) *Handler {
	return &Handler{
		Service: service,
		Logger:  logger,
		Client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// RegisterRoutes registers the analytics routes on a chi router
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/order/analytics", func(r chi.Router) {
		r.Get("/events/{eventId}", h.GetEventAnalytics)
		r.Get("/events/{eventId}/discounts", h.GetEventDiscountAnalytics)
		r.Get("/events/{eventId}/sessions", h.GetEventSessionsAnalytics)
		r.Get("/events/{eventId}/sessions/{sessionId}", h.GetSessionAnalytics)
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

	token, err := auth.GetM2MToken(config, h.Client)
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

	analytics, err := h.Service.GetEventAnalytics(r.Context(), eventID)
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

	discountAnalytics, err := h.Service.GetEventDiscountAnalytics(r.Context(), eventID)
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

	sessionsAnalytics, err := h.Service.GetEventSessionsAnalytics(r.Context(), eventID)
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

	analytics, err := h.Service.GetSessionAnalytics(r.Context(), eventID, sessionID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting session analytics: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get analytics"})
		return
	}

	sendJSONResponse(w, http.StatusOK, analytics)
}
