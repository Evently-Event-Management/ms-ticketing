package ticket_api

import (
	"encoding/json"
	"fmt"
	"ms-ticketing/internal/auth"
	"ms-ticketing/internal/config"
	"ms-ticketing/internal/models"
	qr_genrator "ms-ticketing/internal/tickets/qr_genrator"
	tickets "ms-ticketing/internal/tickets/service" // Ensure this path matches the actual location of TicketService
	"net/http"
	"net/url"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-redis/redis/v8"
)

const SCANNER_ROLE = "SCANNER"

type OrderDBLayer interface {
	GetOrderByID(id string) (*models.Order, error)
}

type Handler struct {
	TicketService *tickets.TicketService
	OrderDB       OrderDBLayer
	Config        *config.Config
	QRGenerator   *qr_genrator.QRGenerator
	HTTPClient    *http.Client
	RedisClient   interface{} // Using interface{} to avoid import issues, can be *redis.Client
}

// NewHandler creates a new Handler instance
func NewHandler(ticketService *tickets.TicketService, orderDB OrderDBLayer, cfg *config.Config, httpClient *http.Client, redisClient interface{}) *Handler {
	secretKey := os.Getenv("QR_SECRET_KEY")
	return &Handler{
		TicketService: ticketService,
		OrderDB:       orderDB,
		Config:        cfg,
		QRGenerator:   qr_genrator.NewQRGenerator(secretKey),
		HTTPClient:    httpClient,
		RedisClient:   redisClient,
	}
}

// CheckinTicket handles ticket check-in with QR code verification and scanner role validation
// Expected POST request body: {"encrypted_qr": "base64_encrypted_string"}
func (h *Handler) CheckinTicket(w http.ResponseWriter, r *http.Request) {
	// Parse the encrypted QR string from POST request body
	var requestBody struct {
		EncryptedQR string `json:"encrypted_qr"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if requestBody.EncryptedQR == "" {
		http.Error(w, "encrypted_qr is required", http.StatusBadRequest)
		return
	}

	// Step 1: Extract token from request and get user ID
	tokenString, err := auth.ExtractTokenFromRequest(r)
	if err != nil {
		http.Error(w, "Authorization required: "+err.Error(), http.StatusUnauthorized)
		return
	}

	userID, err := auth.ExtractUserIDFromJWT(tokenString)
	if err != nil {
		http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Step 2: Decrypt QR code to get ticket information
	if h.QRGenerator == nil {
		secretKey := os.Getenv("QR_SECRET_KEY")
		h.QRGenerator = qr_genrator.NewQRGenerator(secretKey)
	}

	ticket, err := h.QRGenerator.DecryptQRData(requestBody.EncryptedQR)
	if err != nil {
		http.Error(w, "Invalid QR code: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Step 3: Get order information to find session_id
	order, err := h.OrderDB.GetOrderByID(ticket.OrderID)
	if err != nil {
		http.Error(w, "Order not found: "+err.Error(), http.StatusNotFound)
		return
	}
	fmt.Printf("%s", order.SessionID)
	// Step 4: Verify scanner role with event seating service
	err = h.verifyScannerRole(order.SessionID, userID)
	if err != nil {
		http.Error(w, "Scanner verification failed: "+err.Error(), http.StatusForbidden)
		return
	}

	// Step 5: Proceed with ticket check-in
	ok, err := h.TicketService.Checkin(ticket.TicketID)
	if err != nil {
		http.Error(w, "Checkin failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if !ok {
		http.Error(w, "failed to checkin ticket", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("✅ Checkin successful."))
}

// verifyScannerRole makes an M2M authenticated request to verify if the user has SCANNER role for the session
func (h *Handler) verifyScannerRole(sessionID, userID string) error {
	// Check if M2M authentication should be skipped (for development/testing)
	if os.Getenv("SKIP_M2M_AUTH") == "true" {
		fmt.Printf("DEBUG: Skipping M2M auth for session_id=%s, user_id=%s, role=SCANNER\n", sessionID, userID)
		return nil
	}

	// Get M2M token for service-to-service authentication
	m2mConfig := models.Config{
		KeycloakURL:   h.Config.Auth.KeycloakURL,
		KeycloakRealm: h.Config.Auth.KeycloakRealm,
		ClientID:      h.Config.Auth.ClientID,
		ClientSecret:  h.Config.Auth.ClientSecret,
	}

	var redisClient *redis.Client
	if h.RedisClient != nil {
		if rc, ok := h.RedisClient.(*redis.Client); ok {
			redisClient = rc
		}
	}

	token, err := auth.GetM2MToken(m2mConfig, h.HTTPClient, redisClient, nil)
	if err != nil {
		return fmt.Errorf("failed to get M2M token: %w", err)
	}

	// Create URL with query parameters
	baseURL := h.Config.EventSeatingService.URL
	endpoint := fmt.Sprintf("%s/internal/v1/sessions/verify-role", baseURL)

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid service URL: %w", err)
	}

	q := u.Query()
	q.Set("sessionId", sessionID)
	q.Set("userId", userID)
	q.Set("role", SCANNER_ROLE)
	u.RawQuery = q.Encode()
	// Create authenticated request
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add M2M token to Authorization header
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Make authenticated request to event seating service
	resp, err := h.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to verify scanner role: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scanner role verification failed with status: %d", resp.StatusCode)
	}

	return nil
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

// GetCheckedInCountBySession returns the count of checked-in tickets for a given session
// Expected POST request body: {"session_id": "uuid-string"}
func (h *Handler) GetCheckedInCountBySession(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req struct {
		SessionID string `json:"session_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate session_id is provided
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	// Get checked-in count from service
	count, err := h.TicketService.GetCheckedInCountBySession(req.SessionID)
	if err != nil {
		http.Error(w, "Failed to get checked-in count: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	response := map[string]interface{}{
		"session_id":        req.SessionID,
		"checked_in_count": count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
