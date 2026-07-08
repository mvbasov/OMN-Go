#!/bin/sh
# android/fdroid_prepare_aar.sh
#
# Copies the prebuilt Go backend (omngo.aar) into app/libs/ before Gradle
# runs, for the F-Droid build path specifically.
#
# WHY THIS EXISTS: F-Droid's own build server compiles this app from
# source - it will not accept a binary .aar committed to this repo (see
# the project's F-Droid inclusion-policy notes). The actual `gomobile
# bind` step that produces omngo.aar lives in a SEPARATE repository (Go
# backend + its own GitLab CI), referenced from the F-Droid build recipe
# as a "srclib" - F-Droid's mechanism for exactly this situation (an
# artifact built by a different toolchain than the main app). This
# script is the one thing that needs to know where that srclib's build
# output landed and where Gradle expects to find it; keeping that logic
# HERE instead of inline in the fdroiddata YAML means it can be tested
# and iterated on without touching the separate metadata submission.
#
# EXPECTED USAGE from the (not yet written) fdroiddata build recipe:
#
#   srclibs:
#     - GoBackend@<tag-or-commit>
#   prebuild:
#     - sh android/fdroid_prepare_aar.sh $$GoBackend$$
#
# $$GoBackend$$ is substituted by fdroidserver with the absolute path to
# the checked-out (and, per that srclib's own Prepare:/build steps,
# already-built) srclib directory. This script does not know or care how
# that directory got its .aar - only where to find it and where to put it.
#
# TODO once the Go backend repo exists: confirm the exact output path
# (adjust SEARCH_NAMES below if `gomobile bind`'s -o flag there produces
# a different filename), and confirm the srclib name used in the
# fdroiddata recipe matches what's referenced above.

set -eu

SRC_DIR="${1:-}"
if [ -z "$SRC_DIR" ]; then
    echo "usage: $0 <path-to-built-go-backend-checkout>" >&2
    echo "  (in the F-Droid build recipe, this is \$\$<srclib-name>\$\$)" >&2
    exit 1
fi
if [ ! -d "$SRC_DIR" ]; then
    echo "fdroid_prepare_aar.sh: source directory not found: $SRC_DIR" >&2
    exit 1
fi

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
DEST_DIR="$SCRIPT_DIR/app/libs"
DEST_FILE="$DEST_DIR/omngo.aar"

# Accept a couple of plausible output locations/names from the srclib
# build without having to hardcode one exact convention yet - narrow this
# once the actual Go backend repo's build output is known.
CANDIDATES="
$SRC_DIR/omngo.aar
$SRC_DIR/build/omngo.aar
$SRC_DIR/out/omngo.aar
"

FOUND=""
for candidate in $CANDIDATES; do
    if [ -f "$candidate" ]; then
        FOUND="$candidate"
        break
    fi
done

if [ -z "$FOUND" ]; then
    echo "fdroid_prepare_aar.sh: could not find omngo.aar under $SRC_DIR" >&2
    echo "  looked in:" >&2
    printf '    %s\n' $CANDIDATES >&2
    echo "  adjust CANDIDATES in this script once the Go backend repo's" >&2
    echo "  actual build output path is known." >&2
    exit 1
fi

mkdir -p "$DEST_DIR"
cp "$FOUND" "$DEST_FILE"
echo "fdroid_prepare_aar.sh: placed $FOUND -> $DEST_FILE"
