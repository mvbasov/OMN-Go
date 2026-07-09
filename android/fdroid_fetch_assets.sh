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
github-markdown.min.css|https://cdnjs.cloudflare.com/ajax/libs/github-markdown-css/5.5.0/github-markdown.min.css|CSS_DIR/markdown.css|a7a15c52ec7512eb6c15c593fb289616c6987dd0e33e8e072d9be3fe79eedb18
highlight.min.js|https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js|JS_DIR/highlight.min.js|837a6fa5b0c736b52bbde2b2b6190f305da3fc9ed41681db5321507057b5c846
highlight.default.min.css|https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/default.min.css|CSS_DIR/highlight.default.min.css|fbde0ac0921d86c356c41532e7319c887a23bd1b8ff00060cab447249f03c7cf
katex.min.js|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/katex.min.js|JS_DIR/katex.min.js|dc84b296ec3e884de093158f760fd9d45b6c7abe58b5381557f4e138f46a58ae
auto-render.min.js|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/contrib/auto-render.min.js|JS_DIR/auto-render.min.js|9cb8dacfc086c2966c9ec4ba54f4a2dc43b7cbe2b33cec1a2743d886c7fb47a7
katex.min.css|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/katex.min.css|CSS_DIR/katex.min.css|505d5f829022bb7b4f24dfee0aa1141cd7bba67afe411d1240335f820960b5c3
KaTeX_AMS-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_AMS-Regular.woff2|FONT_DIR/KaTeX_AMS-Regular.woff2|0cdd387c9590a1a9f9794560022dbb59654a7d86f187aa0c81495ad42d3a7308
KaTeX_Caligraphic-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Caligraphic-Bold.woff2|FONT_DIR/KaTeX_Caligraphic-Bold.woff2|de7701e42cf1f4cf0b766c03fb27977207eee2f4fd5d76fa82188406da43ea4c
KaTeX_Caligraphic-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Caligraphic-Regular.woff2|FONT_DIR/KaTeX_Caligraphic-Regular.woff2|5d53e70ad607c2352162dec9e0923fb54ecdafaccbf604cd8dcf7d00facb989b
KaTeX_Fraktur-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Fraktur-Bold.woff2|FONT_DIR/KaTeX_Fraktur-Bold.woff2|74444efd593c005e3f4573b44524704c0af0a937fe911cca9e94068d0d140d3f
KaTeX_Fraktur-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Fraktur-Regular.woff2|FONT_DIR/KaTeX_Fraktur-Regular.woff2|51814d270d06ff0255dba0799994fa4d8c84d11f09951d47595f4abb1f3602dc
KaTeX_Main-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-Bold.woff2|FONT_DIR/KaTeX_Main-Bold.woff2|0f60d1b897938ec918c8ce073092411baf9438f6739465693ff18b0f9d20b021
KaTeX_Main-BoldItalic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-BoldItalic.woff2|FONT_DIR/KaTeX_Main-BoldItalic.woff2|99cd42a3c072d918f2f44984a807cf7aa16e13545fd0875fc07c6c65f99e715b
KaTeX_Main-Italic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-Italic.woff2|FONT_DIR/KaTeX_Main-Italic.woff2|97479ca6cce906abc961ecac96faa5f9ca2e61b8e7670d475826bcdee9a7c267
KaTeX_Main-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Main-Regular.woff2|FONT_DIR/KaTeX_Main-Regular.woff2|c2342cd8b869e01752a9321dc17213fc40d4d04c79688c1d43f2cf316abd7866
KaTeX_Math-BoldItalic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Math-BoldItalic.woff2|FONT_DIR/KaTeX_Math-BoldItalic.woff2|dc47344dbb6cb5b655c8460d561f4df5f501b90c804ad3c6cec65fe322351ab1
KaTeX_Math-Italic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Math-Italic.woff2|FONT_DIR/KaTeX_Math-Italic.woff2|7af58c5ec8f132a2ddde9027c6d7814decce4d3b822a11192a42a20e2e973264
KaTeX_SansSerif-Bold.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_SansSerif-Bold.woff2|FONT_DIR/KaTeX_SansSerif-Bold.woff2|e99ae51144bf1232efcc1bfe5add36262c6866b0faab24fa75740e1b98577a62
KaTeX_SansSerif-Italic.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_SansSerif-Italic.woff2|FONT_DIR/KaTeX_SansSerif-Italic.woff2|00b26ac825e2095056396e0553b8ac26d3f8ad158c3826e28b4c45b385c4714a
KaTeX_SansSerif-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_SansSerif-Regular.woff2|FONT_DIR/KaTeX_SansSerif-Regular.woff2|68e8c73ef42afd3ccec58bf0fba302cce448938e7fc020a5e31f8a952eee1342
KaTeX_Script-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Script-Regular.woff2|FONT_DIR/KaTeX_Script-Regular.woff2|036d4e95149b69ff9bcc0cd55771efeb25ffa3947293e69acd78d5ac328c684b
KaTeX_Size1-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size1-Regular.woff2|FONT_DIR/KaTeX_Size1-Regular.woff2|6b47c40166b6dbe21a5dfca7718413f2147fd2399be1ba605d8ad39cedf25dfe
KaTeX_Size2-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size2-Regular.woff2|FONT_DIR/KaTeX_Size2-Regular.woff2|d04c54219f9eaec6d4d4fd42dfb28785975a4794d6b2fc71e566b9cd6db842dd
KaTeX_Size3-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size3-Regular.woff2|FONT_DIR/KaTeX_Size3-Regular.woff2|73d591271b1604960cb10bb90fee021670af7297017e0e98480b332d11f51995
KaTeX_Size4-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Size4-Regular.woff2|FONT_DIR/KaTeX_Size4-Regular.woff2|a4af7d414440a1c1790825cfb700cf9cf43b0f2c4b04f0ebc523011ad9853ec0
KaTeX_Typewriter-Regular.woff2|https://cdnjs.cloudflare.com/ajax/libs/KaTeX/0.16.9/fonts/KaTeX_Typewriter-Regular.woff2|FONT_DIR/KaTeX_Typewriter-Regular.woff2|71d517d67827787cfabdf186914cc3358eda539e37931941f2b2fd4a21f68c0b
material-icons.woff2|https://fonts.gstatic.com/s/materialicons/v143/flUhRq6tzZclQEJ-Vdg-IuiaDsNc.woff2|FONT_DIR/material-icons.woff2|8265f64786397d6b832d1ca0aafdf149ad84e72759fffa9f7272e91a0fb015d1
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
