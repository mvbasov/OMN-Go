#!/bin/sh
# Get the current F-Droid recipe of OMN-Go from fdroiddata.
# Usage: sh local/fdroid_fetch_metadata.sh [output file]
# The default output file is metadata/net.basov.omngo.fdroid.yml.
#
# The script:
#   1. Downloads the recipe with wget. It tries GitLab first, because
#      fdroiddata lives there. When GitLab fails, it tries the GitHub mirror,
#      which can be some minutes older.
#   2. Stops with an error when the download is not a recipe, for example
#      an HTML error page.
#   3. Replaces the output file and shows the difference.
#
# After a change, run the tests. The tests compare the last build block of
# the recipe with the Dockerfile and the build scripts.
set -eu
cd "$(dirname "$0")/.."

NAME=net.basov.omngo.fdroid.yml
OUT=${1:-metadata/$NAME}
URLS="https://gitlab.com/fdroid/fdroiddata/-/raw/master/metadata/$NAME
https://raw.githubusercontent.com/f-droid/fdroiddata/master/metadata/$NAME"

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

SOURCE=
for url in $URLS; do
	if wget -q -T 30 -O "$TMP" "$url" && grep -q '^UpdateCheckMode:' "$TMP"; then
		SOURCE=$url
		break
	fi
	echo "Cannot get the recipe from $url" >&2
done
[ -n "$SOURCE" ] || { echo "No source gave the recipe" >&2; exit 1; }

VERSION=$(sed -n 's/^CurrentVersion: *//p' "$TMP")
CODE=$(sed -n 's/^CurrentVersionCode: *//p' "$TMP")
echo "Source: $SOURCE"
echo "F-Droid version: $VERSION ($CODE)"

if [ -f "$OUT" ] && cmp -s "$TMP" "$OUT"; then
	echo "$OUT has no change"
	exit 0
fi

mkdir -p "$(dirname "$OUT")"
cp "$TMP" "$OUT"
echo "$OUT changed:"
git diff --stat -- "$OUT" 2>/dev/null || true
