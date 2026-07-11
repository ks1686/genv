package adapter

import (
	"reflect"
	"testing"
)

func TestParseCondaEnvPkg(t *testing.T) {
	tests := []struct {
		spec    string
		wantEnv string
		wantPkg string
		wantErr bool
	}{
		{"env:myenv:mypkg", "myenv", "mypkg", false},
		{"myenv:mypkg", "myenv", "mypkg", false},
		{"mypkg", "", "", true},
		{"env:myenv:mypkg:extra", "myenv", "mypkg:extra", false},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			env, pkg, err := parseCondaEnvPkg(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCondaEnvPkg(%q) error = %v, wantErr %v", tt.spec, err, tt.wantErr)
				return
			}
			if env != tt.wantEnv {
				t.Errorf("parseCondaEnvPkg(%q) env = %q, want %q", tt.spec, env, tt.wantEnv)
			}
			if pkg != tt.wantPkg {
				t.Errorf("parseCondaEnvPkg(%q) pkg = %q, want %q", tt.spec, pkg, tt.wantPkg)
			}
		})
	}
}

func TestCondaPlanInstall(t *testing.T) {
	c := Conda{}
	cmd := c.PlanInstall("myenv:mypkg")
	expected := []string{"conda", "install", "-y", "-n", "myenv", "mypkg"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("got %v, want %v", cmd, expected)
	}

	cmdErr := c.PlanInstall("mypkg")
	if len(cmdErr) != 3 || cmdErr[0] != "sh" || cmdErr[1] != "-c" {
		t.Errorf("expected error command, got %v", cmdErr)
	}
}

func TestMambaPlanInstall(t *testing.T) {
	m := Mamba{}
	cmd := m.PlanInstall("myenv:mypkg")
	expected := []string{"mamba", "install", "-y", "-n", "myenv", "mypkg"}
	if !reflect.DeepEqual(cmd, expected) {
		t.Errorf("got %v, want %v", cmd, expected)
	}

	cmdErr := m.PlanInstall("mypkg")
	if len(cmdErr) != 3 || cmdErr[0] != "sh" || cmdErr[1] != "-c" {
		t.Errorf("expected error command, got %v", cmdErr)
	}
}

func TestParseCondaListJSON(t *testing.T) {
	data := []byte(`[
  {"name": "black", "version": "24.2.0"},
  {"name": "ruff", "version": "0.3.0"}
]`)
	entries, err := parseCondaListJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []pythonEntry{
		{"black", "24.2.0"},
		{"ruff", "0.3.0"},
	}
	if !reflect.DeepEqual(entries, expected) {
		t.Errorf("got %v, want %v", entries, expected)
	}
}
