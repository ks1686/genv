# bash completion for genv

_genv() {
	local i j cur prev opts cmds
	COMPREPLY=()
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[COMP_CWORD - 1]}"
	cmd=""
	opts=""

	for i in "${COMP_WORDS[@]}"; do
		case "${i}" in
		add | remove | rm | adopt | disown | list | ls | apply | edit | clean | scan | status | completion | validate | upgrade | updates | migrate | export | map | pull | init | env | shell | service | profile | version | help)
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
		cmds="add remove rm adopt disown list ls apply edit clean scan status completion validate upgrade updates migrate export map pull init env shell service profile version help"
		mapfile -t COMPREPLY < <(compgen -W "${cmds}" -- "${cur}")
		return 0
	fi

	# Resolve --file value from the command line if present, so __complete reads
	# the right spec when the user has specified a custom --file path.
	local file_arg=""
	for ((i = 1; i < ${#COMP_WORDS[@]} - 1; i++)); do
		if [[ "${COMP_WORDS[i]}" == "--file" ]]; then
			file_arg="--file ${COMP_WORDS[i + 1]}"
			break
		fi
	done

	case "${cmd}" in
	remove | rm)
		# Complete positional arg with tracked package IDs.
		if [[ "${cur}" != -* ]]; then
			# shellcheck disable=SC2086
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete packages ${file_arg} 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --no-hooks --hook-timeout --host --target"
		;;
	disown)
		if [[ "${cur}" != -* ]]; then
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete packages ${file_arg} 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --target"
		;;
	add)
		# Complete --prefer value with available managers.
		if [[ "${prev}" == "--prefer" ]]; then
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete managers 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --version --prefer --manager --no-search --no-hooks --hook-timeout --host --target"
		;;
	adopt)
		# Complete --prefer value with available managers.
		if [[ "${prev}" == "--prefer" ]]; then
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete managers 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --version --prefer --manager --host --target --files --json"
		;;
	upgrade)
		# Complete positional arg (if any) with tracked package IDs.
		if [[ "${cur}" != -* ]]; then
			# shellcheck disable=SC2086
			mapfile -t COMPREPLY < <(compgen -W "$(genv __complete packages ${file_arg} 2>/dev/null)" -- "${cur}")
			return 0
		fi
		opts="--file --lock-file --dry-run --yes --no-hooks --json --only --skip --only-manager --skip-manager --hook-timeout --debug --host --target"
		;;
	updates)
		local updates_sub=""
		for ((i = 1; i < ${#COMP_WORDS[@]}; i++)); do
			case "${COMP_WORDS[i]}" in
			check | start | stop | status)
				updates_sub="${COMP_WORDS[i]}"
				break
				;;
			esac
		done
		if [[ -z "${updates_sub}" ]]; then
			if [[ "${cur}" == -* ]]; then
				opts="--file"
			else
				mapfile -t COMPREPLY < <(compgen -W "check start stop status" -- "${cur}")
				return 0
			fi
		else
			case "${updates_sub}" in
			check) opts="--file --lock-file --json --only --skip --only-manager --skip-manager --host --target" ;;
			start) opts="--file --lock-file --host --target" ;;
			*) opts="" ;;
			esac
		fi
		;;
	apply)
		opts="--file --lock-file --dry-run --force --backup --strict --yes --quiet --json --timeout --no-hooks --hook-timeout --debug --host --target --force-new-lock"
		;;
	migrate)
		opts="--file --write"
		;;
	export)
		opts="--file --target --out --strict --from-v7"
		;;
	map)
		opts="--file --target"
		;;
	status)
		opts="--file --lock-file --json --debug --files --host --target"
		;;
	scan)
		opts="--file --lock-file --json --debug --target"
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
			set) opts="--file --sensitive --target" ;;
			unset) opts="--file --target" ;;
			list | ls) opts="--file --json --target" ;;
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
					set) opts="--file --shell --target" ;;
					unset) opts="--file --target" ;;
					esac
				fi
				;;
			status) opts="--file --json --target" ;;
			edit) opts="--file" ;;
			esac
		fi
		;;
	profile)
		local prof_sub=""
		for ((i = 1; i < ${#COMP_WORDS[@]}; i++)); do
			case "${COMP_WORDS[i]}" in
			list | ls | create | switch)
				prof_sub="${COMP_WORDS[i]}"
				break
				;;
			esac
		done
		if [[ -z "${prof_sub}" ]]; then
			if [[ "${cur}" == -* ]]; then
				opts="--file"
			else
				mapfile -t COMPREPLY < <(compgen -W "list ls create switch" -- "${cur}")
				return 0
			fi
		else
			case "${prof_sub}" in
			list | ls) opts="--file --lock-file" ;;
			create) opts="--file" ;;
			switch) opts="--file --lock-file --dry-run --force --backup --strict --yes --quiet --json --timeout --debug --host" ;;
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
			add) opts="--file --start --stop --restart --status --brew-formula --target" ;;
			remove | rm) opts="--file --target" ;;
			list | ls) opts="--file --target" ;;
			start | stop | status) opts="--file --target" ;;
			*) opts="--file" ;;
			esac
		fi
		;;
	completion)
		# Detect the "install" subcommand, if present.
		local comp_sub=""
		for ((i = 1; i < ${#COMP_WORDS[@]}; i++)); do
			if [[ "${COMP_WORDS[i]}" == "install" ]]; then
				comp_sub="install"
				break
			fi
		done
		if [[ "${comp_sub}" == "install" ]]; then
			if [[ "${cur}" != -* ]]; then
				mapfile -t COMPREPLY < <(compgen -W "bash zsh fish" -- "${cur}")
				return 0
			fi
			opts="--dir"
		else
			mapfile -t COMPREPLY < <(compgen -W "bash zsh fish install" -- "${cur}")
			return 0
		fi
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
