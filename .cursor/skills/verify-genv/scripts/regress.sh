#!/usr/bin/env bash
# Proves a failed launch cannot clean another run, capture refuses '..' and
# symlinks, and evidence is mode 700 with mode 600 files.
set -euo pipefail

here=$(CDPATH='' cd "$(dirname "$0")" && pwd)
harness=$here/verify-genv
work=$(mktemp -d /tmp/genv-verify-regress.XXXXXX)
legacy=/tmp/genv-verify-current
legacy_existed=0
legacy_backup=
A_ID=
B_ID=

if [[ -f "$legacy" ]]; then
  legacy_existed=1
  legacy_backup=$(mktemp)
  cp "$legacy" "$legacy_backup"
fi

tmux_cmd() {
  if [[ -f /exec-daemon/tmux.portal.conf ]]; then
    tmux -f /exec-daemon/tmux.portal.conf "$@"
  else
    tmux "$@"
  fi
}

finish() {
  local rc=$?
  if [[ -n "$A_ID" ]]; then
    "$harness" cleanup "$A_ID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$B_ID" ]]; then
    "$harness" cleanup "$B_ID" >/dev/null 2>&1 || true
  fi
  tmux_cmd kill-session -t "=genv-verify-prefix-12" >/dev/null 2>&1 || true
  tmux_cmd kill-session -t "=genv-verify-prefix-1234" >/dev/null 2>&1 || true
  if [[ -d "$work" ]]; then
    chmod -R u+w "$work" 2>/dev/null || true
    rm -rf "$work"
  fi
  if [[ "$legacy_existed" -eq 1 ]]; then
    cp "$legacy_backup" "$legacy"
    rm -f "$legacy_backup"
  else
    rm -f "$legacy"
  fi
  exit "$rc"
}
trap finish EXIT

export GENV_VERIFY_RUNS=$work/runs
export GENV_VERIFY_EVIDENCE=$work/evidence
unset GENV_VERIFY_RUN || true
unset GENV_VERIFY_GOCACHE || true
unset GENV_VERIFY_GOMODCACHE || true

if [[ -e "${HOME}/.config/genv" ]]; then
  printf 'operator config already exists at %s\n' "${HOME}/.config/genv" >&2
  exit 1
fi

stamp=$work/stamp
touch "$stamp"
sleep 1

require() {
  local name=$1 got=$2 want=$3
  printf '%s=%s\n' "$name" "$got"
  [[ "$got" == "$want" ]]
}

expect_fail() {
  local name=$1
  shift
  local out rc
  out=$work/${name}.out
  set +e
  "$@" >"$out" 2>&1
  rc=$?
  set -e
  printf '%s_exit=%s\n' "$name" "$rc"
  cat "$out"
  [[ "$rc" -ne 0 ]]
}

printf '=== classify debian ===\n'
printf '%s\n' 'ID=debian' 'VERSION_ID="13"' >"$work/debian-os"
printf '%s\n' 'Linux version 6.1.0-1' >"$work/proc-plain"
"$harness" classify "$work/debian-os" "$work/proc-plain" Linux | tee "$work/debian.class"
grep -qx 'host_class=' "$work/debian.class"
grep -qx 'session_target=linux' "$work/debian.class"

printf '=== classify ubuntu ===\n'
printf '%s\n' 'ID=ubuntu' 'ID_LIKE=debian' >"$work/ubuntu-os"
"$harness" classify "$work/ubuntu-os" "$work/proc-plain" Linux | tee "$work/ubuntu.class"
grep -qx 'host_class=ubuntu' "$work/ubuntu.class"
grep -qx 'session_target=' "$work/ubuntu.class"

printf '=== classify wsl-arch ===\n'
printf '%s\n' 'ID=arch' >"$work/arch-os"
printf '%s\n' 'Linux version 6.6.0-microsoft-standard-WSL2' >"$work/proc-wsl"
"$harness" classify "$work/arch-os" "$work/proc-wsl" Linux | tee "$work/wsl.class"
grep -qx 'host_class=wsl-arch' "$work/wsl.class"
grep -qx 'session_target=' "$work/wsl.class"

