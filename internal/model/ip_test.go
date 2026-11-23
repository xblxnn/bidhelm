package model

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestIPNormalization(t *testing.T) {
	input := IPTargeting{Enabled: true, Entries: []string{" 131.34.123.159 ", "", "131.34.123.159/24", "131.34.123.0/24", "2001:0db8:0000::1", "::ffff:131.34.123.159"}}
	got, err := NormalizeIP(input)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"131.34.123.159", "131.34.123.0/24", "2001:db8::1"}
	if !reflect.DeepEqual(got.Entries, want) {
		t.Fatal(got)
	}
	again, err := NormalizeIP(got)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("not idempotent")
	}
	if input.Entries[0] != " 131.34.123.159 " {
		t.Fatal("mutated caller slice")
	}
	for _, enabled := range []bool{false, true} {
		for _, v := range []string{"131.34.*.*", "1.2.3.4-1.2.3.5", "1.2.3.4:80", "fe80::1%eth0", "::ffff:1.2.3.0/120", "1.2.3.4/33", "2001:db8::/129", "garbage"} {
			if _, err = NormalizeIP(IPTargeting{Enabled: enabled, Entries: []string{"", v}}); err == nil || !strings.Contains(err.Error(), "line 2") {
				t.Fatalf("accepted %q enabled=%v: %v", v, enabled, err)
			}
		}
	}
	if _, err = NormalizeIP(IPTargeting{Enabled: true}); err == nil {
		t.Fatal("empty enabled accepted")
	}
	if _, err = NormalizeIP(IPTargeting{}); err != nil {
		t.Fatal(err)
	}
}
func TestIPFilter(t *testing.T) {
	filter, err := CompileIP(IPTargeting{Enabled: true, Entries: []string{"131.34.123.0/24", "10.0.0.1", "2001:db8::/126", "2001:db8:1::1"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"131.34.123.0", "131.34.123.255", "10.0.0.1", "::ffff:10.0.0.1", "2001:db8::", "2001:db8::3", "2001:db8:1::1"} {
		if !filter.Match(ParseUserIP(v)) {
			t.Fatalf("did not match %s", v)
		}
	}
	for _, v := range []string{"131.34.122.255", "131.34.124.0", "10.0.0.2", "2001:db8::4", "2001:db8:1::2", "", "bad", "fe80::1%eth0"} {
		if filter.Match(ParseUserIP(v)) {
			t.Fatalf("matched %s", v)
		}
	}
	all, err := CompileIP(IPTargeting{Entries: []string{"10.0.0.1"}})
	if err != nil || !all.Match(netip.Addr{}) || !all.Match(ParseUserIP("8.8.8.8")) {
		t.Fatal("disabled != All")
	}
}
