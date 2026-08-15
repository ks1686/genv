package adapter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestNpm_Search_Parseable(t *testing.T) {
	installFakeBinary(t, "npm", `if [ "$1" != "search" ] || [ "$2" != "--parseable" ] || [ "$3" != "lodash" ]; then
  echo "unexpected args: $*" >&2
  exit 1
fi
printf 'lodash\tdesc\tdate\tver\tkeywords\n'
printf '@types/lodash\tdesc\tdate\tver\t\n'
printf 'unrelated\tdescription mentions lodash\tdate\tver\t\n'`)

	got, err := Npm{}.Search("lodash")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"lodash", "@types/lodash"}
	if !slices.Equal(got, want) {
		t.Fatalf("Search() = %v, want %v", got, want)
	}
}

func TestNpm_SearchContext_killsTimedOutCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cmd.exe .cmd shims do not reliably kill the bash child on deadline")
	}
	marker := filepath.Join(t.TempDir(), "completed")
	t.Setenv("NPM_COMPLETION_MARKER", marker)
	installFakeBinary(t, "npm", `sleep 1
echo completed > "$NPM_COMPLETION_MARKER"`)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := Npm{}.SearchContext(ctx, "lodash")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SearchContext() error = %v, want deadline exceeded", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("timed-out npm command completed: Stat() error = %v", err)
	}
}
