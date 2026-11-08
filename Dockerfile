# The one Dockerfile of OMN-Go. The local build (local/build.sh), GitHub
# (.github/workflows/) and GitLab (.gitlab-ci.yml) all build this file,
# with the same stages and the same secret ids.
#
# All stages are in one file, thus one BuildKit graph builds them. A
# builder with the "docker-container" driver (GitHub, GitLab) has its own
# image store. A second file with "FROM omn-go-base:latest" then fails,
# because that builder cannot find the image of the first build.
#
# THE STAGES.
#
#   go_env          Go, the JDK, Node, and the resolved go.mod and go.sum.
#   base_env        go_env plus the Android SDK, the NDK, Gradle and gomobile.
#   test            go_env plus the source. It runs the quality gate.
#   project_builder base_env plus the source. It makes the artifacts.
#   export          The artifacts alone. This is the default target.
#
# project_builder copies a file from the test stage. A release build thus
# runs the gate, and it runs the gate at the same time as the Android set-up.
# The fast workflow (.github/workflows/test.yml) builds the test stage
# alone. The gate thus has one definition for each build.
#
# THE IMAGE FOLLOWS THE F-DROID BUILD SERVER. The F-Droid server runs
# Debian trixie with JDK 21. This image uses the same Debian and the same
# JDK. The image is golang:1.26.0-trixie, and not the moving tag
# golang:1.26-trixie. The F-Droid recipe builds with the srclib go@go1.26.0.
# The two builds thus use the same compiler. Change both together.
#
# THE GOMOBILE STEPS are in android/build_aar.sh, and not here. The F-Droid
# recipe can run the same script. See
# doc/decisions/0027-build-the-android-library-with-one-script.md.

# ============================== STAGE 1 ===============================
FROM golang:1.26.0-trixie AS go_env

ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -y \
    openjdk-21-jdk nodejs wget unzip cmake ninja-build

WORKDIR /app

# The repository holds no go.sum, and the host has no Go toolchain. This
# step thus makes go.sum here, one time for each change of go.mod or of
# the script. It also adds the pinned golang.org/x/mobile as a tool
# dependency, because the committed go.mod does not name it.
#
# The step keeps the result in /root/lockfiles, and not in /app. The later
# "COPY . ." brings in the go.mod of the host, which has no go.sum and no
# x/mobile. /root/lockfiles is outside that copy, thus the next step can
# restore the full files.
#
# A change of android/build_aar.sh starts this stage again, and also the
# Android set-up of base_env. The download caches keep that step short.
COPY go.mod ./
COPY android/build_aar.sh android/
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    sh android/build_aar.sh tools && \
    mkdir -p /root/lockfiles && cp go.mod go.sum /root/lockfiles/

# ============================== STAGE 2 ===============================
FROM go_env AS base_env

RUN --mount=type=cache,target=/tmp/dl,sharing=locked \
    wget -q https://dl.google.com/android/repository/commandlinetools-linux-10406996_latest.zip -O /tmp/dl/cmd.zip && \
    mkdir -p /opt/android/cmdline-tools && \
    unzip -q /tmp/dl/cmd.zip -d /opt/android/cmdline-tools && \
    mv /opt/android/cmdline-tools/cmdline-tools /opt/android/cmdline-tools/latest

ENV ANDROID_HOME=/opt/android
ENV PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools

# build-tools 34.0.0, because android/app/build.gradle pins that version.
# Without the package in the image, the gradle step downloads it on each
# build. The SDK lives here, and not under the /root/.gradle cache mount.
#
# The Gradle configuration stays as it is. F-Droid builds this project
# from that configuration, on their own server and with their own SDK.
# The image follows the build, and not the other way.
RUN --mount=type=cache,target=/opt/android/.android/cache,sharing=locked \
    yes | sdkmanager --licenses && \
    sdkmanager "platforms;android-34" "build-tools;34.0.0" "platform-tools" "ndk;25.2.9519653"

