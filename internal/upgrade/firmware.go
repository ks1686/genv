package upgrade

// PlanFirmware returns firmware update commands when a clean vendor tool exists.
//
// Linux (arch, ubuntu, wsl-arch, linux): sudo fwupdmgr update when fwupdmgr is
// on PATH. macOS firmware is delivered by softwareupdate in the system step
// (`--include-config-data` lists config data, not a separate firmware tool).
// Windows firmware is vendor-specific (Lenovo Vantage, Dell Command, etc.)
// and is skipped rather than faked.
func PlanFirmware(env Env) CommandPlan {
	switch env.Target {
	case "macos":
		return skipPlan("macOS firmware is installed by the system step (softwareupdate -i -a)")
	case "windows":
		return skipPlan("Windows firmware updates are vendor-specific; no built-in tool without extra software")
	case "arch", "ubuntu", "wsl-arch", "linux":
		if !env.has("fwupdmgr") {
			return skipPlan("fwupdmgr not found")
		}
		return cmdPlan([]string{"sudo", "fwupdmgr", "update"})
	case "":
		return skipPlan("no genv target for firmware updates")
	default:
		return skipPlan("no firmware updater for target " + env.Target)
	}
}
