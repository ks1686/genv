package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
	"time"

	"github.com/ks1686/genv/internal/genvfile"
)

// LockMutation must be held across every lock read-modify-write, and the lock
// re-read inside it.
//
// The interleaving mirrors the reported failure: the scheduled worker takes the
// mutation lock, reads a snapshot, and writes that snapshot back at the end.
// An append landing in that window is erased by the worker's stale write, and
// the next apply reinstalls the package. Holding the lock makes the append
// wait and then re-read the worker's result instead.
//
// The worker waits on appendDone but cannot require it: with the fix in place
// the append is correctly blocked until the worker releases, so the wait times
// out. Without the fix the append completes immediately and the worker's stale
// write erases it.
func TestAppendLockEntry_MergesWithConcurrentWriter(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "genv.lock.json")
	if err := genvfile.WriteLock(lockPath, &genvfile.LockFile{Packages: []genvfile.LockedPackage{
		{ID: "doomed", Manager: "brew", PkgName: "doomed"},
	}}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	readDone := make(chan struct{})
	appendDone := make(chan struct{})
	workerFinished := make(chan error, 1)

	go func() {
		unlock, err := genvfile.LockMutation(lockPath)
		if err != nil {
			workerFinished <- err
			return
		}
		defer unlock()
		snapshot, err := genvfile.ReadLock(lockPath)
		if err != nil {
			workerFinished <- err
			return
		}
		close(readDone)
		select {
		case <-appendDone:
		case <-time.After(300 * time.Millisecond):
		}
		snapshot.Packages = filterOut(snapshot.Packages, "doomed")
		workerFinished <- genvfile.WriteLock(lockPath, snapshot)
	}()

	<-readDone // the worker now holds the lock and has its stale snapshot

	if code := appendLockEntry(lockPath, genvfile.LockedPackage{ID: "fresh", Manager: "brew", PkgName: "fresh"}, "macos"); code != exitOK {
		t.Fatalf("appendLockEntry code = %d", code)
	}
	close(appendDone)
	if err := <-workerFinished; err != nil {
		t.Fatalf("worker: %v", err)
	}

	lf, err := genvfile.ReadLock(lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	var ids []string
	for _, p := range lf.Packages {
		ids = append(ids, p.ID)
	}
	if len(ids) != 1 || ids[0] != "fresh" {
		t.Fatalf("lock ids = %v, want [fresh] (the worker's removal kept, the append not erased)", ids)
	}
}

// TestEveryProductionLockWriteHasMutationGuard is a source-level regression
// guard for #97. The runtime interleaving test above proves the critical
// append path; this one makes a newly added main-package WriteLock fail review
// unless its enclosing function acquires LockMutation, or is a helper whose
// every caller in main*.go does. It checks production files only, since tests
// seed locks directly.
func TestLockWriteGuardRejectsWriteBeforeLock(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture
func bad() {
	_ = genvfile.WriteLock(path, lf)
	unlock, _ := genvfile.LockMutation(path)
	defer unlock()
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	lockPos := firstGenvfileCall(fn.Body, "LockMutation")
	writePos := firstGenvfileCall(fn.Body, "WriteLock")
	if !lockPos.IsValid() || !writePos.IsValid() || lockPos < writePos {
		t.Fatalf("fixture not constructed with write-before-lock order: lock=%v write=%v", lockPos, writePos)
	}
}

func TestLockWriteGuardRejectsConditionalLock(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture
func bad(cond bool) {
	if cond {
		unlock, _ := genvfile.LockMutation(path)
		defer unlock()
	}
	_ = genvfile.WriteLock(path, lf)
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	for _, stmt := range fn.Body.List {
		if isTopLevelLockCall(stmt) {
			t.Fatal("conditional lock must not be treated as a top-level guard")
		}
	}
}

func TestEveryProductionLockWriteHasMutationGuard(t *testing.T) {
	paths, err := filepath.Glob("main*.go")
	if err != nil {
		t.Fatal(err)
	}
	type fnInfo struct {
		path          string
		locks         bool
		writes        bool
		firstLock     token.Pos
		firstWrite    token.Pos
		topLevelLock  bool
		callees       map[string]bool
		callPositions map[string][]token.Pos
	}
	funcs := map[string]*fnInfo{}
	for _, path := range paths {
		if hasTestSuffix(path) {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			info := &fnInfo{
				path:          path,
				locks:         callsGenvfile(fn.Body, "LockMutation"),
				writes:        callsGenvfile(fn.Body, "WriteLock"),
				callees:       map[string]bool{},
				callPositions: map[string][]token.Pos{},
			}
			info.firstLock, info.firstWrite = firstGenvfileCall(fn.Body, "LockMutation"), firstGenvfileCall(fn.Body, "WriteLock")
			for _, stmt := range fn.Body.List {
				if isTopLevelLockCall(stmt) {
					info.topLevelLock = true
					break
				}
			}
			// These helpers execute a write under a caller-held lock instead of
			// acquiring another flock (which would deadlock). runApply acquires
			// before dispatching to runApplyJSON/runApplyText; mutateLock holds
			// it unless alreadyHeld is true, which its restart-phase callers
			// inherit from the enclosing locked apply/worker operation.
			if fn.Name.Name == "writeLockAfterApply" || fn.Name.Name == "runApplyJSON" || fn.Name.Name == "runApplyText" || fn.Name.Name == "mutateLock" {
				info.locks = true
				info.topLevelLock = true
				info.firstLock = fn.Pos()
			}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok {
						info.callees[id.Name] = true
						info.callPositions[id.Name] = append(info.callPositions[id.Name], call.Pos())
					}
				}
				return true
			})
			funcs[fn.Name.Name] = info
		}
	}
	// A function is guarded when it takes the mutation lock itself, or when it
	// has callers and every one of them is guarded (transitively). Helpers that
	// write the lock on behalf of a locked caller are therefore fine; an
	// unlocked writer or an orphan helper is not.
	guarded := map[string]bool{}
	var isGuarded func(name string, visiting map[string]bool) bool
	isGuarded = func(name string, visiting map[string]bool) bool {
		if v, ok := guarded[name]; ok {
			return v
		}
		info := funcs[name]
		if info == nil {
			return false
		}
		if info.locks && info.topLevelLock && info.firstLock.IsValid() && info.firstWrite.IsValid() && info.firstLock < info.firstWrite {
			guarded[name] = true
			return true
		}
		if visiting[name] {
			return true // recursion adds no unguarded path of its own
		}
		visiting[name] = true
		callers := 0
		ok := true
		for callerName, caller := range funcs {
			if caller.callees[name] && callerName != name {
				callers++
				protectedCall := !caller.locks
				if caller.locks {
					protectedCall = caller.firstLock.IsValid()
					for _, callPos := range caller.callPositions[name] {
						if callPos <= caller.firstLock {
							protectedCall = false
						}
					}
				}
				if !protectedCall || !isGuarded(callerName, visiting) {
					ok = false
				}
			}
		}
		delete(visiting, name)
		ok = ok && callers > 0
		guarded[name] = ok
		return ok
	}
	for name, info := range funcs {
		if info.writes && !isGuarded(name, map[string]bool{}) {
			t.Errorf("%s: function %s writes the lock but is not (transitively) protected by genvfile.LockMutation", info.path, name)
		}
	}
}

