#!/usr/bin/env bash
# Fail if total statement coverage is below COVER_MIN (default 80).
set -euo pipefail

COVER_PROFILE="${1:-coverage.out}"
COVER_MIN="${COVER_MIN:-80}"

if [[ ! -f "$COVER_PROFILE" ]]; then
	echo "cover-gate: missing coverage profile: $COVER_PROFILE" >&2
	exit 1
fi

pct="$(go tool cover -func="$COVER_PROFILE" | awk '/^total:/ { gsub(/%/, "", $NF); print $NF }')"
if [[ -z "$pct" ]]; then
	echo "cover-gate: could not parse total coverage from $COVER_PROFILE" >&2
	exit 1
fi

awk -v pct="$pct" -v min="$COVER_MIN" 'BEGIN {
	if ((pct + 0) < (min + 0)) {
		printf "FAIL: total coverage %.1f%% is below floor %s%%\n", pct, min > "/dev/stderr"
		exit 1
	}
	printf "cover-gate: total coverage %.1f%% (floor %s%%)\n", pct, min
}'
