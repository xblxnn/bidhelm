package model

import (
	"fmt"
	"net/url"
	"strings"
)

const PlacementMacro = "{placement_id}"

type PlacementTargeting struct {
	Enabled bool     `json:"enabled"`
	Mode    string   `json:"mode"`
	Entries []string `json:"entries"`
}

func NormalizePlacement(t PlacementTargeting) (PlacementTargeting, error) {
	if t.Mode == "" && !t.Enabled {
		t.Mode = "blacklist"
	}
	if t.Mode != "blacklist" && t.Mode != "whitelist" {
		return PlacementTargeting{}, fmt.Errorf("Placements: select blacklist or whitelist")
	}
	out := PlacementTargeting{Enabled: t.Enabled, Mode: t.Mode, Entries: []string{}}
	seen := map[string]bool{}
	for _, raw := range t.Entries {
		v := strings.TrimSpace(raw)
		if v != "" && !seen[v] {
			out.Entries = append(out.Entries, v)
			seen[v] = true
		}
	}
	return out, nil
}

type PlacementFilter struct {
	enabled bool
	mode    string
	entries map[string]struct{}
}

func CompilePlacement(t PlacementTargeting) (PlacementFilter, error) {
	n, err := NormalizePlacement(t)
	if err != nil {
		return PlacementFilter{}, err
	}
	f := PlacementFilter{enabled: n.Enabled, mode: n.Mode}
	if n.Enabled {
		f.entries = make(map[string]struct{}, len(n.Entries))
		for _, v := range n.Entries {
			f.entries[v] = struct{}{}
		}
	}
	return f, nil
}
func (f PlacementFilter) Match(id string) bool {
	if !f.enabled {
		return true
	}
	_, hit := f.entries[id]
	if f.mode == "whitelist" {
		return hit
	}
	return !hit
}

// Validate macro locations without reserializing the URL, preserving signed queries.
func ValidPlacementURL(raw string) bool {
	if !strings.Contains(raw, PlacementMacro) {
		return true
	}
	base, query, ok := strings.Cut(raw, "?")
	if !ok || strings.Contains(base, "#") || strings.Contains(base, PlacementMacro) {
		return false
	}
	query, fragment, _ := strings.Cut(query, "#")
	if strings.Contains(fragment, PlacementMacro) {
		return false
	}
	for _, part := range strings.Split(query, "&") {
		key, _, hasValue := strings.Cut(part, "=")
		if strings.Contains(key, PlacementMacro) || (!hasValue && strings.Contains(part, PlacementMacro)) {
			return false
		}
	}
	return true
}
func PlacementURL(template, id string) string {
	return strings.ReplaceAll(template, PlacementMacro, url.QueryEscape(id))
}
