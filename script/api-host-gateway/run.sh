#!/usr/bin/env bash
# Entry point for the api_host black box test. Runs the test inside a Linux
# container because it needs root to bind port 443 and needs Go to honour
# SSL_CERT_FILE, which it does not do on macOS. See README.md.

set -euo pipefail

IMAGE=${GH_APIHOST_IMAGE:-golang:1.26}
MODULE_CACHE_VOLUME=gh-api-host-gateway-gomodcache
BUILD_CACHE_VOLUME=gh-api-host-gateway-gobuildcache

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

command -v docker >/dev/null 2>&1 || {
	printf 'error: docker is required\n' >&2
	exit 1
}

# The container builds gh from the working tree, not from a commit, so a dirty
# tree silently tests something other than what you think you are testing. That
# matters here more than usual: a result is only evidence if you can say which
# revision produced it.
revision=$(git -C "$repo_root" rev-parse --short HEAD 2>/dev/null || printf 'unknown')
if [ -n "$(git -C "$repo_root" status --porcelain 2>/dev/null)" ]; then
	if [ "${GH_APIHOST_ALLOW_DIRTY:-no}" != "yes" ]; then
		printf 'error: the working tree at %s is dirty\n' "$repo_root" >&2
		printf '\n' >&2
		printf 'gh is built from the working tree, so the result would not\n' >&2
		printf 'describe %s or any other revision. Check out a clean tree,\n' "$revision" >&2
		printf 'or set GH_APIHOST_ALLOW_DIRTY=yes to run anyway.\n' >&2
		exit 1
	fi
	revision="$revision-dirty"
fi
printf 'Testing gh at %s\n\n' "$revision"

# prompt_pat <what> <preset> <env name> <url> prints a fine-grained PAT on
# stdout, using the preset value when one was supplied and prompting otherwise.
#
# The tokens are written into a YAML file unquoted, so a stray control character
# makes that file unparseable and every acceptance script fails for a reason
# that has nothing to do with routing. Arrow keys at the prompt are the easy way
# to do this, because read -s accepts the escape sequence as part of the token.
prompt_pat() {
	local what="$1" value="$2" envname="$3" url="$4"

	if [ -z "$value" ]; then
		if [ ! -t 0 ]; then
			printf 'error: stdin is not a terminal; set %s instead\n' "$envname" >&2
			exit 1
		fi
		printf '\n%s\n\nCreate one at:\n  %s\n\n' "$what" "$url" >&2
		printf 'Paste the token: ' >&2
		read -rs value </dev/tty || true
		printf '\n' >&2
	fi

	if [ -z "$value" ]; then
		printf 'error: the %s is required for the acceptance run\n' "$envname" >&2
		exit 1
	fi

	case $value in
	*[![:print:]]*)
		printf 'error: the token pasted for %s contains non-printable characters\n' "$envname" >&2
		printf '\n' >&2
		printf 'Arrow keys and other control characters are captured verbatim at\n' >&2
		printf 'the prompt. Paste the token without editing it, or set %s\n' "$envname" >&2
		printf 'in the environment instead.\n' >&2
		exit 1
		;;
	esac

	printf '%s' "$value"
}

# Outside the acceptance run only phases 1 to 3 happen, and they read public
# data as whoever is logged in, so an ordinary OAuth token will do.
token=${GH_TOKEN:-}
if [ -z "$token" ]; then
	token=$(gh auth token --hostname github.com) || {
		printf 'error: set GH_TOKEN or run gh auth login for github.com\n' >&2
		exit 1
	}
fi

