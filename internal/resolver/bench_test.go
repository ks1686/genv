package resolver

import (
	"context"
	"io"
	"testing"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/schema"
)

func BenchmarkExecuteApply(b *testing.B) {
	// Create a large number of packages to install to simulate O(N) lookup
	var toInstall []Action
	for i := 0; i < 1000; i++ {
		toInstall = append(toInstall, Action{
			Pkg:     schema.Package{ID: "pkg"},
			Manager: "brew", // Existing adapter
			PkgName: "pkg",
			Cmd:     []string{"true"}, // Fast command
		})
	}

	result := ReconcileResult{
		ToInstall: toInstall,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExecuteApply(context.Background(), result, nil, io.Discard, io.Discard)
	}
}

func BenchmarkAdapterLookup(b *testing.B) {
	b.Run("Uncached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = adapter.ByName("linuxbrew")
		}
	})

	adapters := make(map[string]adapter.Adapter)
	getAdapter := func(name string) adapter.Adapter {
		if mgr, ok := adapters[name]; ok {
			return mgr
		}
		mgr := adapter.ByName(name)
		adapters[name] = mgr
		return mgr
	}

	b.Run("Cached", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = getAdapter("linuxbrew")
		}
	})
}
