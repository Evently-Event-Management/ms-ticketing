package ticket_api

import (
	"encoding/json"
	"net/http"
)

// TicketCountResponse is the response format for the GetTotalTicketsCount endpoint
type TicketCountResponse struct {
	TotalCount int `json:"total_count"`
}

// GetTotalTicketsCount handles the request to get the total ticket count
func (h *Handler) GetTotalTicketsCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.TicketService.GetTotalTicketsCount()
	if err != nil {
		http.Error(w, "Error retrieving ticket count: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create the response
	response := TicketCountResponse{
		TotalCount: count,
	}

	// Set content type and status code
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Write the response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
