# fish completion for genv

function __fish_genv_no_subcommand
    for i in (commandline -opc)
        if contains -- $i add remove rm adopt disown list ls apply scan status clean edit completion validate upgrade init version help
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

# Helper: extract the value of --file from the current command line tokens,
# then pass it through to __complete packages so it reads the right spec.
function __fish_genv_file_arg
    set -l tokens (commandline -opc)
    for i in (seq 1 (count $tokens))
        if test $tokens[$i] = --file
            and test (math $i + 1) -le (count $tokens)
            echo --file $tokens[(math $i + 1)]
            return
        end
    end
end

# Dynamic completions from the binary.
function __fish_genv_packages
    genv __complete packages (__fish_genv_file_arg) 2>/dev/null
end

function __fish_genv_managers
    genv __complete managers 2>/dev/null
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
complete -c genv -n __fish_genv_no_subcommand -f -a completion -d 'Print shell completion script'
complete -c genv -n __fish_genv_no_subcommand -f -a validate -d 'Validate genv.json against the schema'
complete -c genv -n __fish_genv_no_subcommand -f -a upgrade -d 'Upgrade all tracked packages to their latest versions'
complete -c genv -n __fish_genv_no_subcommand -f -a init -d 'Create a new genv.json interactively'
complete -c genv -n __fish_genv_no_subcommand -f -a version -d 'Show genv build version information'
complete -c genv -n __fish_genv_no_subcommand -f -a help -d 'Show this help text'

# Common flags
complete -c genv -l file -d 'Path to genv.json' -r

# remove / rm / disown — complete positional arg with tracked package IDs
complete -c genv -n '__fish_genv_using_command remove; or __fish_genv_using_command rm; or __fish_genv_using_command disown' \
    -f -a '(__fish_genv_packages)' -d 'Tracked package'

# add / adopt
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l version -d 'Version constraint' -x
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' \
    -l prefer -d 'Preferred manager' -x -a '(__fish_genv_managers)'
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l manager -d 'Manager-specific names' -x
complete -c genv -n '__fish_genv_using_command add' -l no-search -d 'Skip interactive package search'

# upgrade — complete positional arg with tracked package IDs
complete -c genv -n '__fish_genv_using_command upgrade' \
    -f -a '(__fish_genv_packages)' -d 'Tracked package'
complete -c genv -n '__fish_genv_using_command upgrade' -l dry-run -d 'Print the upgrade commands without executing'
complete -c genv -n '__fish_genv_using_command upgrade' -l yes -d 'Skip the confirmation prompt'
complete -c genv -n '__fish_genv_using_command upgrade' -l debug -d 'Emit debug-level structured logs to stderr'

# apply
complete -c genv -n '__fish_genv_using_command apply' -l dry-run -d 'Print the reconcile plan without executing'
complete -c genv -n '__fish_genv_using_command apply' -l strict -d 'Exit with an error if any package cannot be resolved'
complete -c genv -n '__fish_genv_using_command apply' -l yes -d 'Skip the confirmation prompt'
complete -c genv -n '__fish_genv_using_command apply' -l quiet -d 'Suppress plan output'
complete -c genv -n '__fish_genv_using_command apply' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_using_command apply' -l timeout -d 'Per-subprocess timeout' -x
complete -c genv -n '__fish_genv_using_command apply' -l debug -d 'Emit debug-level structured logs to stderr'

# status / scan
complete -c genv -n '__fish_genv_using_command status; or __fish_genv_using_command scan' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_using_command status; or __fish_genv_using_command scan' -l debug -d 'Emit debug-level structured logs to stderr'

# clean
complete -c genv -n '__fish_genv_using_command clean' -l dry-run -d 'Print the clean commands without executing'

# completion
complete -c genv -n '__fish_genv_using_command completion' -f -a 'bash zsh fish' -d 'Shell type'
