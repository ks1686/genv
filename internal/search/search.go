// Package search provides cross-manager package discovery. It queries every
// available adapter that implements adapter.Searchable and returns a deduplicated
// list of candidates for the user to choose from.
package search

import "github.com/ks1686/genv/internal/adapter"

// Candidate represents a package found in a specific manager's repository.
type Candidate struct {
	Manager string
	PkgName string
}

// All queries every available, searchable adapter for packages matching query
// and returns a deduplicated list of candidates ordered by adapter registry
// priority (brew → apt → flatpak → …). Adapters that are unavailable or do
// not implement adapter.Searchable are silently skipped, as are search errors.
func All(query string, available map[string]bool) []Candidate {
	var results []Candidate
	seen := make(map[string]bool)
	for _, a := range adapter.All {
		if !available[a.Name()] {
			continue
		}
		s, ok := a.(adapter.Searchable)
		if !ok {
			continue
		}
		names, err := s.Search(query)
		if err != nil || len(names) == 0 {
			continue
		}
		for _, name := range names {
			key := a.Name() + ":" + name
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, Candidate{Manager: a.Name(), PkgName: name})
		}
	}
	return results
}
