# bash completion for genv

_genv() {
	local i j cur prev opts cmds
	COMPREPLY=()
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[COMP_CWORD-1]}"
	cmd=""
	opts=""

	for i in "${COMP_WORDS[@]}"; do
		case "${i}" in
		add | remove | rm | adopt | disown | list | ls | apply | edit | clean | scan | status | completion | validate | upgrade | pull | init | env | shell | service | version | help)
			cmd="${i}"
			break
			;;
		esac
	done

	if [[ -z "${cmd}" ]]; then
		if [[ "${cur}" == -* ]]; then
			mapfile -t COMPREPLY < <(compgen -W "--file" -- "${cur}")
			return 0
		fi
		cmds="add remove rm adopt disown list ls apply edit clean scan status completion validate upgrade pull init env shell service version help"
		mapfile -t COMPREPLY < <(compgen -W "${cmds}" -- "${cur}")
		return 0
	fi

	# Resolve --file value from the command line if present, so __complete reads
	# the right spec when the user has specified a custom --file path.
	local file_arg=""
	for ((i = 1; i < ${#COMP_WORDS[@]} - 1; i++)); do
		if [[ "${COMP_WORDS[i]}" == "--file" ]]; then
			file_arg="--file ${COMP_WORDS[i+1]}"
			break
		fi
	done

	case "${cmd}" in
	remove | rm | disown)
		# Complete positional arg with tracked package IDs.
		if [[ "${cur}" != -* ]]; then
			# shellcheck disable=SC2086
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete packages ${file_arg} 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file"
		;;
	add)
		# Complete --prefer value with available managers.
		if [[ "${prev}" == "--prefer" ]]; then
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete managers 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --version --prefer --manager --no-search"
		;;
	adopt)
		# Complete --prefer value with available managers.
		if [[ "${prev}" == "--prefer" ]]; then
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete managers 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --version --prefer --manager --host --files --json"
		;;
	upgrade)
		# Complete positional arg (if any) with tracked package IDs.
		if [[ "${cur}" != -* ]]; then
			# shellcheck disable=SC2086
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete packages ${file_arg} 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --dry-run --yes --debug --host"
		;;
	apply)
		opts="--file --lock-file --dry-run --force --strict --yes --quiet --json --timeout --debug --host"
		;;
	status)
		opts="--file --lock-file --json --debug --files --host"
		;;
	scan)
		opts="--file --lock-file --json --debug"
		;;
	clean)
		opts="--dry-run"
		;;
	pull)
		opts="--file --url --ref --dry-run"
		;;
	env)
		# Resolve the env subcommand, if any, from the command line.
		local env_sub=""
		for ((i = 1; i < ${#COMP_WORDS[@]}; i++)); do
			case "${COMP_WORDS[i]}" in
			set | unset | list | ls)
				env_sub="${COMP_WORDS[i]}"
				break
				;;
			esac
		done
		if [[ -z "${env_sub}" ]]; then
			if [[ "${cur}" == -* ]]; then
				opts="--file"
			else
				mapfile -t COMPREPLY < <(compgen -W "set unset list ls" -- "${cur}")
				return 0
			fi
		else
			case "${env_sub}" in
			set) opts="--file --sensitive" ;;
			unset) opts="--file" ;;
			list | ls) opts="--file --json" ;;
			esac
		fi
		;;
	shell)
		# Complete the --shell flag value.
		if [[ "${prev}" == "--shell" ]]; then
			mapfile -t COMPREPLY < <(compgen -W "bash zsh fish" -- "${cur}")
			return 0
		fi
		# Resolve the shell subcommand (and alias sub-subcommand), if any.
		local shell_sub="" shell_sub2=""
		for ((i = 1; i < ${#COMP_WORDS[@]}; i++)); do
			case "${COMP_WORDS[i]}" in
			alias | status | edit)
				shell_sub="${COMP_WORDS[i]}"
				if [[ "${shell_sub}" == "alias" ]]; then
					for ((j = i + 1; j < ${#COMP_WORDS[@]}; j++)); do
						case "${COMP_WORDS[j]}" in
						set | unset)
							shell_sub2="${COMP_WORDS[j]}"
							break
							;;
						esac
					done
				fi
				break
				;;
			esac
		done
		if [[ -z "${shell_sub}" ]]; then
			if [[ "${cur}" == -* ]]; then
				opts="--file"
			else
				mapfile -t COMPREPLY < <(compgen -W "alias status edit" -- "${cur}")
				return 0
			fi
		else
			case "${shell_sub}" in
			alias)
				if [[ -z "${shell_sub2}" ]]; then
					if [[ "${cur}" == -* ]]; then
						opts="--file"
					else
						mapfile -t COMPREPLY < <(compgen -W "set unset" -- "${cur}")
						return 0
					fi
				else
					case "${shell_sub2}" in
					set) opts="--file --shell" ;;
					unset) opts="--file" ;;
					esac
				fi
				;;
			status) opts="--file --json" ;;
			edit) opts="--file" ;;
			esac
		fi
		;;
	service)
		# Resolve the service subcommand, if any, from the command line.
		local svc_sub=""
		for ((i = 1; i < ${#COMP_WORDS[@]}; i++)); do
			case "${COMP_WORDS[i]}" in
			add | remove | rm | list | ls | start | stop | status)
				svc_sub="${COMP_WORDS[i]}"
				break
				;;
			esac
		done
		if [[ -z "${svc_sub}" ]]; then
			if [[ "${cur}" == -* ]]; then
				opts="--file"
			else
				mapfile -t COMPREPLY < <(compgen -W "add remove rm list ls start stop status" -- "${cur}")
				return 0
			fi
		else
			case "${svc_sub}" in
			add) opts="--file --start --stop --restart --status --brew-formula" ;;
			*) opts="--file" ;;
			esac
		fi
		;;
	completion)
		mapfile -t COMPREPLY < <(compgen -W "bash zsh fish" -- "${cur}")
		return 0
		;;
	*)
		opts="--file"
		;;
	esac

	if [[ "${cur}" == -* ]]; then
		mapfile -t COMPREPLY < <(compgen -W "${opts}" -- "${cur}")
		return 0
	fi

	return 0
}

complete -F _genv genv
