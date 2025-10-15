package order_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"
	"ms-ticketing/internal/sse"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-redis/redis/v8"
)

// SSEHandler manages Server-Sent Events endpoints for checkout events
type SSEHandler struct {
	Logger       *logger.Logger
	EventEmitter *sse.CheckoutEventEmitter
	RedisClient  *redis.Client
}

// NewSSEHandler creates a new SSE handler for checkout events
func NewSSEHandler(logger *logger.Logger, redisClient *redis.Client) *SSEHandler {
	return &SSEHandler{
		Logger:       logger,
		EventEmitter: sse.NewCheckoutEventEmitter(),
		RedisClient:  redisClient,
	}
}

// HandleOrganizationCheckouts streams checkout events for a specific organization
func (h *SSEHandler) HandleOrganizationCheckouts(w http.ResponseWriter, r *http.Request) {
	// Extract organization ID from URL
	organizationID := chi.URLParam(r, "organizationID")
	if organizationID == "" {
		http.Error(w, "Organization ID is required", http.StatusBadRequest)
		return
	}

	// Verify ownership/permissions
	err := h.verifyOrganizationAccess(r, organizationID)
	if err != nil {
		h.Logger.Error("SSE", fmt.Sprintf("Organization access verification failed: %v", err))
		http.Error(w, "Unauthorized access", http.StatusUnauthorized)
		return
	}

	// Set headers for SSE
	h.setupSSEHeaders(w)

	// Create a context that cancels when the client disconnects
	ctx := r.Context()

	// Subscribe to events for this organization
	eventChan := h.EventEmitter.SubscribeToOrganization(ctx, organizationID)

	// Send initial connection established message
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\",\"organizationID\":\"%s\"}\n\n", organizationID)
	w.(http.Flusher).Flush()

	h.Logger.Info("SSE", fmt.Sprintf("Client connected to organization checkout events for organization: %s", organizationID))

	// Stream events
	for {
		select {
		case order, ok := <-eventChan:
			if !ok {
				h.Logger.Debug("SSE", fmt.Sprintf("Channel closed for organization: %s", organizationID))
				return
			}

			// Serialize the order
			jsonData, err := json.Marshal(order)
			if err != nil {
				h.Logger.Error("SSE", fmt.Sprintf("Failed to serialize checkout event: %v", err))
				continue
			}

			// Send the event
			fmt.Fprintf(w, "event: checkout\ndata: %s\n\n", jsonData)
			w.(http.Flusher).Flush()

		case <-ctx.Done():
			h.Logger.Debug("SSE", fmt.Sprintf("Client disconnected from organization checkout events for: %s", organizationID))
			return
		}
	}
}

// HandleEventCheckouts streams checkout events for a specific event
func (h *SSEHandler) HandleEventCheckouts(w http.ResponseWriter, r *http.Request) {
	// Extract event ID from URL
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "Event ID is required", http.StatusBadRequest)
		return
	}

	// Verify ownership/permissions for this event
	err := h.verifyEventAccess(r, eventID)
	if err != nil {
		h.Logger.Error("SSE", fmt.Sprintf("Event access verification failed: %v", err))
		http.Error(w, "Unauthorized access", http.StatusUnauthorized)
		return
	}

	// Set headers for SSE
	h.setupSSEHeaders(w)

	// Create a context that cancels when the client disconnects
	ctx := r.Context()

	// Subscribe to events for this event
	eventChan := h.EventEmitter.SubscribeToEvent(ctx, eventID)

	// Send initial connection established message
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\",\"eventID\":\"%s\"}\n\n", eventID)
	w.(http.Flusher).Flush()

	h.Logger.Info("SSE", fmt.Sprintf("Client connected to event checkout events for event: %s", eventID))

	// Stream events
	for {
		select {
		case order, ok := <-eventChan:
			if !ok {
				h.Logger.Debug("SSE", fmt.Sprintf("Channel closed for event: %s", eventID))
				return
			}

			// Serialize the order
			jsonData, err := json.Marshal(order)
			if err != nil {
				h.Logger.Error("SSE", fmt.Sprintf("Failed to serialize checkout event: %v", err))
				continue
			}

			// Send the event
			fmt.Fprintf(w, "event: checkout\ndata: %s\n\n", jsonData)
			w.(http.Flusher).Flush()

		case <-ctx.Done():
			h.Logger.Debug("SSE", fmt.Sprintf("Client disconnected from event checkout events for: %s", eventID))
			return
		}
	}
}

// EmitCheckoutEvent broadcasts a checkout event to all subscribed clients
// This method should be called when a checkout is successful
func (h *SSEHandler) EmitCheckoutEvent(order models.OrderWithTickets) {
	h.EventEmitter.EmitCheckoutEvent(order)
}

// Helper function to set up SSE headers
func (h *SSEHandler) setupSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream;charset=UTF-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "0")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

// Helper function to verify organization access
func (h *SSEHandler) verifyOrganizationAccess(r *http.Request, organizationID string) error {
	// Extract JWT token
	token, err := auth.ExtractTokenFromRequest(r)
	if err != nil {
		return fmt.Errorf("failed to extract token: %w", err)
	}

	// Get user ID from token
	userID, err := auth.ExtractUserIDFromJWT(token)
	if err != nil {
		return fmt.Errorf("failed to extract user ID: %w", err)
	}

	// Use our standard verification method
	isMember, err := h.verifyOrganizationOwnership(organizationID, userID)
	if err != nil {
		return fmt.Errorf("failed to verify organization ownership: %w", err)
	}

	if !isMember {
		return fmt.Errorf("user %s is not a member of organization %s", userID, organizationID)
	}

	return nil
}

// Helper function to verify event access
func (h *SSEHandler) verifyEventAccess(r *http.Request, eventID string) error {
	// Extract JWT token
	token, err := auth.ExtractTokenFromRequest(r)
	if err != nil {
		return fmt.Errorf("failed to extract token: %w", err)
	}

	// Get user ID from token
	userID, err := auth.ExtractUserIDFromJWT(token)
	if err != nil {
		return fmt.Errorf("failed to extract user ID: %w", err)
	}

	// Use our standard verification method
	isOwner, err := h.verifyEventOwnership(eventID, userID)
	if err != nil {
		return fmt.Errorf("failed to verify event ownership: %w", err)
	}

	if !isOwner {
		return fmt.Errorf("user %s does not have access to event %s", userID, eventID)
	}

	return nil
}

// Helper function to get config from environment variables
func getConfigFromEnv() models.Config {
	return models.Config{
		ClientID:      getEnv("TICKET_CLIENT_ID", ""),
		ClientSecret:  getEnv("TICKET_CLIENT_SECRET", ""),
		KeycloakURL:   getEnv("KEYCLOAK_URL", ""),
		KeycloakRealm: getEnv("KEYCLOAK_REALM", ""),
	}
}

// Helper function to get environment variable with default
func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
