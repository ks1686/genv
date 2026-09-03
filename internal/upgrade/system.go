package upgrade

// PlanSystem returns OS vendor update commands for genv targets.
//
//	macos:    sudo softwareupdate -i -a
//	windows:  Windows Update Agent COM via pwsh (else powershell / powershell.exe)
//	arch / wsl-arch: sudo pacman -Syu --noconfirm
//	ubuntu:   sudo apt-get update && sudo apt-get upgrade -y (apt if apt-get is absent)
//
// paru/yay remain tracked-package adapters; they are not the OS vendor updater.
// snap stays tracked-only — genv never refreshes untracked snaps here.
// sudo/elevation is part of the planned argv; missing sudo is an apply failure,
// not a silent skip.
func PlanSystem(env Env) CommandPlan {
	switch env.Target {
	case "macos":
		if !env.has("softwareupdate") {
			return skipPlan("softwareupdate not found")
		}
		return cmdPlan([]string{"sudo", "softwareupdate", "-i", "-a"})
	case "windows":
		bin := env.look("pwsh")
		if bin == "" {
			bin = env.look("powershell")
		}
		if bin == "" {
			bin = env.look("powershell.exe")
		}
		if bin == "" {
			return skipPlan("PowerShell not found (Windows Update Agent COM API needs pwsh or powershell)")
		}
		return cmdPlan([]string{bin, "-NoProfile", "-Command", windowsUpdateInstallScript})
	case "arch", "wsl-arch":
		if !env.has("pacman") {
			return skipPlan("pacman not found")
		}
		return cmdPlan([]string{"sudo", "pacman", "-Syu", "--noconfirm"})
	case "ubuntu":
		apt := ""
		if env.has("apt-get") {
			apt = "apt-get"
		} else if env.has("apt") {
			apt = "apt"
		}
		if apt == "" {
			return skipPlan("apt-get not found")
		}
		return cmdPlan(
			[]string{"sudo", apt, "update"},
			[]string{"sudo", apt, "upgrade", "-y"},
		)
	case "linux":
		return skipPlan("no OS vendor updater for target linux (use arch or ubuntu)")
	case "":
		return skipPlan("no genv target for OS vendor updates")
	default:
		return skipPlan("no OS vendor updater for target " + env.Target)
	}
}

// windowsUpdateInstallScript installs pending Software updates through the
// built-in Windows Update Agent COM API. It needs no extra modules (not
// PSWindowsUpdate, not winget — winget upgrades packages, not the OS).
const windowsUpdateInstallScript = `$ErrorActionPreference = 'Stop'
$session = New-Object -ComObject Microsoft.Update.Session
$searcher = $session.CreateUpdateSearcher()
$result = $searcher.Search('IsInstalled=0 and Type=''Software'' and IsHidden=0')
if ($result.Updates.Count -eq 0) { exit 0 }
$collection = New-Object -ComObject Microsoft.Update.UpdateColl
foreach ($update in $result.Updates) { [void]$collection.Add($update) }
$downloader = $session.CreateUpdateDownloader()
$downloader.Updates = $collection
[void]$downloader.Download()
$installer = $session.CreateUpdateInstaller()
$installer.Updates = $collection
$install = $installer.Install()
if ($install.RebootRequired) { Write-Output 'A reboot is required to finish Windows Update.' }
if ($install.ResultCode -ne 2) { exit [int]$install.ResultCode }
`
