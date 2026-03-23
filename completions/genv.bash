# bash completion for genv

_genv() {
	local i cur prev opts cmds
	COMPREPLY=()
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[COMP_CWORD-1]}"
	cmd=""
	opts=""

	for i in ${COMP_WORDS[@]}; do
		case "${i}" in
		add | remove | rm | adopt | disown | list | ls | apply | scan | status | clean | edit | version | help)
			cmd="${i}"
			break
			;;
		esac
	done

	if [[ -z "${cmd}" ]]; then
		if [[ "${cur}" == -* ]]; then
			COMPREPLY=($(compgen -W "--file" -- ${cur}))
			return 0
		fi
		cmds="add remove rm adopt disown list ls apply scan status clean edit version help"
		COMPREPLY=($(compgen -W "${cmds}" -- ${cur}))
		return 0
	fi

	case "${cmd}" in
	add | adopt)
		opts="--file --version --prefer --manager"
		;;
	apply)
		opts="--file --dry-run --strict --yes --json --timeout --debug"
		;;
	status | scan)
		opts="--file --json --debug"
		;;
	clean)
		opts="--file --dry-run"
		;;
	*)
		opts="--file"
		;;
	esac

	if [[ "${cur}" == -* ]]; then
		COMPREPLY=($(compgen -W "${opts}" -- ${cur}))
		return 0
	fi

	return 0
}

complete -F _genv genv
