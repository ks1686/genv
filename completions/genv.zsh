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
		'pull:Fetch the spec from a git repository and update genv.json'
		'init:Create a new genv.json interactively'
		'env:Manage shell environment variables'
		'shell:Manage shell aliases and config'
		'service:Manage background services'
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
		remove | rm | disown)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
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
				'--no-search[Skip interactive package search]'
			;;
		adopt)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--version=[Version constraint]:version:' \
				"--prefer=[Preferred manager]:manager:($(genv __complete managers 2>/dev/null))" \
				'--manager=[Manager-specific names]:manager:' \
				'--host=[Host name for host-specific records]:host:' \
				'--files[Adopt matching files block entries into the lock without changing targets]' \
				'--json[Emit machine-readable JSON to stdout]'
			;;
		upgrade)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--dry-run[Print the upgrade commands without executing]' \
				'--yes[Skip the confirmation prompt]' \
				'--debug[Emit debug-level structured logs to stderr]' \
				'--host=[Host name for host-specific records]:host:' \
				'1: :->pkgid'
			if [[ $state == pkgid ]]; then
				local -a pkgs
				# shellcheck disable=SC2086
				pkgs=(${(f)"$(genv __complete packages ${file_arg} 2>/dev/null)"})
				_describe -t packages 'tracked package' pkgs
			fi
			;;
		apply)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--dry-run[Print the reconcile plan without executing]' \
				'--force[Overwrite mismatched managed files]' \
				'--strict[Exit with an error if any package cannot be resolved]' \
				'--yes[Skip the confirmation prompt]' \
				'--quiet[Suppress plan output]' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--timeout=[Per-subprocess timeout]:timeout:' \
				'--debug[Emit debug-level structured logs to stderr]' \
				'--host=[Host name for host-specific records]:host:'
			;;
		status)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--debug[Emit debug-level structured logs to stderr]' \
				'--files[Check files block against the live filesystem only]' \
				'--host=[Host name for host-specific records]:host:'
			;;
		scan)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--lock-file=[Path to genv lock file]:path:_files' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--debug[Emit debug-level structured logs to stderr]'
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
						'--sensitive[Mark value as sensitive (redacted in output and logs)]'
					;;
				unset)
					_arguments \
						'--file=[Path to genv.json]:path:_files'
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
								'--shell=[Target shell]:shell:(bash zsh fish)'
							;;
						unset)
							_arguments \
								'--file=[Path to genv.json]:path:_files'
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
						'--brew-formula=[Homebrew formula to manage via brew services (macOS only)]:formula:'
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
			_values 'shell' bash zsh fish
			;;
		esac
		;;
	esac
}

_genv "$@"
