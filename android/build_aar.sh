#!/bin/sh
# android/build_aar.sh
#
# Makes android/app/libs/omngo.aar from ./backend with gomobile. This
# script is the only place that holds the gomobile steps of the Android
# build. The Dockerfile calls it. The F-Droid recipe can call it too, thus
# a change of a flag or of the x/mobile version needs no change of the
# recipe. See doc/decisions/0027-build-the-android-library-with-one-script.md.
#
# USAGE (from any directory):
#   sh android/build_aar.sh          # all steps, in this sequence: tools, tidy, init, bind
#   sh android/build_aar.sh tools    # add the pinned x/mobile tools to go.mod and download the modules
#   sh android/build_aar.sh init     # install gobind and gomobile, then run gomobile init
#   sh android/build_aar.sh bind     # make android/app/libs/omngo.aar
#
# The Dockerfile runs the steps one at a time, because each step has its
# own image layer and cache. The F-Droid recipe runs all steps.
#
# The script needs Go, and $GOPATH/bin on PATH. The bind step also needs
# ANDROID_HOME and the NDK that android/app/build.gradle names.

set -eu

# THE ONE PIN OF golang.org/x/mobile. A "go get -tool" with no version
# takes the newest x/mobile. That version can also raise golang.org/x/crypto,
# golang.org/x/net and golang.org/x/sys, and these modules go into
# libgojni.so. Two builds of one commit on two days then give two
# different libraries. Change this value on purpose, and only here.
XMOBILE_VERSION=v0.0.0-20261011034528-197770444867

# Use only the Go that the build installed. With "auto", Go downloads a
# newer toolchain when a module asks for it. The build then uses a second
# compiler and reports no error. With "local", the build stops.
export GOTOOLCHAIN=local

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO_ROOT"

# The bind step runs from this path on each host. See step_bind.
BIND_DIR=/tmp/omn-go-src

# THE COMMIT OF THE BUILD. The status page shows it. OMN_GO_COMMIT and
# OMN_GO_COMMIT_TIME come from the caller: the Dockerfile has them as build
# arguments, because its build context holds no .git. When they are absent
# and the checkout is a git repository (the F-Droid build), the script asks
# git. The F-Droid value never has "-dirty": fdroidserver changes files of the
# checkout, and the library must be the same as the library of the GitHub
# build of the same commit. Only local/build.sh adds "-dirty".
if [ -z "${OMN_GO_COMMIT:-}" ] && git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1; then
    OMN_GO_COMMIT=$(git -C "$REPO_ROOT" rev-parse HEAD)
    OMN_GO_COMMIT_TIME=$(git -C "$REPO_ROOT" log -1 --format=%cI)
fi
GO_LDFLAGS="-s -w -X net.basov.omngo/backend.buildCommit=${OMN_GO_COMMIT:-} -X net.basov.omngo/backend.buildCommitTime=${OMN_GO_COMMIT_TIME:-}"

step_tools() {
    go get -tool \
        "golang.org/x/mobile/cmd/gobind@$XMOBILE_VERSION" \
        "golang.org/x/mobile/cmd/gomobile@$XMOBILE_VERSION"
    go mod download
}

step_init() {
    # go.mod names the tools, thus "go install" takes the pinned version.
    go install golang.org/x/mobile/cmd/gobind golang.org/x/mobile/cmd/gomobile
    gomobile init
}

step_bind() {
    # -trimpath is a flag of gomobile, and not of the linker. gomobile
    # writes the generated Go code into a new temporary directory for each
    # run. Without -trimpath, libgojni.so holds that path.
    #
    # -androidapi is the minSdk of android/app/build.gradle. A library for
    # a higher level can fail to load on the lowest Android that installs
    # the APK.
    #
    # The fixed directory: gomobile writes a go.mod in its temporary
    # directory, with "replace net.basov.omngo => <this directory>". Go
    # records that path in libgojni.so, also with -trimpath. The build
    # thus runs from one fixed path on each host, through a symbolic link.
    # Go reads the path from PWD, thus it records the link and not the
    # real directory.
    if [ -e "$BIND_DIR" ] && [ ! -L "$BIND_DIR" ]; then
        echo "build_aar.sh: $BIND_DIR exists and is not a symbolic link" >&2
        exit 1
    fi
    ln -sfn "$REPO_ROOT" "$BIND_DIR"
    cd "$BIND_DIR"
    mkdir -p android/app/libs
    gomobile bind -trimpath -target=android -androidapi 23 -javapkg net.basov.omngo -ldflags="$GO_LDFLAGS" -o android/app/libs/omngo.aar ./backend
    cd "$REPO_ROOT"
    rm -f "$BIND_DIR"
}

case "${1:-all}" in
    tools) step_tools ;;
    init)  step_init ;;
    bind)  step_bind ;;
    all)
        step_tools
        go mod tidy
        step_init
        step_bind
        ;;
    *)
        echo "usage: sh android/build_aar.sh [all|tools|init|bind]" >&2
        exit 2
        ;;
esac
