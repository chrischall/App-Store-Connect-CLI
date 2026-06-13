package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// SubscriptionPlanPricesResult identifies the paired billing-plan prices created.
type SubscriptionPlanPricesResult struct {
	SubscriptionID      string `json:"subscriptionId"`
	UpfrontPricePointID string `json:"upfrontPricePointId"`
	MonthlyPricePointID string `json:"monthlyPricePointId"`
}

// CreateSubscriptionPlanPrices creates paired upfront and monthly prices through
// the inline subscription PATCH used by App Store Connect.
func (c *Client) CreateSubscriptionPlanPrices(ctx context.Context, subscriptionID, upfrontPricePointID, monthlyPricePointID string) (*SubscriptionPlanPricesResult, error) {
	subscriptionID = strings.TrimSpace(subscriptionID)
	upfrontPricePointID = strings.TrimSpace(upfrontPricePointID)
	monthlyPricePointID = strings.TrimSpace(monthlyPricePointID)
	if subscriptionID == "" {
		return nil, fmt.Errorf("subscription id is required")
	}
	if upfrontPricePointID == "" {
		return nil, fmt.Errorf("upfront price point id is required")
	}
	if monthlyPricePointID == "" {
		return nil, fmt.Errorf("monthly price point id is required")
	}

	upfrontID := "${current-upfront}"
	monthlyID := "${current-monthly}"
	priceRef := func(id string) map[string]string {
		return map[string]string{"type": "subscriptionPrices", "id": id}
	}
	includedPrice := func(id, planType, pricePointID string) map[string]any {
		return map[string]any{
			"type":       "subscriptionPrices",
			"id":         id,
			"attributes": map[string]string{"planType": planType},
			"relationships": map[string]any{
				"subscriptionPricePoint": map[string]any{
					"data": map[string]string{
						"type": "subscriptionPricePoints",
						"id":   pricePointID,
					},
				},
			},
		}
	}
	requestBody := map[string]any{
		"data": map[string]any{
			"type": "subscriptions",
			"id":   subscriptionID,
			"relationships": map[string]any{
				"prices": map[string]any{
					"data": []map[string]string{
						priceRef(upfrontID),
						priceRef(monthlyID),
					},
				},
			},
		},
		"included": []map[string]any{
			includedPrice(upfrontID, "UPFRONT", upfrontPricePointID),
			includedPrice(monthlyID, "MONTHLY", monthlyPricePointID),
		},
	}

	responseBody, err := c.doRequest(ctx, http.MethodPatch, "/subscriptions/"+url.PathEscape(subscriptionID), requestBody)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data jsonAPIResource `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse subscription pricing response: %w", err)
	}
	if returnedID := strings.TrimSpace(payload.Data.ID); returnedID != "" && returnedID != subscriptionID {
		return nil, fmt.Errorf("apple returned subscription %q after patching %q", returnedID, subscriptionID)
	}
	return &SubscriptionPlanPricesResult{
		SubscriptionID:      subscriptionID,
		UpfrontPricePointID: upfrontPricePointID,
		MonthlyPricePointID: monthlyPricePointID,
	}, nil
}
