package runtime

import (
	"bidhelm/internal/model"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestPlacementAuctionAndURL(t *testing.T) {
	e, s := fixture()
	s.Schema = model.Schema
	c := s.Campaigns[0]
	c.ID = "target"
	c.CPM = 1200000
	c.URL = "https://example/?site={placement_id}"
	c.Targeting.Placement = model.PlacementTargeting{Enabled: true, Mode: "whitelist", Entries: []string{"445566"}}
	all := s.Campaigns[0]
	all.CPM = 1300000
	s.Campaigns = []model.Campaign{c, all}
	apply := func() {
		t.Helper()
		if err := e.Apply(bytes(s)); err != nil {
			t.Fatal(err)
		}
		s.Revision++
	}
	bid := func(sub any) (int, string) {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"id": "r", "type": "popunder", "sub": sub})
		w := httptest.NewRecorder()
		e.Traffic(w, httptest.NewRequest("POST", "/source/s", strings.NewReader(string(b))))
		var out struct{ Bid struct{ URL string } }
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Bid.URL
	}
	apply()
	if _, u := bid(445566); u != "https://example/?site=445566" {
		t.Fatal("matching whitelist must win", u)
	}
	s.Campaigns[0].CPM = 2000000
	apply()
	if _, u := bid(445566); u != "https://example/?site=445566" {
		t.Fatal(u)
	}
	if _, u := bid("other"); u != all.URL {
		t.Fatal("WL miss", u)
	}
	s.Campaigns[0].Targeting.Placement.Mode = "blacklist"
	apply()
	if _, u := bid(445566); u != all.URL {
		t.Fatal("BL hit", u)
	}
	if _, u := bid("other"); u != "https://example/?site=other" {
		t.Fatal("BL miss", u)
	}
	s.Campaigns[0].Targeting.Countries = []string{"GB"}
	apply()
	if _, u := bid("other"); u != all.URL {
		t.Fatal("AND country", u)
	}
	s.Campaigns[0].Targeting.Countries = nil
	s.Campaigns = s.Campaigns[:1]
	s.Campaigns[0].Targeting.Placement.Mode = "whitelist"
	s.Campaigns[0].Targeting.Placement.Entries = nil
	apply()
	if status, _ := bid("other"); status != 204 {
		t.Fatal("empty WL", status)
	}
	s.Campaigns[0].Targeting.Placement.Enabled = false
	apply()
	if _, u := bid("x& +"); u != "https://example/?site=x%26+%2B" {
		t.Fatal(u)
	}
	e.Enrich = func(string, string) model.Facts { t.Fatal("invalid request reached enrichment"); return model.Facts{} }
	if status, _ := bid(nil); status != 400 {
		t.Fatal(status)
	}
}
func TestPlacementConcurrentURL(t *testing.T) {
	e, s := fixture()
	s.Schema = model.Schema
	s.Campaigns[0].URL = "https://example/?site={placement_id}"
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				id := fmt.Sprintf("%d-%d", i, j)
				body := `{"id":"r","type":"popunder","sub":"` + id + `"}`
				w := httptest.NewRecorder()
				e.Traffic(w, httptest.NewRequest("POST", "/source/s", strings.NewReader(body)))
				var out struct{ Bid struct{ URL string } }
				json.Unmarshal(w.Body.Bytes(), &out)
				if out.Bid.URL != "https://example/?site="+id {
					t.Errorf("mixed URL %s", w.Body)
				}
			}
		}(i)
	}
	for i := 2; i < 20; i++ {
		s.Revision = int64(i)
		if err := e.Apply(bytes(s)); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if e.current.Load().Candidates[0].Campaign.URL != s.Campaigns[0].URL {
		t.Fatal("snapshot mutated")
	}
}
