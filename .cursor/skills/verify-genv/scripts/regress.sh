#!/usr/bin/env bash
# Proves private roots, a non-executed run.env, distro-neutral targets,
# capture races, reserved names, strict run ids, and process-group cleanup.
set -euo pipefail

here=$(CDPATH='' cd "$(dirname "$0")" && pwd)
harness=$here/verify-genv
work=$(mktemp -d /tmp/genv-verify-regress.XXXXXX)
legacy=/tmp/genv-verify-current
legacy_existed=0
legacy_backup=
A_ID=
B_ID=
BG_PID=
SWAP_PID=
ENV_BACKUP=
PROC_BACKUP=
FOREIGN_DIR=
PREFIX_ID=20260101T000000Z-12
PREFIX_LONG_ID=20260101T000000Z-1234

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
  if [[ -n "$SWAP_PID" ]]; then
    kill -TERM "$SWAP_PID" 2>/dev/null || true
    sleep 0.1
    kill -KILL "$SWAP_PID" 2>/dev/null || true
  fi
  if [[ -n "$ENV_BACKUP" && -n "$A_ID" && -f "$ENV_BACKUP" && -f "$GENV_VERIFY_RUNS/$A_ID/run.env" ]]; then
    cp "$ENV_BACKUP" "$GENV_VERIFY_RUNS/$A_ID/run.env" || true
  fi
  if [[ -n "$PROC_BACKUP" && -n "$A_ID" && -f "$PROC_BACKUP" && -d "$GENV_VERIFY_RUNS/$A_ID" ]]; then
    cp "$PROC_BACKUP" "$GENV_VERIFY_RUNS/$A_ID/run.proc" || true
  fi
  if [[ -n "$A_ID" ]]; then
    "$harness" cleanup "$A_ID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$B_ID" ]]; then
    "$harness" cleanup "$B_ID" >/dev/null 2>&1 || true
  fi
  if [[ -n "$BG_PID" ]]; then
    kill -TERM "$BG_PID" 2>/dev/null || true
  fi
  tmux_cmd kill-session -t "=genv-verify-$PREFIX_ID" >/dev/null 2>&1 || true
  tmux_cmd kill-session -t "=genv-verify-$PREFIX_LONG_ID" >/dev/null 2>&1 || true
  if [[ -n "$FOREIGN_DIR" && -d "$FOREIGN_DIR" ]]; then
    sudo -n chown "$(id -u)" "$FOREIGN_DIR" 2>/dev/null || true
  fi
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

mkdir -m 700 "$work/runs" "$work/evidence"
export GENV_VERIFY_RUNS=$work/runs
export GENV_VERIFY_EVIDENCE=$work/evidence
unset GENV_VERIFY_RUN || true
unset GENV_VERIFY_GOCACHE || true
unset GENV_VERIFY_GOMODCACHE || true

if [[ -e "${HOME}/.config/genv" ]]; then
  printf 'operator config already exists at %s\n' "${HOME}/.config/genv" >&2
  exit 1
fi

if grep -E -q '^[[:space:]]*source[[:space:]]' "$harness"; then
  printf 'harness sources a file\n' >&2
  exit 1
fi
# The dollar is the literal text of the removed test.
# shellcheck disable=SC2016
if grep -F -q '[[ -L "$src_real" ]]' "$harness"; then
  printf 'dead symlink check remains\n' >&2
  exit 1
fi
printf 'run_env_not_sourced=yes\n'
printf 'dead_symlink_check_removed=yes\n'

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

printf '=== private roots ===\n'
world=$work/world
mkdir "$world"
chmod 0777 "$world"
expect_fail world_writable env GENV_VERIFY_RUNS="$world" "$harness" launch
require world_mode "$(stat -c %a "$world")" 777
[[ -z "$(find "$world" -mindepth 1 -print)" ]]
printf 'world_writable_not_chmodded=yes\n'

ln -s "$GENV_VERIFY_RUNS" "$work/runs-link"
expect_fail symlink_root env GENV_VERIFY_RUNS="$work/runs-link" "$harness" launch
printf 'symlink_root_refused=yes\n'

expect_fail missing_root env GENV_VERIFY_RUNS="$work/no-such-runs" "$harness" launch
[[ ! -e "$work/no-such-runs" ]]
printf 'missing_root_not_created=yes\n'

if ! sudo -n true >/dev/null 2>&1; then
  printf 'sudo is required to prove a foreign-owned root is refused\n' >&2
  exit 1
fi
FOREIGN_DIR=$work/foreign
mkdir -m 700 "$FOREIGN_DIR"
sudo -n chown nobody "$FOREIGN_DIR"
expect_fail foreign_owner env GENV_VERIFY_RUNS="$FOREIGN_DIR" "$harness" launch
sudo -n chown "$(id -u)" "$FOREIGN_DIR"
printf 'foreign_root_refused=yes\n'

