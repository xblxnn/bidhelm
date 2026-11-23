package model

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const Schema = 3
const AdapterEXADS = "exads_json_pop"

type Targeting struct {
	Placement     PlacementTargeting `json:"placement"`
	IP            IPTargeting        `json:"ip"`
	Countries     []string           `json:"countries"`
	Platforms     []string           `json:"platforms"`
	Browsers      []string           `json:"browsers"`
	BrowserMajors []string           `json:"browser_majors"`
	OS            []string           `json:"os"`
	OSMajors      []string           `json:"os_majors"`
}
type Source struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Adapter string `json:"adapter"`
}
type Campaign struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	URL       string    `json:"url"`
	CPM       int64     `json:"cpm_microusd"`
	Active    bool      `json:"active"`
	Targeting Targeting `json:"targeting"`
}
type Snapshot struct {
	Schema    int        `json:"schema"`
	Revision  int64      `json:"revision"`
	Sources   []Source   `json:"sources"`
	Campaigns []Campaign `json:"campaigns"`
}
type Facts struct {
	PlacementID                                           string
	Country, Platform, Browser, BrowserMajor, OS, OSMajor string
	IP                                                    netip.Addr
}

func ID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func Money(s string) (int64, error) {
	p := strings.Split(strings.TrimSpace(s), ".")
	if len(p) > 2 || len(p) == 0 || p[0] == "" {
		return 0, errors.New("CPM must be a positive decimal with up to six fractional digits")
	}
	for _, v := range p {
		for _, c := range v {
			if c < '0' || c > '9' {
				return 0, errors.New("invalid CPM")
			}
		}
	}
	whole, err := strconv.ParseInt(p[0], 10, 64)
	if err != nil || whole > 1_000_000_000 {
		return 0, errors.New("CPM out of range")
	}
	frac := ""
	if len(p) == 2 {
		frac = p[1]
	}
	if len(frac) > 6 {
		return 0, errors.New("CPM supports at most six decimal places")
	}
	frac += strings.Repeat("0", 6-len(frac))
	f, _ := strconv.ParseInt(frac, 10, 64)
	n := whole*1_000_000 + f
	if n <= 0 {
		return 0, errors.New("CPM must be positive")
	}
	return n, nil
}
func FormatMoney(n int64) string { return fmt.Sprintf("%d.%06d", n/1_000_000, n%1_000_000) }
func ValidURL(s string) bool {
	u, e := url.Parse(s)
	return ValidPlacementURL(s) && e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
func Match(t Targeting, f Facts) bool {
	return contains(t.Countries, f.Country) && contains(t.Platforms, f.Platform) && contains(t.Browsers, f.Browser) && contains(t.BrowserMajors, f.BrowserMajor) && contains(t.OS, f.OS) && contains(t.OSMajors, f.OSMajor)
}
func contains(xs []string, x string) bool {
	if len(xs) == 0 {
		return true
	}
	if x == "" {
		return false
	}
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

var Browsers = []string{"Chrome", "Chrome Mobile", "Chrome Mobile iOS", "Firefox", "Firefox Mobile", "Firefox iOS", "Safari", "Mobile Safari", "Edge", "Edge Mobile", "Opera", "Opera Mobile", "Opera Mini", "Samsung Internet", "Yandex Browser", "UC Browser", "Amazon Silk"}
var OperatingSystems = []string{"Windows", "Mac OS X", "Linux", "Android", "iOS", "Chrome OS", "Ubuntu", "Debian", "Fedora"}

func ValidateTarget(t Targeting) error {
	if _, err := NormalizePlacement(t.Placement); err != nil {
		return err
	}
	if _, err := NormalizeIP(t.IP); err != nil {
		return err
	}
	for _, c := range t.Countries {
		if !ValidCountry(c) {
			return fmt.Errorf("unknown country %q", c)
		}
	}
	for _, v := range t.Platforms {
		if v != "mobile" && v != "desktop" {
			return fmt.Errorf("unknown platform %q", v)
		}
	}
	for _, v := range t.Browsers {
		if !member(Browsers, v) {
			return fmt.Errorf("unknown browser %q", v)
		}
	}
	for _, v := range t.OS {
		if !member(OperatingSystems, v) {
			return fmt.Errorf("unknown OS %q", v)
		}
	}
	for _, vs := range [][]string{t.BrowserMajors, t.OSMajors} {
		for _, v := range vs {
			n, e := strconv.Atoi(v)
			if e != nil || n < 0 || n > 9999 || strconv.Itoa(n) != v {
				return fmt.Errorf("invalid major version %q", v)
			}
		}
	}
	return nil
}
func member(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func Validate(s Snapshot, known func(string) bool) error {
	if (s.Schema < 1 || s.Schema > Schema) || s.Revision < 1 {
		return errors.New("invalid schema or revision")
	}
	seen := map[string]bool{}
	for _, v := range s.Sources {
		if v.ID == "" || strings.ContainsAny(v.ID, "/ ?#") || seen[v.ID] || !known(v.Adapter) {
			return errors.New("invalid, duplicate source or unknown adapter")
		}
		seen[v.ID] = true
	}
	seen = map[string]bool{}
	for _, c := range s.Campaigns {
		if s.Schema < 3 && (c.Targeting.Placement.Enabled || strings.Contains(c.URL, PlacementMacro)) {
			return errors.New("placement targeting and macros require schema 3")
		}
		if s.Schema == 1 && c.Targeting.IP.Enabled {
			return errors.New("schema 1 does not support enabled IP targeting")
		}
		if c.ID == "" || seen[c.ID] || !c.Active || !ValidURL(c.URL) || c.CPM <= 0 || c.CPM > 1_000_000_000_999999 {
			return errors.New("invalid or duplicate active campaign")
		}
		seen[c.ID] = true
		if err := ValidateTarget(c.Targeting); err != nil {
			return err
		}
	}
	return nil
}