org_token=
if [ "${GH_APIHOST_ACCEPTANCE:-no}" = "yes" ]; then
	# A fine-grained PAT has exactly one resource owner, and the two kinds of
	# permission are mutually exclusive: Organization permissions require an
	# organization owner, Account permissions require a user owner. The
	# acceptance scripts need both, so they need two tokens, one per owner.
	login=${GH_APIHOST_EXPECTED_LOGIN:-}
	if [ -z "$login" ]; then
		login=$(GH_TOKEN="$token" gh api user --jq .login) || {
			printf 'error: could not determine the authenticated login\n' >&2
			exit 1
		}
	fi

	org=${GH_APIHOST_ORG:-}
	[ -n "$org" ] || {
		printf 'error: GH_APIHOST_ORG is required for the acceptance run\n' >&2
		exit 1
	}

	org_token=$(prompt_pat \
		"The org-scoped scripts need a fine-grained PAT owned by $org, scoped to
all repositories because the scripts create repositories with random
names that cannot be listed ahead of time." \
		"${GH_APIHOST_ORG_TOKEN:-}" GH_APIHOST_ORG_TOKEN \
		"https://github.com/settings/personal-access-tokens/new?name=gh-acceptance-api-host-org&description=Acceptance+subset+for+the+api_host+gateway+loop&target_name=$org&expires_in=7&administration=write&contents=write&actions=write&workflows=write&issues=write&pull_requests=write&secrets=write&actions_variables=write&environments=write&discussions=write&organization_secrets=write&organization_actions_variables=write&organization_projects=write")

	# Gists and account keys belong to an account rather than to an organization,
	# so they need a token whose resource owner is the user. This token also
	# serves phases 1 to 3, which read cli/cli, hence read access to contents.
	#
	# The permission slugs come from the X-Accepted-GitHub-Permissions response
	# header, which GitHub returns for fine-grained tokens. Account SSH keys are
	# "keys", which is not what the form's "Git SSH keys" label suggests.
	token=$(prompt_pat \
		"The user-scoped scripts need a fine-grained PAT owned by $login, because
gists belong to an account rather than to an organization." \
		"${GH_APIHOST_USER_TOKEN:-}" GH_APIHOST_USER_TOKEN \
		"https://github.com/settings/personal-access-tokens/new?name=gh-acceptance-api-host-user&description=Acceptance+subset+for+the+api_host+gateway+loop&target_name=$login&expires_in=7&contents=read&gists=write&gpg_keys=write&keys=write")
fi

# A go.mod replace pointing at a directory on this machine is invisible inside
# the container unless we mount it, so collect any such directories and bind
# them at the same absolute path the replace names. There are none once the
# temporary go-gh replace is reverted, and none at older revisions.
replace_mounts=()
while read -r target; do
	case "$target" in
	/*) ;;
	*) continue ;;
	esac
	[ -d "$target" ] || {
		printf 'error: go.mod replaces a module with %s, which does not exist\n' "$target" >&2
		exit 1
	}
	replace_mounts+=(-v "$target:$target:ro")
done < <(awk '/^replace|=>/ { for (i = 1; i < NF; i++) if ($i == "=>") print $(i + 1) }' "$repo_root/go.mod")

docker volume create "$MODULE_CACHE_VOLUME" >/dev/null
docker volume create "$BUILD_CACHE_VOLUME" >/dev/null

exec docker run --rm -t \
	-v "$repo_root:/src:ro" \
	${replace_mounts[@]+"${replace_mounts[@]}"} \
	-v "$MODULE_CACHE_VOLUME:/go/pkg/mod" \
	-v "$BUILD_CACHE_VOLUME:/root/.cache/go-build" \
	-e GH_APIHOST_TOKEN="$token" \
	-e GH_APIHOST_ORG_TOKEN="$org_token" \
	-e GH_APIHOST_EXPECTED_LOGIN="${GH_APIHOST_EXPECTED_LOGIN:-}" \
	-e GH_APIHOST_ORG="${GH_APIHOST_ORG:-}" \
	-e GH_APIHOST_ACCEPTANCE="${GH_APIHOST_ACCEPTANCE:-no}" \
	-e GH_APIHOST_ALL="${GH_APIHOST_ALL:-no}" \
	-e GH_APIHOST_FUNCS="${GH_APIHOST_FUNCS:-}" \
	-w /src \
	"$IMAGE" \
	/src/script/api-host-gateway/test.sh
