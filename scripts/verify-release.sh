#!/usr/bin/env bash
set -euo pipefail

fail() { echo "Release verification failed: $*" >&2; exit 1; }

[[ "${GITHUB_EVENT_NAME:-}" == workflow_dispatch ]] || fail 'manual dispatch required'
[[ "${GITHUB_REF:-}" == refs/heads/main ]] || fail 'workflow must run from main'
[[ "${GITHUB_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'invalid workflow commit'
[[ "${RELEASE_TAG:-}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail 'expected stable vMAJOR.MINOR.PATCH without leading zeroes'
[[ "${EXPECTED_RELEASE_SHA:-$GITHUB_SHA}" == "$GITHUB_SHA" ]] || fail 'validated commit changed'

# All code and workflow instructions come from the same immutable main commit.
# Fetch the exact ref, not a revision expression supplied by the caller. Peeling
# accepts lightweight and annotated tags, but rejects tags pointing to a tree/blob.
git fetch --no-tags origin "+refs/heads/main:refs/remotes/origin/main" "+refs/tags/$RELEASE_TAG:refs/tags/$RELEASE_TAG"
main_sha="$(git rev-parse --verify refs/remotes/origin/main^{commit})"
tag_sha="$(git rev-parse --verify "refs/tags/$RELEASE_TAG^{commit}")"
[[ "$main_sha" == "$GITHUB_SHA" ]] || fail 'workflow commit is no longer the main tip'
[[ "$tag_sha" == "$GITHUB_SHA" ]] || fail 'tag does not point to the exact main tip'
[[ "$(git rev-parse HEAD)" == "$GITHUB_SHA" ]] || fail 'checkout differs from workflow commit'

# A check in tag-owned YAML cannot protect itself. Require the separately
# installed server-side tag policy as well as these trusted-main checks.
python3 scripts/release-tag-policy.py --check-public

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  printf 'sha=%s\ntag=%s\nversion=%s\n' "$GITHUB_SHA" "$RELEASE_TAG" "${RELEASE_TAG#v}" >> "$GITHUB_OUTPUT"
fi
echo "Verified $RELEASE_TAG at $GITHUB_SHA"
