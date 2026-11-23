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

func TestIPForm(t *testing.T) {
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
	form := url.Values{"name": {"IP campaign"}, "url": {"https://example.com"}, "cpm": {"1.2"}, "active": {"on"}, "ip_enabled": {"on"}, "ip_entries": {"131.34.123.159/24\r\n2001:db8::/32"}}
	if w := post(form); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	cs, _ := s.Campaigns()
	id := cs[0].ID
	if !cs[0].Targeting.IP.Enabled || cs[0].Targeting.IP.Entries[0] != "131.34.123.0/24" {
		t.Fatal(cs)
	}
	r := httptest.NewRequest("GET", "/campaign?id="+id, nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "131.34.123.0/24\n2001:db8::/32") || !strings.Contains(w.Body.String(), `id="ip_enabled" checked`) {
		t.Fatal("edit form lost IP settings")
	}
	form.Set("id", id)
	form.Del("ip_enabled")
	if w = post(form); w.Code != 303 {
		t.Fatal(w.Code)
	}
	cs, _ = s.Campaigns()
	if cs[0].Targeting.IP.Enabled || len(cs[0].Targeting.IP.Entries) != 2 {
		t.Fatal("disabled list lost")
	}
	_, rev, _ := s.Snapshot()
	form.Set("ip_enabled", "on")
	form.Set("ip_entries", "131.34.123.159\n\nbad-entry")
	w = post(form)
	b, _ := io.ReadAll(w.Result().Body)
	if w.Code != 400 || !strings.Contains(string(b), "line 3") || !strings.Contains(string(b), "131.34.123.159\n\nbad-entry") || !strings.Contains(string(b), `value="1.2"`) {
		t.Fatal("bad form not preserved", w.Code, string(b))
	}
	_, after, _ := s.Snapshot()
	if rev != after {
		t.Fatal("invalid form published")
	}
	form.Set("ip_entries", "")
	if w = post(form); w.Code != 400 {
		t.Fatal("enabled empty accepted")
	}
	form.Set("ip_entries", "131.34.123.0/24")
	if w = post(form); w.Code != 303 {
		t.Fatal(w.Code)
	}
}
