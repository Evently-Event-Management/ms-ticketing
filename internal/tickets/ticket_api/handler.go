package ticket_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/models"
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

func (h *Handler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	var ticket models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&ticket); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.TicketService.PlaceTicket(ticket); err != nil {
		http.Error(w, "Failed to create ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("Ticket created: %s", ticket.TicketID)))
}

func (h *Handler) DeleteTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")
	if err := h.TicketService.CancelTicket(ticketID); err != nil {
		http.Error(w, "Failed to delete ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")
	var updateData models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.TicketService.UpdateTicket(ticketID, updateData); err != nil {
		http.Error(w, "Failed to update ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ticket updated successfully"))
}

func (h *Handler) ViewTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")
	ticket, err := h.TicketService.GetTicket(ticketID)
	if err != nil {
		http.Error(w, "Ticket not found: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ticket)
}

func (h *Handler) ListTicketsByOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderID")
	tickets, err := h.TicketService.GetTicketsByOrder(orderID)
	if err != nil {
		http.Error(w, "Failed to fetch tickets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tickets)
}

func (h *Handler) ListTicketsByUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	tickets, err := h.TicketService.GetTicketsByUser(userID)
	if err != nil {
		http.Error(w, "Failed to fetch tickets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tickets)
}
