# fish completion for genv

function __fish_genv_no_subcommand
    for i in (commandline -opc)
        if contains -- $i add remove rm adopt disown list ls apply edit clean scan status completion validate upgrade updates migrate export map pull init env shell service profile version help
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

# True when 'genv <cmd>' is ready to complete a subcommand. Pass the command
# followed by its known subcommands so partial current tokens still complete.
function __fish_genv_at_subcommand
    set -l tokens (commandline -opc)
    if test (count $tokens) -ge 2
        and test $tokens[2] = $argv[1]
        if test (count $tokens) -eq 2
            return 0
        end
        if not contains -- $tokens[3] $argv[2..-1]
            return 0
        end
    end
    return 1
end

# True when 'genv <cmd> <sub>' is present (argv[1]=cmd, argv[2]=sub).
function __fish_genv_seen_sub
    set -l tokens (commandline -opc)
    if test (count $tokens) -ge 3
        and test $tokens[2] = $argv[1]
        and test $tokens[3] = $argv[2]
        return 0
    end
    return 1
end

# True when 'genv <cmd> <sub>' is ready for a sub-subcommand. Pass the command,
# subcommand, and then its known sub-subcommands.
function __fish_genv_at_subsubcommand
    set -l tokens (commandline -opc)
    if test (count $tokens) -ge 3
        and test $tokens[2] = $argv[1]
        and test $tokens[3] = $argv[2]
        if test (count $tokens) -eq 3
            return 0
        end
        if not contains -- $tokens[4] $argv[3..-1]
            return 0
        end
    end
    return 1
end

# True when 'genv <cmd> <sub> <subsub>' is present.
function __fish_genv_seen_subsub
    set -l tokens (commandline -opc)
    if test (count $tokens) -ge 4
        and test $tokens[2] = $argv[1]
        and test $tokens[3] = $argv[2]
        and test $tokens[4] = $argv[3]
        return 0
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

function __fish_genv_repo_packages
    set -l cur (commandline -ct)
    genv __complete repo-packages $cur 2>/dev/null
end

# Commands
complete -c genv -n __fish_genv_no_subcommand -f -a add -d 'Add a package to the spec and install it now'
complete -c genv -n __fish_genv_no_subcommand -f -a 'remove rm' -d 'Remove a package from the spec and uninstall it now'
complete -c genv -n __fish_genv_no_subcommand -f -a adopt -d 'Track an already-installed package in genv.json without reinstalling'
complete -c genv -n __fish_genv_no_subcommand -f -a disown -d 'Stop tracking a package in genv.json without uninstalling it'
complete -c genv -n __fish_genv_no_subcommand -f -a 'list ls' -d 'List all packages installed by genv'
complete -c genv -n __fish_genv_no_subcommand -f -a apply -d 'Reconcile system state with genv.json'
complete -c genv -n __fish_genv_no_subcommand -f -a edit -d 'Open genv.json in $EDITOR'
complete -c genv -n __fish_genv_no_subcommand -f -a clean -d 'Clear the cache of all detected package managers'
complete -c genv -n __fish_genv_no_subcommand -f -a scan -d 'Discover all installed packages and bulk-adopt them into genv.json'
complete -c genv -n __fish_genv_no_subcommand -f -a status -d 'Show diff between genv.json, the lock file, and recorded versions'
complete -c genv -n __fish_genv_no_subcommand -f -a completion -d 'Print shell completion script'
complete -c genv -n __fish_genv_no_subcommand -f -a validate -d 'Validate genv.json against the schema'
complete -c genv -n __fish_genv_no_subcommand -f -a upgrade -d 'Upgrade tracked packages plus OS vendor updates'
complete -c genv -n __fish_genv_no_subcommand -f -a updates -d 'Check available updates for genv-tracked packages'
complete -c genv -n __fish_genv_no_subcommand -f -a migrate -d 'Convert legacy host predicates to schemaVersion 8 targets'
complete -c genv -n __fish_genv_no_subcommand -f -a export -d 'Build a single-target portable snapshot and report'
complete -c genv -n __fish_genv_no_subcommand -f -a map -d 'Print assist-only manager mapping suggestions for a target'
complete -c genv -n __fish_genv_no_subcommand -f -a pull -d 'Fetch the spec from a git repository and update genv.json'
complete -c genv -n __fish_genv_no_subcommand -f -a init -d 'Create a new genv.json interactively'
complete -c genv -n __fish_genv_no_subcommand -f -a env -d 'Manage shell environment variables'
complete -c genv -n __fish_genv_no_subcommand -f -a shell -d 'Manage shell aliases and config'
complete -c genv -n __fish_genv_no_subcommand -f -a service -d 'Manage background services'
complete -c genv -n __fish_genv_no_subcommand -f -a profile -d 'Manage named environment profiles'
complete -c genv -n __fish_genv_no_subcommand -f -a version -d 'Show genv build version information'
complete -c genv -n __fish_genv_no_subcommand -f -a help -d 'Show this help text'

