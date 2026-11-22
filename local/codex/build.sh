#!/usr/bin/env bash

set -euo pipefail

fail() {
    printf '%s\n' "$*" >&2
    exit 1
}

target=${1:-all}
case "$target" in
    all|test|desktop) ;;
    *) fail "Usage: bash local/codex/build.sh [all|test|desktop]" ;;
esac

for tool in go node java javac; do
    command -v "$tool" >/dev/null || fail "Install $tool and add it to PATH."
done
if [[ "$target" == all ]]; then
    for tool in gradle flock; do
        command -v "$tool" >/dev/null || fail "Install $tool and add it to PATH."
    done
    : "${ANDROID_HOME:?Set ANDROID_HOME to the Android SDK directory.}"
    : "${KEYSTORE_FILE:?Set KEYSTORE_FILE to the Android signing key.}"
    : "${KEYSTORE_PASSWORD:?Set KEYSTORE_PASSWORD for the signing key.}"
    : "${KEY_ALIAS:?Set KEY_ALIAS for the signing key.}"
    : "${KEY_PASSWORD:?Set KEY_PASSWORD for the signing key.}"
    ANDROID_HOME=$(realpath -e -- "$ANDROID_HOME")
    KEYSTORE_FILE=$(realpath -e -- "$KEYSTORE_FILE")
    [[ -f "$KEYSTORE_FILE" ]] || fail "KEYSTORE_FILE must name a file."
    export ANDROID_HOME KEYSTORE_FILE KEYSTORE_PASSWORD KEY_ALIAS KEY_PASSWORD
fi

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)
output=$(realpath -m -- "${OMN_GO_OUTPUT_DIR:-$root/output-binaries/native}")
cd "$root"
export OMN_GO_COMMIT=$(git rev-parse HEAD)
export OMN_GO_COMMIT_TIME=$(git log -1 --format=%cI)
if [[ -n "$(git status --porcelain)" ]]; then
    OMN_GO_COMMIT+=-dirty
fi

stage=$(mktemp -d "$(realpath -- "${TMPDIR:-/tmp}")/omn-go-native.XXXXXXXX")
bind_path=/tmp/omn-go-src
cleanup() {
    if [[ "$(readlink -- "$bind_path")" == "$stage" ]]; then
        rm -- "$bind_path"
    fi
    rm -rf -- "$stage"
}
trap cleanup EXIT

# Copy the current contents of each tracked file. Skip deleted files.
git ls-files -z --deduplicate | while IFS= read -r -d '' path; do
    if [[ -e "$path" || -L "$path" ]]; then
        printf '%s\0' "$path"
    fi
done | tar -cf - --null -T - | tar -xf - -C "$stage"

cd "$stage"
export GOTOOLCHAIN=local GOWORK=off GOFLAGS=-buildvcs=false
sh android/build_aar.sh tools
go mod tidy
go vet ./backend/...
go test -count=1 ./backend/...
if [[ "$target" == test ]]; then
    exit 0
fi

version=$(awk -F'"' '/^const APP_VERSION = / {print $2}' backend/version.go)
[[ -n "$version" ]] || fail "Read APP_VERSION in backend/version.go."
ldflags="-s -w -X net.basov.omngo/backend.buildCommit=$OMN_GO_COMMIT -X net.basov.omngo/backend.buildCommitTime=$OMN_GO_COMMIT_TIME"
mkdir bin
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$ldflags" \
    -o "bin/omn-go-v${version}-desktop-linux-amd64" main_desktop.go
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$ldflags" \
    -o "bin/omn-go-v${version}-desktop-windows-amd64.exe" main_desktop.go
artifacts=(bin/*)

if [[ "$target" == all ]]; then
    export GOBIN="$stage/go-bin"
    export PATH="$GOBIN:$PATH"
    sh android/build_aar.sh init
    exec 9>>"${bind_path}.lock"
    flock 9
    [[ ! -e "$bind_path" && ! -L "$bind_path" ]] || fail "$bind_path is in use."
    sh android/build_aar.sh bind
    exec 9>&-
    (cd android && gradle --no-daemon assembleStandardRelease)
    artifacts+=(android/app/build/outputs/apk/standard/release/*.apk)
fi

mkdir -p -- "$output"
cp -- "${artifacts[@]}" "$output/"
printf 'Build outputs are in %s\n' "$output"
