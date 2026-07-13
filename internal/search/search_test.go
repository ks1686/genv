package search

import (
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
)

func TestAllOnGOOS_PlatformHomebrew(t *testing.T) {
	originalAll := adapter.All
	t.Cleanup(func() {
		adapter.All = originalAll
	})

	var brewCalls atomic.Int32
	var linuxbrewCalls atomic.Int32
	adapter.All = []adapter.Adapter{
		mockSearchableAdapter{
			mockAdapter: mockAdapter{name: "brew"},
			searchFunc: func(string) ([]string, error) {
				brewCalls.Add(1)
				return []string{"brew-result"}, nil
			},
		},
		mockSearchableAdapter{
			mockAdapter: mockAdapter{name: "linuxbrew"},
			searchFunc: func(string) ([]string, error) {
				linuxbrewCalls.Add(1)
				return []string{"linuxbrew-result"}, nil
			},
		},
	}

	tests := []struct {
		name               string
		goos               string
		want               []Candidate
		wantBrewCalls      int32
		wantLinuxbrewCalls int32
	}{
		{
			name:          "Darwin queries Homebrew only",
			goos:          "darwin",
			want:          []Candidate{{Manager: "brew", PkgName: "brew-result"}},
			wantBrewCalls: 1,
		},
		{
			name:               "Linux queries Linuxbrew only",
			goos:               "linux",
			want:               []Candidate{{Manager: "linuxbrew", PkgName: "linuxbrew-result"}},
			wantLinuxbrewCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brewCalls.Store(0)
			linuxbrewCalls.Store(0)

			got := allOnGOOS("pkg", map[string]bool{"brew": true, "linuxbrew": true}, tt.goos)

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("candidates = %#v, want %#v", got, tt.want)
			}
			if got := brewCalls.Load(); got != tt.wantBrewCalls {
				t.Errorf("brew search calls = %d, want %d", got, tt.wantBrewCalls)
			}
			if got := linuxbrewCalls.Load(); got != tt.wantLinuxbrewCalls {
				t.Errorf("linuxbrew search calls = %d, want %d", got, tt.wantLinuxbrewCalls)
			}
		})
	}
}

// mockAdapter implements adapter.Adapter but NOT adapter.Searchable
type mockAdapter struct {
	name string
}

func (m mockAdapter) Name() string                                                 { return m.name }
func (m mockAdapter) Available() bool                                              { return true }
func (m mockAdapter) NormalizeID(id string, mgrs map[string]string) (string, bool) { return id, false }
func (m mockAdapter) PlanInstall(pkg string) []string                              { return nil }
func (m mockAdapter) PlanUninstall(pkg string) []string                            { return nil }
func (m mockAdapter) PlanUpgrade(pkg string) []string                              { return nil }
func (m mockAdapter) PlanClean() [][]string                                        { return nil }
func (m mockAdapter) Query(pkg string) (bool, error)                               { return false, nil }
func (m mockAdapter) ListInstalled() ([]string, error)                             { return nil, nil }
func (m mockAdapter) QueryVersion(pkg string) (string, error)                      { return "", nil }

// mockSearchableAdapter implements both adapter.Adapter and adapter.Searchable
type mockSearchableAdapter struct {
	mockAdapter
	searchFunc func(query string) ([]string, error)
}

func (m mockSearchableAdapter) Search(query string) ([]string, error) {
	if m.searchFunc != nil {
		return m.searchFunc(query)
	}
	return nil, nil
}

