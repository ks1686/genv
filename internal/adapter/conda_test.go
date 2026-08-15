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
	wantErr := []string{"sh", "-c", "printf '%s\n' 'genv: conda/mamba requires env-qualified format <env>:<pkg>' >&2; exit 1", "genv-conda-invalid", "mypkg"}
	if !reflect.DeepEqual(cmdErr, wantErr) {
		t.Errorf("got %v, want %v", cmdErr, wantErr)
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
	wantErr := []string{"sh", "-c", "printf '%s\n' 'genv: conda/mamba requires env-qualified format <env>:<pkg>' >&2; exit 1", "genv-conda-invalid", "mypkg"}
	if !reflect.DeepEqual(cmdErr, wantErr) {
		t.Errorf("got %v, want %v", cmdErr, wantErr)
	}
}

func TestCondaInvalidPlans(t *testing.T) {
	want := condaInvalidCommand("mypkg")
	if !reflect.DeepEqual(Conda{}.PlanUninstall("mypkg"), want) {
		t.Fatalf("PlanUninstall invalid = %v", Conda{}.PlanUninstall("mypkg"))
	}
	if !reflect.DeepEqual(Conda{}.PlanUpgrade("mypkg"), want) {
		t.Fatalf("PlanUpgrade invalid = %v", Conda{}.PlanUpgrade("mypkg"))
	}
	if !reflect.DeepEqual(Mamba{}.PlanUninstall("mypkg"), want) {
		t.Fatalf("Mamba PlanUninstall invalid = %v", Mamba{}.PlanUninstall("mypkg"))
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
