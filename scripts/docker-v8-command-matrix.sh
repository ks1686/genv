#!/usr/bin/env bash
# Real-binary CLI command matrix for schemaVersion 8.
#
# Builds genv from mounted sources and exercises every top-level command against
# a live Arch container (pacman). This is not an install-only smoke test — each
# command must succeed with correct v8 materialize behaviour.
#
# Host (from repo root):
#   docker run --rm -v "$PWD:/src:ro" -e GENV_SRC_ROOT=/src archlinux:latest \
#     bash /src/scripts/docker-v8-command-matrix.sh
#
# Exit 0 only if FAIL=0.
set -euo pipefail

ROOT="${GENV_SRC_ROOT:-/src}"
if [[ ! -f "$ROOT/go.mod" ]]; then
	echo "fatal: go.mod not found at $ROOT (mount the repo read-only at /src)" >&2
	exit 2
fi

export PATH="/usr/local/go/bin:${PATH}"
export GENV_NO_INTERACTIVE=1
export GENV_TARGET=arch

WORK="$(mktemp -d /tmp/genv-v8-matrix.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT
export XDG_CONFIG_HOME="$WORK/xdg"
export HOME="$WORK/home"
mkdir -p "$XDG_CONFIG_HOME/genv" "$WORK/out" "$WORK/bin" "$HOME"

PASS=0
FAIL=0
SKIP=0
RESULTS=()

log() { printf '%s\n' "$*"; }

record() {
	local status="$1" name="$2" detail="${3:-}"
	RESULTS+=("$status|$name|$detail")
	case "$status" in
	PASS) PASS=$((PASS + 1)) ;;
	FAIL) FAIL=$((FAIL + 1)) ;;
	SKIP) SKIP=$((SKIP + 1)) ;;
	esac
}

assert_eq() {
	local name="$1" got="$2" want="$3" detail="${4:-}"
	if [[ "$got" == "$want" ]]; then
		record PASS "$name" "$detail"
	else
		record FAIL "$name" "got=$got want=$want ${detail}"
	fi
}

assert_ok() {
	local name="$1" code="$2" detail="${3:-}"
	if [[ "$code" -eq 0 ]]; then
		record PASS "$name" "$detail"
	else
		record FAIL "$name" "exit=$code ${detail}"
	fi
}

assert_contains() {
	local name="$1" haystack="$2" needle="$3"
	if [[ "$haystack" == *"$needle"* ]]; then
		record PASS "$name"
	else
		record FAIL "$name" "missing '$needle' in: ${haystack:0:300}"
	fi
}

assert_not_contains() {
	local name="$1" haystack="$2" needle="$3"
	if [[ "$haystack" != *"$needle"* ]]; then
		record PASS "$name"
	else
		record FAIL "$name" "unexpected '$needle' in: ${haystack:0:300}"
	fi
}

setup_go_and_tools() {
	log "==> bootstrap Arch packages + Go"
	sed -i 's/^CheckSpace/#CheckSpace/; s/^#DisableSandbox/DisableSandbox/' /etc/pacman.conf || true
	pacman -Syu --noconfirm >/tmp/pacman-syu.log
	pacman -S --noconfirm base-devel git curl jq tree vim >/tmp/pacman-pkgs.log

	local goversion
	goversion="$(grep '^go ' "$ROOT/go.mod" | awk '{print $2}')"
	curl -fsSL "https://go.dev/dl/go${goversion}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
	tar -C /usr/local -xzf /tmp/go.tar.gz
	ln -sf /usr/local/go/bin/go /usr/local/bin/go
	ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
	go version
}

build_genv() {
	log "==> build genv from mounted sources"
	cp -a "$ROOT" "$WORK/src"
	cd "$WORK/src"
	go build -buildvcs=false -o "$WORK/bin/genv" .
	GENV="$WORK/bin/genv"
	"$GENV" version
}

