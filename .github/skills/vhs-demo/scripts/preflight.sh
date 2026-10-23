#!/bin/sh

set -eu

fail() {
	printf 'VHS demo preflight failed: %s\n' "$*" >&2
	exit 1
}

probe() {
	tool=$1
	shift

	path=$(command -v "$tool" 2>/dev/null) ||
		fail "missing required tool: $tool"
	output=$("$path" "$@" 2>&1) ||
		fail "$tool exists at $path but could not execute"
	first_line=$(printf '%s\n' "$output" | sed -n '1p')
	[ -n "$first_line" ] ||
		fail "$tool returned no version information"
	printf '%s: %s (%s)\n' "$tool" "$path" "$first_line"

	if [ "$tool" = vhs ]; then
		vhs_version=$(printf '%s\n' "$output" |
			sed -nE 's/^[^0-9]*v?([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/p' |
			sed -n '1p')
		[ -n "$vhs_version" ] ||
			fail "unable to parse VHS version from: $first_line"
		printf '%s\n' "$vhs_version" |
			awk -F. '$1 > 0 || ($1 == 0 && $2 >= 11) { ok=1 } END { exit !ok }' ||
			fail "VHS 0.11.0 or newer is required; found $vhs_version"
	fi
}

probe vhs --version
probe ffmpeg -version
probe ffprobe -version
probe ttyd --version

printf 'VHS demo prerequisites satisfied.\n'
