// Package search provides cross-manager package discovery. It queries every
// available adapter that implements adapter.Searchable and returns a deduplicated
// list of candidates for the user to choose from.
package search

import (
	"runtime"
	"sync"

	"github.com/ks1686/genv/internal/adapter"
)

const maxConcurrentSearches = 4

// Candidate represents a package found in a specific manager's repository.
type Candidate struct {
	Manager string
	PkgName string
}

type searchJob struct {
	searchable adapter.Searchable
	manager    string
	index      int
}

type searchResult struct {
	manager string
	names   []string
	err     error
}

// All queries every available, searchable adapter for packages matching query
// and returns a deduplicated list of candidates ordered by adapter registry
// priority (brew → pacman → paru → …). Adapters that are unavailable or do
// not implement adapter.Searchable are silently skipped, as are search errors.
func All(query string, available map[string]bool) []Candidate {
	return allOnGOOS(query, available, runtime.GOOS)
}

func allOnGOOS(query string, available map[string]bool, goos string) []Candidate {
	jobs := make([]searchJob, 0, len(adapter.All))
	for _, a := range adapter.All {
		manager := a.Name()
		if !available[manager] || !adapter.AutomaticOnGOOS(manager, goos) {
			continue
		}
		s, ok := a.(adapter.Searchable)
		if !ok {
			continue
		}
		jobs = append(jobs, searchJob{searchable: s, manager: manager, index: len(jobs)})
	}
	if len(jobs) == 0 {
		return nil
	}

	searchResults := runSearches(query, jobs)

	var results []Candidate
	seen := make(map[string]bool)
	for _, result := range searchResults {
		if result.err != nil || len(result.names) == 0 {
			continue
		}
		for _, name := range result.names {
			key := result.manager + ":" + name
			if seen[key] {
				continue
			}
			seen[key] = true
			results = append(results, Candidate{Manager: result.manager, PkgName: name})
		}
	}
	return results
}

func runSearches(query string, jobs []searchJob) []searchResult {
	results := make([]searchResult, len(jobs))
	jobCh := make(chan searchJob)

	workerCount := len(jobs)
	if workerCount > maxConcurrentSearches {
		workerCount = maxConcurrentSearches
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for job := range jobCh {
				names, err := job.searchable.Search(query)
				results[job.index] = searchResult{manager: job.manager, names: names, err: err}
			}
		}()
	}

	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)
	wg.Wait()

	return results
}
