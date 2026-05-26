package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateInAppPurchaseSubmissionSendsHiddenAttachPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inAppPurchaseSubmissions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var payload struct {
			Data struct {
				Type          string `json:"type"`
				Attributes    map[string]bool
				Relationships map[string]struct {
					Data struct {
						Type string `json:"type"`
						ID   string `json:"id"`
					} `json:"data"`
				} `json:"relationships"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if payload.Data.Type != "inAppPurchaseSubmissions" {
			t.Fatalf("expected type inAppPurchaseSubmissions, got %q", payload.Data.Type)
		}
		if !payload.Data.Attributes["submitWithNextAppStoreVersion"] {
			t.Fatalf("expected hidden attach flag to be true, got %#v", payload.Data.Attributes)
		}
		relationship := payload.Data.Relationships["inAppPurchaseV2"].Data
		if relationship.Type != "inAppPurchases" || relationship.ID != "iap-1" {
			t.Fatalf("unexpected inAppPurchaseV2 relationship: %#v", relationship)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "submission-1",
				"type": "inAppPurchaseSubmissions",
				"attributes": {"submitWithNextAppStoreVersion": true},
				"relationships": {
					"inAppPurchaseV2": {"data": {"type": "inAppPurchases", "id": "iap-1"}}
				}
			}
		}`))
	}))
	defer server.Close()

	client := testWebClient(server)
	got, err := client.CreateInAppPurchaseSubmission(context.Background(), "iap-1")
	if err != nil {
		t.Fatalf("CreateInAppPurchaseSubmission() error = %v", err)
	}
	if got.ID != "submission-1" || got.InAppPurchaseID != "iap-1" || !got.SubmitWithNextAppStoreVersion {
		t.Fatalf("unexpected submission payload: %#v", got)
	}
}

func TestCreateInAppPurchaseSubmissionFallsBackToRequestedIDWhenRelationshipMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"id": "submission-2",
				"type": "inAppPurchaseSubmissions",
				"attributes": {"submitWithNextAppStoreVersion": true}
			}
		}`))
	}))
	defer server.Close()

	client := testWebClient(server)
	got, err := client.CreateInAppPurchaseSubmission(context.Background(), "iap-fallback")
	if err != nil {
		t.Fatalf("CreateInAppPurchaseSubmission() error = %v", err)
	}
	if got.InAppPurchaseID != "iap-fallback" {
		t.Fatalf("expected fallback InAppPurchaseID, got %q", got.InAppPurchaseID)
	}
	if got.ID != "submission-2" || !got.SubmitWithNextAppStoreVersion {
		t.Fatalf("unexpected payload: %#v", got)
	}
}

func TestCreateInAppPurchaseSubmissionRejectsEmptyID(t *testing.T) {
	client := &Client{}
	if _, err := client.CreateInAppPurchaseSubmission(context.Background(), "  "); err == nil {
		t.Fatal("expected error for empty iap id, got nil")
	}
}
