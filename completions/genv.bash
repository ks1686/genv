# bash completion for genv

_genv() {
	local i cur prev opts cmds
	COMPREPLY=()
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[COMP_CWORD-1]}"
	cmd=""
	opts=""

	for i in "${COMP_WORDS[@]}"; do
		case "${i}" in
		add | remove | rm | adopt | disown | list | ls | apply | scan | status | clean | edit | completion | validate | upgrade | init | version | help)
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
		cmds="add remove rm adopt disown list ls apply scan status clean edit completion validate upgrade init version help"
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
		# Complete --prefer value with available managers.
		if [[ "${prev}" == "--prefer" ]]; then
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete managers 2>/dev/null)" -- "${cur}")
			return 0
		fi
		# Complete positional arg with tracked package IDs.
		if [[ "${cur}" != -* ]]; then
			# shellcheck disable=SC2086
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete packages ${file_arg} 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file"
		;;
	add | adopt)
		# Complete --prefer / --manager key with available managers.
		if [[ "${prev}" == "--prefer" ]]; then
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete managers 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --version --prefer --manager --no-search"
		;;
	upgrade)
		# Complete positional arg (if any) with tracked package IDs.
		if [[ "${cur}" != -* ]]; then
			# shellcheck disable=SC2086
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete packages ${file_arg} 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --dry-run --yes --debug"
		;;
	apply)
		opts="--file --dry-run --strict --yes --quiet --json --timeout --debug"
		;;
	status | scan)
		opts="--file --json --debug"
		;;
	clean)
		opts="--file --dry-run"
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
