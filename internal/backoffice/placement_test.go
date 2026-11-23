package backoffice

import (
	"bidhelm/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlacementForm(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	token, csrf, err := s.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: s, Publisher: &Publisher{Store: s}}).Handler()
	post := func(v url.Values) *httptest.ResponseRecorder {
		v.Set("csrf", csrf)
		r := httptest.NewRequest("POST", "/campaign", strings.NewReader(v.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	form := url.Values{"name": {"Placement campaign"}, "url": {"https://example.com"}, "cpm": {"1.2"}, "active": {"on"}, "placement_mode": {"whitelist"}, "placement_enabled": {"on"}, "placement_entries": {" 445566 \r\nsite-A\n445566"}}
	if w := post(form); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	cs, _ := s.Campaigns()
	id := cs[0].ID
	if !cs[0].Targeting.Placement.Enabled || cs[0].Targeting.Placement.Entries[0] != "445566" {
		t.Fatal(cs)
	}
	r := httptest.NewRequest("GET", "/campaign?id="+id, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "445566\nsite-A") || !strings.Contains(w.Body.String(), `id="placement_enabled" checked`) {
		t.Fatal("edit form lost placement settings")
	}
	form.Set("id", id)
	form.Del("placement_enabled")
	if w = post(form); w.Code != 303 {
		t.Fatal(w.Code)
	}
	cs, _ = s.Campaigns()
	if cs[0].Targeting.Placement.Enabled || len(cs[0].Targeting.Placement.Entries) != 2 {
		t.Fatal("disabled list lost")
	}
	_, rev, _ := s.Snapshot()
	form.Set("placement_enabled", "on")
	form.Set("placement_mode", "bogus")
	form.Set("placement_entries", "445566\n\nsite-A")
	w = post(form)
	b, _ := io.ReadAll(w.Result().Body)
	if w.Code != 400 || !strings.Contains(string(b), "blacklist or whitelist") || !strings.Contains(string(b), "445566\n\nsite-A") || !strings.Contains(string(b), `value="1.2"`) {
		t.Fatal("bad form not preserved", w.Code, string(b))
	}
	_, after, _ := s.Snapshot()
	if rev != after {
		t.Fatal("invalid form published")
	}
	form.Set("placement_mode", "whitelist")
	form.Set("placement_entries", "")
	if w = post(form); w.Code != 303 {
		t.Fatal("empty WL must save", w.Code)
	}
}
