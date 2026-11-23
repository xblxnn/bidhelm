package runtime

import (
	"bidhelm/internal/adapters"
	"bidhelm/internal/model"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestMetricsAuthAndCounts(t *testing.T) {
	e, s := fixture()
	call := func(body string) {
		w := httptest.NewRecorder()
		e.Traffic(w, httptest.NewRequest("POST", "/source/s", strings.NewReader(body)))
	}
	body := `{"id":"r","type":"popunder","sub":123}`
	call(body)
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	call(body)
	call(`{}`)
	s.Revision++
	s.Campaigns = nil
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	call(body)
	m := e.Metrics()
	if m.Started != 4 || m.Completed != 4 || m.Active != 0 || m.Outcomes["bid"] != 1 || m.Outcomes["no_bid"] != 1 || m.Outcomes["invalid"] != 1 || m.Outcomes["not_ready"] != 1 {
		t.Fatal(m)
	}
	if m.Duration.Count != 4 || m.Duration.Sum <= 0 || m.Duration.Buckets[len(m.Duration.Buckets)-1].Count != 4 {
		t.Fatal(m.Duration)
	}
	for i := 1; i < len(m.Duration.Buckets); i++ {
		if m.Duration.Buckets[i].Count < m.Duration.Buckets[i-1].Count {
			t.Fatal("noncumulative buckets")
		}
	}
	for _, token := range []string{"", "wrong", e.Token} {
		req := httptest.NewRequest("GET", "/internal/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		e.Internal().ServeHTTP(w, req)
		if token != e.Token {
			if w.Code != 401 {
				t.Fatal(w.Code)
			}
		} else if w.Code != 200 || !strings.Contains(w.Body.String(), `"requests_completed_total":4`) {
			t.Fatal(w.Code, w.Body)
		}
	}
	if e.Metrics().Started != 4 {
		t.Fatal("metrics counted itself")
	}
}

type blockedAdapter struct {
	entered chan struct{}
	release chan struct{}
}

func (a blockedAdapter) Key() string { return "blocked" }
func (a blockedAdapter) Parse(*http.Request) (adapters.Request, error) {
	return adapters.Request{PlacementID: "p"}, nil
}
func (a blockedAdapter) Respond(http.ResponseWriter, adapters.Request, *model.Campaign) error {
	a.entered <- struct{}{}
	<-a.release
	return errors.New("write failed")
}
func TestMetricsInflightAndWriteErrors(t *testing.T) {
	e, s := fixture()
	a := blockedAdapter{make(chan struct{}, 8), make(chan struct{})}
	e.Registry = adapters.Registry{"blocked": a}
	s.Sources[0].Adapter = "blocked"
	if err := e.Apply(bytes(s)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Traffic(httptest.NewRecorder(), httptest.NewRequest("POST", "/source/s", nil))
		}()
	}
	for i := 0; i < 8; i++ {
		<-a.entered
	}
	if m := e.Metrics(); m.Active != 8 || m.Completed != 0 {
		t.Fatal(m)
	}
	close(a.release)
	wg.Wait()
	if m := e.Metrics(); m.Active != 0 || m.Completed != 8 || m.Outcomes["server_error"] != 8 {
		t.Fatal(m)
	}
}
