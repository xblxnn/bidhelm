package runtime

import "testing"

// Verify the distributable database against the same enrichment path as live traffic.
func TestBundledCountryDatabase(t *testing.T) {
	enricher, err := NewEnricher("../../assets/geo/dbip-country-lite.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	defer enricher.Close()
	// Expectations describe the pinned November 2026 dataset, not IP ownership.
	for ip, country := range map[string]string{"8.8.8.8": "US", "2001:4860:4860::8888": "CA", "81.2.69.160": "GB"} {
		if facts := enricher.Facts(ip, ""); facts.Country != country {
			t.Errorf("bundled DB-IP lookup %s: country=%q, want %s", ip, facts.Country, country)
		}
	}
}
