package store

import (
	"bidhelm/internal/model"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestPersistenceAndAtomicSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	if e = s.Init("admin", "long-password-123"); e != nil {
		t.Fatal(e)
	}
	if !s.Authenticate("admin", "long-password-123") {
		t.Fatal("login")
	}
	if e = s.Init("other", "another-password-123"); e == nil {
		t.Fatal("overwrote admin")
	}
	if e = s.SaveSource(model.Source{ID: "src", Name: "Source", Adapter: model.AdapterEXADS}); e != nil {
		t.Fatal(e)
	}
	c := model.Campaign{ID: "c", Name: "Campaign", Active: true, CPM: 130000, URL: "https://example.com"}
	if e = s.SaveCampaign(c); e != nil {
		t.Fatal(e)
	}
	b, rev, e := s.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	var snap model.Snapshot
	json.Unmarshal(b, &snap)
	if len(snap.Campaigns) != 1 || snap.Campaigns[0].Name != "" || rev != 3 {
		t.Fatalf("snapshot %s", b)
	}
	c.Active = false
	if e = s.SaveCampaign(c); e != nil {
		t.Fatal(e)
	}
	b, rev, _ = s.Snapshot()
	json.Unmarshal(b, &snap)
	if len(snap.Campaigns) != 0 || rev != 4 {
		t.Fatal("stop did not remove campaign")
	}
	c.URL = "http://example.com"
	if e = s.SaveCampaign(c); e == nil {
		t.Fatal("accepted HTTP")
	}
	_, after, _ := s.Snapshot()
	if after != rev {
		t.Fatal("failed save changed revision")
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	copy, e := Open(backup)
	if e != nil {
		t.Fatal(e)
	}
	defer copy.DB.Close()
	if !copy.Authenticate("admin", "long-password-123") {
		t.Fatal("backup lost admin")
	}
	_, copyRev, _ := copy.Snapshot()
	if copyRev != rev {
		t.Fatal("backup lost revision")
	}
	token, _, _ := s.NewSession()
	if e = s.Password("changed-password-123"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Session(token); e == nil {
		t.Fatal("old session survived reset")
	}
}

func TestRefuseNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec("INSERT INTO migrations VALUES(2)")
	if e != nil {
		t.Fatal(e)
	}
	s.DB.Close()
	if newer, e := Open(path); e == nil {
		newer.DB.Close()
		t.Fatal("silent schema downgrade")
	}
}