write_v8_spec() {
	SPEC="$WORK/genv.json"
	LOCK="$WORK/genv.lock.json"
	HOOK_LOG="$WORK/hooks.log"
	cat >"$SPEC" <<EOF
{
  "schemaVersion": "8",
  "defaults": {
    "env": { "EDITOR": { "value": "nvim" } },
    "shell": { "aliases": { "ll": { "value": "ls -la" } } },
    "hooks": {
      "preAdd": [{ "command": "printf preadd >> $HOOK_LOG" }],
      "postAdd": [{ "command": "printf postadd >> $HOOK_LOG" }],
      "preRemove": [{ "command": "printf preremove >> $HOOK_LOG" }],
      "postRemove": [{ "command": "printf postremove >> $HOOK_LOG" }]
    }
  },
  "targets": {
    "arch": {
      "packages": [
        { "id": "tree", "prefer": "pacman" }
      ],
      "env": { "LANG": { "value": "C.UTF-8" } },
      "files": {
        "links": [
          {
            "source": "testrc",
            "target": "~/.testrc",
            "mode": "managed-link",
            "backup": true
          }
        ]
      },
      "services": {
        "pulse": {
          "start": ["true"],
          "stop": ["true"],
          "status": ["true"]
        }
      }
    }
  }
}
EOF
	: >"$HOOK_LOG"
}

