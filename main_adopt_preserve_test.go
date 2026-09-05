package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ks1686/genv/internal/genvfile"
)

// specWithEveryEmptyBlock is a schemaVersion 8 document that keeps every
// optional empty object (and nested empty maps/arrays) plus a non-struct key
// order. adopt/disown must leave all of that untouched.
const specWithEveryEmptyBlock = `{
  "updates": {},
  "defaults": {
    "env": {},
    "shell": {
      "aliases": {},
      "functions": {}
    },
    "services": {},
    "files": {
      "links": [],
      "templates": [],
      "dirs": []
    },
    "hooks": {
      "preApply": [],
      "postApply": [],
      "preAdd": [],
      "postAdd": [],
      "preRemove": [],
      "postRemove": [],
      "preUpgrade": [],
      "postUpgrade": []
    }
  },
  "targets": {
    "windows": {
      "env": {},
      "shell": {},
      "services": {},
      "files": {},
      "hooks": {}
    },
    "macos": {
      "packages": [
        {
          "id": "git"
        }
      ],
      "env": {},
      "shell": {},
      "services": {},
      "files": {},
      "hooks": {}
    }
  },
  "schemaVersion": "8"
}
`

const specWithEveryEmptyBlockTwoPkgs = `{
  "updates": {},
  "defaults": {
    "env": {},
    "shell": {
      "aliases": {},
      "functions": {}
    },
    "services": {},
    "files": {
      "links": [],
      "templates": [],
      "dirs": []
    },
    "hooks": {
      "preApply": [],
      "postApply": [],
      "preAdd": [],
      "postAdd": [],
      "preRemove": [],
      "postRemove": [],
      "preUpgrade": [],
      "postUpgrade": []
    }
  },
  "targets": {
    "windows": {
      "env": {},
      "shell": {},
      "services": {},
      "files": {},
      "hooks": {}
    },
    "macos": {
      "packages": [
        {
          "id": "git"
        },
        {
          "id": "neovim"
        }
      ],
      "env": {},
      "shell": {},
      "services": {},
      "files": {},
      "hooks": {}
    }
  },
  "schemaVersion": "8"
}
`

func TestAdoptCmd_PreservesEmptyBlocksAndKeyOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	withInstalledBrew(t)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	writeTestFile(t, path, specWithEveryEmptyBlock)
	writeLock(t, lockPath, nil)

	code := run([]string{"adopt", "--file", path, "--lock-file", lockPath, "--target", "macos", "bash"})
	if code != exitOK {
		t.Fatalf("adopt: expected exitOK (%d), got %d", exitOK, code)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := strings.Replace(specWithEveryEmptyBlock, `{
          "id": "git"
        }`, `{
          "id": "git"
        },
        {
          "id": "bash"
        }`, 1)
	if string(got) != want {
		t.Fatalf("adopt rewrote more than the new package entry\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestDisownCmd_PreservesEmptyBlocksAndKeyOrder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "genv.json")
	lockPath := filepath.Join(dir, "genv.lock.json")
	writeTestFile(t, path, specWithEveryEmptyBlockTwoPkgs)
	writeLock(t, lockPath, []genvfile.LockedPackage{
		{ID: "git", Manager: "brew", PkgName: "git"},
		{ID: "neovim", Manager: "brew", PkgName: "neovim"},
	})

	code := run([]string{"disown", "--file", path, "--lock-file", lockPath, "--target", "macos", "git"})
	if code != exitOK {
		t.Fatalf("disown: expected exitOK (%d), got %d", exitOK, code)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := strings.Replace(specWithEveryEmptyBlock, `"id": "git"`, `"id": "neovim"`, 1)
	if string(got) != want {
		t.Fatalf("disown rewrote more than the removed package entry\nwant:\n%s\ngot:\n%s", want, got)
	}
}
