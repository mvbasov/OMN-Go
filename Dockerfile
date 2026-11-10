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
#   sys_env         The Debian packages: the JDK, Node, wget and unzip.
#   android_env     sys_env plus the Android SDK, the NDK and Gradle.
#   go_env          sys_env plus the resolved go.mod and go.sum.
#   base_env        android_env plus the files of go_env, gobind and gomobile.
#   test            go_env plus the source. It runs the quality gate.
#   project_builder base_env plus the source. It makes the artifacts.
#   export          The artifacts alone. This is the default target.
#
# android_env does not read a file of the repository. A change of go.mod
# or of android/build_aar.sh thus starts only go_env and the last step of
# base_env again. The SDK and the NDK are large, and their downloads stay
# in the layer cache.
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
#
# THE DOWNLOAD CACHE /tmp/dl. Each zip file there has its version in its
# name. A step downloads a file only when the cache does not hold it. The
# download goes to a .part file first, thus a stopped download leaves no
# bad zip file in the cache.

# ============================== STAGE 1 ===============================
FROM golang:1.26.0-trixie AS sys_env

ENV DEBIAN_FRONTEND=noninteractive

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -y \
    openjdk-21-jdk nodejs wget unzip cmake ninja-build

WORKDIR /app

# ============================== STAGE 2 ===============================
FROM sys_env AS android_env

RUN --mount=type=cache,target=/tmp/dl,sharing=locked \
    zip=/tmp/dl/commandlinetools-linux-10406996_latest.zip && \
    if [ ! -f "$zip" ]; then \
        wget -q https://dl.google.com/android/repository/commandlinetools-linux-10406996_latest.zip -O "$zip.part" && \
        mv "$zip.part" "$zip"; \
    fi && \
    mkdir -p /opt/android/cmdline-tools && \
    unzip -q "$zip" -d /opt/android/cmdline-tools && \
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
    zip=/tmp/dl/gradle-8.5-bin.zip && \
    if [ ! -f "$zip" ]; then \
        wget -q https://services.gradle.org/distributions/gradle-8.5-bin.zip -O "$zip.part" && \
        mv "$zip.part" "$zip"; \
    fi && \
    mkdir -p /opt/gradle && \
    unzip -q "$zip" -d /opt/gradle
ENV PATH=$PATH:/opt/gradle/gradle-8.5/bin

# ============================== STAGE 3 ===============================
FROM sys_env AS go_env

# The repository holds no go.sum, and the host has no Go toolchain. This
# step thus makes go.sum here, one time for each change of go.mod or of
# the script. It also adds the pinned golang.org/x/mobile as a tool
# dependency, because the committed go.mod does not name it.
#
# The step keeps the result in /root/lockfiles, and not in /app. The later
# "COPY . ." brings in the go.mod of the host, which has no go.sum and no
# x/mobile. /root/lockfiles is outside that copy, thus the next step can
# restore the full files.
COPY go.mod ./
COPY android/build_aar.sh android/
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    sh android/build_aar.sh tools && \
    mkdir -p /root/lockfiles && cp go.mod go.sum /root/lockfiles/

# ============================== STAGE 4 ===============================
FROM android_env AS base_env

# go.mod, go.sum and the script of go_env, and the copy in /root/lockfiles.
# The modules are in the /go/pkg/mod cache mount, which each stage shares.
COPY --from=go_env /app/ /app/
COPY --from=go_env /root/lockfiles/ /root/lockfiles/

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    sh android/build_aar.sh init

# ============================== STAGE 5 ===============================
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

# ============================== STAGE 6 ===============================
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

# The commit of the build, for the status page. The build context holds no
# .git, thus the caller gives the commit: local/build.sh, the GitHub
# workflow and .gitlab-ci.yml. A build without them shows "unknown". Both
# arguments are here, in the last stage, thus a new commit starts no
# toolchain stage again. RUN sees each ARG as an environment variable, and
# android/build_aar.sh reads them from there.
ARG OMN_GO_COMMIT=
ARG OMN_GO_COMMIT_TIME=

# The desktop binaries. CGO_ENABLED=0 for both targets: the binary then
# needs no C library on the computer of the user, and it does not depend
# on the C library of the image. TestBinarySize builds with the same flags.
# The -X values are the same as in android/build_aar.sh.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    export GOFLAGS="-trimpath" CGO_ENABLED=0 && \
    GO_LDFLAGS="-s -w -X net.basov.omngo/backend.buildCommit=${OMN_GO_COMMIT} -X net.basov.omngo/backend.buildCommitTime=${OMN_GO_COMMIT_TIME}" && \
    GOOS=linux GOARCH=amd64 go build -ldflags="$GO_LDFLAGS" -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    GOOS=windows GOARCH=amd64 go build -ldflags="$GO_LDFLAGS" -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

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