run_matrix() {
	local out err code
	GENV="$WORK/bin/genv"
	SPEC="$WORK/genv.json"
	LOCK="$WORK/genv.lock.json"
	HOOK_LOG="$WORK/hooks.log"

	run() {
		set +e
		out="$("$GENV" "$@" 2>"$WORK/stderr.txt")"
		code=$?
		set -e
		err="$(cat "$WORK/stderr.txt")"
	}

	# ── meta ──────────────────────────────────────────────────────────────
	run version
	assert_ok "version" "$code" "$out"
	run help
	assert_ok "help" "$code"
	assert_contains "help documents --target" "$out$err" "--target"
	run --help
	assert_ok "--help" "$code"
	run completion bash
	assert_ok "completion bash" "$code"
	assert_contains "completion offers status --target" "$out" "--target"
	run completion zsh
	assert_ok "completion zsh" "$code"
	run completion fish
	assert_ok "completion fish" "$code"

	# ── validate / migrate / init ─────────────────────────────────────────
	run validate --file "$SPEC"
	assert_ok "validate v8" "$code" "$out$err"
	run migrate --file "$SPEC"
	assert_ok "migrate v8 no-op/ok" "$code" "$out$err"

	local init_spec="$WORK/init.json"
	printf 'n\n' | run init --file "$init_spec" || true
	# init may prompt; accept written file or clean non-zero
	if [[ -f "$init_spec" ]]; then
		record PASS "init wrote file"
	elif [[ "$code" -ne 0 ]]; then
		record PASS "init non-interactive exit $code"
	else
		record FAIL "init" "no file and exit 0"
	fi

	# ── apply / status / list (core materialize regression) ───────────────
	run apply --file "$SPEC" --lock-file "$LOCK" --target arch --yes --no-hooks
	assert_ok "apply --target arch" "$code" "$out$err"
	assert_contains "apply mentions tree" "$out$err" "tree"

	run status --file "$SPEC" --lock-file "$LOCK" --target arch
	assert_ok "status v8" "$code" "$out$err"
	assert_contains "status reports ok" "$out" "ok"
	assert_not_contains "status has no extras" "$out" "extra"

	run status --file "$SPEC" --lock-file "$LOCK" --target arch --json
	assert_ok "status --json" "$code"
	assert_contains "status json ok" "$out" '"ok":true'

	run list --file "$SPEC" --lock-file "$LOCK"
	assert_ok "list" "$code"
	assert_contains "list shows tree" "$out" "tree"
	run ls --file "$SPEC" --lock-file "$LOCK"
	assert_ok "ls alias" "$code"

	# ── upgrade / updates (must see tracked packages) ─────────────────────
	run upgrade --file "$SPEC" --lock-file "$LOCK" --target arch --dry-run --all
	assert_ok "upgrade --dry-run --all" "$code" "$out$err"
	assert_contains "upgrade plans tree" "$out$err" "tree"
	assert_not_contains "upgrade not falsely empty" "$out" "no upgradeable packages found"

	run updates check --file "$SPEC" --lock-file "$LOCK" --target arch
	assert_ok "updates check" "$code" "$out$err"
	# Up-to-date packages may yield an empty outdated plan; that is OK as long
	# as materialize worked (proven by upgrade --all above). Still require a
	# tracked-only header so the command path ran.
	assert_contains "updates check tracked-only header" "$out" "genv-tracked"

	run updates status
	if [[ "$code" -eq 0 || "$code" -eq 4 ]]; then
		record PASS "updates status (exit $code)"
	else
		record FAIL "updates status" "exit=$code stderr=$err"
	fi

	# ── add / remove with v8 defaults.hooks ───────────────────────────────
	run remove --file "$SPEC" --lock-file "$LOCK" --target arch --no-hooks tree
	assert_ok "remove tree (prep)" "$code" "$out$err"
	: >"$HOOK_LOG"
	run add --file "$SPEC" --lock-file "$LOCK" --target arch --prefer pacman --no-search tree
	assert_ok "add tree" "$code" "$out$err"
	assert_contains "add hooks fired" "$(cat "$HOOK_LOG")" "preadd"
	assert_contains "add post hook fired" "$(cat "$HOOK_LOG")" "postadd"

	: >"$HOOK_LOG"
	run remove --file "$SPEC" --lock-file "$LOCK" --target arch tree
	assert_ok "remove tree" "$code" "$out$err"
	assert_contains "remove hooks fired" "$(cat "$HOOK_LOG")" "preremove"
	assert_contains "remove post hook fired" "$(cat "$HOOK_LOG")" "postremove"

	run add --file "$SPEC" --lock-file "$LOCK" --target arch --prefer pacman --no-search --no-hooks tree
	assert_ok "re-add tree" "$code" "$out$err"
	run rm --file "$SPEC" --lock-file "$LOCK" --target arch --no-hooks tree
	assert_ok "rm alias" "$code" "$out$err"
	run add --file "$SPEC" --lock-file "$LOCK" --target arch --prefer pacman --no-search --no-hooks tree
	assert_ok "restore tree after rm" "$code" "$out$err"

	# ── adopt / disown / scan ─────────────────────────────────────────────
	run disown --file "$SPEC" --lock-file "$LOCK" --target arch tree
	assert_ok "disown" "$code" "$out$err"
	run adopt --file "$SPEC" --lock-file "$LOCK" --target arch --prefer pacman tree
	assert_ok "adopt" "$code" "$out$err"
	run scan --file "$SPEC" --lock-file "$LOCK" --target arch --json
	assert_ok "scan --json" "$code" "$out$err"
	assert_contains "scan json ok" "$out" '"ok":true'

	# ── env / shell / service (materialized reads + lifecycle) ────────────
	run env set --file "$SPEC" --target arch FOO bar
	assert_ok "env set" "$code" "$out$err"
	run env list --file "$SPEC" --target arch
	assert_ok "env list" "$code" "$out$err"
	assert_contains "env list EDITOR" "$out" "EDITOR"
	assert_contains "env list FOO" "$out" "FOO"
	assert_contains "env list LANG" "$out" "LANG"
	run env unset --file "$SPEC" --target arch FOO
	assert_ok "env unset" "$code"
	run env ls --file "$SPEC" --target arch
	assert_ok "env ls alias" "$code"

	run shell alias set --file "$SPEC" --target arch gs "git status"
	assert_ok "shell alias set" "$code"
	run shell status --file "$SPEC" --target arch
	# Drift (exit 4) is OK — aliases are declared but not yet applied.
	if [[ "$code" -eq 0 || "$code" -eq 4 ]]; then
		record PASS "shell status (exit $code)"
	else
		record FAIL "shell status" "exit=$code stderr=$err"
	fi
	assert_contains "shell status ll" "$out" "ll"
	assert_contains "shell status gs" "$out" "gs"
	run shell alias unset --file "$SPEC" --target arch gs
	assert_ok "shell alias unset" "$code"

	run service list --file "$SPEC" --target arch
	assert_ok "service list" "$code" "$out$err"
	assert_contains "service list pulse" "$out" "pulse"
	run service start --file "$SPEC" --target arch pulse
	assert_ok "service start" "$code" "$out$err"
	run service status --file "$SPEC" --target arch pulse
	assert_ok "service status" "$code" "$out$err"
	run service stop --file "$SPEC" --target arch pulse
	assert_ok "service stop" "$code" "$out$err"
	run service ls --file "$SPEC" --target arch
	assert_ok "service ls alias" "$code"

	printf 'live-testrc\n' >"$HOME/.testrc"
	run files adopt --file "$SPEC" --lock-file "$LOCK" --target arch --dry-run ~/.testrc
	assert_ok "files adopt --dry-run" "$code" "$out$err"
	assert_contains "files adopt dry-run copy" "$out" "copy"
	assert_contains "files adopt dry-run backup" "$out" "backup"
	assert_contains "files adopt dry-run link" "$out" "link"
	run files adopt --file "$SPEC" --lock-file "$LOCK" --target arch ~/.testrc
	assert_ok "files adopt" "$code" "$out$err"

	# ── export / map / apply dry-run / clean ──────────────────────────────
	run export --file "$SPEC" --target arch --out "$WORK/out/export"
	assert_ok "export" "$code" "$out$err"
	[[ -f "$WORK/out/export/genv.json" ]] && record PASS "export wrote genv.json" || record FAIL "export wrote genv.json"
	run map --file "$SPEC" --target ubuntu
	assert_ok "map" "$code" "$out$err"
	run apply --file "$SPEC" --lock-file "$LOCK" --target arch --dry-run --yes
	assert_ok "apply --dry-run" "$code" "$out$err"
	run clean --dry-run
	assert_ok "clean --dry-run" "$code" "$out$err"

	# ── profile / pull / edit ─────────────────────────────────────────────
	run profile list --file "$SPEC"
	assert_ok "profile list" "$code" "$out$err"
	run pull --file "$SPEC" --dry-run
	if [[ "$code" -ne 0 ]]; then
		record PASS "pull without repo fails cleanly (exit $code)"
	else
		record FAIL "pull without repo fails cleanly" "unexpected success"
	fi
	# Stub a safe editor name so edit is non-interactive in CI.
	printf '#!/bin/sh\nexit 0\n' >"$WORK/bin/vim"
	chmod +x "$WORK/bin/vim"
	export PATH="$WORK/bin:$PATH"
	set +e
	out="$(EDITOR=vim "$GENV" edit --file "$SPEC" 2>"$WORK/stderr.txt")"
	code=$?
	set -e
	err="$(cat "$WORK/stderr.txt")"
	assert_ok "edit with vim stub" "$code" "$out$err"

	# ── negative: unknown target ──────────────────────────────────────────
	run status --file "$SPEC" --lock-file "$LOCK" --target ubuntu
	assert_eq "status unknown target exits 3" "$code" "3" "$err"
	assert_contains "status unknown target message" "$err" "targets.ubuntu"
}

print_report() {
	log ""
	log "======== v8 command matrix report ========"
	local line status name detail
	for line in "${RESULTS[@]}"; do
		IFS='|' read -r status name detail <<<"$line"
		printf '%-4s  %s' "$status" "$name"
		if [[ -n "$detail" ]]; then
			printf '  (%s)' "$detail"
		fi
		printf '\n'
	done
	log "------------------------------------------"
	log "PASS=$PASS FAIL=$FAIL SKIP=$SKIP"
	[[ "$FAIL" -eq 0 ]]
}

main() {
	setup_go_and_tools
	build_genv
	write_v8_spec
	run_matrix
	print_report
}

main "$@"
