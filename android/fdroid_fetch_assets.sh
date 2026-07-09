#!/bin/sh
# android/fdroid_fetch_assets.sh
#
# Fetches and sha256-verifies the vendored, offline-first third-party
# frontend assets (highlight.js, KaTeX + its fonts, Material Icons font,
# github-markdown-css) into backend/frontend/html/{js,css,css/fonts}/,
# instead of relying on the copies normally committed to the repo for
# local/Docker builds.
#
# WHY THIS EXISTS: these files are committed to the repo so the app is
# fully offline out of the box (see UserManual.md) and so
# local/build.sh / Dockerfile.base builds don't need network access.
# But F-Droid's own build recipe (metadata/net.basov.omngo.fdroid.yml)
# would rather fetch-and-verify third-party binaries at build time than
# trust ones already sitting in the source tree - this script is called
# from that recipe's prebuild: step for exactly that reason. It is not
# used by the normal Docker pipeline, which keeps using the committed
# copies as-is (this script is safe to run there too - it just
# re-fetches and overwrites the same files - but nothing currently
# requires that).
#
# USAGE:
#   sh android/fdroid_fetch_assets.sh                 # fetch + verify, run from repo root
#   sh android/fdroid_fetch_assets.sh --print-hashes   # fetch only, print "name sha256sum"
#                                                       # for every asset instead of verifying -
#                                                       # use this once, on any machine with
#                                                       # normal internet access, to populate the
#                                                       # SHA256 map below, then commit the result.
#
# STATUS: every entry in the SHA256 map below is a REPLACE_WITH_SHA256
# placeholder. This script fails closed (refuses to proceed) as long as
# any placeholder remains, the same way the fdroiddata recipe's Go
# toolchain checksum TODO does - see the recipe file for the matching
# note. Run this script with --print-hashes on a normal machine, then
# replace the placeholders below with the printed values before this can
# be used in an actual F-Droid build.

set -eu

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
JS_DIR="$REPO_ROOT/backend/frontend/html/js"
CSS_DIR="$REPO_ROOT/backend/frontend/html/css"
FONT_DIR="$REPO_ROOT/backend/frontend/html/css/fonts"

PRINT_HASHES=0
if [ "${1:-}" = "--print-hashes" ]; then
    PRINT_HASHES=1
fi

mkdir -p "$JS_DIR" "$CSS_DIR" "$FONT_DIR"

# name|url|dest|sha256 (one asset per line; sha256 is a placeholder until
# populated via --print-hashes - see STATUS above)
ASSETS='
github-markdown.min.css|https://cdnjs.cloudflare.com/ajax/libs/github-markdown-css/5.5.0/github-markdown.min.css|CSS_DIR/markdown.css|REPLACE_WITH_SHA256
highlight.min.js|https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js|JS_DIR/highlight.min.js|REPLACE_WITH_SHA256
highlight.default.min.css|https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/default.min.css|CSS_DIR/highlight.default.min.css|REPLACE_WITH_SHA256
katex.min.js|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/katex.min.js|JS_DIR/katex.min.js|REPLACE_WITH_SHA256
auto-render.min.js|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/contrib/auto-render.min.js|JS_DIR/auto-render.min.js|REPLACE_WITH_SHA256
katex.min.css|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/katex.min.css|CSS_DIR/katex.min.css|REPLACE_WITH_SHA256
KaTeX_AMS-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_AMS-Regular.woff2|FONT_DIR/KaTeX_AMS-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Caligraphic-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Caligraphic-Bold.woff2|FONT_DIR/KaTeX_Caligraphic-Bold.woff2|REPLACE_WITH_SHA256
KaTeX_Caligraphic-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Caligraphic-Regular.woff2|FONT_DIR/KaTeX_Caligraphic-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Fraktur-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Fraktur-Bold.woff2|FONT_DIR/KaTeX_Fraktur-Bold.woff2|REPLACE_WITH_SHA256
KaTeX_Fraktur-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Fraktur-Regular.woff2|FONT_DIR/KaTeX_Fraktur-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Main-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-Bold.woff2|FONT_DIR/KaTeX_Main-Bold.woff2|REPLACE_WITH_SHA256
KaTeX_Main-BoldItalic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-BoldItalic.woff2|FONT_DIR/KaTeX_Main-BoldItalic.woff2|REPLACE_WITH_SHA256
KaTeX_Main-Italic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-Italic.woff2|FONT_DIR/KaTeX_Main-Italic.woff2|REPLACE_WITH_SHA256
KaTeX_Main-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-Regular.woff2|FONT_DIR/KaTeX_Main-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Math-BoldItalic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Math-BoldItalic.woff2|FONT_DIR/KaTeX_Math-BoldItalic.woff2|REPLACE_WITH_SHA256
KaTeX_Math-Italic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Math-Italic.woff2|FONT_DIR/KaTeX_Math-Italic.woff2|REPLACE_WITH_SHA256
KaTeX_SansSerif-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_SansSerif-Bold.woff2|FONT_DIR/KaTeX_SansSerif-Bold.woff2|REPLACE_WITH_SHA256
KaTeX_SansSerif-Italic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_SansSerif-Italic.woff2|FONT_DIR/KaTeX_SansSerif-Italic.woff2|REPLACE_WITH_SHA256
KaTeX_SansSerif-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_SansSerif-Regular.woff2|FONT_DIR/KaTeX_SansSerif-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Script-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Script-Regular.woff2|FONT_DIR/KaTeX_Script-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Size1-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size1-Regular.woff2|FONT_DIR/KaTeX_Size1-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Size2-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size2-Regular.woff2|FONT_DIR/KaTeX_Size2-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Size3-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size3-Regular.woff2|FONT_DIR/KaTeX_Size3-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Size4-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size4-Regular.woff2|FONT_DIR/KaTeX_Size4-Regular.woff2|REPLACE_WITH_SHA256
KaTeX_Typewriter-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Typewriter-Regular.woff2|FONT_DIR/KaTeX_Typewriter-Regular.woff2|REPLACE_WITH_SHA256
material-icons.woff2|https://fonts.gstatic.com/s/materialicons/v143/flUhRq6tzZclQEJ-Vdg-IuiaDsNc.woff2|FONT_DIR/material-icons.woff2|REPLACE_WITH_SHA256
'

