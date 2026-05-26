package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ReviewIAPSubmission captures the hidden submission resource returned by the
// web flow that attaches a non-renewing in-app purchase to the next app version
// review. Mirrors ReviewSubscriptionSubmission but for non-subscription IAPs.
type ReviewIAPSubmission struct {
	ID                            string `json:"id"`
	InAppPurchaseID               string `json:"inAppPurchaseId,omitempty"`
	SubmitWithNextAppStoreVersion bool   `json:"submitWithNextAppStoreVersion"`
}

// CreateInAppPurchaseSubmission attaches a non-renewing in-app purchase to the
// next app version review via the private web flow.
//
// This is the iris-API equivalent of clicking the IAP checkbox in
// "Add App In-App Purchase or Subscription" on the version submission page
// in App Store Connect. POST /iris/v1/inAppPurchaseSubmissions with the
// `submitWithNextAppStoreVersion` attribute set; the public ASC REST API
// has no equivalent for non-subscription IAPs.
func (c *Client) CreateInAppPurchaseSubmission(ctx context.Context, iapID string) (ReviewIAPSubmission, error) {
	iapID = strings.TrimSpace(iapID)
	if iapID == "" {
		return ReviewIAPSubmission{}, fmt.Errorf("iap id is required")
	}

	body := map[string]any{
		"data": map[string]any{
			"type": "inAppPurchaseSubmissions",
			"attributes": map[string]any{
				"submitWithNextAppStoreVersion": true,
			},
			"relationships": map[string]any{
				"inAppPurchaseV2": map[string]any{
					"data": map[string]string{
						"type": "inAppPurchases",
						"id":   iapID,
					},
				},
			},
		},
	}

	responseBody, err := c.doRequest(ctx, http.MethodPost, "/inAppPurchaseSubmissions", body)
	if err != nil {
		return ReviewIAPSubmission{}, fmt.Errorf("iris POST /inAppPurchaseSubmissions: %w", err)
	}

	var payload struct {
		Data jsonAPIResource `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		return ReviewIAPSubmission{}, fmt.Errorf("failed to parse iap submission response: %w", err)
	}

	result := ReviewIAPSubmission{
		ID:                            strings.TrimSpace(payload.Data.ID),
		SubmitWithNextAppStoreVersion: boolAttr(payload.Data.Attributes, "submitWithNextAppStoreVersion"),
	}
	if ref := firstRelationshipRef(payload.Data, "inAppPurchaseV2"); ref != nil {
		result.InAppPurchaseID = strings.TrimSpace(ref.ID)
	}
	if result.InAppPurchaseID == "" {
		result.InAppPurchaseID = iapID
	}
	return result, nil
}
