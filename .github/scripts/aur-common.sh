#!/usr/bin/env bash
# Shared helpers for AUR publish scripts.
# shellcheck shell=bash

aur_setup_ssh() {
	mkdir -p ~/.ssh
	printf '%s\n' "${AUR_KEY}" > ~/.ssh/aur
	chmod 600 ~/.ssh/aur
	# Refresh host keys; tolerate transient ssh-keyscan failures.
	local attempt=1
	while (( attempt <= 5 )); do
		if ssh-keyscan -H aur.archlinux.org >> ~/.ssh/known_hosts 2>/dev/null; then
			break
		fi
		if (( attempt == 5 )); then
			echo "aur-common: ssh-keyscan failed after ${attempt} attempts" >&2
			return 1
		fi
		sleep $((attempt * 3))
		attempt=$((attempt + 1))
	done
	export GIT_SSH_COMMAND="ssh -i ~/.ssh/aur -o StrictHostKeyChecking=yes -o ConnectTimeout=30"
}

# Clone an AUR package repo with retries (aur.archlinux.org often drops SSH).
aur_clone() {
	local pkg="$1"
	local dest="$2"
	local attempt=1
	local delay=5
	while (( attempt <= 6 )); do
		rm -rf "${dest}"
		if git clone "ssh://aur@aur.archlinux.org/${pkg}.git" "${dest}"; then
			return 0
		fi
		echo "aur-common: clone ${pkg} attempt ${attempt} failed; retrying in ${delay}s..." >&2
		sleep "${delay}"
		delay=$((delay * 2))
		attempt=$((attempt + 1))
	done
	echo "aur-common: clone ${pkg} failed after retries" >&2
	return 1
}

aur_push() {
	local attempt=1
	local delay=5
	while (( attempt <= 6 )); do
		if git push origin master; then
			return 0
		fi
		echo "aur-common: push attempt ${attempt} failed; retrying in ${delay}s..." >&2
		sleep "${delay}"
		delay=$((delay * 2))
		attempt=$((attempt + 1))
	done
	echo "aur-common: push failed after retries" >&2
	return 1
}