resolve_dest() {
    # translate the FONT_DIR/JS_DIR/CSS_DIR prefix in the table above to
    # the real absolute path, without relying on eval
    case "$1" in
        JS_DIR/*)   echo "$JS_DIR/${1#JS_DIR/}" ;;
        CSS_DIR/*)  echo "$CSS_DIR/${1#CSS_DIR/}" ;;
        FONT_DIR/*) echo "$FONT_DIR/${1#FONT_DIR/}" ;;
        *) echo "$1" ;;
    esac
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

placeholders_remaining=0
failures=0

echo "$ASSETS" | while IFS='|' read -r name url dest_tmpl expected; do
    [ -z "$name" ] && continue
    dest=$(resolve_dest "$dest_tmpl")

    if [ "$PRINT_HASHES" -eq 0 ] && [ "$expected" = "REPLACE_WITH_SHA256" ]; then
        echo "fdroid_fetch_assets.sh: $name has no pinned sha256 yet - run with --print-hashes first" >&2
        echo "PLACEHOLDER_REMAINING" >&2
        continue
    fi

    echo -n "Fetching $name... "
    if ! curl -fsSL -o "$dest" "$url"; then
        echo "FAILED to download" >&2
        echo "DOWNLOAD_FAILED" >&2
        continue
    fi

    actual=$(sha256_of "$dest")

    if [ "$PRINT_HASHES" -eq 1 ]; then
        echo "ok"
        printf '%s %s\n' "$name" "$actual"
        continue
    fi

    if [ "$actual" != "$expected" ]; then
        echo "CHECKSUM MISMATCH" >&2
        echo "  expected: $expected" >&2
        echo "  actual:   $actual" >&2
        rm -f "$dest"
        echo "CHECKSUM_MISMATCH" >&2
        continue
    fi

    echo "ok (sha256 verified)"
done > /tmp/fdroid_fetch_assets.out 2>&1 || true

cat /tmp/fdroid_fetch_assets.out

if grep -q PLACEHOLDER_REMAINING /tmp/fdroid_fetch_assets.out; then
    echo "fdroid_fetch_assets.sh: refusing to proceed - unpinned assets remain (see above)." >&2
    rm -f /tmp/fdroid_fetch_assets.out
    exit 1
fi
if grep -q DOWNLOAD_FAILED /tmp/fdroid_fetch_assets.out; then
    echo "fdroid_fetch_assets.sh: one or more downloads failed (see above)." >&2
    rm -f /tmp/fdroid_fetch_assets.out
    exit 1
fi
if grep -q CHECKSUM_MISMATCH /tmp/fdroid_fetch_assets.out; then
    echo "fdroid_fetch_assets.sh: one or more checksums did not match (see above)." >&2
    rm -f /tmp/fdroid_fetch_assets.out
    exit 1
fi

rm -f /tmp/fdroid_fetch_assets.out
echo "fdroid_fetch_assets.sh: all assets fetched$([ "$PRINT_HASHES" -eq 1 ] || echo ' and verified')."
