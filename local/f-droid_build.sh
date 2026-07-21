#!/usr/bin/env bash
#
# build-omngo.sh — local F-Droid build check of net.basov.omngo.fdroid
# in the F-Droid buildserver podman image.
#
# Now that the app repo pins Gradle 8.5 via gradle-wrapper.properties
# (runs on JDK 17 AND 21), the build runs on the image's stock JDK 21 —
# the closest match to F-Droid's real (Debian trixie) buildserver. No
# JDK mount needed anymore.
#
# Builds the LATEST build entry in the metadata (-l), so a version bump
# in the .yml needs no change here.

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
# --------------------------------------

mkdir -p "$CACHE"

# 1. Seed a persistent copy of the image's SDK (one-time)
if [ ! -e "$SDKCACHE/licenses" ]; then
    echo "==> seeding persistent Android SDK to $SDKCACHE (one-time)..."
    podman run --rm -v "$CACHE":/hc:z --entrypoint /bin/bash "$IMAGE" \
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

# 3. SDK platform + build-tools (one-time) — same versions as the Docker build.
#    F-Droid python sdkmanager; each package quoted so the ';' survives.
if ! ls -d "$SDKCACHE"/platforms/android-34 >/dev/null 2>&1; then
    echo "==> installing platforms;android-34 build-tools;33.0.1 ..."
    podman run --rm -v "$SDKCACHE":/opt/android-sdk:z --entrypoint /bin/bash "$IMAGE" \
        -c 'export ANDROID_HOME=/opt/android-sdk ANDROID_SDK_ROOT=/opt/android-sdk; \
            yes | sdkmanager "platforms;android-34" "build-tools;33.0.1"'
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
mkdir -p "$CACHE/gradle" "$CACHE/vagrant-cache"
echo "==> building $APP (latest build entry) ..."
podman run --rm -it --http-proxy=false --userns=keep-id \
    -v "$FDROIDSERVER":/home/vagrant/fdroidserver:z \
    -v "$FDROIDDATA":/build:z \
    -v "$SDKCACHE":/opt/android-sdk:z \
    -v "$CACHE/gradle":/home/vagrant/.gradle:z \
    -v "$CACHE/vagrant-cache":/home/vagrant/.cache:z \
    --entrypoint /bin/bash \
    "$IMAGE" \
    -c 'export ANDROID_HOME=/opt/android-sdk ANDROID_SDK_ROOT=/opt/android-sdk \
              PATH=/home/vagrant/fdroidserver:$PATH \
              PYTHONPATH=/home/vagrant/fdroidserver; \
        java -version; \
        cd /build && fdroid build -v -l '"$APP"

echo
echo "==> if the build succeeded, the APK is under:"
echo "    $FDROIDDATA/build/net.basov.omngo.fdroid/android/app/build/outputs/apk/fdroid/release/"
