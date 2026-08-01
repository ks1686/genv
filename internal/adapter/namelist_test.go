package adapter

import "testing"

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]int, len(want))
	for _, s := range want {
		seen[s]++
	}
	for _, s := range got {
		if seen[s] == 0 {
			return false
		}
		seen[s]--
	}
	return true
}

func TestPacman_ListNames(t *testing.T) {
	installFakeBinary(t, "pacman", `#!/bin/sh
[ "$1" = "-Slq" ] || exit 1
echo "ripgrep"
echo "git"
`)
	names, err := Pacman{}.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if !sameStringSet(names, []string{"git", "ripgrep"}) {
		t.Fatalf("got %v", names)
	}
}

func TestParu_ListNames(t *testing.T) {
	installFakeBinary(t, "paru", `#!/bin/sh
[ "$1" = "-Slq" ] || exit 1
echo "ripgrep"
echo "git"
`)
	names, err := Paru{}.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if !sameStringSet(names, []string{"git", "ripgrep"}) {
		t.Fatalf("got %v", names)
	}
}

func TestYay_ListNames(t *testing.T) {
	installFakeBinary(t, "yay", `#!/bin/sh
[ "$1" = "-Slq" ] || exit 1
echo "ripgrep"
echo "git"
`)
	names, err := Yay{}.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if !sameStringSet(names, []string{"git", "ripgrep"}) {
		t.Fatalf("got %v", names)
	}
}

func TestBrew_ListNames_formulaeAndCasks(t *testing.T) {
	installFakeBinary(t, "brew", `#!/bin/sh
[ "$HOMEBREW_COMPLETION" = "1" ] || exit 1
case "$1" in
  formulae) echo "openjdk"; echo "wget" ;;
  casks) echo "docker"; echo "openjdk" ;;
  *) exit 1 ;;
esac
`)
	names, err := Brew{}.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	want := []string{"docker", "openjdk", "wget"}
	if !sameStringSet(names, want) {
		t.Fatalf("ListNames = %v, want %v", names, want)
	}
}