printf '=== launch A ===\n'
"$harness" launch | tee "$work/launchA.out"
A_ID=$(sed -n 's/^run_id=//p' "$work/launchA.out")
grep -q '^session_target=$' "$work/launchA.out"
require launch_session_target "$(sed -n 's/^session_target=//p' "$work/launchA.out")" ""
[[ -n "$A_ID" ]]
export GENV_VERIFY_RUN=$A_ID
printf '=== doctor ===\n'
"$harness" doctor | tee "$work/doctor.out"
grep -q '^doctor ok$' "$work/doctor.out"
home=$("$harness" paths home)
ev=$("$harness" paths evidence)
sessionA=$("$harness" paths session)
printf 'A=%s\n' "$A_ID"
printf 'home=%s\n' "$home"
printf 'evidence=%s\n' "$ev"

require evidence_mode "$(stat -c %a "$ev")" 700
require pane_mode "$(stat -c %a "$ev/pane.log")" 600
require run_env_mode "$(stat -c %a "$ev/run.env")" 600
require build_log_mode "$(stat -c %a "$ev/build.log")" 600
grep -q "^build_home=${home}$" "$ev/build.log"
grep -q "^gocache=${GENV_VERIFY_RUNS}/${A_ID}/gocache$" "$ev/build.log"
printf 'build_home_is_throwaway=yes\n'
printf 'gocache_is_run_local=yes\n'

printf '=== capture ===\n'
echo SECRET-OPERATOR-FILE >"$work/secret.txt"
expect_fail dotdot "$harness" capture leak "$home/../../../secret.txt"
ln -s "$work/secret.txt" "$home/innocent"
expect_fail symlink "$harness" capture sym "$home/innocent"
mkdir -p "${home}-evil"
echo SECRET-OPERATOR-FILE >"${home}-evil/secret.txt"
expect_fail sibling "$harness" capture sib "${home}-evil/secret.txt"
echo hello >"$home/note.txt"
chmod 644 "$home/note.txt"
expect_fail dotdot_inside "$harness" capture inside "$home/../home/note.txt"
"$harness" capture note "$home/note.txt" >/dev/null
require note_mode "$(stat -c %a "$ev/note")" 600
require note_source_mode "$(stat -c %a "$home/note.txt")" 644
if grep -R -q -- 'SECRET-OPERATOR-FILE' "$ev"; then
  printf 'capture leaked operator file contents\n' >&2
  exit 1
fi
printf 'capture_secret_absent=yes\n'

printf '=== joined pane ===\n'
pad=$(printf '%74s' '' | tr ' ' x)
long="${pad}already exists${pad}"
"$harness" drive -- "printf '%s\n' '$long'" >/dev/null
drive_file=$(find "$ev/drives" -name '*.txt' | LC_ALL=C sort | tail -n 1)
if ! grep -q -F "$long" "$drive_file"; then
  printf 'joined drive file lost the long line\n' >&2
  exit 1
fi
printf 'joined_already_exists=%s\n' "$(grep -c -F 'already exists' "$drive_file")"
[[ "$(grep -c -F 'already exists' "$drive_file")" -ge 1 ]]

printf '=== fail fast ===\n'
start=$SECONDS
expect_fail missing_prompt "$harness" drive --until 'this-prompt-is-absent' -- true
printf 'missing_prompt_seconds=%s\n' "$((SECONDS - start))"
[[ $((SECONDS - start)) -lt 10 ]]
expect_fail respond_at_prompt "$harness" respond -- git
pane=$(tmux_cmd capture-pane -J -p -t "=$sessionA:" -S -)
if printf '%s\n' "$pane" | grep -q 'command not found'; then
  printf 'respond typed into the shell\n' >&2
  exit 1
fi
if printf '%s\n' "$pane" | grep -q -x 'git'; then
  printf 'respond typed git into the shell\n' >&2
  exit 1
fi
printf 'respond_did_not_type=yes\n'

