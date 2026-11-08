#!/usr/bin/env bash
#
# local/f-droid_build.sh - the local check of the F-Droid build of
# net.basov.omngo.fdroid, in the image of the F-Droid build server.
#
# USAGE:
#   bash local/f-droid_build.sh                  # podman (the default)
#   ENGINE=docker bash local/f-droid_build.sh    # Docker
#
# The script builds the LATEST build entry of the metadata file in
# $FDROIDDATA. A new version in the .yml file thus needs no change here.
# fdroid build reads the metadata from the working tree of $FDROIDDATA, also
# a change that has no commit. It clones the app source from the Repo: URL,
# thus the commit of the build entry must be on GitHub.
#
# THE SUDO LINES. The F-Droid server runs "fdroid build --on-server" in the
# container. That runs the sudo: lines of the recipe, locks root and removes
# sudo, and then builds. Before that, the host fetched the source and the
# srclibs, because --on-server does not fetch them. This script does the
# same in one container: it runs the sudo: lines of the latest build entry,
# locks root, removes sudo, and then runs "fdroid build -l". Without this
# step, fdroid build skips the sudo: lines, and the recipe finds no Go to
# bootstrap the srclib go.
#
# THE USER. The image runs the build as vagrant, uid 1000. With podman,
# --userns=keep-id maps your user to that uid. With Docker, --user gives
# your uid and gid. Your uid must thus be the uid of vagrant. The script
# checks that.
#
# The image has the JDK 21 of Debian trixie, the same as the F-Droid server
# and the Dockerfile of this repository.

# Repeat build
# cd ~/git/fdroiddata
# rm -f unsigned/net.basov.omngo.fdroid_*.apk unsigned/net.basov.omngo.fdroid_*_src.tar.gz
# rm -rf ~/git/fdroiddata/tmp                            # fdroid scratch
# rm -rf ~/fdroid-cache/gradle/caches

set -euo pipefail

# ---- paths (edit if yours differ) ----
FDROIDDATA="$HOME/git/fdroiddata"           # metadata checkout (mounts to /build)
FDROIDSERVER="$HOME/git/fdroidserver"       # git fdroidserver checkout
CACHE="$HOME/fdroid-cache"
SDKCACHE="$CACHE/android-sdk"               # persistent SDK on the host
APP="net.basov.omngo.fdroid"                # -l picks the latest Builds entry
IMAGE="registry.gitlab.com/fdroid/fdroidserver:buildserver"
NDK_URL="https://dl.google.com/android/repository/android-ndk-r25c-linux.zip"
NDK_DIR="android-ndk-r25c"                  # r25c == 25.2.9519653 (matches recipe + Docker)
ENGINE="${ENGINE:-podman}"                  # podman or docker
# --------------------------------------

# SETUP_OPTS: the one-time set-up steps 1 and 3. BUILD_OPTS: the build of step 5.
# Rootless podman maps the root of the container to your user, thus the set-up
# steps need no user option there. Docker writes as the real root without
# --user, and the build user then cannot write to the cache.
case "$ENGINE" in
    podman)
        SETUP_OPTS=()
        BUILD_OPTS=(--userns=keep-id --http-proxy=false)
        ;;
    docker)
        SETUP_OPTS=(--user "$(id -u):$(id -g)")
        BUILD_OPTS=(--user "$(id -u):$(id -g)")
        ;;
    *)
        echo "ENGINE must be podman or docker, not '$ENGINE'" >&2
        exit 2
        ;;
esac

vagrant_uid=$("$ENGINE" run --rm --entrypoint id "$IMAGE" -u vagrant)
if [ "$vagrant_uid" != "$(id -u)" ]; then
    echo "your uid is $(id -u), but vagrant in the image has uid $vagrant_uid." >&2
    echo "The build would not own its files. Run the script as a user with uid $vagrant_uid." >&2
    exit 1
fi

mkdir -p "$CACHE"

# 1. Seed a persistent copy of the image's SDK (one-time)
if [ ! -e "$SDKCACHE/licenses" ]; then
    echo "==> seeding persistent Android SDK to $SDKCACHE (one-time)..."
    "$ENGINE" run --rm "${SETUP_OPTS[@]}" -v "$CACHE":/hc:z --entrypoint /bin/bash "$IMAGE" \
        -c 'cp -a /opt/android-sdk /hc/android-sdk'
fi

# 2. NDK into the persistent SDK (one-time)
if [ ! -d "$SDKCACHE/ndk/$NDK_DIR" ]; then
    echo "==> installing NDK $NDK_DIR ..."
    mkdir -p "$SDKCACHE/ndk"
    curl -fsSL -o /tmp/ndk.zip "$NDK_URL"
    unzip -q /tmp/ndk.zip -d "$SDKCACHE/ndk"
    rm -f /tmp/ndk.zip
