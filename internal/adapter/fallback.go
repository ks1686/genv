package adapter

// DefaultFallbackEligible is an optional marker implemented only by the
// OS/system package managers that are safe to use as a blind default when a
// package declares no `prefer` hint and no `managers` mapping. Ecosystem,
// language, and plugin managers (npm, cargo, pipx, krew, vscode, ...) do NOT
// implement it, so `genv add git` never silently resolves to, say, npm just
// because npm happens to be the only tool installed.
//
// The marker carries no data: implementing it is the opt-in. The default is
// "not eligible", which is the safe direction — a newly added ecosystem
// adapter that forgets everything cannot become a blind fallback.
type DefaultFallbackEligible interface {
	DefaultFallbackEligible()
}

// IsDefaultFallbackEligible reports whether a is eligible to be used as a blind
// default-fallback manager during resolution (resolver step 3). Explicit
// selection via `prefer` or the `managers` map bypasses this gate entirely.
func IsDefaultFallbackEligible(a Adapter) bool {
	_, ok := a.(DefaultFallbackEligible)
	return ok
}

// Only OS/system package managers opt in. These match the historical
// default-fallback set before ecosystem adapters were added.
func (Brew) DefaultFallbackEligible()      {}
func (Mas) DefaultFallbackEligible()       {}
func (Pacman) DefaultFallbackEligible()    {}
func (Paru) DefaultFallbackEligible()      {}
func (Yay) DefaultFallbackEligible()       {}
func (Snap) DefaultFallbackEligible()      {}
func (Apt) DefaultFallbackEligible()       {}
func (Dnf) DefaultFallbackEligible()       {}
func (Apk) DefaultFallbackEligible()       {}
func (Linuxbrew) DefaultFallbackEligible() {}
func (Winget) DefaultFallbackEligible()    {}
func (Scoop) DefaultFallbackEligible()     {}
func (Choco) DefaultFallbackEligible()     {}
