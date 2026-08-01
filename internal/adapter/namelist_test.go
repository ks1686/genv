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

func TestBrew_ListNames_formulaeAndCasks(t *testing.T) {
	installFakeBinary(t, "brew", `#!/bin/sh
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