# Common flags
complete -c genv -l file -d 'Path to genv.json' -r

# remove / rm / disown — complete positional arg with tracked package IDs
complete -c genv -n '__fish_genv_using_command remove; or __fish_genv_using_command rm; or __fish_genv_using_command disown' \
    -f -a '(__fish_genv_packages)' -d 'Tracked package'
complete -c genv -n '__fish_genv_using_command remove; or __fish_genv_using_command rm; or __fish_genv_using_command disown' \
    -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_using_command remove; or __fish_genv_using_command rm' -l no-hooks -d 'Skip pre-remove and post-remove hooks'
complete -c genv -n '__fish_genv_using_command remove; or __fish_genv_using_command rm' -l hook-timeout -d 'Per-hook timeout' -x
complete -c genv -n '__fish_genv_using_command remove; or __fish_genv_using_command rm' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_using_command remove; or __fish_genv_using_command rm; or __fish_genv_using_command disown' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# list / ls
complete -c genv -n '__fish_genv_using_command list; or __fish_genv_using_command ls' -l lock-file -d 'Path to genv lock file' -r

# add / adopt — complete positional arg with repository package names
complete -c genv -n '__fish_genv_using_command add' -f -a '(__fish_genv_repo_packages)' -d 'Package'
complete -c genv -n '__fish_genv_using_command adopt' -f -a '(__fish_genv_repo_packages)' -d 'Package'
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l version -d 'Version constraint' -x
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' \
    -l prefer -d 'Preferred manager' -x -a '(__fish_genv_managers)'
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l manager -d 'Manager-specific names' -x
complete -c genv -n '__fish_genv_using_command add' -l no-search -d 'Skip interactive package search'
complete -c genv -n '__fish_genv_using_command add' -l no-hooks -d 'Skip pre-add and post-add hooks'
complete -c genv -n '__fish_genv_using_command add' -l hook-timeout -d 'Per-hook timeout' -x
complete -c genv -n '__fish_genv_using_command add' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_using_command add; or __fish_genv_using_command adopt' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# adopt-only
complete -c genv -n '__fish_genv_using_command adopt' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_using_command adopt' -l files -d 'Adopt matching files block entries into the lock without changing targets'
complete -c genv -n '__fish_genv_using_command adopt' -l json -d 'Emit machine-readable JSON to stdout'

# upgrade — complete positional arg with tracked package IDs
complete -c genv -n '__fish_genv_using_command upgrade' \
    -f -a '(__fish_genv_packages)' -d 'Tracked package'
