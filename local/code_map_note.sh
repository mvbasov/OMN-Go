#!/bin/sh
# Make the OMN-Go note CodeMap.md from doc/CODE_MAP.md.
# Usage: sh local/code_map_note.sh [output file]
# The default output file is CodeMap.md in the repository root.
#
# The script:
#   1. Writes the header block and the description of the note.
#   2. Removes the first heading and the Summary comment of the document.
#   3. Changes each link "../path" to the file on GitHub:
#      "../dir/" goes to ".../tree/master/dir", and "../file" goes to
#      ".../blob/master/file".
#   4. Stops with an error when a link names a file that is not in the tree.
#
# The script needs only sh, sed and grep. It does not need Go or Python.
set -eu
cd "$(dirname "$0")/.."

SRC=doc/CODE_MAP.md
OUT=${1:-CodeMap.md}
GH=https://github.com/mvbasov/OMN-Go

[ -f "$SRC" ] || { echo "$SRC is not found" >&2; exit 1; }

# Each link target of the document must be in the tree.
missing=0
for target in $(grep -o '](\.\./[^)]*)' "$SRC" | sed 's/^](\.\.\///; s/)$//' | sort -u); do
	if [ ! -e "$target" ]; then
		echo "The link target $target is not in the tree" >&2
		missing=1
	fi
done
[ "$missing" = 0 ] || exit 1

VERSION=$(sed -n 's/^State of the tree: version \([0-9]*\.[0-9]*\.[0-9]*\).*/\1/p' "$SRC")
[ -n "$VERSION" ] || { echo "$SRC has no line \"State of the tree: version ...\"" >&2; exit 1; }

{
	cat <<EOF
Title: OMN-Go code map
Date: 2026-11-13 18:46:23
Category: Development
Tags: OMN-Go, Development

<!--- DESCRIPTION:
A map of the OMN-Go source code, version $VERSION. It shows where each feature of the application is in the code, from the three layers to the files and the Go functions. Each file name is a link to the file on GitHub.
--->

<style>
/* A long file name in a line or a table breaks, thus a phone shows the whole line. */
#omn-preview :not(pre) > code { overflow-wrap: anywhere; }
</style>

EOF
	# Line 1 is the heading "# OMN-Go code map". The header block gives the
	# title, thus the note does not repeat it. The Summary comment is for the
	# repository only. The blank line after each of the two also goes.
	sed -e '1{N;d;}' \
	    -e '/^<!-- Summary:.*-->$/{N;d;}' \
	    -e "s#](\.\./\([^)]*\)/)#]($GH/tree/master/\1)#g" \
	    -e "s#](\.\./\([^)]*\))#]($GH/blob/master/\1)#g" \
	    "$SRC"
} > "$OUT"

echo "$OUT: version $VERSION, $(grep -o "]($GH/" "$OUT" | wc -l | tr -d ' ') links"
