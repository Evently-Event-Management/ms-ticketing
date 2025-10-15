package sse

import (
	"context"
	"ms-ticketing/internal/models"
	"sync"
)

// CheckoutEventEmitter manages SSE connections and event broadcasting for checkout events
type CheckoutEventEmitter struct {
	// Organization channel clients map - key: organizationID, value: slice of client channels
	orgClients     map[string][]chan models.OrderWithTickets
	orgClientMutex sync.RWMutex

	// Event channel clients map - key: eventID, value: slice of client channels
	eventClients     map[string][]chan models.OrderWithTickets
	eventClientMutex sync.RWMutex
}

// NewCheckoutEventEmitter creates a new SSE event emitter for checkout events
func NewCheckoutEventEmitter() *CheckoutEventEmitter {
	return &CheckoutEventEmitter{
		orgClients:   make(map[string][]chan models.OrderWithTickets),
		eventClients: make(map[string][]chan models.OrderWithTickets),
	}
}

// SubscribeToOrganization adds a client to the organization's checkout events
func (e *CheckoutEventEmitter) SubscribeToOrganization(ctx context.Context, orgID string) chan models.OrderWithTickets {
	clientChan := make(chan models.OrderWithTickets, 10)

	e.orgClientMutex.Lock()
	if e.orgClients[orgID] == nil {
		e.orgClients[orgID] = []chan models.OrderWithTickets{}
	}
	e.orgClients[orgID] = append(e.orgClients[orgID], clientChan)
	e.orgClientMutex.Unlock()

	// Remove client when context is done
	go func() {
		<-ctx.Done()
		e.removeOrgClient(orgID, clientChan)
	}()

	return clientChan
}

// SubscribeToEvent adds a client to the event's checkout events
func (e *CheckoutEventEmitter) SubscribeToEvent(ctx context.Context, eventID string) chan models.OrderWithTickets {
	clientChan := make(chan models.OrderWithTickets, 10)

	e.eventClientMutex.Lock()
	if e.eventClients[eventID] == nil {
		e.eventClients[eventID] = []chan models.OrderWithTickets{}
	}
	e.eventClients[eventID] = append(e.eventClients[eventID], clientChan)
	e.eventClientMutex.Unlock()

	// Remove client when context is done
	go func() {
		<-ctx.Done()
		e.removeEventClient(eventID, clientChan)
	}()

	return clientChan
}

// EmitCheckoutEvent broadcasts a checkout event to all subscribed clients
func (e *CheckoutEventEmitter) EmitCheckoutEvent(order models.OrderWithTickets) {
	// Broadcast to organization subscribers
	e.orgClientMutex.RLock()
	orgID := order.Order.OrganizationID
	clients := e.orgClients[orgID]
	e.orgClientMutex.RUnlock()

	for _, clientChan := range clients {
		// Non-blocking send to avoid slowing down emitter if client is slow
		select {
		case clientChan <- order:
			// Successfully sent
		default:
			// Channel buffer full, skip this client for now
		}
	}

	// Broadcast to event subscribers
	e.eventClientMutex.RLock()
	eventID := order.Order.EventID
	eventClients := e.eventClients[eventID]
	e.eventClientMutex.RUnlock()

	for _, clientChan := range eventClients {
		// Non-blocking send to avoid slowing down emitter if client is slow
		select {
		case clientChan <- order:
			// Successfully sent
		default:
			// Channel buffer full, skip this client for now
		}
	}
}

// Helper methods to remove clients when they disconnect
func (e *CheckoutEventEmitter) removeOrgClient(orgID string, clientChan chan models.OrderWithTickets) {
	e.orgClientMutex.Lock()
	defer e.orgClientMutex.Unlock()

	clients := e.orgClients[orgID]
	for i, ch := range clients {
		if ch == clientChan {
			// Remove client from slice
			e.orgClients[orgID] = append(clients[:i], clients[i+1:]...)
			close(clientChan)
			break
		}
	}

	// Clean up map entry if no more clients
	if len(e.orgClients[orgID]) == 0 {
		delete(e.orgClients, orgID)
	}
}

func (e *CheckoutEventEmitter) removeEventClient(eventID string, clientChan chan models.OrderWithTickets) {
	e.eventClientMutex.Lock()
	defer e.eventClientMutex.Unlock()

	clients := e.eventClients[eventID]
	for i, ch := range clients {
		if ch == clientChan {
			// Remove client from slice
			e.eventClients[eventID] = append(clients[:i], clients[i+1:]...)
			close(clientChan)
			break
		}
	}

	// Clean up map entry if no more clients
	if len(e.eventClients[eventID]) == 0 {
		delete(e.eventClients, eventID)
	}
}

// GetOrgClientCount returns the number of clients currently subscribed to an organization
func (e *CheckoutEventEmitter) GetOrgClientCount(orgID string) int {
	e.orgClientMutex.RLock()
	defer e.orgClientMutex.RUnlock()
	return len(e.orgClients[orgID])
}

// GetEventClientCount returns the number of clients currently subscribed to an event
func (e *CheckoutEventEmitter) GetEventClientCount(eventID string) int {
	e.eventClientMutex.RLock()
	defer e.eventClientMutex.RUnlock()
	return len(e.eventClients[eventID])
}
