package schema

import "testing"

func TestValidRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "https", url: "https://github.com/example/dotfiles.git"},
		{name: "ssh scheme", url: "ssh://git@github.com/example/dotfiles.git"},
		{name: "scp git@", url: "git@github.com:example/dotfiles.git"},
		{name: "file scheme", url: "file:///tmp/dotfiles.git"},
		{name: "tilde path", url: "~/terminal-config"},
		{name: "windows drive", url: `C:\Users\me\dotfiles.git`},
		{name: "empty", url: "", wantErr: true},
		{name: "leading dash", url: "-u", wantErr: true},
		{name: "ext helper", url: "ext::sh -c evil", wantErr: true},
		{name: "http", url: "http://example.com/repo.git", wantErr: true},
		{name: "relative", url: "local", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidRepoURL(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidGitRef(t *testing.T) {
	if err := ValidGitRef("main"); err != nil {
		t.Fatalf("main: %v", err)
	}
	if err := ValidGitRef("-evil"); err == nil {
		t.Fatal("expected error for leading dash")
	}
	if err := ValidGitRef("feat branch"); err == nil {
		t.Fatal("expected error for whitespace")
	}
}
