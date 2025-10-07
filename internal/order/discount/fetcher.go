package discount

import (
	"encoding/json"
	"fmt"
	"io"
	"ms-ticketing/internal/models"
	"net/http"
	"os"

	"ms-ticketing/internal/logger"
)

// DiscountFetcher fetches discount information from the discount service
type DiscountFetcher struct {
	client *http.Client
	logger *logger.Logger
}

// NewDiscountFetcher creates a new DiscountFetcher
func NewDiscountFetcher(client *http.Client) *DiscountFetcher {
	return &DiscountFetcher{
		client: client,
		logger: logger.NewLogger(),
	}
}

// FetchDiscountByID fetches a discount by its ID from the discount service
func (df *DiscountFetcher) FetchDiscountByID(discountID string, m2mToken string) (*models.Discount, error) {
	if discountID == "" {
		return nil, nil
	}

	discountServiceURL := os.Getenv("DISCOUNT_SERVICE_URL")
	if discountServiceURL != "" && discountServiceURL[len(discountServiceURL)-1] == '/' {
		discountServiceURL = discountServiceURL[:len(discountServiceURL)-1]
	}

	url := fmt.Sprintf("%s/internal/v1/discounts/%s", discountServiceURL, discountID)
	df.logger.Debug("DISCOUNT", fmt.Sprintf("Fetching discount: %s", url))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		df.logger.Error("DISCOUNT", fmt.Sprintf("Failed to create discount request: %v", err))
		return nil, fmt.Errorf("failed to create discount request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+m2mToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := df.client.Do(req)
	if err != nil {
		df.logger.Error("DISCOUNT", fmt.Sprintf("Discount service error: %v", err))
		return nil, fmt.Errorf("discount service error: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			df.logger.Error("DISCOUNT", fmt.Sprintf("Failed to close discount response body: %v", err))
		}
	}(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		df.logger.Warn("DISCOUNT", fmt.Sprintf("Discount not found: %s", discountID))
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		df.logger.Error("DISCOUNT", fmt.Sprintf("Discount service returned status: %d", resp.StatusCode))
		return nil, fmt.Errorf("discount service returned status: %d", resp.StatusCode)
	}

	var discount models.Discount
	if err := json.NewDecoder(resp.Body).Decode(&discount); err != nil {
		df.logger.Error("DISCOUNT", fmt.Sprintf("Failed to decode discount response: %v", err))
		return nil, fmt.Errorf("failed to decode discount response: %w", err)
	}

	df.logger.Info("DISCOUNT", fmt.Sprintf("Discount fetched successfully: %s", discount.Code))
	return &discount, nil
}

// IncrementDiscountUsage increments the usage count for a discount
func (df *DiscountFetcher) IncrementDiscountUsage(discountID string, m2mToken string) error {
	if discountID == "" {
		return nil
	}

	discountServiceURL := os.Getenv("DISCOUNT_SERVICE_URL")
	if discountServiceURL != "" && discountServiceURL[len(discountServiceURL)-1] == '/' {
		discountServiceURL = discountServiceURL[:len(discountServiceURL)-1]
	}

	url := fmt.Sprintf("%s/internal/v1/discounts/%s/increment-usage", discountServiceURL, discountID)
	df.logger.Debug("DISCOUNT", fmt.Sprintf("Incrementing discount usage: %s", url))

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		df.logger.Error("DISCOUNT", fmt.Sprintf("Failed to create increment usage request: %v", err))
		return fmt.Errorf("failed to create increment usage request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+m2mToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := df.client.Do(req)
	if err != nil {
		df.logger.Error("DISCOUNT", fmt.Sprintf("Increment usage service error: %v", err))
		return fmt.Errorf("increment usage service error: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			df.logger.Error("DISCOUNT", fmt.Sprintf("Failed to close increment usage response body: %v", err))
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		df.logger.Error("DISCOUNT", fmt.Sprintf("Increment usage service returned status: %d", resp.StatusCode))
		return fmt.Errorf("increment usage service returned status: %d", resp.StatusCode)
	}

	df.logger.Info("DISCOUNT", fmt.Sprintf("Discount usage incremented successfully: %s", discountID))
	return nil
}
