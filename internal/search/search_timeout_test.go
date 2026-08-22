package search

import (
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/resolver"
)

// TestAll_HangingSearchDoesNotBlockOthers: one stalled registry query must not
// hang the whole search — the stalled backend is dropped after the deadline
// while healthy backends still contribute results.
func TestAll_HangingSearchDoesNotBlockOthers(t *testing.T) {
	origTimeout := resolver.DefaultLiveListTimeout
	resolver.DefaultLiveListTimeout = 100 * time.Millisecond
	t.Cleanup(func() { resolver.DefaultLiveListTimeout = origTimeout })

	hanging := mockSearchableAdapter{
		mockAdapter: mockAdapter{name: "hangingmgr"},
		searchFunc: func(string) ([]string, error) {
			time.Sleep(time.Hour)
			return nil, nil
		},
	}
	healthy := mockSearchableAdapter{
		mockAdapter: mockAdapter{name: "healthymgr"},
		searchFunc: func(string) ([]string, error) {
			return []string{"jq"}, nil
		},
	}

	originalAll := adapter.All
	t.Cleanup(func() { adapter.All = originalAll })
	adapter.All = []adapter.Adapter{hanging, healthy}
	available := map[string]bool{"hangingmgr": true, "healthymgr": true}

	started := time.Now()
	candidates := All("jq", available)
	elapsed := time.Since(started)

	if elapsed > 5*time.Second {
		t.Fatalf("search took %s; a hanging backend must not stall All()", elapsed)
	}
	for _, c := range candidates {
		if c.Manager == "hangingmgr" {
			t.Fatalf("hanging backend contributed results: %v", candidates)
		}
	}
	found := false
	for _, c := range candidates {
		if c.Manager == "healthymgr" && c.PkgName == "jq" {
			found = true
		}
	}
	if !found {
		t.Fatalf("healthy backend results missing from %v", candidates)
	}
}
