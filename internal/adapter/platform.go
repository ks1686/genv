package adapter

// AutomaticOnGOOS reports whether manager is eligible for automatic candidate
// generation on goos. Explicit manager selection does not use this policy.
func AutomaticOnGOOS(manager, goos string) bool {
	switch manager {
	case "brew":
		return goos == "darwin"
	case "linuxbrew":
		return goos == "linux"
	default:
		return true
	}
}
