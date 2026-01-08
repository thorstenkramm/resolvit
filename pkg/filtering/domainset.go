package filtering

import "strings"

// DomainSet stores exact and wildcard domain matches.
type DomainSet struct {
	Exact     map[string]struct{}
	Wildcards map[string]struct{}
}

// NewDomainSet creates an empty DomainSet.
func NewDomainSet() *DomainSet {
	return &DomainSet{
		Exact:     make(map[string]struct{}),
		Wildcards: make(map[string]struct{}),
	}
}

// AddExact adds an exact domain to the set.
func (s *DomainSet) AddExact(domain string) {
	if domain == "" {
		return
	}
	s.Exact[domain] = struct{}{}
}

// AddWildcard adds a wildcard domain suffix to the set.
func (s *DomainSet) AddWildcard(domain string) {
	if domain == "" {
		return
	}
	s.Wildcards[domain] = struct{}{}
}

// Merge merges another DomainSet into this one.
func (s *DomainSet) Merge(other *DomainSet) {
	if other == nil {
		return
	}
	for domain := range other.Exact {
		s.Exact[domain] = struct{}{}
	}
	for domain := range other.Wildcards {
		s.Wildcards[domain] = struct{}{}
	}
}

// Matches checks if name matches the set based on subdomain rules.
func (s *DomainSet) Matches(name string, includeSubdomains bool) bool {
	if s == nil {
		return false
	}
	normalised := normalizeLookupName(name)
	if normalised == "" {
		return false
	}
	if _, ok := s.Exact[normalised]; ok {
		return true
	}
	labels := strings.Split(normalised, ".")
	if matchesWildcard(labels, s.Wildcards) {
		return true
	}
	if !includeSubdomains {
		return false
	}
	return matchesSuffix(labels, s.Exact)
}

func matchesWildcard(labels []string, wildcards map[string]struct{}) bool {
	for i := 1; i < len(labels); i++ {
		suffix := strings.Join(labels[i:], ".")
		if _, ok := wildcards[suffix]; ok {
			return true
		}
	}
	return false
}

func matchesSuffix(labels []string, exact map[string]struct{}) bool {
	for i := 1; i < len(labels); i++ {
		suffix := strings.Join(labels[i:], ".")
		if _, ok := exact[suffix]; ok {
			return true
		}
	}
	return false
}

func normalizeLookupName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, ".")
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}
