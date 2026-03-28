package search

import (
	"errors"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
)

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
		mockAdapter: mockAdapter{name: "apt"},
		searchFunc: func(query string) ([]string, error) {
			return []string{"pkg2", "pkg3", "pkg3"}, nil // pkg3 repeated to test deduplication within manager
		},
	}

	adapter3 := mockAdapter{name: "flatpak"} // Not searchable

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
				"brew":    true,
				"apt":     true,
				"flatpak": true,
			},
			want: []Candidate{
				{Manager: "brew", PkgName: "pkg1"},
				{Manager: "brew", PkgName: "pkg2"},
				{Manager: "apt", PkgName: "pkg2"}, // different manager, should be kept
				{Manager: "apt", PkgName: "pkg3"}, // deduplicated within apt
			},
		},
		{
			name:  "brew unavailable",
			query: "test",
			available: map[string]bool{
				"brew":    false,
				"apt":     true,
				"flatpak": true,
			},
			want: []Candidate{
				{Manager: "apt", PkgName: "pkg2"},
				{Manager: "apt", PkgName: "pkg3"},
			},
		},
		{
			name:  "search error",
			query: "error", // triggers error in brew mock
			available: map[string]bool{
				"brew": true,
				"apt":  true,
			},
			want: []Candidate{
				{Manager: "apt", PkgName: "pkg2"},
				{Manager: "apt", PkgName: "pkg3"},
			},
		},
		{
			name:  "no available adapters",
			query: "test",
			available: map[string]bool{
				"brew": false,
				"apt":  false,
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
