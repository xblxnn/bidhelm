package backoffice

import (
	"bidhelm/internal/adapters"
	"bidhelm/internal/adapters/exads_json_pop"
	"bidhelm/internal/model"
	rt "bidhelm/internal/runtime"
	"bidhelm/internal/store"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestBackofficeFlow(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	if e = s.Init("admin", "long-password-123"); e != nil {
		t.Fatal(e)
	}
	p := &Publisher{Store: s}
	web := &Server{Store: s, Publisher: p, BaseURL: "http://localhost:8080"}
	ts := httptest.NewServer(web.Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrfRE := regexp.MustCompile(`name="csrf" value="([a-f0-9]+)"`)
	get := func(path string) string {
		t.Helper()
		res, e := client.Get(ts.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("GET %s: %d %s", path, res.StatusCode, b)
		}
		return string(b)
	}
	page := get("/sources")
	if !strings.Contains(page, `<html lang="en">`) || !strings.Contains(page, "<title>BidHelm</title>") || !strings.Contains(page, "Sign in to BidHelm") {
		t.Fatal("auth bypass")
	}
	token := csrfRE.FindStringSubmatch(page)[1]
	// A second login GET (another tab/favicon redirect) must not invalidate the first form.
	second := get("/login")
	if csrfRE.FindStringSubmatch(second)[1] != token {
		t.Fatal("login CSRF changed across tabs")
	}
	res, e := client.PostForm(ts.URL+"/login", url.Values{"login": {"admin"}, "password": {"long-password-123"}, "csrf": {token}})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(b), "Create source") {
		t.Fatalf("login failed: %s", b)
	}
	if !strings.Contains(string(b), "Traffic Sources · BidHelm") || !strings.Contains(string(b), "Saved revision:") || !strings.Contains(string(b), "Campaigns") {
		t.Fatal("source page is missing English navigation or BidHelm branding")
	}
	token = csrfRE.FindStringSubmatch(string(b))[1]
	res, e = client.PostForm(ts.URL+"/sources", url.Values{"name": {"Test"}, "adapter": {model.AdapterEXADS}})
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("CSRF bypass")
	}
	res, e = client.PostForm(ts.URL+"/sources", url.Values{"csrf": {token}, "name": {"<script>bad</script>"}, "adapter": {model.AdapterEXADS}})
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || strings.Contains(string(b), "<script>bad</script>") {
		t.Fatal("source save or XSS")
	}
	campaignPage := get("/campaign")
	if !strings.Contains(campaignPage, "Campaign · BidHelm") || !strings.Contains(campaignPage, "Save and apply") || !strings.Contains(campaignPage, "Placement targeting") {
		t.Fatal("campaign form is missing English text or BidHelm branding")
	}
	res, e = client.PostForm(ts.URL+"/campaign", url.Values{"csrf": {token}, "name": {"Test"}, "url": {"https://example.com"}, "cpm": {"0.13"}, "active": {"on"}, "countries": {"US"}, "browser_majors": {"120"}})
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("campaign create", res.StatusCode)
	}
	cs, _ := s.Campaigns()
	if len(cs) != 1 || cs[0].CPM != 130000 {
		t.Fatal(cs)
	}
	get("/campaign?id=" + cs[0].ID)
	res, e = client.PostForm(ts.URL+"/campaign", url.Values{"csrf": {token}, "id": {cs[0].ID}, "name": {"Test"}, "url": {"https://example.com"}, "cpm": {"0.13"}, "active": {"off"}})
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	snap, _, _ := s.Snapshot()
	var v model.Snapshot
	json.Unmarshal(snap, &v)
	if len(v.Campaigns) != 0 {
		t.Fatal("stop failed")
	}
	res, e = client.PostForm(ts.URL+"/logout", url.Values{"csrf": {token}})
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if !strings.Contains(get("/sources"), "Sign in to BidHelm") {
		t.Fatal("logout failed")
	}
}
func TestPublisherRetryAndRestart(t *testing.T) {
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	a := exads_json_pop.Adapter{}
	engine := &rt.Engine{Token: "token", Registry: adapters.Registry{a.Key(): a}}
	drop := true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && drop {
			drop = false
			b, _ := io.ReadAll(r.Body)
			if e := engine.Apply(b); e != nil {
				t.Error(e)
			}
			http.Error(w, "response lost", 502)
			return
		}
		engine.Internal().ServeHTTP(w, r)
	}))
	defer ts.Close()
	p := &Publisher{Store: s, URL: ts.URL, Token: "token", Client: &http.Client{Timeout: time.Second}}
	p.Sync(context.Background())
	_, _, errText := p.State()
	if errText == "" {
		t.Fatal("expected lost response")
	}
	p.Sync(context.Background())
	rev, _, errText := p.State()
	if rev != 1 || errText != "" {
		t.Fatal(rev, errText)
	}
	// Replace engine to emulate a fresh DSP process with empty in-memory state.
	engine = &rt.Engine{Token: "token", Registry: adapters.Registry{a.Key(): a}}
	p.Sync(context.Background())
	if engine.Status().Revision != 1 {
		t.Fatal("restart not restored")
	}
}
