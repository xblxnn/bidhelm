package backoffice

import (
	"bidhelm/internal/model"
	"bidhelm/internal/store"
	"crypto/subtle"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed assets/* templates.html
var files embed.FS
var templates = template.Must(template.New("templates.html").Funcs(template.FuncMap{"money": model.FormatMoney, "cpm": func(n int64) string {
	return strings.TrimRight(strings.TrimRight(model.FormatMoney(n), "0"), ".")
}, "selected": func(xs []string, x string) bool {
	for _, v := range xs {
		if x == v {
			return true
		}
	}
	return false
}, "join": func(xs []string) string { return strings.Join(xs, ",") }}).ParseFS(files, "templates.html"))

type attempt struct {
	Count int
	Start time.Time
}
type Server struct {
	Store     *store.Store
	Publisher *Publisher
	BaseURL   string
	Secure    bool
	mu        sync.Mutex
	attempts  map[string]attempt
}
type page struct {
	Title, CSRF, Error, BaseURL string
	PlacementEntries            string
	IPEntries, CPMInput         string
	Sources                     []model.Source
	Campaigns                   []model.Campaign
	Campaign                    model.Campaign
	Countries, Browsers, OS     []string
	Saved, Applied              int64
	Ready                       bool
}

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	sub, _ := fs.Sub(files, "assets")
	m.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(sub))))
	m.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	m.HandleFunc("/login", s.login)
	m.HandleFunc("/", s.auth)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		m.ServeHTTP(w, r)
	})
}
func (s *Server) render(w http.ResponseWriter, name string, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = templates.ExecuteTemplate(w, name, p)
}
func (s *Server) cookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.Secure, MaxAge: maxAge})
}
func (s *Server) allow(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attempts == nil {
		s.attempts = map[string]attempt{}
	}
	now := time.Now()
	for k, v := range s.attempts {
		if now.Sub(v.Start) > time.Minute {
			delete(s.attempts, k)
		}
	}
	a := s.attempts[key]
	if a.Start.IsZero() {
		a.Start = now
	}
	a.Count++
	s.attempts[key] = a
	return a.Count <= 10
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		token := model.ID()
		if c, err := r.Cookie("login_csrf"); err == nil && len(c.Value) == 48 {
			token = c.Value
		}
		s.cookie(w, "login_csrf", token, 600)
		s.render(w, "login", page{CSRF: token})
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !s.allow(host) {
		http.Error(w, "Try again in a minute", 429)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	c, e := r.Cookie("login_csrf")
	if e != nil || !equal(c.Value, r.FormValue("csrf")) {
		token := model.ID()
		s.cookie(w, "login_csrf", token, 600)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		s.render(w, "login", page{CSRF: token, Error: "Your sign-in form has expired. Please enter your login and password again."})
		return
	}
	if !s.Store.Authenticate(r.FormValue("login"), r.FormValue("password")) {
		w.WriteHeader(401)
		s.render(w, "login", page{CSRF: c.Value, Error: "Invalid login or password"})
		return
	}
	token, _, e := s.Store.NewSession()
	if e != nil {
		http.Error(w, "session unavailable", 500)
		return
	}
	s.cookie(w, "session", token, 43200)
	s.cookie(w, "login_csrf", "", -1)
	http.Redirect(w, r, "/sources", 303)
}
func equal(a, b string) bool { return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *Server) auth(w http.ResponseWriter, r *http.Request) {
	c, e := r.Cookie("session")
	if e != nil {
		http.Redirect(w, r, "/login", 303)
		return
	}
	csrf, e := s.Store.Session(c.Value)
	if e != nil {
		http.Redirect(w, r, "/login", 303)
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if r.Method == "POST" {
		if e = r.ParseForm(); e != nil {
			http.Error(w, "invalid form", 400)
			return
		}
		if !equal(csrf, r.FormValue("csrf")) {
			http.Error(w, "invalid CSRF", 403)
			return
		}
	}
	_, saved, e := s.Store.Snapshot()
	if e != nil {
		http.Error(w, "storage unavailable", 500)
		return
	}
	applied, ready, pubErr := s.Publisher.State()
	p := page{CSRF: csrf, BaseURL: s.BaseURL, Saved: saved, Applied: applied, Ready: ready, Error: pubErr, Countries: model.Countries, Browsers: model.Browsers, OS: model.OperatingSystems}
	switch r.URL.Path {
	case "/":
		http.Redirect(w, r, "/sources", 303)
	case "/logout":
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		s.Store.Logout(c.Value)
		s.cookie(w, "session", "", -1)
		http.Redirect(w, r, "/login", 303)
	case "/status":
		s.render(w, "status", p)
	case "/sources":
		if r.Method == "POST" {
			v := model.Source{ID: model.ID(), Name: strings.TrimSpace(r.FormValue("name")), Adapter: r.FormValue("adapter")}
			if e = s.Store.SaveSource(v); e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			http.Redirect(w, r, "/sources", 303)
			return
		}
		p.Title = "Traffic Sources"
		p.Sources, e = s.Store.Sources()
		if e != nil {
			http.Error(w, "storage unavailable", 500)
			return
		}
		s.render(w, "sources", p)
	case "/campaigns":
		p.Title = "Campaigns"
		p.Campaigns, e = s.Store.Campaigns()
		if e != nil {
			http.Error(w, "storage unavailable", 500)
			return
		}
		s.render(w, "campaigns", p)
	case "/campaign":
		s.campaign(w, r, p)
	default:
		http.NotFound(w, r)
	}
}
func (s *Server) campaign(w http.ResponseWriter, r *http.Request, p page) {
	p.Title = "Campaign"
	p.Campaign = model.Campaign{}
	id := r.URL.Query().Get("id")
	if r.Method == "POST" {
		id = r.FormValue("id")
	}
	if id != "" {
		cs, e := s.Store.Campaigns()
		if e != nil {
			http.Error(w, "storage unavailable", 500)
			return
		}
		found := false
		for _, c := range cs {
			if c.ID == id {
				p.Campaign = c
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
	}
	if r.Method == "GET" {
		p.PlacementEntries = strings.Join(p.Campaign.Targeting.Placement.Entries, "\n")
		p.IPEntries = strings.Join(p.Campaign.Targeting.IP.Entries, "\n")
		if p.Campaign.CPM > 0 {
			p.CPMInput = strings.TrimRight(strings.TrimRight(model.FormatMoney(p.Campaign.CPM), "0"), ".")
		}
		s.render(w, "campaign", p)
		return
	}
	c := model.Campaign{ID: id, Name: strings.TrimSpace(r.FormValue("name")), URL: strings.TrimSpace(r.FormValue("url")), Active: r.FormValue("active") == "on"}
	if c.ID == "" {
		c.ID = model.ID()
	}
	p.PlacementEntries = r.FormValue("placement_entries")
	p.IPEntries = r.FormValue("ip_entries")
	p.CPMInput = r.FormValue("cpm")
	c.Targeting = model.Targeting{Placement: model.PlacementTargeting{Enabled: r.FormValue("placement_enabled") == "on", Mode: r.FormValue("placement_mode"), Entries: strings.Split(p.PlacementEntries, "\n")}, IP: model.IPTargeting{Enabled: r.FormValue("ip_enabled") == "on", Entries: strings.Split(p.IPEntries, "\n")}, Countries: targetingSelection(r.Form["countries"]), Platforms: targetingSelection(r.Form["platforms"]), Browsers: r.Form["browsers"], BrowserMajors: split(r.FormValue("browser_majors")), OS: r.Form["os"], OSMajors: split(r.FormValue("os_majors"))}
	var e error
	c.CPM, e = campaignCPM(p.CPMInput)
	if e == nil {
		e = s.Store.SaveCampaign(c)
	}
	if e != nil {
		p.Campaign = c
		p.Campaign.ID = id
		p.Error = e.Error()
		w.WriteHeader(400)
		s.render(w, "campaign", p)
		return
	}
	http.Redirect(w, r, "/campaigns", 303)
}
func split(s string) []string {
	out := []string{}
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func targetingSelection(values []string) []string {
	for _, v := range values {
		if v == "ALL" {
			return nil
		}
	}
	return values
}

func campaignCPM(input string) (int64, error) {
	if _, frac, ok := strings.Cut(strings.TrimSpace(input), "."); ok && len(frac) > 2 {
		return 0, errors.New("CPM: use no more than two decimal places")
	}
	n, err := model.Money(input)
	if err == nil && n > 1_000_000_000_000_000 {
		return 0, errors.New("CPM exceeds the maximum")
	}
	return n, err
}
