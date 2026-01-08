package filtering

import "testing"

func TestDomainSetMatches(t *testing.T) {
	set := NewDomainSet()
	set.AddExact("example.com")
	set.AddWildcard("wild.example.com")

	if !set.Matches("example.com.", false) {
		t.Error("expected exact match for example.com")
	}
	if set.Matches("sub.example.com", false) {
		t.Error("expected sub.example.com to be allowed when subdomain blocking is off")
	}
	if !set.Matches("sub.example.com", true) {
		t.Error("expected sub.example.com to match when subdomain blocking is on")
	}
	if !set.Matches("host.wild.example.com", false) {
		t.Error("expected wildcard match for host.wild.example.com")
	}
	if set.Matches("other.example.net", true) {
		t.Error("did not expect match for other.example.net")
	}
}
