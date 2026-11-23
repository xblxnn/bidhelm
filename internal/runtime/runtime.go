package runtime

import (
	"bidhelm/internal/adapters"
	"bidhelm/internal/model"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Candidate struct {
	broader   []int
	Campaign  model.Campaign
	IP        model.IPFilter
	Placement model.PlacementFilter
}

func PrepareCampaigns(cs []model.Campaign) ([]Candidate, error) {
	out := make([]Candidate, 0, len(cs))
	for _, c := range cs {
		filter, err := model.CompileIP(c.Targeting.IP)
		if err != nil {
			return nil, err
		}
		placement, err := model.CompilePlacement(c.Targeting.Placement)
		if err != nil {
			return nil, err
		}
		out = append(out, Candidate{Campaign: c, IP: filter, Placement: placement})
	}
	prepareSpecificity(out)
	return out, nil
}

type state struct {
	Candidates []Candidate
	Snapshot   model.Snapshot
	Hash       [32]byte
	Sources    map[string]model.Source
}
type Engine struct {
	metrics  trafficMetrics
	Registry adapters.Registry
	Enrich   func(string, string) model.Facts
	GeoError string
	Token    string
	current  atomic.Pointer[state]
	mu       sync.Mutex
}

func (e *Engine) Apply(b []byte) error {
	var s model.Snapshot
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("expected one snapshot")
	}
	if err := model.Validate(s, e.Registry.Known); err != nil {
		return err
	}
	candidates, err := PrepareCampaigns(s.Campaigns)
	if err != nil {
		return err
	}
	canonical, _ := json.Marshal(s)
	hash := sha256.Sum256(canonical)
	e.mu.Lock()
	defer e.mu.Unlock()
	old := e.current.Load()
	if old != nil {
		if s.Revision < old.Snapshot.Revision {
			return errors.New("stale revision")
		}
		if s.Revision == old.Snapshot.Revision {
			if hash == old.Hash {
				return nil
			}
			return errors.New("revision content conflict")
		}
	}
	next := &state{Candidates: candidates, Snapshot: s, Hash: hash, Sources: map[string]model.Source{}}
	for _, v := range s.Sources {
		next.Sources[v.ID] = v
	}
	e.current.Store(next)
	return nil
}

type Status struct {
	Revision int64  `json:"revision"`
	Ready    bool   `json:"ready"`
	Error    string `json:"error,omitempty"`
}

func (e *Engine) Status() Status {
	v := Status{Error: e.GeoError}
	if s := e.current.Load(); s != nil {
		v.Revision = s.Snapshot.Revision
		v.Ready = e.GeoError == ""
	} else if v.Error == "" {
		v.Error = "configuration not received"
	}
	return v
}
func (e *Engine) Internal() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if e.Token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+e.Token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/internal/metrics" && r.Method == "GET" {
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(e.Metrics())
			return
		}
		if r.URL.Path == "/internal/config/status" && r.Method == "GET" {
			json.NewEncoder(w).Encode(e.Status())
			return
		}
		if r.URL.Path != "/internal/config" || r.Method != "PUT" {
			http.NotFound(w, r)
			return
		}
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err = e.Apply(b); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"revision": e.Status().Revision, "status": "applied"})
	})
}
func Choose(cs []Candidate, f model.Facts) *model.Campaign {
	// Bit 0 records eligibility; bit 1 excludes a matched broader campaign.
	state := make([]uint8, len(cs))
	for i := range cs {
		c := &cs[i]
		if c.Placement.Match(f.PlacementID) && c.IP.Match(f.IP) && model.Match(c.Campaign.Targeting, f) {
			state[i] |= 1
			for _, j := range c.broader {
				state[j] |= 2
			}
		}
	}
	var winner *model.Campaign
	n := 0
	for i, flags := range state {
		if flags != 1 {
			continue
		}
		c := &cs[i].Campaign
		if winner == nil || c.CPM > winner.CPM {
			winner = c
			n = 1
		} else if c.CPM == winner.CPM {
			n++
			if rand.IntN(n) == 0 {
				winner = c
			}
		}
	}
	return winner
}
func (e *Engine) Traffic(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	outcome := resultError
	e.metrics.started.Add(1)
	e.metrics.active.Add(1)
	defer func() { e.metrics.finish(start, outcome) }()
	if r.Method != "POST" {
		outcome = resultInvalid
		w.Header().Set("Allow", "POST")
		http.Error(w, "POST required", 405)
		return
	}
	s := e.current.Load()
	if s == nil || e.GeoError != "" {
		outcome = resultNotReady
		http.Error(w, "DSP is not ready", 503)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/source/")
	src, ok := s.Sources[id]
	if !ok {
		outcome = resultInvalid
		http.NotFound(w, r)
		return
	}
	a := e.Registry[src.Adapter]
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	req, err := a.Parse(r)
	if err != nil {
		outcome = resultInvalid
		http.Error(w, err.Error(), 400)
		return
	}
	if strings.TrimSpace(req.PlacementID) == "" {
		outcome = resultInvalid
		http.Error(w, "placement is required", 400)
		return
	}
	facts := e.Enrich(req.IP, req.UA)
	facts.PlacementID = req.PlacementID
	winner := Choose(s.Candidates, facts)
	if winner != nil {
		response := *winner
		response.URL = model.PlacementURL(winner.URL, req.PlacementID)
		winner = &response
	}
	if err := a.Respond(w, req, winner); err != nil {
		return
	}
	if winner == nil {
		outcome = resultNoBid
	} else {
		outcome = resultBid
	}
}
