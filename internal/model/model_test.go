package model

import "testing"

func TestMoney(t *testing.T) {
	for _, s := range []string{"0.13", "1", "0.000001", "15.123456"} {
		n, e := Money(s)
		if e != nil {
			t.Fatal(e)
		}
		round, e := Money(FormatMoney(n))
		if e != nil || n != round {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"0", "-1", "1.1234567", "NaN", "1e9", "999999999999999999999"} {
		if _, e := Money(s); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestTargeting(t *testing.T) {
	f := Facts{Country: "US", Platform: "desktop", Browser: "Chrome", BrowserMajor: "120", OS: "Linux", OSMajor: "6"}
	cases := []struct {
		name string
		t    Targeting
		f    Facts
		want bool
	}{
		{"all unknown", Targeting{}, Facts{}, true}, {"unknown country", Targeting{Countries: []string{"US"}}, Facts{}, false},
		{"country OR", Targeting{Countries: []string{"GB", "US"}}, f, true}, {"fields AND", Targeting{Countries: []string{"US"}, Platforms: []string{"mobile"}}, f, false},
		{"browser major", Targeting{Browsers: []string{"Chrome"}, BrowserMajors: []string{"120"}}, f, true}, {"wrong major", Targeting{BrowserMajors: []string{"121"}}, f, false},
		{"OS major", Targeting{OS: []string{"Linux"}, OSMajors: []string{"6"}}, f, true}, {"unknown browser", Targeting{Browsers: []string{"Chrome"}}, Facts{}, false},
		{"unknown OS version", Targeting{OSMajors: []string{"17"}}, Facts{}, false}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Match(c.t, c.f); got != c.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}
