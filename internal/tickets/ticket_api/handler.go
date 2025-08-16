package ticket_api

import (
	"github.com/go-chi/chi/v5"
	"ms-ticketing/internal/tickets"
	"net/http"
)

type Handler struct {
	TicketService *tickets.TicketService
}

func (h *Handler) CheckoutTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")

	pdf, err := h.TicketService.Checkout(ticketID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=ticket.pdf")
	w.Write(pdf)
}
