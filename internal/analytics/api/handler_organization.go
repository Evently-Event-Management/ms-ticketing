package analytics_api

import (
	"fmt"
	"ms-ticketing/internal/auth"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetOrganizationAnalytics handles analytics request for an organization
func (h *Handler) GetOrganizationAnalytics(w http.ResponseWriter, r *http.Request) {
	organizationID := chi.URLParam(r, "organizationId")
	if organizationID == "" {
		h.Logger.Error("ANALYTICS", "organization_id is required")
		sendJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "organization_id is required"})
		return
	}

	// Extract user ID from context (injected by auth middleware)
	userID := auth.UserID(r.Context())
	if userID == "" {
		h.Logger.Error("ANALYTICS", "User ID not found in context")
		sendJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized access"})
		return
	}

	// Verify organization membership before proceeding
	isMember, err := h.verifyOrganizationOwnership(organizationID, userID)
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error verifying organization membership: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify organization membership"})
		return
	}

	if !isMember {
		h.Logger.Warn("ANALYTICS", fmt.Sprintf("User %s attempted to access analytics for organization %s without membership", userID, organizationID))
		sendJSONResponse(w, http.StatusForbidden, map[string]string{"error": "You do not have permission to access these analytics"})
		return
	}

	// Only consider orders with status "completed"
	analytics, err := h.Service.GetOrganizationAnalytics(r.Context(), organizationID, "completed")
	if err != nil {
		h.Logger.Error("ANALYTICS", "Error getting organization analytics: "+err.Error())
		sendJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": "Failed to get analytics"})
		return
	}

	sendJSONResponse(w, http.StatusOK, analytics)
}