complete -c genv -n '__fish_genv_using_command upgrade' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_using_command upgrade' -l dry-run -d 'Print the upgrade commands without executing'
complete -c genv -n '__fish_genv_using_command upgrade' -l yes -d 'Skip the confirmation prompt'
complete -c genv -n '__fish_genv_using_command upgrade' -l all -d 'Upgrade every unconstrained tracked package'
complete -c genv -n '__fish_genv_using_command upgrade' -l no-hooks -d 'Skip pre-upgrade and post-upgrade hooks'
complete -c genv -n '__fish_genv_using_command upgrade' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_using_command upgrade' -l only -d 'Package IDs or names to upgrade' -x
complete -c genv -n '__fish_genv_using_command upgrade' -l skip -d 'Package IDs or names to skip' -x
complete -c genv -n '__fish_genv_using_command upgrade' -l only-manager -d 'Managers to upgrade' -x
complete -c genv -n '__fish_genv_using_command upgrade' -l skip-manager -d 'Managers to skip' -x
complete -c genv -n '__fish_genv_using_command upgrade' -l hook-timeout -d 'Per-hook timeout' -x
complete -c genv -n '__fish_genv_using_command upgrade' -l debug -d 'Emit debug-level structured logs to stderr'
complete -c genv -n '__fish_genv_using_command upgrade' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_using_command upgrade' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# updates
complete -c genv -n '__fish_genv_at_subcommand updates check start stop status' -f -a check -d 'Plan available updates for genv-tracked packages only'
complete -c genv -n '__fish_genv_at_subcommand updates check start stop status' -f -a start -d 'Register the managed background updates checker'
complete -c genv -n '__fish_genv_at_subcommand updates check start stop status' -f -a stop -d 'Stop and unregister the managed background updates checker'
complete -c genv -n '__fish_genv_at_subcommand updates check start stop status' -f -a status -d 'Show managed background updates checker status'
complete -c genv -n '__fish_genv_seen_sub updates check' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_seen_sub updates check' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_seen_sub updates check' -l only -d 'Package IDs or names to check' -x
complete -c genv -n '__fish_genv_seen_sub updates check' -l skip -d 'Package IDs or names to skip' -x
complete -c genv -n '__fish_genv_seen_sub updates check' -l only-manager -d 'Managers to check' -x
complete -c genv -n '__fish_genv_seen_sub updates check' -l skip-manager -d 'Managers to skip' -x
complete -c genv -n '__fish_genv_seen_sub updates check' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_seen_sub updates check' -l target -d 'Portable target id for schemaVersion 8 specs' -x
complete -c genv -n '__fish_genv_seen_sub updates start' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_seen_sub updates start' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_seen_sub updates start' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# apply
complete -c genv -n '__fish_genv_using_command apply' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_using_command apply' -l dry-run -d 'Print the reconcile plan without executing'
complete -c genv -n '__fish_genv_using_command apply' -l force -d 'Overwrite mismatched managed files'
complete -c genv -n '__fish_genv_using_command apply' -l backup -d 'Back up mismatched files before overwrite'
complete -c genv -n '__fish_genv_using_command apply' -l strict -d 'Exit with an error if any package cannot be resolved'
complete -c genv -n '__fish_genv_using_command apply' -l yes -d 'Skip the confirmation prompt'
complete -c genv -n '__fish_genv_using_command apply' -l quiet -d 'Suppress plan output'
complete -c genv -n '__fish_genv_using_command apply' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_using_command apply' -l timeout -d 'Per-subprocess timeout' -x
complete -c genv -n '__fish_genv_using_command apply' -l no-hooks -d 'Skip pre-apply and post-apply hooks'
complete -c genv -n '__fish_genv_using_command apply' -l skip-packages -d 'Skip package install/remove; still apply env, shell, files, and services'
complete -c genv -n '__fish_genv_using_command apply' -l hook-timeout -d 'Per-hook timeout' -x
complete -c genv -n '__fish_genv_using_command apply' -l debug -d 'Emit debug-level structured logs to stderr'
complete -c genv -n '__fish_genv_using_command apply' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_using_command apply' -l target -d 'Portable target id for schemaVersion 8 specs' -x
complete -c genv -n '__fish_genv_using_command apply' -l force-new-lock -d 'Back up a foreign lock and start a new local lock'

# migrate
complete -c genv -n '__fish_genv_using_command migrate' -l write -d 'Overwrite genv.json with the migrated schemaVersion 8 spec'