func TestAll(t *testing.T) {
	// Save the original adapter.All to restore it later
	originalAll := adapter.All
	defer func() {
		adapter.All = originalAll
	}()

	// Define our mock adapters
	adapter1 := mockSearchableAdapter{
		mockAdapter: mockAdapter{name: "brew"},
		searchFunc: func(query string) ([]string, error) {
			if query == "error" {
				return nil, errors.New("search failed")
			}
			return []string{"pkg1", "pkg2"}, nil
		},
	}

	adapter2 := mockSearchableAdapter{
		mockAdapter: mockAdapter{name: "pacman"},
		searchFunc: func(query string) ([]string, error) {
			return []string{"pkg2", "pkg3", "pkg3"}, nil // pkg3 repeated to test deduplication within manager
		},
	}

	adapter3 := mockAdapter{name: "snap"} // Not searchable

	adapter.All = []adapter.Adapter{adapter1, adapter2, adapter3}

	tests := []struct {
		name      string
		query     string
		available map[string]bool
		want      []Candidate
	}{
		{
			name:  "all adapters available",
			query: "test",
			available: map[string]bool{
				"brew":   true,
				"pacman": true,
				"snap":   true,
			},
			want: []Candidate{
				{Manager: "brew", PkgName: "pkg1"},
				{Manager: "brew", PkgName: "pkg2"},
				{Manager: "pacman", PkgName: "pkg2"}, // different manager, should be kept
				{Manager: "pacman", PkgName: "pkg3"},
			},
		},
		{
			name:  "brew unavailable",
			query: "test",
			available: map[string]bool{
				"brew":   false,
				"pacman": true,
				"snap":   true,
			},
			want: []Candidate{
				{Manager: "pacman", PkgName: "pkg2"},
				{Manager: "pacman", PkgName: "pkg3"},
			},
		},
		{
			name:  "search error",
			query: "error", // triggers error in brew mock
			available: map[string]bool{
				"brew":   true,
				"pacman": true,
			},
			want: []Candidate{
				{Manager: "pacman", PkgName: "pkg2"},
				{Manager: "pacman", PkgName: "pkg3"},
			},
		},
		{
			name:  "no available adapters",
			query: "test",
			available: map[string]bool{
				"brew":   false,
				"pacman": false,
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := All(tc.query, tc.available)

			if len(got) != len(tc.want) {
				t.Fatalf("got %d candidates, want %d", len(got), len(tc.want))
			}

			for i, c := range got {
				if c.Manager != tc.want[i].Manager || c.PkgName != tc.want[i].PkgName {
					t.Errorf("candidate %d: got {Manager: %q, PkgName: %q}, want {Manager: %q, PkgName: %q}",
						i, c.Manager, c.PkgName, tc.want[i].Manager, tc.want[i].PkgName)
				}
			}
		})
	}
}

func TestAll_preservesRegistryOrderAndDedupeWhenSearchesCompleteOutOfOrder(t *testing.T) {
	originalAll := adapter.All
	t.Cleanup(func() {
		adapter.All = originalAll
	})

	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	firstAdapter := mockSearchableAdapter{
		mockAdapter: mockAdapter{name: "brew"},
		searchFunc: func(query string) ([]string, error) {
			select {
			case <-secondStarted:
				return []string{"shared", "brew-only", "shared"}, nil
			case <-releaseFirst:
				return nil, errors.New("released blocked search")
			}
		},
	}
	secondAdapter := mockSearchableAdapter{
		mockAdapter: mockAdapter{name: "apt"},
		searchFunc: func(query string) ([]string, error) {
			close(secondStarted)
			return []string{"shared", "apt-only", "shared"}, nil
		},
	}

	adapter.All = []adapter.Adapter{firstAdapter, secondAdapter}

	done := make(chan []Candidate, 1)
	go func() {
		done <- All("pkg", map[string]bool{"brew": true, "apt": true})
	}()

	var got []Candidate
	select {
	case got = <-done:
	case <-time.After(200 * time.Millisecond):
		close(releaseFirst)
		<-done
		t.Fatal("searches did not start concurrently")
	}

	want := []Candidate{
		{Manager: "brew", PkgName: "shared"},
		{Manager: "brew", PkgName: "brew-only"},
		{Manager: "apt", PkgName: "shared"},
		{Manager: "apt", PkgName: "apt-only"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
