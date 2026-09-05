# PowerShell argument completer for genv
# Install: genv completion install powershell
# Or: . (genv completion powershell | Out-String)

$script:GenvCommands = @(
	'add', 'remove', 'rm', 'adopt', 'disown', 'list', 'ls', 'apply', 'edit',
	'clean', 'scan', 'status', 'completion', 'validate', 'upgrade', 'updates',
	'migrate', 'export', 'map', 'pull', 'init', 'env', 'shell', 'service', 'profile', 'version', 'help'
)

function script:Get-GenvCompletions {
	param(
		[string]$WordToComplete,
		[string]$CommandAst,
		[int]$CursorPosition
	)

	$tokens = @()
	if ($null -ne $CommandAst) {
		$tokens = $CommandAst.CommandElements | ForEach-Object { $_.Extent.Text }
	}
	# Drop the leading 'genv' token when present.
	if ($tokens.Count -gt 0 -and $tokens[0] -match '(?i)genv(\.exe)?$') {
		$tokens = $tokens[1..($tokens.Count - 1)]
	}

	$cmd = $null
	foreach ($t in $tokens) {
		if ($script:GenvCommands -contains $t) {
			$cmd = $t
			break
		}
	}

	if (-not $cmd) {
		return $script:GenvCommands |
			Where-Object { $_ -like "$WordToComplete*" } |
			ForEach-Object {
				[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
			}
	}

	$completeCandidates = {
		param(
			[string[]]$Candidates,
			[string]$ResultType = 'ParameterName'
		)
		$Candidates |
			Where-Object { $_ -like "$WordToComplete*" } |
			ForEach-Object {
				[System.Management.Automation.CompletionResult]::new($_, $_, $ResultType, $_)
			}
	}
	$completePackages = {
		try {
			$pkgs = & genv __complete packages 2>$null
			if ($pkgs) {
				($pkgs -split '\s+') |
					Where-Object { $_ -and ($_ -like "$WordToComplete*") } |
					ForEach-Object {
						[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
					}
			}
		} catch {}
	}
	$completeRepoPackages = {
		try {
			$pkgs = & genv __complete repo-packages $WordToComplete 2>$null
			if ($pkgs) {
				($pkgs -split '\s+') |
					Where-Object { $_ -and ($_ -like "$WordToComplete*") } |
					ForEach-Object {
						[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
					}
			}
		} catch {}
	}
	$previous = $null
	if ($tokens.Count -ge 2) {
		$last = $tokens[-1]
		if ($WordToComplete -and ($last -eq $WordToComplete -or $last -like "$WordToComplete*")) {
			$previous = $tokens[-2]
		} else {
			$previous = $last
		}
	} elseif ($tokens.Count -eq 1) {
		$previous = $tokens[0]
	}
	$cmdIndex = [array]::IndexOf($tokens, $cmd)
	$after = @()
	if ($cmdIndex -ge 0 -and ($cmdIndex + 1) -lt $tokens.Count) {
		$after = $tokens[($cmdIndex + 1)..($tokens.Count - 1)]
	}

	switch ($cmd) {
		{ $_ -in 'apply' } {
			$flags = @('--file', '--lock-file', '--state-dir', '--dry-run', '--force', '--backup', '--strict', '--yes',
				'--quiet', '--json', '--timeout', '--no-hooks', '--skip-packages', '--hook-timeout', '--debug',
				'--host', '--target', '--force-new-lock')
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'migrate' } {
			$flags = @('--file', '--write')
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'export' } {
			$flags = @('--file', '--target', '--out', '--strict', '--from-v7')
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'map' } {
			$flags = @('--file', '--target')
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'add' } {
			if ($WordToComplete -like '-*') {
				$flags = @('--file', '--lock-file', '--version', '--prefer', '--manager',
					'--no-search', '--no-hooks', '--hook-timeout', '--host', '--target')
				return (& $completeCandidates -Candidates $flags)
			}
			if ($previous -eq '--prefer') {
				try {
					$managers = & genv __complete managers 2>$null
					if ($managers) {
						return ($managers -split '\s+') |
							Where-Object { $_ -and ($_ -like "$WordToComplete*") } |
							ForEach-Object {
								[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
							}
					}
				} catch {}
			}
			return (& $completeRepoPackages)
		}
		{ $_ -in 'adopt' } {
			if ($WordToComplete -like '-*') {
				$flags = @('--file', '--lock-file', '--state-dir', '--version', '--prefer', '--manager',
					'--host', '--target', '--files', '--json')
				return (& $completeCandidates -Candidates $flags)
			}
			if ($previous -eq '--prefer') {
				try {
					$managers = & genv __complete managers 2>$null
					if ($managers) {
						return ($managers -split '\s+') |
							Where-Object { $_ -and ($_ -like "$WordToComplete*") } |
							ForEach-Object {
								[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
							}
					}
				} catch {}
			}
			return (& $completeRepoPackages)
		}
		{ $_ -in 'remove', 'rm' } {
			if ($WordToComplete -like '-*') {
				$flags = @('--file', '--lock-file', '--no-hooks', '--hook-timeout', '--host', '--target')
				return (& $completeCandidates -Candidates $flags)
			}
			return (& $completePackages)
		}
		{ $_ -in 'disown' } {
			if ($WordToComplete -like '-*') {
				$flags = @('--file', '--lock-file', '--target')
				return (& $completeCandidates -Candidates $flags)
			}
			return (& $completePackages)
		}
		{ $_ -in 'upgrade' } {
			if ($WordToComplete -like '-*') {
				$flags = @('--file', '--lock-file', '--dry-run', '--yes', '--no-hooks', '--json', '--only', '--skip', '--only-manager', '--skip-manager', '--hook-timeout', '--debug', '--host', '--target')
				return (& $completeCandidates -Candidates $flags)
			}
			return (& $completePackages)
		}
		{ $_ -in 'env' } {
			$envSubs = @('set', 'unset', 'list', 'ls')
			$envSub = $after | Where-Object { $envSubs -contains $_ } | Select-Object -First 1
			if (-not $envSub -and $WordToComplete -notlike '-*') {
				return (& $completeCandidates -Candidates $envSubs -ResultType 'ParameterValue')
			}
			switch ($envSub) {
				'set' { $flags = @('--file', '--sensitive', '--target') }
				'unset' { $flags = @('--file', '--target') }
				{ $_ -in 'list', 'ls' } { $flags = @('--file', '--json') }
				default { $flags = @('--file') }
			}
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'shell' } {
			$shellSubs = @('alias', 'status', 'edit')
			$shellSub = $after | Where-Object { $shellSubs -contains $_ } | Select-Object -First 1
			if (-not $shellSub -and $WordToComplete -notlike '-*') {
				return (& $completeCandidates -Candidates $shellSubs -ResultType 'ParameterValue')
			}
			if ($shellSub -eq 'alias') {
				$aliasSubs = @('set', 'unset')
				$aliasSub = $after | Where-Object { $aliasSubs -contains $_ } | Select-Object -First 1
				if (-not $aliasSub -and $WordToComplete -notlike '-*') {
					return (& $completeCandidates -Candidates $aliasSubs -ResultType 'ParameterValue')
				}
				switch ($aliasSub) {
					'set' { $flags = @('--file', '--shell', '--target') }
					'unset' { $flags = @('--file', '--target') }
					default { $flags = @('--file') }
				}
				return (& $completeCandidates -Candidates $flags)
			}
			switch ($shellSub) {
				'status' { $flags = @('--file', '--json') }
				'edit' { $flags = @('--file') }
				default { $flags = @('--file') }
			}
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'service' } {
			$serviceSubs = @('add', 'remove', 'rm', 'list', 'ls', 'start', 'stop', 'status')
			$serviceSub = $after | Where-Object { $serviceSubs -contains $_ } | Select-Object -First 1
			if (-not $serviceSub -and $WordToComplete -notlike '-*') {
				return (& $completeCandidates -Candidates $serviceSubs -ResultType 'ParameterValue')
			}
			switch ($serviceSub) {
				'add' { $flags = @('--file', '--start', '--stop', '--restart', '--status', '--brew-formula', '--target') }
				{ $_ -in 'remove', 'rm' } { $flags = @('--file', '--target') }
				default { $flags = @('--file') }
			}
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'scan' } {
			$flags = @('--file', '--lock-file', '--dry-run', '--yes', '--all', '--deps', '--json', '--debug', '--target')
			return (& $completeCandidates -Candidates $flags)
		}
		{ $_ -in 'completion' } {
			$shells = @('bash', 'zsh', 'fish', 'powershell', 'install')
			return (& $completeCandidates -Candidates $shells -ResultType 'ParameterValue')
		}
	}
}

Register-ArgumentCompleter -Native -CommandName genv -ScriptBlock {
	param($wordToComplete, $commandAst, $cursorPosition)
	Get-GenvCompletions -WordToComplete $wordToComplete -CommandAst $commandAst -CursorPosition $cursorPosition
}
