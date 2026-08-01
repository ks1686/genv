package adapter

import (
	"slices"
	"testing"
)

func TestMas_Search_ReturnsMatchingProductIDs(t *testing.T) {
	installFakeBinary(t, "mas", `if [ "$1" != "search" ] || [ "$2" != "CODE" ]; then
  echo "unexpected args: $*" >&2
  exit 1
fi
echo "497799835  Xcode (16.0)"
echo "123  Other App (1.0)"
echo "456  Code Runner (2.0)"`)

	got, err := Mas{}.Search("CODE")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"497799835", "456"}
	if !slices.Equal(got, want) {
		t.Fatalf("Search() = %v, want %v", got, want)
	}
}

func TestMas_CompletionNames_ReturnsLowercaseSlugs(t *testing.T) {
	installFakeBinary(t, "mas", `if [ "$1" != "search" ] || [ "$2" != "cut" ]; then
  echo "unexpected args: $*" >&2
  exit 1
fi
echo "1631624924  Final Cut Pro (12.3)"
echo "497799835  Xcode (16.0)"`)

	got, err := Mas{}.CompletionNames("cut")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"final-cut-pro"}
	if !slices.Equal(got, want) {
		t.Fatalf("CompletionNames() = %v, want %v", got, want)
	}
}
