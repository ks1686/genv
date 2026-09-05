package genvfile

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLockMutation_CreatesMutexAndUnlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")

	unlock, err := LockMutation(path)
	if err != nil {
		t.Fatalf("LockMutation: %v", err)
	}
	if _, err := os.Stat(path + ".mutex"); err != nil {
		t.Fatalf("mutex file missing: %v", err)
	}
	unlock()
	if _, err := os.Stat(path + ".mutex"); !os.IsNotExist(err) {
		t.Fatalf("mutex leftover after unlock: stat err=%v", err)
	}
}

func TestLockMutation_EmptyPathIsNoop(t *testing.T) {
	unlock, err := LockMutation("")
	if err != nil {
		t.Fatalf("empty path: %v", err)
	}
	unlock()
}

func TestLockMutation_SerializesConcurrentHolders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genv.lock.json")

	var (
		mu    sync.Mutex
		inCS  int
		maxCS int
		wg    sync.WaitGroup
	)
	const n = 8
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			unlock, err := LockMutation(path)
			if err != nil {
				t.Errorf("LockMutation: %v", err)
				return
			}
			defer unlock()
			mu.Lock()
			inCS++
			if inCS > maxCS {
				maxCS = inCS
			}
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			inCS--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if maxCS != 1 {
		t.Fatalf("max concurrent holders = %d, want 1", maxCS)
	}
	if _, err := os.Stat(path + ".mutex"); !os.IsNotExist(err) {
		t.Fatalf("mutex leftover after concurrent unlocks: stat err=%v", err)
	}
}
