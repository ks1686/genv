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
	manager       string
	completion    func(string) ([]string, error)
	completionCtx func(context.Context, string) ([]string, error)
	opaqueValues  bool
	list          func() ([]string, error)
	listCtx       func(context.Context) ([]string, error)
	search        func(string) ([]string, error)
	searchCtx     func(context.Context, string) ([]string, error)
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
	overall := time.Until(now.Add(OverallTimeout))
	if overall <= 0 {
		return nil
	}
	return collectRepoNames(prefix, repoJobs(available, goos), overall, SearchTimeout)
}

func repoJobs(available map[string]bool, goos string) []repoJob {
	jobs := make([]repoJob, 0, len(adapter.All))
	for _, candidate := range adapter.All {
		manager := candidate.Name()
		if !available[manager] || !adapter.AutomaticOnGOOS(manager, goos) {
			continue
		}

		job := repoJob{manager: manager}
		if namer, ok := candidate.(adapter.CompletionNamer); ok {
			job.completion = namer.CompletionNames
			// mas completion values are installable numeric IDs selected by an
			// already-filtered app-name query, so the IDs cannot match the prefix.
			job.opaqueValues = manager == "mas"
		}
		if namer, ok := candidate.(adapter.ContextCompletionNamer); ok {
			job.completionCtx = namer.CompletionNamesContext
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
		if lister, ok := candidate.(adapter.ContextNameLister); ok {
			job.listCtx = func(ctx context.Context) ([]string, error) {
				if names, hit := ReadDump(manager); hit {
					return names, nil
				}
				names, err := lister.ListNamesContext(ctx)
				if err == nil {
					_ = WriteDump(manager, names)
				}
				return names, err
			}
		}
		if searchable, ok := candidate.(adapter.Searchable); ok {
			job.search = searchable.Search
		}
		if searchable, ok := candidate.(adapter.ContextSearchable); ok {
			job.searchCtx = searchable.SearchContext
		}
		// Bun and npm search the same npm registry. When npm participates,
		// schedule that backend once while retaining Bun.Search for other callers.
		if manager == "bun" && available["npm"] && adapter.AutomaticOnGOOS("npm", goos) {
			job.search = nil
			job.searchCtx = nil
		}
		jobs = append(jobs, job)
	}
	return jobs
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
	if prefix != "" && job.completionCtx != nil {
		names, err := callRepoSearchContext(ctx, searchTimeout, func(ctx context.Context) ([]string, error) {
			return job.completionCtx(ctx, prefix)
		})
		return filterCompletionNames(names, prefix, job.opaqueValues), err
	}
	if prefix != "" && job.completion != nil {
		names, err := callRepoSearch(ctx, searchTimeout, func() ([]string, error) {
			return job.completion(prefix)
		})
		return filterCompletionNames(names, prefix, job.opaqueValues), err
	}
	if job.listCtx != nil {
		names, err := job.listCtx(ctx)
		if err != nil {
			return nil, err
		}
		return filterPrefix(names, prefix), nil
	}
	if job.list != nil {
		names, err := job.list()
		if err != nil {
			return nil, err
		}
		return filterPrefix(names, prefix), nil
	}
	if prefix != "" && job.searchCtx != nil {
		names, err := callRepoSearchContext(ctx, searchTimeout, func(ctx context.Context) ([]string, error) {
			return job.searchCtx(ctx, prefix)
		})
		return filterPrefix(names, prefix), err
	}
	if prefix != "" && job.search != nil {
		names, err := callRepoSearch(ctx, searchTimeout, func() ([]string, error) {
			return job.search(prefix)
		})
		return filterPrefix(names, prefix), err
	}
	return nil, nil
}

func filterCompletionNames(names []string, prefix string, opaqueValues bool) []string {
	if opaqueValues {
		return names
	}
	return filterPrefix(names, prefix)
}

func callRepoSearchContext(
	parent context.Context,
	timeout time.Duration,
	call func(context.Context) ([]string, error),
) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return call(ctx)
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
		err := ctx.Err()
		<-result
		return nil, err
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