# export
complete -c genv -n '__fish_genv_using_command export' -l target -d 'Target id to export' -x
complete -c genv -n '__fish_genv_using_command export' -l out -d 'Directory to write genv.json and report.json' -r
complete -c genv -n '__fish_genv_using_command export' -l strict -d 'Exit nonzero if the report contains errors'
complete -c genv -n '__fish_genv_using_command export' -l from-v7 -d 'Migrate v1-v7 input to schemaVersion 8 in memory first'

# map
complete -c genv -n '__fish_genv_using_command map' -l target -d 'Destination target id' -x

# status / scan
complete -c genv -n '__fish_genv_using_command status; or __fish_genv_using_command scan' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_using_command status; or __fish_genv_using_command scan' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_using_command status; or __fish_genv_using_command scan' -l debug -d 'Emit debug-level structured logs to stderr'
complete -c genv -n '__fish_genv_using_command scan' -l dry-run -d 'List packages that would be adopted without writing'
complete -c genv -n '__fish_genv_using_command scan' -l yes -d 'Skip the confirmation prompt'
complete -c genv -n '__fish_genv_using_command scan' -l target -d 'Portable target id for schemaVersion 8 specs' -x
complete -c genv -n '__fish_genv_using_command status' -l files -d 'Check files block against the live filesystem only'
complete -c genv -n '__fish_genv_using_command status' -l offline -d 'Compare spec vs lock only (skip live manager probe)'
complete -c genv -n '__fish_genv_using_command status' -l host -d 'Host name for host-specific records' -x
complete -c genv -n '__fish_genv_using_command status' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# clean
complete -c genv -n '__fish_genv_using_command clean' -l dry-run -d 'Print the clean commands without executing'

# pull
complete -c genv -n '__fish_genv_using_command pull' -l url -d 'Override the repository URL' -x
complete -c genv -n '__fish_genv_using_command pull' -l ref -d 'Override the repository ref' -x
complete -c genv -n '__fish_genv_using_command pull' -l dry-run -d 'Print what would be pulled without writing'

# env subcommands
complete -c genv -n '__fish_genv_at_subcommand env set unset list ls' -f -a set -d 'Add or update a variable in the spec'
complete -c genv -n '__fish_genv_at_subcommand env set unset list ls' -f -a unset -d 'Remove a variable from the spec'
complete -c genv -n '__fish_genv_at_subcommand env set unset list ls' -f -a 'list ls' -d 'Show all declared variables'
complete -c genv -n '__fish_genv_seen_sub env set' -l sensitive -d 'Mark value as sensitive (redacted in output and logs)'
complete -c genv -n '__fish_genv_seen_sub env set; or __fish_genv_seen_sub env unset' -l target -d 'Portable target id for schemaVersion 8 specs' -x
complete -c genv -n '__fish_genv_seen_sub env list; or __fish_genv_seen_sub env ls' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_seen_sub env list; or __fish_genv_seen_sub env ls' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# shell subcommands
complete -c genv -n '__fish_genv_at_subcommand shell alias status edit' -f -a alias -d 'Add, update, or remove a shell alias'
complete -c genv -n '__fish_genv_at_subcommand shell alias status edit' -f -a status -d 'Show shell config drift'
complete -c genv -n '__fish_genv_at_subcommand shell alias status edit' -f -a edit -d 'Open genv.json in $EDITOR'
complete -c genv -n '__fish_genv_at_subsubcommand shell alias set unset' -f -a set -d 'Add or update an alias'
complete -c genv -n '__fish_genv_at_subsubcommand shell alias set unset' -f -a unset -d 'Remove an alias'
complete -c genv -n '__fish_genv_seen_subsub shell alias set' -l shell -d 'Target shell' -x -a 'bash zsh fish'
complete -c genv -n '__fish_genv_seen_subsub shell alias set; or __fish_genv_seen_subsub shell alias unset' -l target -d 'Portable target id for schemaVersion 8 specs' -x
complete -c genv -n '__fish_genv_seen_sub shell status' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_seen_sub shell status' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# service subcommands
complete -c genv -n '__fish_genv_at_subcommand service add remove rm list ls start stop status' -f -a add -d 'Add or update a service'
complete -c genv -n '__fish_genv_at_subcommand service add remove rm list ls start stop status' -f -a 'remove rm' -d 'Remove a service from the spec'
complete -c genv -n '__fish_genv_at_subcommand service add remove rm list ls start stop status' -f -a 'list ls' -d 'Show all declared services'
complete -c genv -n '__fish_genv_at_subcommand service add remove rm list ls start stop status' -f -a start -d 'Start a service'
complete -c genv -n '__fish_genv_at_subcommand service add remove rm list ls start stop status' -f -a stop -d 'Stop a service'
complete -c genv -n '__fish_genv_at_subcommand service add remove rm list ls start stop status' -f -a status -d 'Show service running status'
complete -c genv -n '__fish_genv_seen_sub service add' -l start -d 'Command to start the service' -x
complete -c genv -n '__fish_genv_seen_sub service add' -l stop -d 'Command to stop the service' -x
complete -c genv -n '__fish_genv_seen_sub service add' -l restart -d 'Command to restart the service' -x
complete -c genv -n '__fish_genv_seen_sub service add' -l status -d 'Command to check service status' -x
complete -c genv -n '__fish_genv_seen_sub service add' -l brew-formula -d 'Homebrew formula to manage via brew services (macOS only)' -x
complete -c genv -n '__fish_genv_seen_sub service add; or __fish_genv_seen_sub service remove; or __fish_genv_seen_sub service rm' -l target -d 'Portable target id for schemaVersion 8 specs' -x
complete -c genv -n '__fish_genv_seen_sub service list; or __fish_genv_seen_sub service ls; or __fish_genv_seen_sub service start; or __fish_genv_seen_sub service stop; or __fish_genv_seen_sub service status' -l target -d 'Portable target id for schemaVersion 8 specs' -x

