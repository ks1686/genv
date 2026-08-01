package adapter

import (
	"slices"
	"testing"
)

func TestNpm_Search_Parseable(t *testing.T) {
	installFakeBinary(t, "npm", `if [ "$1" != "search" ] || [ "$2" != "--parseable" ] || [ "$3" != "lodash" ]; then
  echo "unexpected args: $*" >&2
  exit 1
fi
printf 'lodash\tdesc\tdate\tver\tkeywords\n'
printf '@types/lodash\tdesc\tdate\tver\t\n'`)

	got, err := Npm{}.Search("lodash")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"lodash", "@types/lodash"}
	if !slices.Equal(got, want) {
		t.Fatalf("Search() = %v, want %v", got, want)
	}
}