fi

# 3. SDK platform (one-time). F-Droid python sdkmanager; the package is
#    quoted so the ';' survives.
#    No build-tools here. The prebuild: line of the recipe installs them, the
#    same as on the F-Droid server. android/app/build.gradle asks for 34.0.0.
#    The build log thus shows if the server must download that version.
if ! ls -d "$SDKCACHE"/platforms/android-34 >/dev/null 2>&1; then
    echo "==> installing platforms;android-34 ..."
    "$ENGINE" run --rm "${SETUP_OPTS[@]}" -v "$SDKCACHE":/opt/android-sdk:z --entrypoint /bin/bash "$IMAGE" \
        -c 'export ANDROID_HOME=/opt/android-sdk ANDROID_SDK_ROOT=/opt/android-sdk; \
            yes | sdkmanager "platforms;android-34"'
fi

# 4. Register the NDK path in config.yml (persists on the /build mount)
if ! grep -q '^ndk_paths:' "$FDROIDDATA/config.yml"; then
    echo "==> registering ndk_paths in config.yml..."
    cat >> "$FDROIDDATA/config.yml" <<EOF

ndk_paths:
  r25c: /opt/android-sdk/ndk/$NDK_DIR
  '25.2.9519653': /opt/android-sdk/ndk/$NDK_DIR
EOF
fi
chmod 600 "$FDROIDDATA/config.yml" 2>/dev/null || true

# 5. Build. Whole ~/.cache is mounted (a subdir-only mount leaves a root-owned
#    .cache parent that breaks Go's build cache). Gradle + gradlew-fdroid +
#    go-build caches all persist across runs.
#    The script of the container is an argument of bash -c, and not stdin.
#    A program of the build could otherwise read the rest of the script.
mkdir -p "$CACHE/gradle" "$CACHE/vagrant-cache"
IN_CONTAINER=$(cat <<'CONTAINER'
set -euo pipefail
export ANDROID_HOME=/opt/android-sdk ANDROID_SDK_ROOT=/opt/android-sdk
export PATH=/home/vagrant/fdroidserver:$PATH
export PYTHONPATH=/home/vagrant/fdroidserver
java -version
cd /build

# The sudo: lines of the build entry that "fdroid build -l" takes: the last
# entry without "disable". The parser of fdroidserver reads the file, and
# the lines are joined with "; ", the same as in fdroidserver/build.py.
sudo_cmds=$(python3 - "$APP" <<'PY'
import sys
from fdroidserver import metadata

app = metadata.parse_metadata('metadata/%s.yml' % sys.argv[1])
builds = [b for b in app.get('Builds', []) if not b.disable]
if not builds:
    sys.exit('no build entry without "disable"')
b = builds[-1]
print('==> build entry: %s (%s), commit %s' % (b.versionName, b.versionCode, b.commit), file=sys.stderr)
print('; '.join(b.sudo or []))
PY
)

# The same three steps as "fdroid build --on-server" on the F-Droid server.
if [ -n "$sudo_cmds" ]; then
    echo "==> running the sudo: lines: $sudo_cmds"
    sudo DEBIAN_FRONTEND=noninteractive bash -e -u -o pipefail -x -c "$sudo_cmds"
fi
sudo passwd --lock root
sudo SUDO_FORCE_REMOVE=yes dpkg --purge sudo

fdroid build -v -l "$APP"
CONTAINER
)

TTY_OPTS=()
if [ -t 0 ] && [ -t 1 ]; then
    TTY_OPTS=(-it)
fi

echo "==> building $APP (latest build entry) with $ENGINE ..."
"$ENGINE" run --rm "${TTY_OPTS[@]}" "${BUILD_OPTS[@]}" \
    -e APP="$APP" \
    -v "$FDROIDSERVER":/home/vagrant/fdroidserver:z \
    -v "$FDROIDDATA":/build:z \
    -v "$SDKCACHE":/opt/android-sdk:z \
    -v "$CACHE/gradle":/home/vagrant/.gradle:z \
    -v "$CACHE/vagrant-cache":/home/vagrant/.cache:z \
    --entrypoint /bin/bash \
    "$IMAGE" -c "$IN_CONTAINER"

echo
echo "==> if the build succeeded, the APK is under:"
echo "    $FDROIDDATA/build/net.basov.omngo.fdroid/android/app/build/outputs/apk/fdroid/release/"
