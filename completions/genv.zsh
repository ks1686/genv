#compdef genv

_genv() {
	local state line
	# shellcheck disable=SC2034
	local -a commands
	commands=(
		'add:Add a package to the spec and install it now'
		'remove:Remove a package from the spec and uninstall it now'
		'rm:Remove a package from the spec and uninstall it now'
		'adopt:Track an already-installed package in genv.json without reinstalling'
		'disown:Stop tracking a package in genv.json without uninstalling it'
		'list:List all packages installed by genv'
		'ls:List all packages installed by genv'
		'apply:Reconcile system state with genv.json'
		"edit:Open genv.json in \$EDITOR"
		'clean:Clear the cache of all detected package managers'
		'scan:Discover all installed packages and bulk-adopt them into genv.json'
		'status:Show diff between genv.json, the lock file, and recorded versions'
		'completion:Print shell completion script'
		'validate:Validate genv.json against the schema'
		'upgrade:Upgrade all tracked packages to their latest versions'
		'updates:Check available updates for genv-tracked packages'
		'migrate:Convert legacy host predicates to schemaVersion 8 targets'
		'export:Build a single-target portable snapshot and report'
		'map:Print assist-only manager mapping suggestions for a target'
		'pull:Fetch the spec from a git repository and update genv.json'
		'init:Create a new genv.json interactively'
		'env:Manage shell environment variables'
		'shell:Manage shell aliases and config'
		'service:Manage background services'
		'profile:Manage named environment profiles'
		'version:Show genv build version information'
		'help:Show this help text'
	)

	_arguments -C \
		'--file=[Path to genv.json]:path:_files' \
		'1: :->cmds' \
		'*::arg:->args'

	case $state in
	cmds)
		_describe -t commands 'genv command' commands
		;;
	args)
		# Extract --file value from the current command line so __complete
		# reads the right spec when a custom --file path was given.
		local file_arg=""
		local -i idx
		for idx in {1..${#words[@]}}; do
			if [[ "${words[idx]}" == "--file" && -n "${words[idx+1]}" ]]; then
				file_arg="--file ${words[idx+1]}"
				break
			fi
		done

		case ${line[1]} in
		remove | rm)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--no-hooks[Skip pre-remove and post-remove hooks]' \
				'--hook-timeout=[Per-hook timeout]:timeout:' \
				'--host=[Host name for host-specific records]:host:' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:' \
				'1: :->pkgid'
			if [[ $state == pkgid ]]; then
				local -a pkgs
				pkgs=(${(f)"$(genv __complete packages ${file_arg} 2>/dev/null)"})
				_describe -t packages 'tracked package' pkgs
			fi
			;;
		disown)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:' \
				'1: :->pkgid'
			if [[ $state == pkgid ]]; then
				local -a pkgs
				# shellcheck disable=SC2086
				pkgs=(${(f)"$(genv __complete packages ${file_arg} 2>/dev/null)"})
				_describe -t packages 'tracked package' pkgs
			fi
			;;
		add)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--version=[Version constraint]:version:' \
				"--prefer=[Preferred manager]:manager:($(genv __complete managers 2>/dev/null))" \
				'--manager=[Manager-specific names]:manager:' \
				'--no-search[Skip interactive package search]' \
				'--no-hooks[Skip pre-add and post-add hooks]' \
				'--hook-timeout=[Per-hook timeout]:timeout:' \
				'--host=[Host name for host-specific records]:host:' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:' \
				'1: :->pkgid'
			if [[ $state == pkgid ]]; then
				local -a pkgs
				pkgs=(${(f)"$(genv __complete repo-packages ${words[CURRENT]} 2>/dev/null)"})
				_describe -t packages 'package' pkgs
			fi
			;;
		adopt)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--version=[Version constraint]:version:' \
				"--prefer=[Preferred manager]:manager:($(genv __complete managers 2>/dev/null))" \
				'--manager=[Manager-specific names]:manager:' \
				'--host=[Host name for host-specific records]:host:' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:' \
				'--files[Adopt matching files block entries into the lock without changing targets]' \
				'--json[Emit machine-readable JSON to stdout]' \
				'1: :->pkgid'
			if [[ $state == pkgid ]]; then
				local -a pkgs
				pkgs=(${(f)"$(genv __complete repo-packages ${words[CURRENT]} 2>/dev/null)"})
				_describe -t packages 'package' pkgs
			fi
			;;
		upgrade)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--dry-run[Print the upgrade commands without executing]' \
				'--yes[Skip the confirmation prompt]' \
				'--all[Upgrade every unconstrained tracked package]' \
				'--no-hooks[Skip pre-upgrade and post-upgrade hooks]' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--only=[Package IDs or names to upgrade]:packages:' \
				'--skip=[Package IDs or names to skip]:packages:' \
				'--only-manager=[Managers to upgrade]:managers:' \
				'--skip-manager=[Managers to skip]:managers:' \
				'--hook-timeout=[Per-hook timeout]:timeout:' \
				'--debug[Emit debug-level structured logs to stderr]' \
				'--host=[Host name for host-specific records]:host:' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:' \
				'1: :->pkgid'
			if [[ $state == pkgid ]]; then
				local -a pkgs
				# shellcheck disable=SC2086
				pkgs=(${(f)"$(genv __complete packages ${file_arg} 2>/dev/null)"})
				_describe -t packages 'tracked package' pkgs
			fi
			;;
		updates)
			local -a updates_cmds
			updates_cmds=(
				'check:Plan available updates for genv-tracked packages only'
				'start:Register the managed background updates checker'
				'stop:Stop and unregister the managed background updates checker'
				'status:Show managed background updates checker status'
			)
			_arguments \
				'1: :->updatescmd' \
				'*::arg:->updatesarg'
			case $state in
			updatescmd)
				_describe -t commands 'updates subcommand' updates_cmds
				;;
			updatesarg)
				case ${line[1]} in
				check)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--lock-file=[Path to genv lock file]:path:_files' \
						'--json[Emit machine-readable JSON to stdout]' \
						'--only=[Package IDs or names to check]:packages:' \
						'--skip=[Package IDs or names to skip]:packages:' \
						'--only-manager=[Managers to check]:managers:' \
						'--skip-manager=[Managers to skip]:managers:' \
						'--host=[Host name for host-specific records]:host:' \
						'--target=[Portable target id for schemaVersion 8 specs]:target:'
					;;
				start)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--lock-file=[Path to genv lock file]:path:_files' \
						'--host=[Host name for host-specific records]:host:' \
						'--target=[Portable target id for schemaVersion 8 specs]:target:'
					;;
				esac
				;;
			esac
			;;
		apply)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--dry-run[Print the reconcile plan without executing]' \
				'--force[Overwrite mismatched managed files]' \
				'--backup[Back up mismatched files before overwrite]' \
				'--strict[Exit with an error if any package cannot be resolved]' \
				'--yes[Skip the confirmation prompt]' \
				'--quiet[Suppress plan output]' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--timeout=[Per-subprocess timeout]:timeout:' \
				'--no-hooks[Skip pre-apply and post-apply hooks]' \
				'--hook-timeout=[Per-hook timeout]:timeout:' \
				'--debug[Emit debug-level structured logs to stderr]' \
				'--host=[Host name for host-specific records]:host:' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:' \
				'--force-new-lock[Back up a foreign lock and start a new local lock]'
			;;
		migrate)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--write[Overwrite genv.json with the migrated schemaVersion 8 spec]'
			;;
		export)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--target=[Target id to export]:target:' \
				'--out=[Directory to write genv.json and report.json]:path:_files -/' \
				'--strict[Exit nonzero if the report contains errors]' \
				'--from-v7[Migrate v1-v7 input to schemaVersion 8 in memory first]'
			;;
		map)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--target=[Destination target id]:target:'
			;;
		status)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--debug[Emit debug-level structured logs to stderr]' \
				'--files[Check files block against the live filesystem only]' \
				'--host=[Host name for host-specific records]:host:' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:'
			;;
		scan)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--dry-run[List packages that would be adopted without writing]' \
				'--yes[Skip the confirmation prompt]' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--debug[Emit debug-level structured logs to stderr]' \
				'--target=[Portable target id for schemaVersion 8 specs]:target:'
			;;
		clean)
			_arguments \
				'--dry-run[Print the clean commands without executing]'
			;;
		pull)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--url=[Override the repository URL]:url:' \
				'--ref=[Override the repository ref]:ref:' \
				'--dry-run[Print what would be pulled without writing]'
			;;
		env)
			local -a env_cmds
			env_cmds=(
				'set:Add or update a variable in the spec'
				'unset:Remove a variable from the spec'
				'list:Show all declared variables'
				'ls:Show all declared variables'
			)
			_arguments \
				'1: :->envcmd' \
				'*::arg:->envarg'
			case $state in
			envcmd)
				_describe -t commands 'env subcommand' env_cmds
				;;
			envarg)
				case ${line[1]} in
				set)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--sensitive[Mark value as sensitive (redacted in output and logs)]' \
						'--target=[Portable target id for schemaVersion 8 specs]:target:'
					;;
				unset)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--target=[Portable target id for schemaVersion 8 specs]:target:'
					;;
				list | ls)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--json[Emit machine-readable JSON to stdout]'
					;;
				esac
				;;
			esac
			;;
		shell)
			local -a shell_cmds
			shell_cmds=(
				'alias:Add, update, or remove a shell alias'
				'status:Show shell config drift'
				"edit:Open genv.json in \$EDITOR"
			)
			_arguments \
				'1: :->shellcmd' \
				'*::arg:->shellarg'
			case $state in
			shellcmd)
				_describe -t commands 'shell subcommand' shell_cmds
				;;
			shellarg)
				case ${line[1]} in
				alias)
					local -a alias_cmds
					alias_cmds=(
						'set:Add or update an alias'
						'unset:Remove an alias'
					)
					_arguments \
						'1: :->aliascmd' \
						'*::arg:->aliasarg'
					case $state in
					aliascmd)
						_describe -t commands 'alias subcommand' alias_cmds
						;;
					aliasarg)
						case ${line[1]} in
						set)
							_arguments \
								'--file=[Path to genv.json]:path:_files' \
								'--shell=[Target shell]:shell:(bash zsh fish)' \
								'--target=[Portable target id for schemaVersion 8 specs]:target:'
							;;
						unset)
							_arguments \
								'--file=[Path to genv.json]:path:_files' \
								'--target=[Portable target id for schemaVersion 8 specs]:target:'
							;;
						esac
						;;
					esac
					;;
				status)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--json[Emit machine-readable JSON to stdout]'
					;;
				edit)
					_arguments \
						'--file=[Path to genv.json]:path:_files'
					;;
				esac
				;;
			esac
			;;
		profile)
			local -a profile_cmds
			profile_cmds=(
				'list:List available profiles and mark the active one'
				'ls:List available profiles and mark the active one'
				'create:Scaffold a new profile file'
				'switch:Switch to a named profile and reconcile the system'
			)
			_arguments \
				'1: :->profilecmd' \
				'*::arg:->profilearg'
			case $state in
			profilecmd)
				_describe -t commands 'profile subcommand' profile_cmds
				;;
			profilearg)
				case ${line[1]} in
				list | ls)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--lock-file=[Path to genv lock file]:path:_files'
					;;
				create)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'1:profile name:'
					;;
				switch)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--lock-file=[Path to genv lock file]:path:_files' \
						'--dry-run[Print the reconcile plan without executing]' \
						'--force[Overwrite mismatched managed files]' \
						'--backup[Back up mismatched files before overwrite]' \
						'--strict[Exit with an error if any package cannot be resolved]' \
						'--yes[Skip the confirmation prompt]' \
						'--quiet[Suppress plan output]' \
						'--json[Emit machine-readable JSON to stdout]' \
						'--timeout=[Per-subprocess timeout]:duration:' \
						'--debug[Emit debug-level structured logs to stderr]' \
						'--host=[Host name for host-specific records]:host:' \
						'1:profile name:'
					;;
				esac
				;;
			esac
			;;
		service)
			local -a service_cmds
			service_cmds=(
				'add:Add or update a service'
				'remove:Remove a service from the spec'
				'rm:Remove a service from the spec'
				'list:Show all declared services'
				'ls:Show all declared services'
				'start:Start a service'
				'stop:Stop a service'
				'status:Show service running status'
			)
			_arguments \
				'1: :->servicecmd' \
				'*::arg:->servicearg'
			case $state in
			servicecmd)
				_describe -t commands 'service subcommand' service_cmds
				;;
			servicearg)
				case ${line[1]} in
				add)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--start=[Command to start the service]:command:' \
						'--stop=[Command to stop the service]:command:' \
						'--restart=[Command to restart the service]:command:' \
						'--status=[Command to check service status]:command:' \
						'--brew-formula=[Homebrew formula to manage via brew services (macOS only)]:formula:' \
						'--target=[Portable target id for schemaVersion 8 specs]:target:'
					;;
				remove | rm)
					_arguments \
						'--file=[Path to genv.json]:path:_files' \
						'--target=[Portable target id for schemaVersion 8 specs]:target:'
					;;
				*)
					_arguments \
						'--file=[Path to genv.json]:path:_files'
					;;
				esac
				;;
			esac
			;;
		completion)
			_arguments \
				'1: :->caction' \
				'*::arg:->carg'
			case $state in
			caction)
				_values 'action' bash zsh fish install
				;;
			carg)
				case ${line[1]} in
				install)
					_arguments \
						'--dir=[Target directory (overrides the per-shell default)]:dir:_files -/' \
						'1: :->cshell'
					if [[ $state == cshell ]]; then
						_values 'shell' bash zsh fish
					fi
					;;
				esac
				;;
			esac
			;;
		esac
		;;
	esac
}

_genv "$@"
