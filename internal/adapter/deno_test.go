package adapter

import (
	"slices"
	"strings"
	"testing"
)

func TestDeno_NormalizeID_whenManagerOverrideIsURL(t *testing.T) {
	name, explicit := Deno{}.NormalizeID("serve", map[string]string{"deno": "https://deno.land/std/http/file_server.ts"})

	if !explicit || name != "serve=https://deno.land/std/http/file_server.ts" {
		t.Errorf("NormalizeID = (%q, %v), want command=url true", name, explicit)
	}
}

func TestDeno_PlanCommands_whenSpecHasCommandAndURL(t *testing.T) {
	a := Deno{}
	spec := "serve=https://deno.land/std/http/file_server.ts"

	if got, want := a.PlanInstall(spec), []string{"deno", "install", "--global", "--name", "serve", "https://deno.land/std/http/file_server.ts"}; !slices.Equal(got, want) {
		t.Errorf("PlanInstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUninstall(spec), []string{"deno", "uninstall", "--global", "serve"}; !slices.Equal(got, want) {
		t.Errorf("PlanUninstall = %v, want %v", got, want)
	}
	if got, want := a.PlanUpgrade(spec), []string{"deno", "install", "--global", "--name", "serve", "https://deno.land/std/http/file_server.ts"}; !slices.Equal(got, want) {
		t.Errorf("PlanUpgrade = %v, want %v", got, want)
	}
	assertNoBroadJSUpdate(t, a.PlanUpgrade(spec))
}

func TestDeno_PlanInstall_whenSpecIsMissingURLFailsActionably(t *testing.T) {
	got := Deno{}.PlanInstall("serve")

	if !isFailingDenoCommand(got) {
		t.Fatalf("PlanInstall missing URL = %v, want failing shell command", got)
	}
	if !strings.Contains(got[2], "manager override") {
		t.Errorf("PlanInstall missing URL message = %q, want actionable manager override hint", got[2])
	}
}

func TestDeno_PlanInstall_whenSpecURLIsInvalidFailsActionably(t *testing.T) {
	got := Deno{}.PlanInstall("serve=not-a-url")

	if !isFailingDenoCommand(got) {
		t.Fatalf("PlanInstall invalid URL = %v, want failing shell command", got)
	}
	if slices.Contains(got, "deno") {
		t.Errorf("PlanInstall invalid URL = %v, must not run deno install", got)
	}
}

func TestDeno_Query_whenSpecIsInvalidReturnsFalse(t *testing.T) {
	ok, err := Deno{}.Query("https://deno.land/std/http/file_server.ts")

	if err != nil {
		t.Fatalf("Deno.Query invalid spec: %v", err)
	}
	if ok {
		t.Error("Deno.Query invalid spec = true, want false")
	}
}

func TestDeno_ListInstalled_whenDenoOutputsGlobalInstallList(t *testing.T) {
	installFakeBinary(t, "deno", `if [ "$1" = "install" ] && [ "$2" = "--global" ] && [ "$3" = "--list" ]; then
  echo 'serve https://deno.land/std/http/file_server.ts'
  echo 'deploy jsr:@deno/deployctl'
fi`)

	names, err := Deno{}.ListInstalled()

	if err != nil {
		t.Fatalf("Deno.ListInstalled: %v", err)
	}
	if want := []string{"serve", "deploy"}; !slices.Equal(names, want) {
		t.Errorf("ListInstalled = %v, want %v", names, want)
	}
}

func isFailingDenoCommand(args []string) bool {
	return len(args) >= 4 && args[0] == "sh" && args[1] == "-c" && strings.Contains(args[2], "exit 1")
}
