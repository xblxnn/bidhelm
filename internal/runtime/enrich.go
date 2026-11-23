package runtime

import (
	"bidhelm/internal/model"
	"fmt"
	"github.com/oschwald/maxminddb-golang"
	"github.com/ua-parser/uap-go/uaparser"
	"net"
	"os"
	"strings"
)

type Enricher struct {
	parser *uaparser.Parser
	db     *maxminddb.Reader
}

func NewEnricher(path string) (*Enricher, error) {
	p, e := uaparser.New(uaparser.WithCacheSize(10000), uaparser.WithSort(false))
	if e != nil {
		return nil, e
	}
	v := &Enricher{parser: p}
	b, e := os.ReadFile(path)
	if e != nil {
		return v, fmt.Errorf("MMDB: %w", e)
	}
	v.db, e = maxminddb.FromBytes(b)
	if e != nil {
		return v, fmt.Errorf("MMDB: %w", e)
	}
	if e = v.db.Verify(); e != nil {
		v.db.Close()
		v.db = nil
		return v, fmt.Errorf("MMDB verification: %w", e)
	}
	return v, nil
}
func (e *Enricher) Close() {
	if e.db != nil {
		e.db.Close()
	}
}
func (e *Enricher) Facts(ip, ua string) model.Facts {
	f := model.Facts{IP: model.ParseUserIP(ip)}
	if e.db != nil {
		var record struct {
			Country struct {
				ISO string `maxminddb:"iso_code"`
			} `maxminddb:"country"`
		}
		if f.IP.IsValid() {
			addr := net.IP(f.IP.AsSlice())
			if e.db.Lookup(addr, &record) == nil {
				f.Country = record.Country.ISO
			}
		}
	}
	if ua == "" || len(ua) > 2048 {
		return f
	}
	c := e.parser.Parse(ua)
	if c.UserAgent.Family != "Other" {
		f.Browser = c.UserAgent.Family
		f.BrowserMajor = c.UserAgent.Major
	}
	if c.Os.Family != "Other" {
		f.OS = c.Os.Family
		f.OSMajor = c.Os.Major
	}
	lower := strings.ToLower(ua)
	if strings.Contains(lower, "bot") || strings.Contains(lower, "spider") || strings.Contains(lower, "crawler") {
		return f
	}
	switch f.OS {
	case "Android", "iOS":
		f.Platform = "mobile"
	case "Windows", "Mac OS X", "Chrome OS", "Ubuntu", "Debian", "Fedora":
		f.Platform = "desktop"
	case "Linux":
		if strings.Contains(lower, "x11") && !strings.Contains(lower, "mobile") {
			f.Platform = "desktop"
		}
	}
	if strings.Contains(lower, "mobile") || strings.Contains(lower, "tablet") || strings.Contains(lower, "ipad") || strings.Contains(lower, "kindle") {
		f.Platform = "mobile"
	}
	return f
}
