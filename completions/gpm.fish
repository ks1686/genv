# fish completion for gpm

function __fish_gpm_no_subcommand
    for i in (commandline -opc)
        if contains -- $i add remove rm adopt disown list ls apply scan status clean edit version help
            return 1
        end
    end
    return 0
end

function __fish_gpm_using_command
    set -l cmd (commandline -opc)
    if [ (count $cmd) -gt 1 ]
        if [ $argv[1] = $cmd[2] ]
            return 0
        end
    end
    return 1
end

# Commands
complete -c gpm -n __fish_gpm_no_subcommand -f -a add -d 'Add a package to the spec and install it now'
complete -c gpm -n __fish_gpm_no_subcommand -f -a 'remove rm' -d 'Remove a package from the spec and uninstall it now'
complete -c gpm -n __fish_gpm_no_subcommand -f -a adopt -d 'Track an already-installed package in gpm.json without reinstalling'
complete -c gpm -n __fish_gpm_no_subcommand -f -a disown -d 'Stop tracking a package in gpm.json without uninstalling it'
complete -c gpm -n __fish_gpm_no_subcommand -f -a 'list ls' -d 'List all packages installed by gpm'
complete -c gpm -n __fish_gpm_no_subcommand -f -a apply -d 'Reconcile system state with gpm.json'
complete -c gpm -n __fish_gpm_no_subcommand -f -a scan -d 'Discover all installed packages and bulk-adopt them into gpm.json'
complete -c gpm -n __fish_gpm_no_subcommand -f -a status -d 'Show diff between gpm.json, the lock file, and recorded versions'
complete -c gpm -n __fish_gpm_no_subcommand -f -a clean -d 'Clear the cache of all detected package managers'
complete -c gpm -n __fish_gpm_no_subcommand -f -a edit -d 'Open gpm.json in $EDITOR'
complete -c gpm -n __fish_gpm_no_subcommand -f -a version -d 'Show gpm build version information'
complete -c gpm -n __fish_gpm_no_subcommand -f -a help -d 'Show this help text'

# Common flags
complete -c gpm -l file -d 'Path to gpm.json' -r

# Command specific flags
# add / adopt
complete -c gpm -n '__fish_gpm_using_command add; or __fish_gpm_using_command adopt' -l version -d 'Version constraint' -x
complete -c gpm -n '__fish_gpm_using_command add; or __fish_gpm_using_command adopt' -l prefer -d 'Preferred manager' -x
complete -c gpm -n '__fish_gpm_using_command add; or __fish_gpm_using_command adopt' -l manager -d 'Manager-specific names' -x

# apply
complete -c gpm -n '__fish_gpm_using_command apply' -l dry-run -d 'Print the reconcile plan without executing'
complete -c gpm -n '__fish_gpm_using_command apply' -l strict -d 'Exit with an error if any package cannot be resolved'
complete -c gpm -n '__fish_gpm_using_command apply' -l yes -d 'Skip the confirmation prompt'
complete -c gpm -n '__fish_gpm_using_command apply' -l json -d 'Emit machine-readable JSON to stdout'
complete -c gpm -n '__fish_gpm_using_command apply' -l timeout -d 'Per-subprocess timeout' -x
complete -c gpm -n '__fish_gpm_using_command apply' -l debug -d 'Emit debug-level structured logs to stderr'

# status / scan
complete -c gpm -n '__fish_gpm_using_command status; or __fish_gpm_using_command scan' -l json -d 'Emit machine-readable JSON to stdout'
complete -c gpm -n '__fish_gpm_using_command status; or __fish_gpm_using_command scan' -l debug -d 'Emit debug-level structured logs to stderr'

# clean
complete -c gpm -n '__fish_gpm_using_command clean' -l dry-run -d 'Print the clean commands without executing'