printf '=== failed launch ===\n'
mkdir -p "$work/fakebin"
cat >"$work/fakebin/go" <<'EOF'
#!/bin/sh
echo fake-go-failed >&2
exit 1
EOF
chmod 700 "$work/fakebin/go"
echo KEEP-A >"$home/marker"
printf '%s\n' "$A_ID" >"$legacy"
set +e
PATH="$work/fakebin:$PATH" "$harness" launch >"$work/fail.out" 2>&1
fail_rc=$?
set -e
printf 'failed_launch_exit=%s\n' "$fail_rc"
cat "$work/fail.out"
[[ "$fail_rc" -ne 0 ]]
B_ID=$(sed -n 's/^verify-genv: launch failed for run //p' "$work/fail.out")
[[ -n "$B_ID" && "$B_ID" != "$A_ID" ]]
printf 'failed_run=%s\n' "$B_ID"
[[ -f "$home/marker" ]]
[[ -d "$GENV_VERIFY_RUNS/$A_ID" ]]
[[ ! -d "$GENV_VERIFY_RUNS/$B_ID" ]]
[[ -f "$GENV_VERIFY_EVIDENCE/$B_ID/build.log" ]]
tmux_cmd has-session -t "=$sessionA" >/dev/null
[[ "$(cat "$legacy")" == "$A_ID" ]]
printf 'stale_current_file_ignored=yes\n'
printf 'run_A_survived_failed_launch=yes\n'

unset GENV_VERIFY_RUN || true
expect_fail cleanup_no_id "$harness" cleanup
[[ -f "$home/marker" ]]
tmux_cmd has-session -t "=$sessionA" >/dev/null
printf 'cleanup_without_id_left_A=yes\n'

"$harness" cleanup "$B_ID"
[[ -f "$home/marker" ]]
[[ -d "$GENV_VERIFY_RUNS/$A_ID" ]]
tmux_cmd has-session -t "=$sessionA" >/dev/null
[[ -f "$GENV_VERIFY_EVIDENCE/$B_ID/cleanup-ok.txt" ]]
[[ -f "$GENV_VERIFY_EVIDENCE/$B_ID/run.env" ]]
printf 'cleanup_B_left_A=yes\n'
B_ID=

printf '=== exact tmux target ===\n'
short=genv-verify-prefix-12
long=genv-verify-prefix-1234
tmux_cmd new-session -d -s "$short" -- sleep 300
tmux_cmd new-session -d -s "$long" -- sleep 300
fake_run=$GENV_VERIFY_RUNS/prefix-12
fake_ev=$GENV_VERIFY_EVIDENCE/prefix-12
mkdir -p "$fake_run/home" "$fake_ev"
cp "$ev/operator-watch.tsv" "$fake_ev/operator-watch.tsv"
chmod 600 "$fake_ev/operator-watch.tsv"
cat >"$fake_run/run.env" <<EOF
run_id='prefix-12'
session='$short'
run_dir='$fake_run'
verify_home='$fake_run/home'
xdg='$fake_run/home/.config'
bindir='$fake_run/bin'
evidence='$fake_ev'
repo='/tmp/unused'
host_class='ubuntu'
session_target=''
EOF
cp "$fake_run/run.env" "$fake_ev/run.env"
chmod 600 "$fake_run/run.env" "$fake_ev/run.env"
"$harness" cleanup prefix-12
if tmux_cmd has-session -t "=$short" >/dev/null 2>&1; then
  printf 'short session still alive\n' >&2
  exit 1
fi
tmux_cmd has-session -t "=$long" >/dev/null
printf 'prefix_long_survived=yes\n'
tmux_cmd kill-session -t "=$long"

printf '=== operator cache ===\n'
newer=$work/newer.txt
: >"$newer"
for d in "${HOME}/go" "${HOME}/.cache/go-build" "${HOME}/.config/go"; do
  [[ -e "$d" ]] || continue
  find "$d" -newer "$stamp" -print >>"$newer"
done
if [[ -s "$newer" ]]; then
  printf 'operator go cache changed:\n' >&2
  cat "$newer" >&2
  exit 1
fi
printf 'operator_go_cache_unchanged=yes\n'

export GENV_VERIFY_RUN=$A_ID
run_a=$GENV_VERIFY_RUNS/$A_ID
"$harness" cleanup "$A_ID"
A_ID=
[[ ! -d "$run_a" ]]
[[ ! -d "$home" ]]
[[ -f "$ev/cleanup-ok.txt" ]]
[[ -f "$ev/run.env" ]]
if tmux_cmd has-session -t "=$sessionA" >/dev/null 2>&1; then
  printf 'session A still alive after cleanup\n' >&2
  exit 1
fi
[[ ! -e "${HOME}/.config/genv" ]]
printf 'regress_ok\n'
