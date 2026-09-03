package adapter

// Stack manages globally installed Haskell executables via `stack install`.
// Stack is primarily a project build tool; genv scopes this adapter to global
// package installs only (`stack install <pkg>`), never project builds or
// `stack upgrade` of Stack itself.
type Stack struct{}

func (Stack) Name() string { return "stack" }

func (Stack) Available() bool {
	_, err := lookPath("stack")
	return err == nil
}

func (Stack) NormalizeID(id string, managers map[string]string) (string, bool) {
	return normalizeID("stack", id, managers)
}

func (Stack) PlanInstall(pkgName string) []string {
	return []string{"stack", "install", pkgName}
}

func (Stack) PlanUninstall(pkgName string) []string {
	return stackUnsupportedUninstall(pkgName)
}

func (Stack) PlanUpgrade(pkgName string) []string {
	return []string{"stack", "install", pkgName}
}

func (Stack) PlanClean() [][]string { return nil }

func (Stack) Query(pkgName string) (bool, error) { return queryUnsupported("stack", pkgName) }

func (Stack) ListInstalled() ([]string, error) { return nil, nil }

func (Stack) QueryVersion(string) (string, error) { return "", nil }

// stackUnsupportedUninstall reports that Stack does not offer a safe global
// uninstall command. It returns a failing, non-mutating command instead of
// pretending to remove the executable, so genv never silently succeeds or
// deletes files outside a manager's control.
func stackUnsupportedUninstall(pkgName string) []string {
	return []string{"sh", "-c", "printf '%s\n' 'genv: stack has no safe global uninstall; remove the installed executable manually or disown it' >&2; exit 1", "genv-stack-uninstall", pkgName}
}
