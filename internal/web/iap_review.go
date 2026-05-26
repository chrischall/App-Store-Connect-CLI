package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const reviewIAPFields = "productId,name,state,inAppPurchaseType,isAppStoreReviewInProgress,submitWithNextAppStoreVersion"

// ReviewIAP summarizes a non-subscription IAP's attach state for the next app
// version review.
type ReviewIAP struct {
	ID                            string `json:"id"`
	ProductID                     string `json:"productId,omitempty"`
	Name                          string `json:"name,omitempty"`
	State                         string `json:"state,omitempty"`
	InAppPurchaseType             string `json:"inAppPurchaseType,omitempty"`
	IsAppStoreReviewInProgress    bool   `json:"isAppStoreReviewInProgress"`
	SubmitWithNextAppStoreVersion bool   `json:"submitWithNextAppStoreVersion"`
}

// ReviewIAPSubmission captures the hidden submission resource returned by the
// web flow that attaches a non-renewing in-app purchase to the next app version
// review. Mirrors ReviewSubscriptionSubmission but for non-subscription IAPs.
type ReviewIAPSubmission struct {
	ID                            string `json:"id"`
	InAppPurchaseID               string `json:"inAppPurchaseId,omitempty"`
	SubmitWithNextAppStoreVersion bool   `json:"submitWithNextAppStoreVersion"`
}

func decodeReviewIAP(resource jsonAPIResource) ReviewIAP {
	return ReviewIAP{
		ID:                            strings.TrimSpace(resource.ID),
		ProductID:                     stringAttr(resource.Attributes, "productId"),
		Name:                          stringAttr(resource.Attributes, "name"),
		State:                         stringAttr(resource.Attributes, "state"),
		InAppPurchaseType:             stringAttr(resource.Attributes, "inAppPurchaseType"),
		IsAppStoreReviewInProgress:    boolAttr(resource.Attributes, "isAppStoreReviewInProgress"),
		SubmitWithNextAppStoreVersion: boolAttr(resource.Attributes, "submitWithNextAppStoreVersion"),
	}
}

// FindReviewIAP finds a single app-scoped IAP through the private web flow.
func (c *Client) FindReviewIAP(ctx context.Context, appID, iapID string) (ReviewIAP, bool, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return ReviewIAP{}, false, fmt.Errorf("app id is required")
	}
	iapID = strings.TrimSpace(iapID)
	if iapID == "" {
		return ReviewIAP{}, false, fmt.Errorf("iap id is required")
	}

	query := url.Values{}
	query.Set("fields[inAppPurchases]", reviewIAPFields)
	query.Set("limit", "300")
	query.Set("sort", "name")

	nextPath := queryPath("/apps/"+url.PathEscape(appID)+"/inAppPurchases", query)
	visited := map[string]struct{}{}

	for nextPath != "" {
		if _, seen := visited[nextPath]; seen {
			return ReviewIAP{}, false, fmt.Errorf("review iaps pagination loop detected")
		}
		visited[nextPath] = struct{}{}

		responseBody, err := c.doRequest(ctx, http.MethodGet, nextPath, nil)
		if err != nil {
			return ReviewIAP{}, false, err
		}

		var payload jsonAPIListPayload
		if err := json.Unmarshal(responseBody, &payload); err != nil {
			return ReviewIAP{}, false, fmt.Errorf("failed to parse review iaps response: %w", err)
		}
		for _, resource := range payload.Data {
			if strings.TrimSpace(resource.ID) == iapID {
				return decodeReviewIAP(resource), true, nil
			}
		}

		nextLink, err := extractNextLink(payload.Links)
		if err != nil {
			return ReviewIAP{}, false, fmt.Errorf("failed to parse review iaps pagination links: %w", err)
		}
		if strings.TrimSpace(nextLink) == "" {
			break
		}
		nextPath, err = normalizeNextPath(nextLink, c.baseURL)
		if err != nil {
			return ReviewIAP{}, false, fmt.Errorf("failed to normalize review iaps pagination link: %w", err)
		}
	}

	return ReviewIAP{}, false, nil
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
	if strings.TrimSpace(payload.Data.ID) == "" {
		return ReviewIAPSubmission{}, fmt.Errorf("failed to parse iap submission response: missing submission id")
	}
	if payload.Data.Type != "" && payload.Data.Type != "inAppPurchaseSubmissions" {
		return ReviewIAPSubmission{}, fmt.Errorf("failed to parse iap submission response: unexpected resource type %q", payload.Data.Type)
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
