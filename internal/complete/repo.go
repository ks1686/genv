package complete

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ks1686/genv/internal/adapter"
)

const (
	OverallTimeout = 300 * time.Millisecond
	SearchTimeout  = 150 * time.Millisecond
	MaxWorkers     = 4
)

type repoJob struct {
	manager    string
	completion func(string) ([]string, error)
	list       func() ([]string, error)
	search     func(string) ([]string, error)
}

type repoResult struct {
	names []string
	err   error
}

// RepoPackages returns sorted unique bare names for Tab completion.
func RepoPackages(prefix string, available map[string]bool) []string {
	return repoPackagesOnGOOS(prefix, available, runtime.GOOS, time.Now())
}

func repoPackagesOnGOOS(
	prefix string,
	available map[string]bool,
	goos string,
	now time.Time,
) []string {
	jobs := make([]repoJob, 0, len(adapter.All))
	for _, candidate := range adapter.All {
		manager := candidate.Name()
		if !available[manager] || !adapter.AutomaticOnGOOS(manager, goos) {
			continue
		}

		job := repoJob{manager: manager}
		if namer, ok := candidate.(adapter.CompletionNamer); ok {
			job.completion = namer.CompletionNames
		}
		if lister, ok := candidate.(adapter.NameLister); ok {
			job.list = func() ([]string, error) {
				if names, hit := ReadDump(manager); hit {
					return names, nil
				}
				names, err := lister.ListNames()
				if err == nil {
					_ = WriteDump(manager, names)
				}
				return names, err
			}
		}
		if searchable, ok := candidate.(adapter.Searchable); ok {
			job.search = searchable.Search
		}
		jobs = append(jobs, job)
	}

	overall := time.Until(now.Add(OverallTimeout))
	if overall <= 0 {
		return nil
	}
	return collectRepoNames(prefix, jobs, overall, SearchTimeout)
}

func collectRepoNames(
	prefix string,
	jobs []repoJob,
	overall time.Duration,
	search time.Duration,
) []string {
	if len(jobs) == 0 || overall <= 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), overall)
	defer cancel()

	work := make(chan repoJob)
	results := make(chan repoResult, len(jobs))
	workerCount := min(MaxWorkers, len(jobs))

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-work:
					if !ok {
						return
					}
					names, err := runRepoJob(ctx, prefix, job, search)
					select {
					case results <- repoResult{names: names, err: err}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	go func() {
		defer close(work)
		for _, job := range jobs {
			select {
			case work <- job:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		workers.Wait()
		close(results)
	}()

	unique := make(map[string]struct{})
	for {
		select {
		case <-ctx.Done():
			return sortedRepoNames(unique)
		case result, ok := <-results:
			if !ok {
				return sortedRepoNames(unique)
			}
			if result.err != nil {
				continue
			}
			for _, name := range result.names {
				unique[name] = struct{}{}
			}
		}
	}
}

func runRepoJob(
	ctx context.Context,
	prefix string,
	job repoJob,
	searchTimeout time.Duration,
) ([]string, error) {
	if prefix != "" && job.completion != nil {
		return callRepoSearch(ctx, searchTimeout, func() ([]string, error) {
			return job.completion(prefix)
		})
	}
	if job.list != nil {
		names, err := job.list()
		if err != nil {
			return nil, err
		}
		return filterPrefix(names, prefix), nil
	}
	if prefix != "" && job.search != nil {
		return callRepoSearch(ctx, searchTimeout, func() ([]string, error) {
			return job.search(prefix)
		})
	}
	return nil, nil
}

func callRepoSearch(
	parent context.Context,
	timeout time.Duration,
	call func() ([]string, error),
) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	result := make(chan repoResult, 1)
	go func() {
		names, err := call()
		result <- repoResult{names: names, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-result:
		return completed.names, completed.err
	}
}

func filterPrefix(names []string, prefix string) []string {
	if prefix == "" {
		return names
	}
	foldedPrefix := strings.ToLower(prefix)
	var filtered []string
	for _, name := range names {
		if strings.HasPrefix(strings.ToLower(name), foldedPrefix) {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

func sortedRepoNames(unique map[string]struct{}) []string {
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
