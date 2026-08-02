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
		if ssh-keyscan -4 -H aur.archlinux.org >> ~/.ssh/known_hosts 2>/dev/null; then
			break
		fi
		if (( attempt == 5 )); then
			echo "aur-common: ssh-keyscan failed after ${attempt} attempts" >&2
			return 1
		fi
		sleep $((attempt * 3))
		attempt=$((attempt + 1))
	done
	# Force IPv4: some CI networks reach AUR over IPv6 and get the
	# "down due to maintenance" banner while IPv4 SSH is healthy.
	export GIT_SSH_COMMAND="ssh -4 -i ~/.ssh/aur -o AddressFamily=inet -o StrictHostKeyChecking=yes -o ConnectTimeout=30"
}

# Clone an AUR package repo with retries (aur.archlinux.org often drops SSH).
aur_clone() {
	local pkg="$1"
	local dest="$2"
	local attempt=1
	local delay=5
	while (( attempt <= 6 )); do
		rm -rf "${dest}"
		# Prefer HTTPS for the read-only clone (works during partial SSH outages),
		# then rewrite origin to SSH so aur_push can authenticate with AUR_KEY.
		if git clone "https://aur.archlinux.org/${pkg}.git" "${dest}"; then
			git -C "${dest}" remote set-url origin "ssh://aur@aur.archlinux.org/${pkg}.git"
			return 0
		fi
		echo "aur-common: HTTPS clone ${pkg} attempt ${attempt} failed; trying SSH..." >&2
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

# Portable in-place sed (GNU sed vs BSD sed on macOS runners).
aur_sed_inplace() {
	local file="${!#}"
	local args=("${@:1:$#-1}")
	if sed --version >/dev/null 2>&1; then
		sed -i "${args[@]}" "$file"
	else
		sed -i '' "${args[@]}" "$file"
	fi
}

# Portable sha256 of a file (Linux sha256sum vs macOS shasum).
aur_sha256_file() {
	local file="$1"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{print $1}'
	else
		shasum -a 256 "$file" | awk '{print $1}'
	fi
}
