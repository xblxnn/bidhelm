package store

import (
	"bidhelm/internal/model"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPlacementPersistenceAndSchema2Upgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := model.Campaign{ID: "p", Name: "p", Active: true, CPM: 1, URL: "https://example/?site={placement_id}", Targeting: model.Targeting{Placement: model.PlacementTargeting{Enabled: true, Mode: "whitelist", Entries: []string{" a ", "a", "001"}}}}
	if err = s.SaveCampaign(c); err != nil {
		t.Fatal(err)
	}
	c.Targeting.Placement.Enabled = false
	if err = s.SaveCampaign(c); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := s.Campaigns()
	p := cs[0].Targeting.Placement
	if p.Enabled || p.Mode != "whitelist" || len(p.Entries) != 2 || p.Entries[1] != "001" {
		t.Fatal(p)
	}
	raw, rev, _ := s.Snapshot()
	var snap model.Snapshot
	json.Unmarshal(raw, &snap)
	if snap.Campaigns[0].URL != c.URL || snap.Campaigns[0].Targeting.Placement.Mode != "whitelist" {
		t.Fatal(snap)
	}
	c.Targeting.Placement.Mode = "invalid"
	if s.SaveCampaign(c) == nil {
		t.Fatal("invalid save")
	}
	_, after, _ := s.Snapshot()
	if after != rev {
		t.Fatal("revision changed")
	}
	// Simulate a schema-2 installation with no placement feature.
	s.DB.Exec("DELETE FROM campaigns")
	old := `{"schema":2,"revision":20,"sources":[],"campaigns":[]}`
	if _, err = s.DB.Exec("UPDATE configuration SET revision=20,snapshot=?", []byte(old)); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, rev, _ = s.Snapshot()
	json.Unmarshal(raw, &snap)
	if rev != 21 || snap.Schema != 3 {
		t.Fatal(string(raw))
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	_, rev, _ = s.Snapshot()
	if rev != 21 {
		t.Fatal("repeated upgrade")
	}
}
