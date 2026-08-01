package adapter

import (
	"slices"
	"testing"
)

func TestBun_Search_UsesNpmRegistry(t *testing.T) {
	installFakeBinary(t, "npm", `if [ "$1" != "search" ] || [ "$2" != "--parseable" ] || [ "$3" != "typescript" ]; then
  echo "unexpected args: $*" >&2
  exit 1
fi
printf 'typescript\tdesc\tdate\tver\tkeywords\n'
printf '@types/typescript\tdesc\tdate\tver\t\n'`)

	got, err := Bun{}.Search("typescript")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"typescript", "@types/typescript"}
	if !slices.Equal(got, want) {
		t.Fatalf("Search() = %v, want %v", got, want)
	}
}

func TestBun_Search_MissingNpmReturnsNoResults(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	got, err := Bun{}.Search("typescript")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("Search() = %v, want nil", got)
	}
}
