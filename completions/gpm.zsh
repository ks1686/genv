#compdef gpm

_gpm() {
	local -a commands
	commands=(
		'add:Add a package to the spec and install it now'
		'remove:Remove a package from the spec and uninstall it now'
		'rm:Remove a package from the spec and uninstall it now'
		'adopt:Track an already-installed package in gpm.json without reinstalling'
		'disown:Stop tracking a package in gpm.json without uninstalling it'
		'list:List all packages installed by gpm'
		'ls:List all packages installed by gpm'
		'apply:Reconcile system state with gpm.json'
		'scan:Discover all installed packages and bulk-adopt them into gpm.json'
		'status:Show diff between gpm.json, the lock file, and recorded versions'
		'clean:Clear the cache of all detected package managers'
		'edit:Open gpm.json in $EDITOR'
		'version:Show gpm build version information'
		'help:Show this help text'
	)

	_arguments -C \
		'--file=[Path to gpm.json]:path:_files' \
		'1: :->cmds' \
		'*::arg:->args'

	case $state in
	cmds)
		_describe -t commands 'gpm command' commands
		;;
	args)
		case $line[1] in
		add | adopt)
			_arguments \
				'--file=[Path to gpm.json]:path:_files' \
				'--version=[Version constraint]:version:' \
				'--prefer=[Preferred manager]:manager:' \
				'--manager=[Manager-specific names]:manager:'
			;;
		apply)
			_arguments \
				'--file=[Path to gpm.json]:path:_files' \
				'--dry-run[Print the reconcile plan without executing]' \
				'--strict[Exit with an error if any package cannot be resolved]' \
				'--yes[Skip the confirmation prompt]' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--timeout=[Per-subprocess timeout]:timeout:' \
				'--debug[Emit debug-level structured logs to stderr]'
			;;
		status | scan)
			_arguments \
				'--file=[Path to gpm.json]:path:_files' \
				'--json[Emit machine-readable JSON to stdout]' \
				'--debug[Emit debug-level structured logs to stderr]'
			;;
		clean)
			_arguments \
				'--file=[Path to gpm.json]:path:_files' \
				'--dry-run[Print the clean commands without executing]'
			;;
		esac
		;;
	esac
}

_gpm "$@"