# Gradle 8.5, the version of android/gradle/wrapper/gradle-wrapper.properties.
# F-Droid reads that file. TestGradleVersionsAgree compares the two.
RUN --mount=type=cache,target=/tmp/dl,sharing=locked \
    wget -q https://services.gradle.org/distributions/gradle-8.5-bin.zip -O /tmp/dl/gradle.zip && \
    mkdir -p /opt/gradle && \
    unzip -q /tmp/dl/gradle.zip -d /opt/gradle
ENV PATH=$PATH:/opt/gradle/gradle-8.5/bin

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    sh android/build_aar.sh init

# ============================== STAGE 3 ===============================
# The quality gate: go vet and the unit tests. It needs no Android tool.
# The JDK and Node of go_env also run the Java test and the JavaScript
# tests. The last step writes /gate-passed. project_builder copies that
# file, thus no artifact comes from a build with a failed gate.
FROM go_env AS test

# Set to 1 to skip the gate for an emergency build:
#   docker buildx build --build-arg SKIP_TESTS=1 ...
ARG SKIP_TESTS=0

COPY . .

# Restore the full go.mod and go.sum from /root/lockfiles. The "COPY . ."
# above brought in the files of the host.
RUN cp /root/lockfiles/go.mod /root/lockfiles/go.sum ./

# A check, and not a resolution step. go mod tidy compares go.mod with
# the source. It changes nothing and uses no network while go.mod agrees
# with the imports of the source.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod tidy

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    if [ "$SKIP_TESTS" = "1" ]; then \
        echo "WARNING: SKIP_TESTS=1 - test gate bypassed"; \
    else \
        go vet ./backend/... && \
        go test ./backend/...; \
    fi && \
    touch /gate-passed

# ============================== STAGE 4 ===============================
FROM base_env AS project_builder

# This copy makes the test stage a part of each build of this stage.
COPY --from=test /gate-passed /tmp/gate-passed

COPY . .

# Restore the full go.mod and go.sum from /root/lockfiles. The "COPY . ."
# above brought in the files of the host.
RUN cp /root/lockfiles/go.mod /root/lockfiles/go.sum ./

# A check, and not a resolution step. See the test stage.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod tidy

# The desktop binaries. CGO_ENABLED=0 for both targets: the binary then
# needs no C library on the computer of the user, and it does not depend
# on the C library of the image. TestBinarySize builds with the same flags.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    export GOFLAGS="-trimpath" CGO_ENABLED=0 && \
    GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

# The Go library of the Android package (android/app/libs/omngo.aar).
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    sh android/build_aar.sh bind

# The "standard" flavor only, and never "fdroid". F-Droid builds the fdroid
# flavor on its own server, from its own recipe (see
# metadata/net.basov.omngo.fdroid.yml), and signs it with its own key. A
# local fdroid APK looks official, but F-Droid does not distribute it.
#
# The signing material comes in as BuildKit secrets, and not as build
# arguments. A secret file is at /run/secrets/<id>, only during this RUN,
# and no image layer keeps it. docker history thus shows no password.
# KEYSTORE_FILE points the signingConfig of build.gradle at the keystore.
# Each build must give the four ids: keystore, keystore_password, key_alias
# and key_password. required=true stops the step at once when one is absent.
RUN --mount=type=cache,target=/root/.gradle,sharing=locked \
    --mount=type=secret,id=keystore,required=true \
    --mount=type=secret,id=keystore_password,required=true \
    --mount=type=secret,id=key_alias,required=true \
    --mount=type=secret,id=key_password,required=true \
    export KEYSTORE_FILE="/run/secrets/keystore" && \
    export KEYSTORE_PASSWORD="$(cat /run/secrets/keystore_password)" && \
    export KEY_ALIAS="$(cat /run/secrets/key_alias)" && \
    export KEY_PASSWORD="$(cat /run/secrets/key_password)" && \
    cd android && \
    gradle assembleStandardRelease && \
    cp app/build/outputs/apk/standard/release/*.apk ../bin/

# ============================== EXPORT ===============================
# The build outputs alone. Build this stage with
#   docker buildx build --target export --output type=local,dest=<dir> .
# BuildKit then writes only these files to the host. It does not load the
# large toolchain image into the Docker daemon, and no "docker cp" is
# necessary. This is the last stage, thus also the default target.
FROM scratch AS export
COPY --from=project_builder /app/bin/ /
