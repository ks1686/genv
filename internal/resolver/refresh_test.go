package resolver

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/adapter"
	"github.com/ks1686/genv/internal/genvfile"
	"github.com/ks1686/genv/internal/testutil"
)

type refreshTestMgr struct {
	outdatedTestMgr
	cmd []string
}

func (m *refreshTestMgr) PlanRefresh() []string { return m.cmd }

func swapIndexRefresh(t *testing.T, fn func(ctx context.Context, argv []string) error) {
	t.Helper()
	orig := runIndexRefresh
	runIndexRefresh = func(ctx context.Context, argv []string, _ io.Reader, _, _ io.Writer) error {
		return fn(ctx, argv)
	}
	t.Cleanup(func() { runIndexRefresh = orig })
}

func TestRefreshIndexes_runsOncePerManager(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew": &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "brew"}, cmd: []string{"brew", "update"}},
		"apt":  &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "apt"}, cmd: []string{"sudo", "apt-get", "update"}},
		"mas":  &outdatedTestMgr{name: "mas"},
	})
	var got []string
	swapIndexRefresh(t, func(_ context.Context, argv []string) error {
		got = append(got, strings.Join(argv, " "))
		return nil
	})

	actions, keepAll, warnings := RefreshIndexes([]genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "jq", Manager: "brew", PkgName: "jq"},
		{ID: "curl", Manager: "apt", PkgName: "curl"},
		{ID: "xcode", Manager: "mas", PkgName: "497799835"},
	}, RefreshOptions{})

	if !slices.Equal(got, []string{"brew update", "sudo apt-get update"}) {
		t.Fatalf("ran %v, want brew then apt once each", got)
	}
	if len(actions) != 2 || actions[0].Manager != "brew" || !slices.Equal(actions[0].Cmd, []string{"brew", "update"}) {
		t.Fatalf("actions = %#v, want brew then apt refresh lines", actions)
	}
	if len(keepAll) != 0 {
		t.Fatalf("keepAll = %v, want empty on success", keepAll)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "refresh timing: brew") || !strings.Contains(joined, "refresh timing: apt") {
		t.Fatalf("warnings = %v, want timing lines for brew and apt", warnings)
	}
}

func TestRefreshIndexes_dedupesSharedArgv(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew":      &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "brew"}, cmd: []string{"brew", "update"}},
		"linuxbrew": &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "linuxbrew"}, cmd: []string{"brew", "update"}},
	})
	var n int
	swapIndexRefresh(t, func(context.Context, []string) error {
		n++
		return nil
	})

	actions, keepAll, _ := RefreshIndexes([]genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "jq", Manager: "linuxbrew", PkgName: "jq"},
	}, RefreshOptions{})

	if n != 1 {
		t.Fatalf("refresh runs = %d, want 1 shared brew update", n)
	}
	if len(actions) != 1 || actions[0].Manager != "brew" {
		t.Fatalf("actions = %#v, want one brew line", actions)
	}
	if len(keepAll) != 0 {
		t.Fatalf("keepAll = %v, want empty", keepAll)
	}
}

func TestRefreshIndexes_errorKeepsAllManagersSharingArgv(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew":      &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "brew"}, cmd: []string{"brew", "update"}},
		"linuxbrew": &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "linuxbrew"}, cmd: []string{"brew", "update"}},
	})
	swapIndexRefresh(t, func(context.Context, []string) error {
		return errors.New("network down")
	})

	actions, keepAll, warnings := RefreshIndexes([]genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "jq", Manager: "linuxbrew", PkgName: "jq"},
	}, RefreshOptions{})

	if len(actions) != 1 {
		t.Fatalf("actions = %#v, still want the attempted refresh in the plan", actions)
	}
	if !keepAll["brew"] || !keepAll["linuxbrew"] {
		t.Fatalf("keepAll = %v, want brew and linuxbrew", keepAll)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "could not refresh brew") || !strings.Contains(warnings[0], "keeping all") {
		t.Fatalf("warnings = %v, want keep-all refresh warning", warnings)
	}
}

func TestRefreshIndexes_omitsManagersWithoutIndexRefresher(t *testing.T) {
	swapLookupAdapter(t, map[string]adapter.Adapter{
		"mas": &outdatedTestMgr{name: "mas"},
	})
	var n int
	swapIndexRefresh(t, func(context.Context, []string) error {
		n++
		return nil
	})

	actions, keepAll, warnings := RefreshIndexes([]genvfile.LockedPackage{
		{ID: "xcode", Manager: "mas", PkgName: "497799835"},
	}, RefreshOptions{})

	if n != 0 || len(actions) != 0 || len(keepAll) != 0 || len(warnings) != 0 {
		t.Fatalf("ran=%d actions=%v keepAll=%v warnings=%v, want no refresh", n, actions, keepAll, warnings)
	}
}

func TestRefreshIndexes_timeoutKeepsAll(t *testing.T) {
	orig := DefaultIndexRefreshTimeout
	DefaultIndexRefreshTimeout = 50 * time.Millisecond
	t.Cleanup(func() { DefaultIndexRefreshTimeout = orig })

	swapLookupAdapter(t, map[string]adapter.Adapter{
		"brew": &refreshTestMgr{outdatedTestMgr: outdatedTestMgr{name: "brew"}, cmd: []string{"brew", "update"}},
	})
	var started sync.WaitGroup
	started.Add(1)
	swapIndexRefresh(t, func(ctx context.Context, _ []string) error {
		started.Done()
		<-ctx.Done()
		return ctx.Err()
	})

	_, keepAll, warnings := RefreshIndexes([]genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
	}, RefreshOptions{})

	if !keepAll["brew"] {
		t.Fatalf("keepAll = %v, want brew on timeout", keepAll)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "keeping all") {
		t.Fatalf("warnings = %v, want keep-all", warnings)
	}
}

func TestDefaultRunIndexRefresh_runsArgv(t *testing.T) {
	testutil.InstallFakeBinary(t, "true-refresh", "exit 0")
	if err := defaultRunIndexRefresh(context.Background(), []string{"true-refresh"}, nil, io.Discard, io.Discard); err != nil {
		t.Fatalf("success: %v", err)
	}
	testutil.InstallFakeBinary(t, "false-refresh", "exit 1")
	if err := defaultRunIndexRefresh(context.Background(), []string{"false-refresh"}, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("expected failure from exit 1")
	}
	if err := defaultRunIndexRefresh(context.Background(), nil, nil, nil, nil); err == nil {
		t.Fatal("expected empty command error")
	}
}
