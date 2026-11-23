package model

import (
	"fmt"
	"net/netip"
	"strings"
)

// IPTargeting preserves the saved list when its enforcement is disabled.
type IPTargeting struct {
	Enabled bool     `json:"enabled"`
	Entries []string `json:"entries"`
}

// ParseUserIP never falls back to the SSP connection or proxy headers.
func ParseUserIP(s string) netip.Addr {
	ip, err := netip.ParseAddr(s)
	if err != nil || ip.Zone() != "" {
		return netip.Addr{}
	}
	return ip.Unmap()
}

// NormalizeIP validates every nonempty line, including disabled lists.
// Its result owns a new slice; callers' snapshots are never modified in place.
func NormalizeIP(t IPTargeting) (IPTargeting, error) {
	out := IPTargeting{Enabled: t.Enabled, Entries: []string{}}
	seen := map[string]bool{}
	for i, line := range t.Entries {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		var canonical string
		if strings.Contains(value, "/") {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return IPTargeting{}, fmt.Errorf("IP: line %d — invalid CIDR network %q", i+1, value)
			}
			if prefix.Addr().Is4In6() {
				return IPTargeting{}, fmt.Errorf("IP: line %d — use an IPv4 network instead of IPv4-mapped CIDR", i+1)
			}
			canonical = prefix.Masked().String()
		} else {
			ip := ParseUserIP(value)
			if !ip.IsValid() {
				return IPTargeting{}, fmt.Errorf("IP: line %d — invalid address %q", i+1, value)
			}
			canonical = ip.String()
		}
		if !seen[canonical] {
			out.Entries = append(out.Entries, canonical)
			seen[canonical] = true
		}
	}
	if out.Enabled && len(out.Entries) == 0 {
		return IPTargeting{}, fmt.Errorf("IP: enter at least one IP address or CIDR when targeting is enabled")
	}
	return out, nil
}

// IPFilter is immutable after construction and belongs to one runtime revision.
type IPFilter struct {
	enabled  bool
	exact    map[netip.Addr]struct{}
	prefixes []netip.Prefix
}

func CompileIP(t IPTargeting) (IPFilter, error) {
	normalized, err := NormalizeIP(t)
	if err != nil {
		return IPFilter{}, err
	}
	filter := IPFilter{enabled: t.Enabled}
	if !t.Enabled {
		return filter, nil
	}
	filter.exact = make(map[netip.Addr]struct{})
	for _, value := range normalized.Entries {
		if strings.Contains(value, "/") {
			filter.prefixes = append(filter.prefixes, netip.MustParsePrefix(value))
		} else {
			filter.exact[netip.MustParseAddr(value)] = struct{}{}
		}
	}
	return filter, nil
}
func (f IPFilter) Match(ip netip.Addr) bool {
	if !f.enabled {
		return true
	}
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if _, ok := f.exact[ip]; ok {
		return true
	}
	for _, prefix := range f.prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}
