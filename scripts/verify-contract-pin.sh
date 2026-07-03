#!/usr/bin/env bash
#
# Verify the vendored mlsgrid-sync schema migration against its pin. This makes
# the cross-repo contract test honest: the integration suite builds its fixture
# DB from a copy of mlsgrid-sync's migration vendored at a pinned commit, and
# this check proves that copy still matches upstream and hasn't drifted.
#
# Two independent assertions:
#   1. Local self-consistency — the vendored file's git blob sha1 equals the one
#      recorded in PIN (catches editing the file without updating the pin, or
#      vice versa; needs no network).
#   2. Upstream honesty — the file fetched from mlsgrid-sync at the pinned commit
#      is byte-identical to the vendored copy (catches a moved tag or a stale
#      vendor).
#
# All coordinates come from the PIN file, the single source of truth.
set -euo pipefail

pin="adapters/postgres/testdata/contract/PIN"
vendored="adapters/postgres/testdata/contract/0001_init.sql"

commit=$(awk '/^[[:space:]]+commit[[:space:]]/{print $2}' "$pin")
file=$(awk '/^[[:space:]]+file[[:space:]]/{print $2}' "$pin")
blob=$(awk '/^[[:space:]]+blob sha1[[:space:]]/{print $3}' "$pin")
repo=$(awk '/^[[:space:]]+repo[[:space:]]/{print $2}' "$pin")
slug=${repo#github.com/}

if [ -z "$commit" ] || [ -z "$file" ] || [ -z "$blob" ] || [ -z "$slug" ]; then
	echo "FAIL: could not parse pin coordinates from $pin" >&2
	exit 1
fi

echo "contract pin: $repo@$commit"
echo "  file: $file"
echo "  blob: $blob"

# 1) The vendored file matches the blob sha1 recorded in PIN.
have=$(git hash-object "$vendored")
if [ "$have" != "$blob" ]; then
	echo "FAIL: vendored copy hash $have != PIN blob sha1 $blob" >&2
	echo "      the vendored migration and PIN disagree — update one deliberately." >&2
	exit 1
fi
echo "ok: vendored copy matches the recorded blob sha1"

# 2) The pinned upstream commit really contains this exact file.
url="https://raw.githubusercontent.com/$slug/$commit/$file"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
if ! curl -fsSL "$url" -o "$tmp"; then
	echo "FAIL: could not fetch $url" >&2
	exit 1
fi
upstream=$(git hash-object "$tmp")
if [ "$upstream" != "$blob" ]; then
	echo "FAIL: upstream $slug@$commit blob $upstream != pinned $blob" >&2
	echo "      the pinned commit/file changed — re-vendor and update PIN." >&2
	diff -u "$vendored" "$tmp" || true
	exit 1
fi
echo "ok: upstream $slug@$commit is byte-identical to the vendored copy"
echo "contract pin verified."
