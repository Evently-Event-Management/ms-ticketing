package discount

import (
	"errors"
	"fmt"
	"ms-ticketing/internal/logger"
	"ms-ticketing/internal/models"
	"sort"
	"time"
)

// DiscountService handles validation and calculation of discounts
type DiscountService struct {
	logger *logger.Logger
}

// NewDiscountService creates a new DiscountService instance
func NewDiscountService() *DiscountService {
	return &DiscountService{
		logger: logger.NewLogger(),
	}
}

// ApplyDiscountResult represents the result of applying a discount
type ApplyDiscountResult struct {
	IsValid         bool     // Whether the discount is valid and applicable
	DiscountAmount  float64  // Amount of the discount to be applied
	Reason          string   // Reason why discount was not applied (if invalid)
	ApplicableTiers []string // Tiers to which the discount applies
}

// ValidateAndCalculateDiscount validates and calculates the discount for an order
func (s *DiscountService) ValidateAndCalculateDiscount(
	discount *models.Discount,
	seats []models.SeatDetails,
	orderSessionID string,
) (*ApplyDiscountResult, error) {
	// Step 0: Initialize the result
	result := &ApplyDiscountResult{
		IsValid:         false,
		DiscountAmount:  0,
		Reason:          "",
		ApplicableTiers: make([]string, 0),
	}

	// If no discount provided, return zero discount
	if discount == nil {
		return result, nil
	}

	// Step 1: Perform universal pre-condition checks
	// Check if discount is active
	if !discount.Active {
		result.Reason = "Discount is not active"
		return result, nil
	}

	// Check if current date is within the discount window
	now := time.Now()
	if !now.Equal(discount.ActiveFrom) && !now.After(discount.ActiveFrom) {
		result.Reason = "Discount is not yet active"
		return result, nil
	}
	if !now.Equal(discount.ExpiresAt) && !now.Before(discount.ExpiresAt) {
		result.Reason = "Discount has expired"
		return result, nil
	}

	// Check usage limits
	if discount.MaxUsage > 0 && discount.CurrentUsage >= discount.MaxUsage {
		result.Reason = "Discount usage limit has been reached"
		return result, nil
	}

	// Check session applicability
	if len(discount.ApplicableSessionIds) > 0 {
		sessionFound := false
		for _, sessionID := range discount.ApplicableSessionIds {
			if sessionID == orderSessionID {
				sessionFound = true
				break
			}
		}
		if !sessionFound {
			result.Reason = "Discount is not applicable to this session"
			return result, nil
		}
	}

	// Extract applicable tier IDs for easy lookup
	applicableTierIDs := make(map[string]bool)
	for _, tier := range discount.ApplicableTiers {
		applicableTierIDs[tier.ID] = true
		result.ApplicableTiers = append(result.ApplicableTiers, tier.ID)
	}

	// Identify applicable items and calculate subtotals
	var applicableItems []models.SeatDetails
	var applicableItemsSubtotal float64
	var cartSubtotal float64

	for _, seat := range seats {
		cartSubtotal += seat.Tier.Price

		// If applicable tiers list is empty, all items are applicable
		// Otherwise, check if this item's tier is in the applicable tiers list
		if len(discount.ApplicableTiers) == 0 || applicableTierIDs[seat.Tier.ID] {
			applicableItems = append(applicableItems, seat)
			applicableItemsSubtotal += seat.Tier.Price
		}
	}

	// Check tier applicability - if there are applicable tiers defined but no items match
	if len(discount.ApplicableTiers) > 0 && len(applicableItems) == 0 {
		result.Reason = "No items in cart match the discount's applicable tiers"
		return result, nil
	}

	// Check minimum spend for PERCENTAGE and FLAT_OFF types
	if discount.Parameters.Type == models.PERCENTAGE || discount.Parameters.Type == models.FLAT_OFF {
		if discount.Parameters.MinSpend != nil && cartSubtotal < *discount.Parameters.MinSpend {
			result.Reason = fmt.Sprintf("Cart subtotal does not meet minimum spend requirement of %.2f", *discount.Parameters.MinSpend)
			return result, nil
		}
	}

	// Step 3: Calculate discount amount based on type
	var discountAmount float64
	switch discount.Parameters.Type {
	case models.FLAT_OFF:
		// FLAT_OFF: Fixed amount off, capped at the applicable items subtotal
		if discount.Parameters.Amount == nil {
			return nil, errors.New("amount parameter is required for FLAT_OFF discount type")
		}
		discountAmount = *discount.Parameters.Amount
		if discountAmount > applicableItemsSubtotal {
			discountAmount = applicableItemsSubtotal
		}

	case models.PERCENTAGE:
		// PERCENTAGE: Percentage off the applicable items subtotal, optionally capped
		if discount.Parameters.Percentage == nil {
			return nil, errors.New("percentage parameter is required for PERCENTAGE discount type")
		}
		discountAmount = applicableItemsSubtotal * (*discount.Parameters.Percentage / 100)

		// Apply max discount cap if specified
		if discount.Parameters.MaxDiscount != nil && discountAmount > *discount.Parameters.MaxDiscount {
			discountAmount = *discount.Parameters.MaxDiscount
		}

	case models.BUY_N_GET_N_FREE:
		// BUY_N_GET_N_FREE: Buy N items, get M items free (cheapest items are free)
		if discount.Parameters.BuyQuantity == nil || discount.Parameters.GetQuantity == nil {
			return nil, errors.New("buyQuantity and getQuantity parameters are required for BUY_N_GET_N_FREE discount type")
		}

		buyQty := *discount.Parameters.BuyQuantity
		getQty := *discount.Parameters.GetQuantity

		// Check if there are enough applicable items
		if len(applicableItems) < buyQty {
			result.Reason = fmt.Sprintf("Not enough applicable items for BOGO discount (need %d, have %d)", buyQty, len(applicableItems))
			return result, nil
		}

		// Sort applicable items by price (cheapest first)
		sort.Slice(applicableItems, func(i, j int) bool {
			return applicableItems[i].Tier.Price < applicableItems[j].Tier.Price
		})

		// Calculate number of free items
		numFreeItems := (len(applicableItems) / buyQty) * getQty
		if numFreeItems > len(applicableItems) {
			numFreeItems = len(applicableItems)
		}

		// Sum up the prices of the cheapest items that are free
		for i := 0; i < numFreeItems; i++ {
			discountAmount += applicableItems[i].Tier.Price
		}

	default:
		return nil, fmt.Errorf("unsupported discount type: %s", discount.Parameters.Type)
	}

	// Final validation: discount cannot exceed the cart subtotal
	if discountAmount > cartSubtotal {
		discountAmount = cartSubtotal
	}

	// Set result values
	result.IsValid = true
	result.DiscountAmount = discountAmount

	return result, nil
}
