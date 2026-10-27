// Package spec embeds the canonical portable contract for runtime validation.
package spec

import (
	_ "embed"
	"errors"
	"net/netip"
	"net/url"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed snapshot.schema.json
var SnapshotSchema []byte

// RegisterFormats asserts schema formats and makes URI IP-literal validation
// independent of Go's changing net/url host checks. The URI remains opaque data;
// parsing neither resolves nor rewrites it. RFC 3986 section 3.2.2 permits IPv6
// and IPvFuture literals, but not scoped IPv6 zone identifiers.
func RegisterFormats(c *jsonschema.Compiler) {
	c.AssertFormat()
	c.RegisterFormat(&jsonschema.Format{Name: "uri", Validate: validateURI})
}

func validateURI(v any) error {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	invalid := errors.New("invalid absolute URI")
	parseInput := s
	colon := strings.IndexByte(s, ':')
	if colon >= 0 && strings.HasPrefix(s[colon+1:], "//") {
		start := colon + 3
		end := len(s)
		if i := strings.IndexAny(s[start:], "/?#"); i >= 0 {
			end = start + i
		}
		authority := s[start:end]
		if i := strings.LastIndexByte(authority, '@'); i >= 0 {
			// Raw '@' and brackets are not userinfo characters. Escaped
			// userinfo is left for net/url to validate normally.
			if strings.ContainsAny(authority[:i], "@[]") {
				return invalid
			}
			start += i + 1
		}
		host := s[start:end]
		if strings.HasPrefix(host, "[") {
			close := strings.IndexByte(host, ']')
			if close < 0 {
				return invalid
			}
			literal, port := host[1:close], host[close+1:]
			if port != "" {
				if port[0] != ':' {
					return invalid
				}
				for i := 1; i < len(port); i++ {
					if port[i] < '0' || port[i] > '9' {
						return invalid
					}
				}
			}
			if ipvFuture(literal) {
				// Go's URL parser does not consistently support IPvFuture.
				// Having validated this literal's exact grammar, parse all
				// remaining URI components with an IPv6 placeholder.
				parseInput = s[:start] + "[::1]" + s[start+close+1:]
			} else {
				addr, err := netip.ParseAddr(literal)
				if err != nil || !addr.Is6() || addr.Zone() != "" {
					return invalid
				}
			}
		} else if strings.ContainsAny(host, "[]") || strings.Count(host, ":") > 1 {
			return invalid
		}
	}
	u, err := url.Parse(parseInput)
	if err != nil || !u.IsAbs() {
		return invalid
	}
	return nil
}

// https://www.rfc-editor.org/rfc/rfc3986#section-3.2.2:
// IPvFuture = "v" 1*HEXDIG "." 1*(unreserved / sub-delims / ":").
func ipvFuture(s string) bool {
	if len(s) < 4 || (s[0] != 'v' && s[0] != 'V') {
		return false
	}
	dot := strings.IndexByte(s, '.')
	if dot < 2 || dot == len(s)-1 {
		return false
	}
	for i := 1; i < dot; i++ {
		b := s[i]
		if !((b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')) {
			return false
		}
	}
	for i := dot + 1; i < len(s); i++ {
		b := s[i]
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || strings.ContainsRune("-._~!$&'()*+,;=:", rune(b))) {
			return false
		}
	}
	return true
}
