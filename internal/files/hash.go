package files

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// contentHashPrefix versions the lock digest so a future algorithm can coexist
// with sha256 without a lock schema bump.
const contentHashPrefix = "sha256:"

// HashBytes returns a versioned SHA-256 digest of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return contentHashPrefix + hex.EncodeToString(sum[:])
}

// HashFile returns a versioned SHA-256 digest of a regular file. Directories
// and other non-regular nodes return "" so callers can skip content tracking.
func HashFile(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return HashBytes(data), nil
}

// HashLinkSource hashes the resolved source of a files.links entry.
func HashLinkSource(sourceRoot, source string) (string, error) {
	path, err := resolveSource(sourceRoot, source)
	if err != nil {
		return "", err
	}
	return HashFile(path)
}

// HashTemplate hashes the rendered template body (what apply writes).
func HashTemplate(sourceRoot, source, hostName string) (string, error) {
	path, err := resolveSource(sourceRoot, source)
	if err != nil {
		return "", err
	}
	rendered, err := renderedTemplate(path, hostName)
	if err != nil {
		return "", err
	}
	return HashBytes(rendered), nil
}

// ExpandTarget resolves ~ and $VARS then cleans, matching status/apply targets.
func ExpandTarget(target string) (string, error) {
	expanded, err := expandPath(target)
	if err != nil {
		return "", err
	}
	return filepath.Clean(expanded), nil
}

func applyContentDrift(entry StatusEntry, hashes map[string]string, current func() (string, error)) StatusEntry {
	if entry.Kind != "ok" || len(hashes) == 0 {
		return entry
	}
	want := hashes[entry.Target]
	if want == "" {
		return entry
	}
	got, err := current()
	if err != nil || got == "" {
		return entry
	}
	if got != want {
		entry.Kind = "drifted"
	}
	return entry
}

// HashableLinkMode reports whether a files.links mode stores a content hash.
// merge-dir trees are excluded (per-file lock records are out of scope).
func HashableLinkMode(mode string) bool {
	return mode == "" || mode == "link" || mode == "managed-link"
}
