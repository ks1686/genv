# fish completion for genv

function __fish_genv_no_subcommand
    for i in (commandline -opc)
        if contains -- $i add remove rm adopt disown list ls apply scan status clean edit version help
            return 1
        end
    end
    return 0
end

function __fish_genv_using_command
    set -l cmd (commandline -opc)
    if [ (count $cmd) -gt 1 ]
        if [ $argv[1] = $cmd[2] ]
            return 0
        end
    end
    return 1
end

# Commands
complete -c genv -n __fish_genv_no_subcommand -f -a add -d 'Add a package to the spec and install it now'
complete -c genv -n __fish_genv_no_subcommand -f -a 'remove rm' -d 'Remove a package from the spec and uninstall it now'
complete -c genv -n __fish_genv_no_subcommand -f -a adopt -d 'Track an already-installed package in genv.json without reinstalling'
complete -c genv -n __fish_genv_no_subcommand -f -a disown -d 'Stop tracking a package in genv.json without uninstalling it'
complete -c genv -n __fish_genv_no_subcommand -f -a 'list ls' -d 'List all packages installed by genv'
complete -c genv -n __fish_genv_no_subcommand -f -a apply -d 'Reconcile system state with genv.json'
complete -c genv -n __fish_genv_no_subcommand -f -a scan -d 'Discover all installed packages and bulk-adopt them into genv.json'
complete -c genv -n __fish_genv_no_subcommand -f -a status -d 'Show diff between genv.json, the lock file, and recorded versions'
complete -c genv -n __fish_genv_no_subcommand -f -a clean -d 'Clear the cache of all detected package managers'
complete -c genv -n __fish_genv_no_subcommand -f -a edit -d 'Open genv.json in $EDITOR'
complete -c genv -n __fish_genv_no_subcommand -f -a version -d 'Show genv build version information'
complete -c genv -n __fish_genv_no_subcommand -f -a help -d 'Show this help text'

# Common flags
complete -c genv -l file -d 'Path to genv.json' -r

# Command specific flags
# add / adopt
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l version -d 'Version constraint' -x
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l prefer -d 'Preferred manager' -x
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l manager -d 'Manager-specific names' -x

# apply
complete -c genv -n '__fish_genv_using_command apply' -l dry-run -d 'Print the reconcile plan without executing'
complete -c genv -n '__fish_genv_using_command apply' -l strict -d 'Exit with an error if any package cannot be resolved'
complete -c genv -n '__fish_genv_using_command apply' -l yes -d 'Skip the confirmation prompt'
complete -c genv -n '__fish_genv_using_command apply' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_using_command apply' -l timeout -d 'Per-subprocess timeout' -x
complete -c genv -n '__fish_genv_using_command apply' -l debug -d 'Emit debug-level structured logs to stderr'

# status / scan
complete -c genv -n '__fish_genv_using_command status; or __fish_genv_using_command scan' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_using_command status; or __fish_genv_using_command scan' -l debug -d 'Emit debug-level structured logs to stderr'

# clean
complete -c genv -n '__fish_genv_using_command clean' -l dry-run -d 'Print the clean commands without executing'
