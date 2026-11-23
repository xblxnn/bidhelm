package runtime

import (
	"bidhelm/internal/model"
	"testing"
)

func TestSpecificityFrontier(t *testing.T) {
	ip := func(entries ...string) model.IPTargeting { return model.IPTargeting{Enabled: true, Entries: entries} }
	places := func(mode string, entries ...string) model.PlacementTargeting {
		return model.PlacementTargeting{Enabled: true, Mode: mode, Entries: entries}
	}
	base := model.Targeting{Countries: []string{"US"}, Platforms: []string{"mobile"}, IP: ip("192.0.2.0/24")}
	wl := base
	wl.Placement = places("whitelist", "1", "2", "3")
	bl := base
	bl.Placement = places("blacklist", "9")
	half := base
	half.IP = ip("192.0.2.0/25")
	same := base
	same.IP = ip("192.0.2.0/25", "192.0.2.128/25")
	same.Placement = places("blacklist")
	for _, tc := range []struct {
		name    string
		a, b    model.Targeting
		want    bool
		reverse bool
	}{
		{"WL narrower than RON", wl, base, true, false}, {"WL subset of BL", wl, bl, true, false}, {"half CIDR", half, base, true, false}, {"equivalent CIDRs and empty BL", base, same, true, true}, {"incomparable IP and WL", half, wl, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newScope(Candidate{Campaign: model.Campaign{Targeting: tc.a}})
			b := newScope(Candidate{Campaign: model.Campaign{Targeting: tc.b}})
			if a.within(b) != tc.want || b.within(a) != tc.reverse {
				t.Fatal("incorrect containment")
			}
		})
	}
	broad := model.Campaign{ID: "broad", CPM: 300, Targeting: base}
	narrow := model.Campaign{ID: "wl", CPM: 80, Targeting: wl}
	other := model.Campaign{ID: "half", CPM: 100, Targeting: half}
	for _, ordered := range [][]model.Campaign{{broad, narrow, other}, {other, narrow, broad}} {
		cs := prepared(ordered)
		f := model.Facts{IP: model.ParseUserIP("192.0.2.42"), Country: "US", Platform: "mobile", PlacementID: "2"}
		if Choose(cs, f).ID != "half" {
			t.Fatal("CPM among incomparable survivors")
		}
		f.IP = model.ParseUserIP("192.0.2.200")
		if Choose(cs, f).ID != "wl" {
			t.Fatal("cheaper whitelist should win")
		}
		f.PlacementID = "9"
		if Choose(cs, f).ID != "broad" {
			t.Fatal("RON fallback")
		}
	}
}