# profile
complete -c genv -n '__fish_genv_at_subcommand profile list ls create switch' -f -a 'list ls' -d 'List available profiles and mark the active one'
complete -c genv -n '__fish_genv_at_subcommand profile list ls create switch' -f -a create -d 'Scaffold a new profile file'
complete -c genv -n '__fish_genv_at_subcommand profile list ls create switch' -f -a switch -d 'Switch to a named profile and reconcile the system'

complete -c genv -n '__fish_genv_seen_sub profile list; or __fish_genv_seen_sub profile ls' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_seen_sub profile switch' -l lock-file -d 'Path to genv lock file' -r
complete -c genv -n '__fish_genv_seen_sub profile switch' -l dry-run -d 'Print the reconcile plan without executing'
complete -c genv -n '__fish_genv_seen_sub profile switch' -l force -d 'Overwrite mismatched managed files'
complete -c genv -n '__fish_genv_seen_sub profile switch' -l strict -d 'Exit with an error if any package cannot be resolved'
complete -c genv -n '__fish_genv_seen_sub profile switch' -l yes -d 'Skip the confirmation prompt'
complete -c genv -n '__fish_genv_seen_sub profile switch' -l quiet -d 'Suppress plan output'
complete -c genv -n '__fish_genv_seen_sub profile switch' -l json -d 'Emit machine-readable JSON to stdout'
complete -c genv -n '__fish_genv_seen_sub profile switch' -l timeout -d 'Per-subprocess timeout' -x
complete -c genv -n '__fish_genv_seen_sub profile switch' -l debug -d 'Emit debug-level structured logs to stderr'
complete -c genv -n '__fish_genv_seen_sub profile switch' -l host -d 'Host name for host-specific records' -x

# completion
complete -c genv -n '__fish_genv_at_subcommand completion bash zsh fish install' -f -a 'bash zsh fish powershell' -d 'Shell type'
complete -c genv -n '__fish_genv_at_subcommand completion bash zsh fish install' -f -a install -d 'Install the completion into the shell config directory'
complete -c genv -n '__fish_genv_seen_sub completion install' -f -a 'bash zsh fish powershell' -d 'Shell type'
complete -c genv -n '__fish_genv_seen_sub completion install' -l dir -d 'Target directory (overrides the per-shell default)' -r
