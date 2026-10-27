package spec_test

import (
	"encoding/json"
	"os"
	"testing"
)

func TestURIIPLiteralFormatStableAcrossCompilers(t *testing.T) {
	schema := compile(t)
	example, e := os.ReadFile("example.snapshot.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, uri string
		valid     bool
	}{
		{"ipv6", "http://[2001:db8::1]/log", true},
		{"mapped_ipv4", "http://[::ffff:192.0.2.1]:8080/log", true},
		{"userinfo", "https://user%40name:password@[::1]:443/log", true},
		{"future", "http://[v1.host]/log", true},
		{"future_uppercase_and_subdelims", "custom://user@[VF.a:b!$&'()*+,;=~]:80/log", true},
		{"future_full_hex_version", "http://[v0123456789aBcDeF.name]/log", true},
		{"opaque", "urn:log:valid%20escaped", true},
		{"custom_evidence", "artex://assets/123", true},
		{"file", "file:///tmp/log.txt", true},
		{"registered_host", "https://example.com:443/log", true},
		{"bracket_non_ip", "http://[not-ipv6]/log", false},
		{"bracket_ipv4", "http://[192.0.2.1]/log", false},
		{"unbracketed_ipv6", "http://::1/log", false},
		{"missing_bracket", "http://[::1/log", false},
		{"extra_bracket", "http://[::1]]/log", false},
		{"misplaced_bracket", "http://example[::1]/log", false},
		{"bracket_in_userinfo", "http://[::1]@example.com/log", false},
		{"multiple_raw_userinfo_separators", "http://user@other@[::1]/log", false},
		{"literal_host_suffix", "http://[::1]example.com/log", false},
		{"literal_port", "http://[::1]:port/log", false},
		{"zone", "http://[fe80::1%25eth0]/log", false},
		{"future_bad_version", "http://[vG.name]/log", false},
		{"future_missing_version", "http://[v.name]/log", false},
		{"future_missing_address", "http://[v1.]/log", false},
		{"future_escape", "http://[v1.a%20b]/log", false},
		{"future_userinfo_delimiter", "http://[v1.a@b]/log", false},
		{"relative", "/tmp/log.txt", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var doc map[string]any
			if e := json.Unmarshal(example, &doc); e != nil {
				t.Fatal(e)
			}
			doc["resource_refs"].([]any)[0].(map[string]any)["uri"] = tc.uri
			if e := schema.Validate(doc); (e == nil) != tc.valid {
				t.Fatalf("URI %q valid=%v: %v", tc.uri, tc.valid, e)
			}
		})
	}
}
