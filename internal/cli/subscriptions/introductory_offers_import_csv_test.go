package subscriptions

import (
	"os"
	"testing"
	"time"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
)

func TestParseSubscriptionIntroductoryOffersImportCSVHeader_StripsUTF8BOM(t *testing.T) {
	got, err := parseSubscriptionIntroductoryOffersImportCSVHeader([]string{"\ufeffterritory", "offer_mode"})
	if err != nil {
		t.Fatalf("parseSubscriptionIntroductoryOffersImportCSVHeader() error: %v", err)
	}
	if got["territory"] != 0 {
		t.Fatalf("expected territory column at index 0, got %d", got["territory"])
	}
	if got["offer_mode"] != 1 {
		t.Fatalf("expected offer_mode column at index 1, got %d", got["offer_mode"])
	}
}

func TestWriteSubscriptionIntroductoryOfferImportFailureArtifact_ReturnsWriteError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".asc", []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := writeSubscriptionIntroductoryOfferImportFailureArtifact(&subscriptionIntroductoryOfferImportSummary{
		Failed:  1,
		Results: []subscriptionIntroductoryOfferImportResultItem{{Status: "failed"}},
	})
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
}

func TestSubscriptionIntroductoryOfferImportStateMatchesImmediateUpfrontOffer(t *testing.T) {
	index := &subscriptionIntroductoryOfferImportStateIndex{
		now: time.Date(2026, time.June, 24, 12, 0, 0, 0, time.UTC),
		offers: []subscriptionIntroductoryOfferImportResolvedRow{{
			territory:       "USA",
			offerMode:       "FREE_TRIAL",
			offerDuration:   "ONE_WEEK",
			numberOfPeriods: 1,
			startDate:       "2026-06-23",
			planType:        asc.SubscriptionPlanTypeUpfront,
		}},
	}
	target := subscriptionIntroductoryOfferImportResolvedRow{
		territory:       "USA",
		offerMode:       "FREE_TRIAL",
		offerDuration:   "ONE_WEEK",
		numberOfPeriods: 1,
		planType:        asc.SubscriptionPlanTypeUpfront,
	}
	if !index.matches(target) {
		t.Fatal("expected an already-active UPFRONT offer to match an immediate target")
	}

	index.offers[0].startDate = "2026-06-25"
	if index.matches(target) {
		t.Fatal("expected a future offer not to match an immediate target")
	}

	index.offers[0].startDate = "2026-06-23"
	index.offers[0].planType = asc.SubscriptionPlanTypeMonthly
	if index.matches(target) {
		t.Fatal("expected a MONTHLY offer not to match an UPFRONT target")
	}
}
