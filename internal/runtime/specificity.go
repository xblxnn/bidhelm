package runtime

import (
	"bidhelm/internal/model"
	"net/netip"
	"sort"
	"strings"
)

// Compare the sets of accepted values for each independent targeting field.
type scope struct {
	categories [6]map[string]bool
	placements map[string]bool
	whitelist  bool      // otherwise placements is the excluded set (empty = All)
	ips        []ipRange // nil = All, including unknown IP
}
type ipRange struct{ first, last netip.Addr }

func values(xs []string) map[string]bool {
	var out map[string]bool
	if len(xs) > 0 {
		out = make(map[string]bool, len(xs))
		for _, x := range xs {
			out[x] = true
		}
	}
	return out
}
func newScope(c Candidate) scope {
	t := c.Campaign.Targeting
	s := scope{categories: [6]map[string]bool{values(t.Countries), values(t.Platforms), values(t.Browsers), values(t.BrowserMajors), values(t.OS), values(t.OSMajors)}}
	if t.Placement.Enabled {
		s.whitelist = t.Placement.Mode == "whitelist"
		for _, v := range t.Placement.Entries {
			if v = strings.TrimSpace(v); v != "" {
				if s.placements == nil {
					s.placements = map[string]bool{}
				}
				s.placements[v] = true
			}
		}
	}
	if t.IP.Enabled {
		// PrepareCampaigns validated these entries with CompileIP.
		for _, entry := range t.IP.Entries {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			prefix, err := netip.ParsePrefix(entry)
			if err != nil {
				ip := model.ParseUserIP(entry)
				prefix = netip.PrefixFrom(ip, ip.BitLen())
			}
			first := prefix.Masked().Addr()
			b := first.AsSlice()
			for bit := prefix.Bits(); bit < len(b)*8; bit++ {
				b[bit/8] |= 1 << uint(7-bit%8)
			}
			last, _ := netip.AddrFromSlice(b)
			s.ips = append(s.ips, ipRange{first, last})
		}
		sort.Slice(s.ips, func(i, j int) bool { return s.ips[i].first.Less(s.ips[j].first) })
		merged := s.ips[:0]
		for _, r := range s.ips {
			if n := len(merged); n > 0 {
				p := &merged[n-1]
				if p.first.BitLen() == r.first.BitLen() && (!p.last.Less(r.first) || p.last.Next() == r.first) {
					if p.last.Less(r.last) {
						p.last = r.last
					}
					continue
				}
			}
			merged = append(merged, r)
		}
		s.ips = merged
	}
	return s
}
func subset(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
func (a scope) within(b scope) bool {
	for i, x := range a.categories {
		y := b.categories[i]
		if len(y) > 0 && (len(x) == 0 || !subset(x, y)) {
			return false
		}
	}
	switch {
	case a.whitelist && b.whitelist:
		if !subset(a.placements, b.placements) {
			return false
		}
	case a.whitelist:
		for k := range a.placements {
			if b.placements[k] {
				return false
			}
		}
	case b.whitelist:
		return false
	default:
		if !subset(b.placements, a.placements) {
			return false
		}
	}
	if b.ips == nil {
		return true
	}
	if a.ips == nil {
		return false
	}
	j := 0
	for _, r := range a.ips {
		for j < len(b.ips) && b.ips[j].last.Less(r.first) {
			j++
		}
		if j == len(b.ips) || r.first.Less(b.ips[j].first) || b.ips[j].last.Less(r.last) {
			return false
		}
	}
	return true
}

func prepareSpecificity(cs []Candidate) {
	scopes := make([]scope, len(cs))
	for i, c := range cs {
		scopes[i] = newScope(c)
	}
	for i, a := range scopes {
		for j := 0; j < i; j++ {
			b := scopes[j]
			ab, ba := a.within(b), b.within(a)
			if ab && !ba {
				cs[i].broader = append(cs[i].broader, j)
			}
			if ba && !ab {
				cs[j].broader = append(cs[j].broader, i)
			}
		}
	}
}
