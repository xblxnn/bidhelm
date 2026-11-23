package model

import (
	"net/url"
	"strings"
	"testing"
)

func TestPlacementFilter(t *testing.T) {
	n, err := NormalizePlacement(PlacementTargeting{Mode: "whitelist", Entries: []string{" a ", "a", "", "004", "A"}})
	if err != nil || strings.Join(n.Entries, ",") != "a,004,A" {
		t.Fatal(n, err)
	}
	for _, mode := range []string{"blacklist", "whitelist"} {
		for _, enabled := range []bool{false, true} {
			for _, entries := range [][]string{nil, {"445566"}} {
				f, err := CompilePlacement(PlacementTargeting{Enabled: enabled, Mode: mode, Entries: entries})
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range []string{"445566", "other", "0445566"} {
					hit := len(entries) > 0 && id == "445566"
					want := !enabled || (mode == "whitelist" && hit) || (mode == "blacklist" && !hit)
					if f.Match(id) != want {
						t.Fatal(mode, enabled, entries, id)
					}
				}
			}
		}
	}
	if _, err := NormalizePlacement(PlacementTargeting{Mode: "bogus"}); err == nil {
		t.Fatal("bad mode")
	}
	if _, err := NormalizePlacement(PlacementTargeting{Enabled: true}); err == nil {
		t.Fatal("missing mode")
	}
}
func TestPlacementURL(t *testing.T) {
	for _, raw := range []string{"https://{placement_id}.example/", "https://example/{placement_id}", "https://example/?{placement_id}=x", "https://example/#x?site={placement_id}", "https://example/?site=x#{placement_id}"} {
		if ValidURL(raw) {
			t.Fatal("accepted", raw)
		}
	}
	raw := "https://example/?x=%2f&site={placement_id}&site={placement_id}&encoded=%7Bplacement_id%7D"
	if !ValidURL(raw) {
		t.Fatal("rejected macro")
	}
	id := "a&b+%#?/ пробел"
	got := PlacementURL(raw, id)
	want := strings.ReplaceAll(raw, PlacementMacro, url.QueryEscape(id))
	if got != want {
		t.Fatal(got)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("site") != id || len(u.Query()["site"]) != 2 || u.Query().Get("encoded") != PlacementMacro {
		t.Fatal(u.Query())
	}
	if PlacementURL("https://example/?x=%2f&x=+", id) != "https://example/?x=%2f&x=+" {
		t.Fatal("plain URL changed")
	}
}
func TestPlacementSchema(t *testing.T) {
	for _, schema := range []int{1, 2, 3} {
		s := Snapshot{Schema: schema, Revision: 1, Campaigns: []Campaign{{ID: "c", Active: true, URL: "https://example/", CPM: 1}}}
		if err := Validate(s, func(string) bool { return true }); err != nil {
			t.Fatal(err)
		}
		s.Campaigns[0].Targeting.Placement = PlacementTargeting{Enabled: true, Mode: "whitelist"}
		if (Validate(s, func(string) bool { return true }) == nil) != (schema == 3) {
			t.Fatal(schema)
		}
		s.Campaigns[0].Targeting.Placement = PlacementTargeting{}
		s.Campaigns[0].URL = "https://example/?site={placement_id}"
		if (Validate(s, func(string) bool { return true }) == nil) != (schema == 3) {
			t.Fatal(schema)
		}
	}
}
