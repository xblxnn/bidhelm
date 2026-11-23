package runtime

import (
	"bidhelm/internal/model"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestIPAuctionAndHotRevision(t *testing.T) {
	e, s := fixture()
	s.Schema = model.Schema
	e.Enrich = func(ip, ua string) model.Facts { return model.Facts{IP: model.ParseUserIP(ip), Country: "US"} }
	base := s.Campaigns[0]
	base.ID = "ip"
	base.CPM = 1200000
	base.URL = "https://example.com/ip"
	base.Targeting.IP = model.IPTargeting{Enabled: true, Entries: []string{"131.34.123.0/24"}}
	all := base
	all.ID = "all"
	all.CPM = 1300000
	all.URL = "https://example.com/all"
	all.Targeting.IP = model.IPTargeting{}
	s.Campaigns = []model.Campaign{base, all}
	apply := func() {
		t.Helper()
		if err := e.Apply(bytes(s)); err != nil {
			t.Fatal(err)
		}
	}
	bid := func(ip string) (int, string) {
		body, _ := json.Marshal(map[string]string{"id": "ip-test", "type": "popunder", "sub": "445566", "ip": ip})
		w := httptest.NewRecorder()
		e.Traffic(w, httptest.NewRequest("POST", "/source/s", strings.NewReader(string(body))))
		var out struct {
			Bid struct {
				URL string `json:"url"`
			}
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out.Bid.URL
	}
	apply()
	if status, u := bid("131.34.123.159"); status != 200 || u != base.URL {
		t.Fatal("matched IP 1.2 must beat All 1.3", status, u)
	}
	s.Revision++
	s.Campaigns[0].CPM = 2000000
	apply()
	if _, u := bid("131.34.123.159"); u != base.URL {
		t.Fatal("matching higher bid lost")
	}
	for _, ip := range []string{"8.8.8.8", ""} {
		if _, u := bid(ip); u != all.URL {
			t.Fatal("mismatching IP was admitted")
		}
	}
	s.Revision++
	s.Campaigns[0].Targeting.Countries = []string{"GB"}
	apply()
	if _, u := bid("131.34.123.159"); u != all.URL {
		t.Fatal("IP bypassed country targeting")
	}
	s.Revision++
	s.Campaigns[0].Targeting.Countries = nil
	s.Campaigns[0].Targeting.IP.Enabled = false
	apply()
	if _, u := bid("8.8.8.8"); u != base.URL {
		t.Fatal("disabled not All")
	}
	s.Revision++
	s.Campaigns[0].Targeting.IP.Enabled = true
	apply()
	if _, u := bid("8.8.8.8"); u != all.URL {
		t.Fatal("re-enable lost saved list")
	}
	stable := e.Status().Revision
	s.Revision++
	s.Campaigns[0].Targeting.IP.Entries = []string{"broken"}
	if e.Apply(bytes(s)) == nil || e.Status().Revision != stable {
		t.Fatal("invalid snapshot replaced valid")
	}
}
func TestIPSchemaCompatibility(t *testing.T) {
	e, s := fixture()
	s.Schema = 1
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	s.Revision++
	s.Campaigns[0].Targeting.IP = model.IPTargeting{Enabled: true, Entries: []string{"10.0.0.1"}}
	if e.Apply(bytes(s)) == nil {
		t.Fatal("schema 1 silently accepted IP filter")
	}
	s.Schema = model.Schema
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal("identical retry", err)
	}
	s.Campaigns[0].Targeting.IP.Enabled = false
	if e.Apply(bytes(s)) == nil {
		t.Fatal("same revision content conflict accepted")
	}
	s.Revision = 1
	if e.Apply(bytes(s)) == nil {
		t.Fatal("old revision accepted")
	}
}
func TestIPConcurrentRevision(t *testing.T) {
	e, s := fixture()
	s.Schema = model.Schema
	s.Campaigns[0].Targeting.IP = model.IPTargeting{Enabled: true, Entries: []string{"10.0.0.0/8"}}
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				current := e.current.Load()
				got := Choose(current.Candidates, model.Facts{IP: model.ParseUserIP("10.1.2.3")})
				want := current.Snapshot.Campaigns[0].Targeting.IP.Entries[0] == "10.0.0.0/8"
				if (got != nil) != want {
					t.Error("mixed configuration revision")
				}
			}
		}()
	}
	for i := 2; i < 60; i++ {
		s.Revision = int64(i)
		entry := "10.0.0.0/8"
		if i%2 == 0 {
			entry = "192.168.0.0/16"
		}
		s.Campaigns[0].Targeting.IP.Entries = []string{entry}
		if err := e.Apply(bytes(s)); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}
