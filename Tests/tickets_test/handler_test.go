package tickets_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

// MockTicketService is a mock implementation of the ticket service used for testing handlers
type MockTicketService struct {
	// Storage to simulate behavior
	tickets       map[string]*models.Ticket
	orderTickets  map[string][]models.Ticket
	userTickets   map[string][]models.Ticket
	shouldFailOn  string
	errorToReturn error
}

// NewMockTicketService creates a new mock ticket service with test data
func NewMockTicketService() *MockTicketService {
	mockService := &MockTicketService{
		tickets:      make(map[string]*models.Ticket),
		orderTickets: make(map[string][]models.Ticket),
		userTickets:  make(map[string][]models.Ticket),
	}

	// Add sample data
	ticket := models.Ticket{
		TicketID:        "ticket1",
		OrderID:         "order1",
		SeatID:          "seat1",
		SeatLabel:       "A1",
		Colour:          "blue",
		TierID:          "tier1",
		TierName:        "VIP",
		QRCode:          []byte("qrcode"),
		PriceAtPurchase: 100.0,
		IssuedAt:        time.Now(),
		CheckedIn:       false,
	}

	mockService.tickets[ticket.TicketID] = &ticket
	mockService.orderTickets[ticket.OrderID] = append(mockService.orderTickets[ticket.OrderID], ticket)
	
	// Add a user ticket relationship
	userID := "user1"
	mockService.userTickets[userID] = append(mockService.userTickets[userID], ticket)

	return mockService
}

// SetupFailure configures the mock to fail on specific operations
func (m *MockTicketService) SetupFailure(operation string, err error) {
	m.shouldFailOn = operation
	m.errorToReturn = err
}

// Implement all the service methods required for testing

func (m *MockTicketService) Checkin(ticketID string) (bool, error) {
	if m.shouldFailOn == "Checkin" {
		return false, m.errorToReturn
	}
	
	ticket, exists := m.tickets[ticketID]
	if !exists {
		return false, fmt.Errorf("ticket %s not found", ticketID)
	}
	
	ticket.CheckedIn = true
	ticket.CheckedInTime = time.Now()
	return true, nil
}

func (m *MockTicketService) PlaceTicket(ticket models.Ticket) error {
	if m.shouldFailOn == "PlaceTicket" {
		return m.errorToReturn
	}
	
	m.tickets[ticket.TicketID] = &ticket
	m.orderTickets[ticket.OrderID] = append(m.orderTickets[ticket.OrderID], ticket)
	return nil
}

func (m *MockTicketService) CancelTicket(ticketID string) error {
	if m.shouldFailOn == "CancelTicket" {
		return m.errorToReturn
	}
	
	if _, exists := m.tickets[ticketID]; !exists {
		return fmt.Errorf("ticket %s not found", ticketID)
	}
	
	delete(m.tickets, ticketID)
	return nil
}

func (m *MockTicketService) UpdateTicket(ticketID string, updateData models.Ticket) error {
	if m.shouldFailOn == "UpdateTicket" {
		return m.errorToReturn
	}
	
	if _, exists := m.tickets[ticketID]; !exists {
		return fmt.Errorf("ticket %s not found", ticketID)
	}
	
	// Update allowed fields
	ticket := m.tickets[ticketID]
	ticket.SeatID = updateData.SeatID
	ticket.SeatLabel = updateData.SeatLabel
	ticket.Colour = updateData.Colour
	ticket.TierID = updateData.TierID
	ticket.TierName = updateData.TierName
	ticket.PriceAtPurchase = updateData.PriceAtPurchase
	ticket.QRCode = updateData.QRCode
	
	return nil
}

func (m *MockTicketService) GetTicket(ticketID string) (*models.Ticket, error) {
	if m.shouldFailOn == "GetTicket" {
		return nil, m.errorToReturn
	}
	
	ticket, exists := m.tickets[ticketID]
	if !exists {
		return nil, fmt.Errorf("ticket %s not found", ticketID)
	}
	
	return ticket, nil
}

func (m *MockTicketService) GetTicketsByOrder(orderID string) ([]models.Ticket, error) {
	if m.shouldFailOn == "GetTicketsByOrder" {
		return nil, m.errorToReturn
	}
	
	tickets, exists := m.orderTickets[orderID]
	if !exists || len(tickets) == 0 {
		return nil, fmt.Errorf("no tickets found for order %s", orderID)
	}
	
	return tickets, nil
}

func (m *MockTicketService) GetTicketsByUser(userID string) ([]models.Ticket, error) {
	if m.shouldFailOn == "GetTicketsByUser" {
		return nil, m.errorToReturn
	}
	
	tickets, exists := m.userTickets[userID]
	if !exists || len(tickets) == 0 {
		return nil, fmt.Errorf("no tickets found for user %s", userID)
	}
	
	return tickets, nil
}

// MockHandler is a modified handler that uses our mock service directly
type MockHandler struct {
	MockService *MockTicketService
}

// Implement all handler methods using the mock service

