package runtime

import (
	"bidhelm/internal/adapters"
	"bidhelm/internal/adapters/exads_json_pop"
	"bidhelm/internal/model"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func fixture() (*Engine, model.Snapshot) {
	a := exads_json_pop.Adapter{}
	return &Engine{Registry: adapters.Registry{a.Key(): a}, Token: "test-token", Enrich: func(string, string) model.Facts { return model.Facts{Country: "US"} }}, model.Snapshot{Schema: 1, Revision: 1, Sources: []model.Source{{ID: "s", Adapter: a.Key()}}, Campaigns: []model.Campaign{{ID: "c", Active: true, URL: "https://example.com", CPM: 130000}}}
}
func bytes(s model.Snapshot) []byte { b, _ := json.Marshal(s); return b }
func TestRevisionRules(t *testing.T) {
	e, s := fixture()
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal("retry", err)
	}
	s.Campaigns[0].CPM++
	if e.Apply(bytes(s)) == nil {
		t.Fatal("content conflict accepted")
	}
	s.Revision = 2
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	s.Revision = 1
	if e.Apply(bytes(s)) == nil {
		t.Fatal("stale accepted")
	}
	s.Revision = 3
	s.Sources[0].Adapter = "missing"
	if e.Apply(bytes(s)) == nil || e.Status().Revision != 2 {
		t.Fatal("invalid replaced valid")
	}
}
func TestAuction(t *testing.T) {
	cs := []model.Campaign{{ID: "low", CPM: 1}, {ID: "a", CPM: 10}, {ID: "b", CPM: 10}, {ID: "filtered", CPM: 100, Targeting: model.Targeting{Countries: []string{"GB"}}}}
	counts := map[string]int{}
	for i := 0; i < 10000; i++ {
		w := Choose(prepared(cs), model.Facts{Country: "US"})
		counts[w.ID]++
	}
	if counts["low"] != 0 || counts["filtered"] != 0 || counts["a"] < 4000 || counts["b"] < 4000 {
		t.Fatal(counts)
	}
	if Choose(nil, model.Facts{}) != nil {
		t.Fatal("empty auction")
	}
}
func TestTrafficAndHotApply(t *testing.T) {
	e, s := fixture()
	body := `{"id":"req","type":"popunder","sub":445566,"ip":"131.34.123.159","ua":"test"}`
	before := httptest.NewRecorder()
	e.Traffic(before, httptest.NewRequest("POST", "/source/s", strings.NewReader(body)))
	if before.Code != 503 {
		t.Fatal(before.Code)
	}
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				w := httptest.NewRecorder()
				e.Traffic(w, httptest.NewRequest("POST", "/source/s", strings.NewReader(body)))
				if w.Code != 200 {
					t.Errorf("status %d", w.Code)
				}
				var out struct {
					Bid struct {
						Value float64 `json:"value"`
					}
				}
				if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Bid.Value != 0.13 {
					t.Errorf("response %s", w.Body)
				}
			}
		}()
	}
	for i := 2; i < 50; i++ {
		s.Revision = int64(i)
		if err := e.Apply(bytes(s)); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	s.Revision++
	s.Campaigns = nil
	e.Apply(bytes(s))
	w := httptest.NewRecorder()
	e.Traffic(w, httptest.NewRequest("POST", "/source/s", strings.NewReader(body)))
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal("no bid", w.Code, w.Body.String())
	}
}
func TestInternalAuth(t *testing.T) {
	e, _ := fixture()
	w := httptest.NewRecorder()
	e.Internal().ServeHTTP(w, httptest.NewRequest("GET", "/internal/config/status", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestUA(t *testing.T) {
	e, err := NewEnricher("missing.mmdb")
	if err == nil || e == nil {
		t.Fatal("missing MMDB must fail readiness while parser remains usable")
	}
	cases := []struct{ ua, platform, browser, major string }{
		{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36", "desktop", "Chrome", "66"},
		{"Mozilla/5.0 (iPad; CPU OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1", "mobile", "Mobile Safari", "17"},
		{"unknown", "", "", ""}}
	for _, c := range cases {
		f := e.Facts("invalid", c.ua)
		if f.Platform != c.platform || f.Browser != c.browser || f.BrowserMajor != c.major {
			t.Fatalf("%+v", f)
		}
	}
}
func BenchmarkAuction(b *testing.B) {
	cs := make([]model.Campaign, 1000)
	for i := range cs {
		cs[i] = model.Campaign{CPM: int64(i + 1), Targeting: model.Targeting{Countries: []string{"US"}, Browsers: []string{"Chrome"}}}
	}
	f := model.Facts{Country: "US", Browser: "Chrome"}
	b.ReportAllocs()
	compiled := prepared(cs)
	for b.Loop() {
		Choose(compiled, f)
	}
}
func BenchmarkUA(b *testing.B) {
	for _, unique := range []bool{false, true} {
		b.Run(fmt.Sprintf("unique=%v", unique), func(b *testing.B) {
			e, _ := NewEnricher("missing")
			ua := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.1.2 Safari/537.36"
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				input := ua
				if unique {
					input += fmt.Sprintf(" test/%d", i)
				}
				e.Facts("", input)
			}
		})
	}
}

var _ = http.MethodGet

func TestRealMMDBLookup(t *testing.T) {
	e, err := NewEnricher("testdata/GeoIP2-Country-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	f := e.Facts("81.2.69.160", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36")
	if f.Country != "GB" || f.Browser != "Chrome" || f.BrowserMajor != "66" || f.Platform != "desktop" {
		t.Fatalf("unexpected facts %+v", f)
	}
}
func BenchmarkTraffic(b *testing.B) {
	e, s := fixture()
	geo, err := NewEnricher("testdata/GeoIP2-Country-Test.mmdb")
	if err != nil {
		b.Fatal(err)
	}
	defer geo.Close()
	e.Enrich = geo.Facts
	if err = e.Apply(bytes(s)); err != nil {
		b.Fatal(err)
	}
	body := `{"id":"req","type":"popunder","sub":445566,"ip":"81.2.69.160","ua":"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.1.2 Safari/537.36"}`
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			e.Traffic(w, httptest.NewRequest("POST", "/source/s", strings.NewReader(body)))
			if w.Code != 200 {
				b.Fatal(w.Code)
			}
		}
	})
}

func prepared(cs []model.Campaign) []Candidate {
	out, err := PrepareCampaigns(cs)
	if err != nil {
		panic(err)
	}
	return out
}
