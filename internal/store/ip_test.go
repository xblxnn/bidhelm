package store

import (
	"bidhelm/internal/model"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestUpgradeSnapshotAndPersistIP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a genuine v1 campaign JSON without the new field.
	raw := `{"id":"legacy","name":"Legacy","url":"https://example.com","cpm_microusd":1200000,"active":true,"targeting":{}}`
	if _, err = s.DB.Exec("INSERT INTO campaigns VALUES(?,?)", "legacy", []byte(raw)); err != nil {
		t.Fatal(err)
	}
	old := `{"schema":1,"revision":10,"sources":[],"campaigns":[` + raw + `]}`
	if _, err = s.DB.Exec("UPDATE configuration SET revision=10,snapshot=?", []byte(old)); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b, rev, _ := s.Snapshot()
	var snap model.Snapshot
	json.Unmarshal(b, &snap)
	if rev != 11 || snap.Schema != model.Schema || snap.Campaigns[0].Targeting.IP.Enabled {
		t.Fatal("upgrade failed", string(b))
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	_, rev, _ = s.Snapshot()
	if rev != 11 {
		t.Fatal("upgrade ran twice")
	}
	cs, _ := s.Campaigns()
	c := cs[0]
	c.Targeting.IP = model.IPTargeting{Enabled: true, Entries: []string{"131.34.123.159/24", "131.34.123.0/24"}}
	if err = s.SaveCampaign(c); err != nil {
		t.Fatal(err)
	}
	cs, _ = s.Campaigns()
	if len(cs[0].Targeting.IP.Entries) != 1 || cs[0].Targeting.IP.Entries[0] != "131.34.123.0/24" {
		t.Fatal("not normalized", cs)
	}
	c = cs[0]
	c.Targeting.IP.Enabled = false
	if err = s.SaveCampaign(c); err != nil {
		t.Fatal(err)
	}
	b, rev, _ = s.Snapshot()
	json.Unmarshal(b, &snap)
	if snap.Campaigns[0].Targeting.IP.Enabled || len(snap.Campaigns[0].Targeting.IP.Entries) != 1 {
		t.Fatal("disabled list lost")
	}
	c.Targeting.IP.Enabled = true
	c.Targeting.IP.Entries = nil
	if s.SaveCampaign(c) == nil {
		t.Fatal("empty filter accepted")
	}
	_, after, _ := s.Snapshot()
	if after != rev {
		t.Fatal("failed save incremented revision")
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if err = s.Backup(backup); err != nil {
		t.Fatal(err)
	}
	other, err := Open(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer other.DB.Close()
	persisted, _ := other.Campaigns()
	if persisted[0].Targeting.IP.Enabled || len(persisted[0].Targeting.IP.Entries) != 1 {
		t.Fatal("backup lost IP settings")
	}
}