func (h *MockHandler) CheckinTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")

	ok, err := h.MockService.Checkin(ticketID)
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

func (h *MockHandler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	var ticket models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&ticket); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.MockService.PlaceTicket(ticket); err != nil {
		http.Error(w, "Failed to create ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("Ticket created: %s", ticket.TicketID)))
}

func (h *MockHandler) DeleteTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")
	if err := h.MockService.CancelTicket(ticketID); err != nil {
		http.Error(w, "Failed to delete ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *MockHandler) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")
	var updateData models.Ticket
	if err := json.NewDecoder(r.Body).Decode(&updateData); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.MockService.UpdateTicket(ticketID, updateData); err != nil {
		http.Error(w, "Failed to update ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ticket updated successfully"))
}

func (h *MockHandler) ViewTicket(w http.ResponseWriter, r *http.Request) {
	ticketID := chi.URLParam(r, "ticketID")
	ticket, err := h.MockService.GetTicket(ticketID)
	if err != nil {
		http.Error(w, "Ticket not found: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ticket)
}

func (h *MockHandler) ListTicketsByOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderID")
	tickets, err := h.MockService.GetTicketsByOrder(orderID)
	if err != nil {
		http.Error(w, "Failed to fetch tickets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tickets)
}

func (h *MockHandler) ListTicketsByUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	tickets, err := h.MockService.GetTicketsByUser(userID)
	if err != nil {
		http.Error(w, "Failed to fetch tickets: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tickets)
}

// Helper function to create a handler with a mock service for testing
func setupTestHandler() (*MockHandler, *MockTicketService) {
	mockService := NewMockTicketService()
	handler := &MockHandler{
		MockService: mockService,
	}
	return handler, mockService
}

// Test cases for each handler endpoint

func TestCheckinTicketHandler(t *testing.T) {
	// Test successful checkin
	t.Run("Successful checkin", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create a test request
		req := httptest.NewRequest("POST", "/tickets/ticket1/checkin", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Post("/tickets/{ticketID}/checkin", handler.CheckinTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Checkin successful")
	})
	
	// Test failure due to ticket not found
	t.Run("Ticket not found", func(t *testing.T) {
		handler, mockService := setupTestHandler()
		mockService.SetupFailure("Checkin", fmt.Errorf("ticket not found"))
		
		// Create a test request
		req := httptest.NewRequest("POST", "/tickets/nonexistent/checkin", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Post("/tickets/{ticketID}/checkin", handler.CheckinTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestCreateTicketHandler(t *testing.T) {
	// Test successful ticket creation
	t.Run("Successful ticket creation", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create ticket data
		ticket := models.Ticket{
			TicketID:        "new-ticket",
			OrderID:         "order2",
			SeatID:          "seat2",
			SeatLabel:       "B2",
			TierID:          "tier1",
			TierName:        "Standard",
			PriceAtPurchase: 75.0,
		}
		
		ticketJSON, _ := json.Marshal(ticket)
		
		// Create a test request
		req := httptest.NewRequest("POST", "/tickets", bytes.NewBuffer(ticketJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		// Call the handler directly
		handler.CreateTicket(w, req)
		
		// Check response
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "Ticket created: new-ticket")
	})
	
	// Test failure due to invalid JSON
	t.Run("Invalid JSON", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create invalid JSON
		invalidJSON := []byte(`{"ticketID": "invalid-json`)
		
		// Create a test request
		req := httptest.NewRequest("POST", "/tickets", bytes.NewBuffer(invalidJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		// Call the handler directly
		handler.CreateTicket(w, req)
		
		// Check response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid request body")
	})
	
	// Test failure during ticket creation
	t.Run("Failure during ticket creation", func(t *testing.T) {
		handler, mockService := setupTestHandler()
		mockService.SetupFailure("PlaceTicket", fmt.Errorf("database error"))
		
		// Create ticket data
		ticket := models.Ticket{
			TicketID:        "failed-ticket",
			OrderID:         "order3",
			SeatID:          "seat3",
			SeatLabel:       "C3",
		}
		
		ticketJSON, _ := json.Marshal(ticket)
		
		// Create a test request
		req := httptest.NewRequest("POST", "/tickets", bytes.NewBuffer(ticketJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		// Call the handler directly
		handler.CreateTicket(w, req)
		
		// Check response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to create ticket")
	})
}

func TestDeleteTicketHandler(t *testing.T) {
	// Test successful ticket deletion
	t.Run("Successful ticket deletion", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create a test request
		req := httptest.NewRequest("DELETE", "/tickets/ticket1", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Delete("/tickets/{ticketID}", handler.DeleteTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusNoContent, w.Code)
	})
	
	// Test failure due to ticket not found
	t.Run("Ticket not found", func(t *testing.T) {
		handler, mockService := setupTestHandler()
		mockService.SetupFailure("CancelTicket", fmt.Errorf("ticket not found"))
		
		// Create a test request
		req := httptest.NewRequest("DELETE", "/tickets/nonexistent", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Delete("/tickets/{ticketID}", handler.DeleteTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to delete ticket")
	})
}

func TestUpdateTicketHandler(t *testing.T) {
	// Test successful ticket update
	t.Run("Successful ticket update", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create update data
		updateData := models.Ticket{
			SeatLabel: "A2-Updated",
			TierName:  "Premium",
		}
		
		updateJSON, _ := json.Marshal(updateData)
		
		// Create a test request
		req := httptest.NewRequest("PUT", "/tickets/ticket1", bytes.NewBuffer(updateJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Put("/tickets/{ticketID}", handler.UpdateTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Ticket updated successfully")
	})
	
	// Test failure due to invalid JSON
	t.Run("Invalid JSON", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create invalid JSON
		invalidJSON := []byte(`{"seatLabel": "invalid-json`)
		
		// Create a test request
		req := httptest.NewRequest("PUT", "/tickets/ticket1", bytes.NewBuffer(invalidJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Put("/tickets/{ticketID}", handler.UpdateTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid request body")
	})
	
	// Test failure during ticket update
	t.Run("Ticket not found", func(t *testing.T) {
		handler, mockService := setupTestHandler()
		mockService.SetupFailure("UpdateTicket", fmt.Errorf("ticket not found"))
		
		// Create update data
		updateData := models.Ticket{
			SeatLabel: "Updated",
		}
		
		updateJSON, _ := json.Marshal(updateData)
		
		// Create a test request
		req := httptest.NewRequest("PUT", "/tickets/nonexistent", bytes.NewBuffer(updateJSON))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Put("/tickets/{ticketID}", handler.UpdateTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to update ticket")
	})
}

func TestViewTicketHandler(t *testing.T) {
	// Test successful ticket view
	t.Run("Successful ticket view", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create a test request
		req := httptest.NewRequest("GET", "/tickets/ticket1", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Get("/tickets/{ticketID}", handler.ViewTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Parse response body
		var responseTicket models.Ticket
		json.NewDecoder(w.Body).Decode(&responseTicket)
		
		assert.Equal(t, "ticket1", responseTicket.TicketID)
		assert.Equal(t, "A1", responseTicket.SeatLabel)
	})
	
	// Test ticket not found
	t.Run("Ticket not found", func(t *testing.T) {
		handler, mockService := setupTestHandler()
		mockService.SetupFailure("GetTicket", fmt.Errorf("ticket not found"))
		
		// Create a test request
		req := httptest.NewRequest("GET", "/tickets/nonexistent", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Get("/tickets/{ticketID}", handler.ViewTicket)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Ticket not found")
	})
}

func TestListTicketsByOrderHandler(t *testing.T) {
	// Test successful listing tickets by order
	t.Run("Successful listing tickets by order", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create a test request
		req := httptest.NewRequest("GET", "/orders/order1/tickets", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Get("/orders/{orderID}/tickets", handler.ListTicketsByOrder)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Parse response body
		var responseTickets []models.Ticket
		json.NewDecoder(w.Body).Decode(&responseTickets)
		
		assert.Equal(t, 1, len(responseTickets))
		assert.Equal(t, "ticket1", responseTickets[0].TicketID)
	})
	
	// Test no tickets found for order
	t.Run("No tickets found for order", func(t *testing.T) {
		handler, mockService := setupTestHandler()
		mockService.SetupFailure("GetTicketsByOrder", fmt.Errorf("no tickets found"))
		
		// Create a test request
		req := httptest.NewRequest("GET", "/orders/nonexistent/tickets", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Get("/orders/{orderID}/tickets", handler.ListTicketsByOrder)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to fetch tickets")
	})
}

func TestListTicketsByUserHandler(t *testing.T) {
	// Test successful listing tickets by user
	t.Run("Successful listing tickets by user", func(t *testing.T) {
		handler, _ := setupTestHandler()
		
		// Create a test request
		req := httptest.NewRequest("GET", "/users/user1/tickets", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Get("/users/{userID}/tickets", handler.ListTicketsByUser)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusOK, w.Code)
		
		// Parse response body
		var responseTickets []models.Ticket
		json.NewDecoder(w.Body).Decode(&responseTickets)
		
		assert.Equal(t, 1, len(responseTickets))
		assert.Equal(t, "ticket1", responseTickets[0].TicketID)
	})
	
	// Test no tickets found for user
	t.Run("No tickets found for user", func(t *testing.T) {
		handler, mockService := setupTestHandler()
		mockService.SetupFailure("GetTicketsByUser", fmt.Errorf("no tickets found"))
		
		// Create a test request
		req := httptest.NewRequest("GET", "/users/nonexistent/tickets", nil)
		w := httptest.NewRecorder()
		
		// Create router and add URL parameter
		r := chi.NewRouter()
		r.Get("/users/{userID}/tickets", handler.ListTicketsByUser)
		r.ServeHTTP(w, req)
		
		// Check response
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to fetch tickets")
	})
}