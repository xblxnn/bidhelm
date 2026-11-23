package backoffice

import (
	rt "bidhelm/internal/runtime"
	"bidhelm/internal/store"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Publisher struct {
	Store      *store.Store
	URL, Token string
	Client     *http.Client
	mu         sync.RWMutex
	Applied    int64
	Error      string
	Ready      bool
}

func (p *Publisher) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		p.Sync(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
func (p *Publisher) Sync(ctx context.Context) {
	b, rev, err := p.Store.Snapshot()
	if err != nil {
		p.fail(err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, "GET", p.URL+"/internal/config/status", nil)
	if err != nil {
		p.fail(err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	res, err := p.Client.Do(req)
	if err != nil {
		p.fail(err)
		return
	}
	var status rt.Status
	err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&status)
	res.Body.Close()
	if err != nil || res.StatusCode != 200 {
		p.fail(fmt.Errorf("DSP status unavailable (%d)", res.StatusCode))
		return
	}
	if status.Revision != rev {
		req, err = http.NewRequestWithContext(ctx, "PUT", p.URL+"/internal/config", bytes.NewReader(b))
		if err != nil {
			p.fail(err)
			return
		}
		req.Header.Set("Authorization", "Bearer "+p.Token)
		req.Header.Set("Content-Type", "application/json")
		res, err = p.Client.Do(req)
		if err != nil {
			p.fail(err)
			return
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if readErr != nil {
			p.fail(readErr)
			return
		}
		if res.StatusCode != 200 {
			p.fail(fmt.Errorf("DSP rejected revision: %s", body))
			return
		}
		var ack struct {
			Revision int64  `json:"revision"`
			Status   string `json:"status"`
		}
		if err = json.Unmarshal(body, &ack); err != nil || ack.Revision != rev || ack.Status != "applied" {
			p.fail(fmt.Errorf("invalid DSP acknowledgement"))
			return
		}
		status.Revision = rev
	}
	p.mu.Lock()
	p.Applied = status.Revision
	p.Error = status.Error
	p.Ready = status.Ready
	p.mu.Unlock()
}
func (p *Publisher) fail(e error) { p.mu.Lock(); p.Error = e.Error(); p.Ready = false; p.mu.Unlock() }
func (p *Publisher) State() (int64, bool, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Applied, p.Ready, p.Error
}