func hasTestSuffix(path string) bool {
	base := filepath.Base(path)
	return len(base) >= len("_test.go") && base[len(base)-len("_test.go"):] == "_test.go"
}

func containsGenvfileCall(node ast.Node, method string) bool {
	return firstGenvfileCall(node, method).IsValid()
}

// isTopLevelLockCall accepts a lock acquisition in a simple statement in the
// function body, but rejects calls nested in branches, loops, closures, etc.
func isTopLevelLockCall(stmt ast.Stmt) bool {
	switch stmt.(type) {
	case *ast.AssignStmt, *ast.ExprStmt, *ast.DeferStmt:
		return containsGenvfileCall(stmt, "LockMutation")
	default:
		return false
	}
}

func firstGenvfileCall(node ast.Node, method string) token.Pos {
	var first token.Pos
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != method {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if ok && ident.Name == "genvfile" && (!first.IsValid() || call.Pos() < first) {
			first = call.Pos()
		}
		return true
	})
	return first
}

func callsGenvfile(node ast.Node, method string) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != method {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if ok && ident.Name == "genvfile" {
			found = true
			return false
		}
		return true
	})
	return found
}

func filterOut(pkgs []genvfile.LockedPackage, id string) []genvfile.LockedPackage {
	out := pkgs[:0:0]
	for _, p := range pkgs {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}