printf '=== classify debian ===\n'
printf '%s\n' 'ID=debian' 'VERSION_ID="13"' >"$work/debian-os"
printf '%s\n' 'Linux version 6.1.0-1' >"$work/proc-plain"
"$harness" classify "$work/debian-os" "$work/proc-plain" Linux | tee "$work/debian.class"
grep -qx 'host_class=' "$work/debian.class"
grep -qx 'session_target=linux' "$work/debian.class"
printf 'debian_fixture_session_target=linux\n'

printf '=== classify ubuntu ===\n'
printf '%s\n' 'ID=ubuntu' 'ID_LIKE=debian' >"$work/ubuntu-os"
"$harness" classify "$work/ubuntu-os" "$work/proc-plain" Linux | tee "$work/ubuntu.class"
grep -qx 'host_class=ubuntu' "$work/ubuntu.class"
grep -qx 'session_target=' "$work/ubuntu.class"
printf 'ubuntu_fixture_session_target=\n'

printf '=== classify wsl-arch ===\n'
printf '%s\n' 'ID=arch' >"$work/arch-os"
printf '%s\n' 'Linux version 6.6.0-microsoft-standard-WSL2' >"$work/proc-wsl"
"$harness" classify "$work/arch-os" "$work/proc-wsl" Linux | tee "$work/wsl.class"
grep -qx 'host_class=wsl-arch' "$work/wsl.class"
grep -qx 'session_target=' "$work/wsl.class"

printf '=== launch A ===\n'
"$harness" launch | tee "$work/launchA.out"
A_ID=$(sed -n 's/^run_id=//p' "$work/launchA.out")
[[ -n "$A_ID" ]]
"$harness" classify /etc/os-release /proc/version "$(uname -s)" | tee "$work/live.class"
live_target=$(sed -n 's/^session_target=//p' "$work/live.class")
got_target=$(sed -n 's/^session_target=//p' "$work/launchA.out")
require launch_session_target "$got_target" "$live_target"
printf 'live_host_class=%s\n' "$(sed -n 's/^host_class=//p' "$work/live.class")"
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
grep -q '^go_telemetry=off$' "$ev/build.log"
# The mode file is "off <date>" and has no trailing newline. read would
# return 1 at EOF, so take the first field with awk.
telemetry_word=$(awk '{print $1; exit}' "$home/.config/go/telemetry/mode")
require telemetry_mode "$telemetry_word" off
printf 'build_home_is_throwaway=yes\n'
printf 'gocache_is_run_local=yes\n'
printf 'telemetry_off_in_throwaway=yes\n'

printf '=== run.env is data ===\n'
ENV_BACKUP=$work/run.env.good
cp "$GENV_VERIFY_RUNS/$A_ID/run.env" "$ENV_BACKUP"
printf 'verify_home=/etc\n' >>"$GENV_VERIFY_RUNS/$A_ID/run.env"
require paths_ignore_run_env "$("$harness" paths home)" "$home"
pwn=$work/PWNED-as-victim
{
  printf 'id > %s\n' "$pwn"
  # shellcheck disable=SC2016
  printf 'run_id=$(id)\n'
} >"$GENV_VERIFY_RUNS/$A_ID/run.env"
expect_fail malicious_env "$harness" paths run-id
[[ ! -e "$pwn" ]]
printf 'malicious_run_env_not_executed=yes\n'
cp "$ENV_BACKUP" "$GENV_VERIFY_RUNS/$A_ID/run.env"

printf '=== strict run id ===\n'
expect_fail cleanup_dot "$harness" cleanup .
expect_fail cleanup_dotdot "$harness" cleanup ..
expect_fail run_dot env GENV_VERIFY_RUN=. "$harness" paths home
expect_fail run_dotdot env GENV_VERIFY_RUN=.. "$harness" paths home
[[ -d "$home" ]]
printf 'dot_run_ids_refused=yes\n'

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
watch_sum=$(cksum "$ev/operator-watch.tsv")
expect_fail reserved_name "$harness" capture operator-watch.tsv "$home/note.txt"
require watch_unchanged "$(cksum "$ev/operator-watch.tsv")" "$watch_sum"
printf 'reserved_capture_refused=yes\n'
if grep -R -q -- 'SECRET-OPERATOR-FILE' "$ev"; then
  printf 'capture leaked operator file contents\n' >&2
  exit 1
fi
printf 'capture_secret_absent=yes\n'

printf '=== capture swap ===\n'
mkdir -p "$home/race/realbox" "$work/outbox"
echo SAFE >"$home/race/realbox/file.txt"
echo SECRET-SWAP >"$work/outbox/file.txt"
ln -s "$work/outbox" "$home/race/linkbox"
mv "$home/race/realbox" "$home/race/swap"
python3 - "$home/race/swap" "$home/race/linkbox" <<'PY' &
import ctypes, os, sys, time
libc = ctypes.CDLL("libc.so.6", use_errno=True)
libc.renameat2.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
libc.renameat2.restype = ctypes.c_int
a = os.fsencode(sys.argv[1])
b = os.fsencode(sys.argv[2])
while True:
    libc.renameat2(-100, a, -100, b, 2)
    time.sleep(0.001)
