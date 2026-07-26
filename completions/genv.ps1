# PowerShell argument completer for genv
# Install: genv completion install powershell
# Or: . (genv completion powershell | Out-String)

$script:GenvCommands = @(
	'add', 'remove', 'rm', 'adopt', 'disown', 'list', 'ls', 'apply', 'edit',
	'clean', 'scan', 'status', 'completion', 'validate', 'upgrade', 'updates',
	'pull', 'init', 'env', 'shell', 'service', 'profile', 'version', 'help'
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

	switch ($cmd) {
		{ $_ -in 'apply' } {
			$flags = @('--file', '--lock-file', '--dry-run', '--force', '--strict', '--yes',
				'--quiet', '--json', '--timeout', '--no-hooks', '--hook-timeout', '--debug',
				'--host', '--target', '--force-new-lock')
			return $flags |
				Where-Object { $_ -like "$WordToComplete*" } |
				ForEach-Object {
					[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_)
				}
		}
		{ $_ -in 'remove', 'rm', 'disown', 'upgrade' } {
			try {
				$pkgs = & genv __complete packages 2>$null
				if ($pkgs) {
					return ($pkgs -split '\s+') |
						Where-Object { $_ -and ($_ -like "$WordToComplete*") } |
						ForEach-Object {
							[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
						}
				}
			} catch {}
		}
		{ $_ -in 'completion' } {
			$shells = @('bash', 'zsh', 'fish', 'powershell', 'install')
			return $shells |
				Where-Object { $_ -like "$WordToComplete*" } |
				ForEach-Object {
					[System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
				}
		}
	}
}

Register-ArgumentCompleter -Native -CommandName genv -ScriptBlock {
	param($wordToComplete, $commandAst, $cursorPosition)
	Get-GenvCompletions -WordToComplete $wordToComplete -CommandAst $commandAst -CursorPosition $cursorPosition
}
