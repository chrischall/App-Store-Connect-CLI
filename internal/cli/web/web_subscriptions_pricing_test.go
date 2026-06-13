package web

import (
	"testing"

	webcore "github.com/rudrankriyam/App-Store-Connect-CLI/internal/web"
)

func TestExistingMonthlyAvailabilityOnlyRejectsConfirmedMissingTerritory(t *testing.T) {
	unloaded := webcore.SubscriptionPlanAvailability{
		ID:                         "plan-monthly",
		PlanType:                   "MONTHLY",
		AvailableTerritoriesLoaded: false,
	}
	if availabilityExcludesTerritory(unloaded, "NOR") {
		t.Fatal("unloaded relationship must not be treated as confirmed missing")
	}

	loaded := unloaded
	loaded.AvailableTerritoriesLoaded = true
	loaded.AvailableTerritories = []string{"DEU"}
	if !availabilityExcludesTerritory(loaded, "NOR") {
		t.Fatal("loaded relationship should confirm the territory is missing")
	}
}