PY
SWAP_PID=$!
sleep 0.2
safe=0
leaks=0
refused=0
i=0
while [[ "$i" -lt 80 ]]; do
  i=$((i + 1))
  if "$harness" capture "swap-$i.txt" "$home/race/swap/file.txt" >"$work/swap-one.out" 2>&1; then
    if grep -q 'SECRET-SWAP' "$ev/swap-$i.txt"; then
      leaks=$((leaks + 1))
    elif grep -q 'SAFE' "$ev/swap-$i.txt"; then
      safe=$((safe + 1))
    fi
  else
    refused=$((refused + 1))
  fi
done
kill -TERM "$SWAP_PID" 2>/dev/null || true
wait "$SWAP_PID" 2>/dev/null || true
SWAP_PID=
printf 'swap_safe=%s\n' "$safe"
printf 'swap_refused=%s\n' "$refused"
printf 'swap_leaks=%s\n' "$leaks"
[[ "$leaks" -eq 0 ]]
[[ "$safe" -ge 1 ]]
[[ "$refused" -ge 1 ]]
if grep -R -q -- 'SECRET-SWAP' "$ev"; then
  printf 'capture swap leaked the operator file\n' >&2
  exit 1
fi
printf 'capture_swap_contained=yes\n'
rm -rf "$home/race"

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
short=genv-verify-$PREFIX_ID
long=genv-verify-$PREFIX_LONG_ID
tmux_cmd new-session -d -s "$short" -- sleep 300
tmux_cmd new-session -d -s "$long" -- sleep 300
fake_run=$GENV_VERIFY_RUNS/$PREFIX_ID
fake_ev=$GENV_VERIFY_EVIDENCE/$PREFIX_ID
mkdir -p "$fake_run/home" "$fake_ev"
cp "$ev/operator-watch.tsv" "$fake_ev/operator-watch.tsv"
chmod 600 "$fake_ev/operator-watch.tsv"
cat >"$fake_run/run.env" <<EOF
run_id=$PREFIX_ID
session=$short
host_class=ubuntu
session_target=
verify_home=/etc
evidence=/tmp
EOF
if ps -p 999999 >/dev/null 2>&1; then
  printf 'pid 999999 is alive; refusing to use it as a dead pid\n' >&2
  exit 1
fi
cat >"$fake_run/run.proc" <<EOF
pid=999999
pgid=999999
user=$(id -un)
identity=not-running
EOF
cp "$fake_run/run.env" "$fake_ev/run.env"
chmod 600 "$fake_run/run.env" "$fake_ev/run.env" "$fake_run/run.proc"
"$harness" cleanup "$PREFIX_ID"
if tmux_cmd has-session -t "=$short" >/dev/null 2>&1; then
  printf 'short session still alive\n' >&2
  exit 1
fi
tmux_cmd has-session -t "=$long" >/dev/null
printf 'prefix_long_survived=yes\n'
printf 'dead_pid_not_mismatch=yes\n'
tmux_cmd kill-session -t "=$long"

printf '=== background process ===\n'
export GENV_VERIFY_RUN=$A_ID
"$harness" drive -- 'nohup sleep 777 >/dev/null 2>&1 & echo BG:$!' >/dev/null
pane=$(tmux_cmd capture-pane -J -p -t "=$sessionA:" -S -)
BG_PID=$(printf '%s\n' "$pane" | sed -n 's/^BG://p' | tail -n 1 | tr -d '[:space:]')
[[ "$BG_PID" =~ ^[0-9]+$ ]]
shell_pid=$(tmux_cmd display-message -p -t "=$sessionA:" '#{pane_pid}')
shell_pgid=$(ps -o pgid= -p "$shell_pid" | tr -d '[:space:]')
bg_pgid=$(ps -o pgid= -p "$BG_PID" | tr -d '[:space:]')
require background_pgid "$bg_pgid" "$shell_pgid"
ps -p "$BG_PID" >/dev/null
PROC_BACKUP=$work/run.proc.good
cp "$GENV_VERIFY_RUNS/$A_ID/run.proc" "$PROC_BACKUP"
sed 's/^identity=.*/identity=not-this-process/' "$PROC_BACKUP" >"$GENV_VERIFY_RUNS/$A_ID/run.proc"
expect_fail identity_mismatch "$harness" cleanup "$A_ID"
ps -p "$BG_PID" >/dev/null
tmux_cmd has-session -t "=$sessionA" >/dev/null
[[ -d "$home" ]]
[[ -d "$GENV_VERIFY_RUNS/$A_ID" ]]
printf 'identity_mismatch_kept_state=yes\n'
cp "$PROC_BACKUP" "$GENV_VERIFY_RUNS/$A_ID/run.proc"

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
if ps -p "$BG_PID" >/dev/null 2>&1; then
  printf 'background sleep survived cleanup\n' >&2
  exit 1
fi
BG_PID=
[[ ! -e "${HOME}/.config/genv" ]]
printf 'background_process_stopped=yes\n'
printf 'regress_ok\n'
