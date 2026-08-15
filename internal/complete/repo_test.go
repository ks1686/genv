package complete

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/testutil"
)

func TestCollectRepoNames_filtersSortsAndDeduplicatesListerResults(t *testing.T) {
	got := collectRepoNames("O", []repoJob{
		{
			manager: "brew",
			list: func() ([]string, error) {
				return []string{"wget", "openjdk", "Opera"}, nil
			},
		},
		{
			manager: "snap",
			search: func(query string) ([]string, error) {
				return []string{"opera", "openjdk"}, nil
			},
		},
	}, time.Second, time.Second)

	want := []string{"Opera", "openjdk", "opera"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestCollectRepoNames_emptyPrefixUsesOnlyLister(t *testing.T) {
	got := collectRepoNames("", []repoJob{
		{
			manager: "brew",
			list: func() ([]string, error) {
				return []string{"git"}, nil
			},
			completion: func(string) ([]string, error) {
				return []string{"completion-must-not-run"}, nil
			},
			search: func(string) ([]string, error) {
				return []string{"search-must-not-run"}, nil
			},
		},
		{
			manager: "npm",
			search: func(string) ([]string, error) {
				return []string{"search-only-must-not-run"}, nil
			},
		},
	}, time.Second, time.Second)

	if !slices.Equal(got, []string{"git"}) {
		t.Fatalf("got %v want [git]", got)
	}
}

func TestCollectRepoNames_prefersCompletionNamerForNonEmptyPrefix(t *testing.T) {
	got := collectRepoNames("g", []repoJob{{
		manager: "mas",
		completion: func(prefix string) ([]string, error) {
			return []string{"GarageBand", "Xcode"}, nil
		},
		list: func() ([]string, error) {
			return []string{"list-must-not-run"}, nil
		},
		search: func(string) ([]string, error) {
			return []string{"search-must-not-run"}, nil
		},
	}}, time.Second, time.Second)

	if !slices.Equal(got, []string{"GarageBand"}) {
		t.Fatalf("got %v want [GarageBand]", got)
	}
}

func TestCollectRepoNames_filtersSearchResultsByPrefix(t *testing.T) {
	got := collectRepoNames("open", []repoJob{{
		manager: "search",
		search: func(string) ([]string, error) {
			return []string{"OpenJDK", "not-open-prefix"}, nil
		},
	}}, time.Second, time.Second)

	if !slices.Equal(got, []string{"OpenJDK"}) {
		t.Fatalf("got %v want [OpenJDK]", got)
	}
}

func TestCollectRepoNames_prefersListerToSearch(t *testing.T) {
	got := collectRepoNames("g", []repoJob{{
		manager: "brew",
		list: func() ([]string, error) {
			return []string{"git"}, nil
		},
		search: func(string) ([]string, error) {
			return []string{"search-must-not-run"}, nil
		},
	}}, time.Second, time.Second)

	if !slices.Equal(got, []string{"git"}) {
		t.Fatalf("got %v want [git]", got)
	}
}

func TestCollectRepoNames_searchTimeoutSkipsManager(t *testing.T) {
	got := collectRepoNames("g", []repoJob{
		{
			manager: "slow",
			search: func(string) ([]string, error) {
				time.Sleep(200 * time.Millisecond)
				return []string{"git"}, nil
			},
		},
		{
			manager: "fast",
			list: func() ([]string, error) {
				return []string{"gdb"}, nil
			},
		},
	}, 300*time.Millisecond, 50*time.Millisecond)

	if !slices.Equal(got, []string{"gdb"}) {
		t.Fatalf("got %v want [gdb]", got)
	}
}

func TestCollectRepoNames_completionTimeoutSkipsManager(t *testing.T) {
	got := collectRepoNames("g", []repoJob{
		{
			manager: "slow",
			completion: func(string) ([]string, error) {
				time.Sleep(200 * time.Millisecond)
				return []string{"garageband"}, nil
			},
		},
		{
			manager: "fast",
			list: func() ([]string, error) {
				return []string{"git"}, nil
			},
		},
	}, 300*time.Millisecond, 50*time.Millisecond)

	if !slices.Equal(got, []string{"git"}) {
		t.Fatalf("got %v want [git]", got)
	}
}

func TestCollectRepoNames_overallTimeoutDropsUnfinishedLister(t *testing.T) {
	started := time.Now()
	got := collectRepoNames("", []repoJob{{
		manager: "slow",
		list: func() ([]string, error) {
			time.Sleep(200 * time.Millisecond)
			return []string{"git"}, nil
		},
	}}, 30*time.Millisecond, time.Second)

	if len(got) != 0 {
		t.Fatalf("got %v want no unfinished results", got)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("overall timeout returned after %v", elapsed)
	}
}

func TestCollectRepoNames_skipsManagerErrors(t *testing.T) {
	got := collectRepoNames("g", []repoJob{
		{
			manager: "broken-list",
			list: func() ([]string, error) {
				return nil, errors.New("list failed")
			},
		},
		{
			manager: "broken-search",
			search: func(string) ([]string, error) {
				return nil, errors.New("search failed")
			},
		},
		{
			manager: "working",
			list: func() ([]string, error) {
				return []string{"git"}, nil
			},
		},
	}, time.Second, time.Second)

	if !slices.Equal(got, []string{"git"}) {
		t.Fatalf("got %v want [git]", got)
	}
}

func TestRepoPackagesOnGOOS_usesCachedAutomaticManagers(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := WriteDump("brew", []string{"wget", "OpenJDK"}); err != nil {
		t.Fatal(err)
	}

	got := repoPackagesOnGOOS(
		"open",
		map[string]bool{"brew": true},
		"darwin",
		time.Now(),
	)
	if !slices.Equal(got, []string{"OpenJDK"}) {
		t.Fatalf("got %v want [OpenJDK]", got)
	}

	got = repoPackagesOnGOOS(
		"open",
		map[string]bool{"brew": true},
		"linux",
		time.Now(),
	)
	if len(got) != 0 {
		t.Fatalf("ineligible brew returned %v on linux", got)
	}
}

func TestRepoPackagesOnGOOS_deduplicatesBunAndNpmRegistrySearch(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	countPath := filepath.Join(t.TempDir(), "calls")
	t.Setenv("NPM_SEARCH_COUNT", countPath)
	testutil.InstallFakeBinary(t, "npm", `echo call >> "$NPM_SEARCH_COUNT"
printf 'typescript\tdesc\tdate\tver\tkeywords\n'`)

	got := repoPackagesOnGOOS(
		"type",
		map[string]bool{"bun": true, "npm": true},
		"darwin",
		time.Now(),
	)
	if !slices.Equal(got, []string{"typescript"}) {
		t.Fatalf("got %v want [typescript]", got)
	}
	content, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if calls := strings.Count(string(content), "call\n"); calls != 1 {
		t.Fatalf("npm search calls = %d, want 1", calls)
	}
}

func TestCollectRepoNames_keepsMasProductIDsMatchedByName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	testutil.InstallFakeBinary(t, "mas", `printf '497799835  Xcode (16.0)\n'`)

	mas := adapter.Mas{}
	got := collectRepoNames("xcode", []repoJob{{
		manager:       "mas",
		completionCtx: mas.CompletionNamesContext,
		opaqueValues:  true,
	}}, time.Second, time.Second)
	if !slices.Equal(got, []string{"497799835"}) {
		t.Fatalf("got %v want [497799835]", got)
	}
}

func TestCollectRepoNames_limitsConcurrentManagers(t *testing.T) {
	started := make(chan struct{}, MaxWorkers+1)
	release := make(chan struct{})
	jobs := make([]repoJob, MaxWorkers+1)
	for i := range jobs {
		name := "pkg-" + strconv.Itoa(i)
		jobs[i] = repoJob{
			manager: name,
			list: func() ([]string, error) {
				started <- struct{}{}
				<-release
				return []string{name}, nil
			},
		}
	}

	done := make(chan []string, 1)
	go func() {
		done <- collectRepoNames("", jobs, time.Second, time.Second)
	}()

	for range MaxWorkers {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	select {
	case <-started:
		t.Fatalf("more than %d managers ran concurrently", MaxWorkers)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)

	select {
	case got := <-done:
		if len(got) != len(jobs) {
			t.Fatalf("got %d names want %d", len(got), len(jobs))
		}
	case <-time.After(time.Second):
		t.Fatal("collection did not finish")
	}
}

func TestCollectRepoNames_searchTimeoutRetainsConcurrencySlot(t *testing.T) {
	const jobCount = MaxWorkers * 2

	release := make(chan struct{})
	started := make(chan struct{}, jobCount)
	var inFlight atomic.Int32
	var peak atomic.Int32

	jobs := make([]repoJob, jobCount)
	for i := range jobs {
		jobs[i] = repoJob{
			manager: "search-" + strconv.Itoa(i),
			search: func(string) ([]string, error) {
				current := inFlight.Add(1)
				for {
					previous := peak.Load()
					if current <= previous || peak.CompareAndSwap(previous, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				inFlight.Add(-1)
				return nil, nil
			},
		}
	}

	done := make(chan []string, 1)
	go func() {
		done <- collectRepoNames("g", jobs, time.Second, 25*time.Millisecond)
	}()

	for range MaxWorkers {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("initial searches did not start")
		}
	}

	time.Sleep(100 * time.Millisecond)
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collection did not finish")
	}

	if got := peak.Load(); got > MaxWorkers {
		t.Fatalf("peak in-flight searches = %d, want at most %d", got, MaxWorkers)
	}
}
