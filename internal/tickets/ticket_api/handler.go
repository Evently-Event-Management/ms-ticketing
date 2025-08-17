package ticket_api

import (
	tickets "ms-ticketing/internal/tickets/service" // Ensure this path matches the actual location of TicketService
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	TicketService *tickets.TicketService
}

func (h *Handler) CheckinTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")

	ok, err := h.TicketService.Checkin(ticketID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !ok {
		http.Error(w, "failed to checkin ticket", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("✅ Checkin successful."))
}
