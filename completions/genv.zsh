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
		'scan:Discover all installed packages and bulk-adopt them into genv.json'
		'status:Show diff between genv.json, the lock file, and recorded versions'
		'clean:Clear the cache of all detected package managers'
		"edit:Open genv.json in \$EDITOR"
		'completion:Print shell completion script'
		'validate:Validate genv.json against the schema'
		'upgrade:Upgrade all tracked packages to their latest versions'
		'init:Create a new genv.json interactively'
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
				'1: :->pkgid'
			if [[ $state == pkgid ]]; then
				local -a pkgs
				# shellcheck disable=SC2086
				pkgs=(${(f)"$(genv __complete packages ${file_arg} 2>/dev/null)"})
				_describe -t packages 'tracked package' pkgs
			fi
			;;
		add | adopt)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--version=[Version constraint]:version:' \
				"--prefer=[Preferred manager]:manager:($(genv __complete managers 2>/dev/null))" \
				'--manager=[Manager-specific names]:manager:' \
				'--no-search[Skip interactive package search]'
			;;
		upgrade)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--dry-run[Print the upgrade commands without executing]' \
				'--yes[Skip the confirmation prompt]' \
				'--debug[Emit debug-level structured logs to stderr]' \
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
				'--dry-run[Print the reconcile plan without executing]' \
				'--strict[Exit with an error if any package cannot be resolved]' \
				'--yes[Skip the confirmation prompt]' \
				'--quiet[Suppress plan output]' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--timeout=[Per-subprocess timeout]:timeout:' \
				'--debug[Emit debug-level structured logs to stderr]'
			;;
		status | scan)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--debug[Emit debug-level structured logs to stderr]'
			;;
		clean)
			_arguments \
				'--file=[Path to genv.json]:path:_files' \
				'--dry-run[Print the clean commands without executing]'
			;;
		completion)
			_values 'shell' bash zsh fish
			;;
		esac
		;;
	esac
}

_genv "$@"
