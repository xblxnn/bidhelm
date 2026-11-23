// Package exads_json_pop implements the supplied EXADS JSON popunder request.
// CPM value is USD; btype is always 1. No-bid is HTTP 204.
// The user confirmed on 2026-11-09 that bUrl may be omitted in v1.
package exads_json_pop

import (
	"bidhelm/internal/adapters"
	"bidhelm/internal/model"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type Adapter struct{}
type request struct {
	Sub  json.RawMessage `json:"sub"`
	ID   string          `json:"id"`
	IP   string          `json:"ip"`
	UA   string          `json:"ua"`
	Type string          `json:"type"`
}

func (Adapter) Key() string { return model.AdapterEXADS }
func (a Adapter) Parse(r *http.Request) (adapters.Request, error) {
	if r.Method != http.MethodPost {
		return adapters.Request{}, errors.New("POST required")
	}
	var v request
	d := json.NewDecoder(r.Body)
	if err := d.Decode(&v); err != nil {
		return adapters.Request{}, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return adapters.Request{}, errors.New("expected one JSON object")
	}
	if v.ID == "" || v.Type != "popunder" {
		return adapters.Request{}, errors.New("id and popunder type required")
	}
	placement, err := parsePlacement(v.Sub)
	if err != nil {
		return adapters.Request{}, err
	}
	return adapters.Request{PlacementID: placement, IP: v.IP, UA: v.UA, Context: v.ID}, nil
}
func (a Adapter) Respond(w http.ResponseWriter, r adapters.Request, c *model.Campaign) error {
	if c == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	// Use a JSON number constructed from integer micro-USD, preserving precision.
	out := struct {
		Bid struct {
			ID    string      `json:"id"`
			URL   string      `json:"url"`
			BType int         `json:"btype"`
			Value json.Number `json:"value"`
		} `json:"bid"`
	}{}
	out.Bid.ID, _ = r.Context.(string)
	out.Bid.URL = c.URL
	out.Bid.BType = 1
	out.Bid.Value = json.Number(model.FormatMoney(c.CPM))
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(out)
}

var integerPlacement = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func parsePlacement(raw json.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if strings.HasPrefix(value, `"`) {
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
	} else if !integerPlacement.MatchString(value) {
		return "", errors.New("sub must be a string or decimal integer")
	}
	if value == "" {
		return "", errors.New("sub is required")
	}
	return value, nil
}
