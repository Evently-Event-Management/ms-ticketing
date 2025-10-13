package models

// BatchOwnershipRequest represents a request to verify ownership of multiple events
type BatchOwnershipRequest struct {
	EventIDs []string `json:"eventIds"`
	UserID   string   `json:"userId"`
}

// BatchOwnershipResponse represents the response from a batch ownership verification request
// Contains a list of events that the user owns
type BatchOwnershipResponse struct {
	OwnedEvents []string `json:"ownedEvents"`
}
