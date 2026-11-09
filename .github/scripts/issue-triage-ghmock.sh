#!/usr/bin/env bash
# Prepares ghmock for the issue-triage agent, which runs gh builds against it
# to reproduce bug reports. It builds ghmock and gh from the selected checkout,
# downloads the latest gh release and each release the issue reports,
# and downloads GitHub's schemas.
#
# Everything goes under GHMOCK_ROOT. The agent's sandbox mounts /opt read-only,
# so nothing the agent runs, including a gh alias or extension, can replace a
# build or ghmock itself.
#
# Builds land in $GHMOCK_ROOT/builds/<name>/gh:
#   source    built from the selected checkout, which may be a feature branch
#   release   the latest release
#   X.Y.Z     each version the issue reports, such as "gh version X.Y.Z"
# Verified executable paths and version lines are recorded in $GHMOCK_ROOT/builds.txt.
#
# Reproducing is an extra for triage, so this script never fails the job.
# Whatever it can't prepare is left out, and the agent says what's missing.
#
# Environment:
#   GHMOCK_ROOT   where to install; defaults to /opt/ghmock
#   GHMOCK_GRAPHQL_SCHEMA, GHMOCK_REST_SCHEMA
#                 where to put GitHub's schemas, which is where the agent's
#                 ghmock reads them; default to files in $GHMOCK_ROOT/schemas
#   GH_TOKEN      token for reading cli/cli releases and the issue
#   REPO          the repository the issue is in
#   ISSUE_NUMBER  the issue being triaged
#   ISSUE_BODY    the issue body, when the event includes it. It's untrusted,
#                 so only version numbers matching a strict pattern are read.

set -uo pipefail

root="${GHMOCK_ROOT:-/opt/ghmock}"
# The toolchain in go.mod can be newer than the runner's Go.
export GOTOOLCHAIN=auto

warn() {
	echo "::warning::$*"
}

# step runs a command and warns if it fails, naming what the agent will miss.
step() {
	local what=$1
	shift
	if ! "$@"; then
		warn "$what failed, so the triage agent can't use it this run"
		return 1
	fi
}

prepare_root() {
	if ! mkdir -p "$root" 2>/dev/null; then
		sudo mkdir -p "$root" && sudo chown "$(id -u):$(id -g)" "$root" || return 1
	fi
	mkdir -p "$root/bin" "$root/builds"
}

# build_ghmock builds ghmock to run only the builds here, so ghmock run --gh
# can't name another program. It's not a sandbox; see the bash comment in
# issue-triage.md.
build_ghmock() {
	(cd script/ghmock && go build -ldflags "-X main.buildsDir=$root/builds" -o "$root/bin/ghmock" .)
}

# download_schemas downloads GitHub's schemas to the paths the agent's ghmock
# reads them from.
download_schemas() {
	GHMOCK_GRAPHQL_SCHEMA="${GHMOCK_GRAPHQL_SCHEMA:-$root/schemas/schema.docs.graphql}" \
		GHMOCK_REST_SCHEMA="${GHMOCK_REST_SCHEMA:-$root/schemas/api.github.com.json}" \
		"$root/bin/ghmock" schemas update
}

build_source() {
	local sha
	sha=$(git rev-parse --short HEAD) || return 1
	go build -trimpath -ldflags "-X github.com/cli/cli/v2/internal/build.Version=source-$sha" \
		-o "$root/builds/source/gh" ./cmd/gh
}

# download_release installs a published cli/cli release, checked against the
# release's checksums file.
download_release() {
	local version=$1
	local dir="$root/builds/$version"
	if [ -x "$dir/gh" ]; then
		return 0
	fi
	# A draft, or a version that was never released, stops here.
	if [ "$(gh release view "v$version" -R cli/cli --json isDraft -q .isDraft 2>/dev/null)" != "false" ]; then
		echo "gh $version isn't a published cli/cli release, so it's left out"
		return 1
	fi

	local tmp asset="gh_${version}_linux_amd64.tar.gz"
	tmp=$(mktemp -d) || return 1
	gh release download "v$version" -R cli/cli -D "$tmp" -p "$asset" -p "gh_${version}_checksums.txt" &&
		(cd "$tmp" && sha256sum --check --ignore-missing --strict "gh_${version}_checksums.txt") &&
		tar -xzf "$tmp/$asset" -C "$tmp" &&
		mkdir -p "$dir" &&
		mv "$tmp/gh_${version}_linux_amd64/bin/gh" "$dir/gh"
	local status=$?
	rm -rf "$tmp"
	return $status
}

# reported_versions prints up to 3 distinct versions that follow "gh" in the
# issue body: "gh version X.Y.Z", the format the bug report template asks for,
# and looser forms such as "`gh` X.Y.Z" or "gh vX.Y.Z". A match that isn't a
# published release, such as an extension's version, is dropped later.
reported_versions() {
	grep -oE '(^|[^[:alnum:]_-])gh[^[:alnum:]_-][^0-9]{0,11}[0-9]+\.[0-9]+\.[0-9]+' <<<"$1" |
		grep -oE '[0-9]+\.[0-9]+\.[0-9]+$' | awk '!seen[$0]++' | head -n 3
}

issue_body() {
	if [ -n "${ISSUE_BODY:-}" ]; then
		printf '%s' "$ISSUE_BODY"
	elif [ -n "${ISSUE_NUMBER:-}" ] && [ -n "${REPO:-}" ]; then
		# Manual runs have no issue in the event.
		gh issue view "$ISSUE_NUMBER" -R "$REPO" --json body -q .body
	fi
}

# Direct build execution isn't allowlisted in the agent's sandbox, so verify
# versions here rather than making the agent discover them.
record_builds() {
	local inventory binary version
	inventory=$(mktemp "$root/builds.txt.XXXXXX") || return 1
	for binary in "$root"/builds/*/gh; do
		[ -x "$binary" ] || continue
		if version=$("$binary" version) && [[ "$version" =~ ^gh\ version\ [^[:space:]]+ ]]; then
			if ! printf '%s\n%s\n\n' "$binary" "${version%%$'\n'*}" >>"$inventory"; then
				rm -f "$inventory"
				return 1
			fi
		else
			warn "Reading the version of $binary failed, so it isn't listed in the build inventory"
		fi
	done
	if [ ! -s "$inventory" ]; then
		if ! printf '%s\n' "No verified gh builds are available." >"$inventory"; then
			rm -f "$inventory"
			return 1
		fi
	fi
	if ! chmod 644 "$inventory" || [ -d "$root/builds.txt" ] || ! mv "$inventory" "$root/builds.txt"; then
		rm -f "$inventory"
		return 1
	fi
	echo "Verified builds for the triage agent:"
	cat "$root/builds.txt"
}

main() {
	step "Creating $root" prepare_root || return 0
	# Without ghmock, nothing else is any use.
	step "Building ghmock" build_ghmock || return 0
	if [ -n "${GITHUB_PATH:-}" ]; then
		echo "$root/bin" >>"$GITHUB_PATH"
	fi

	step "Downloading GitHub's schemas" download_schemas
	step "Building gh from the selected checkout" build_source

	local latest
	if latest=$(gh release view -R cli/cli --json tagName -q .tagName) &&
		step "Downloading the latest release, $latest" download_release "${latest#v}"; then
		ln -sfn "${latest#v}" "$root/builds/release"
	fi

	local body version
	body=$(issue_body) || warn "Reading the issue failed, so only the selected source and latest release are available"
	for version in $(reported_versions "$body"); do
		step "Downloading gh $version" download_release "$version"
	done
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main
	step "Recording prepared gh build versions" record_builds || exit 0
fi
