# STAGE 2: Dependency Lock
FROM omn-go-base:latest AS project_builder

ARG KEYSTORE_PASSWORD
ARG KEY_ALIAS
ARG KEY_PASSWORD

# Set to 1 to skip the test gate for an emergency build:
#   docker build --build-arg SKIP_TESTS=1 ...
ARG SKIP_TESTS=0

# go.mod and go.sum are deliberately excluded from this build's context
# (see Dockerfile.dockerignore) so this COPY cannot overwrite the
# already-resolved /app/go.mod + /app/go.sum baked into omn-go-base -
# those are the only copies that exist anywhere (the host has no Go
# toolchain to produce go.sum itself). This is what makes an ordinary
# source-only rebuild fully offline: nothing in this stage can trigger a
# module re-resolution or re-download. To pick up an actual dependency
# change, rebuild Dockerfile.base first (bump go.mod there) - that's the
# one place go.mod is ever written.
COPY . .

# Safety net, not a resolution step: reconciles go.mod against the now-
# fully-present source. Should be a no-op - and touch no network - as
# long as go.mod (inherited from the base image, untouched by the COPY
# above) hasn't drifted from what the source actually imports.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod tidy

# Quality Gate: vet + unit tests must pass before ANY artifact is built.
# Placed after go mod tidy (deps resolved) and before the desktop/APK
# steps, so a red test aborts the build before minutes of gomobile/gradle
# work. Deliberately run WITHOUT the release GOFLAGS (-s -w -trimpath)
# exported inside the desktop build step below - those strip debug info
# tests don't want anyway.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    if [ "$SKIP_TESTS" = "1" ]; then \
        echo "WARNING: SKIP_TESTS=1 - test gate bypassed"; \
    else \
        go vet ./backend/... && \
        go test ./backend/...; \
    fi

# Desktop Binary (OMN-Go naming convention)
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    export GOFLAGS="-ldflags=-s -w -trimpath" && \
    GOOS=linux GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

# Android APK - Webview Wrapper via Gradle & gomobile bind (strictly zero AndroidX/AppCompat)
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    mkdir -p android/app/libs && \
    gomobile bind -target=android -androidapi 24 -javapkg net.basov.omngo -ldflags="-s -w" -o android/app/libs/omngo.aar ./backend

# Gradle's own dependency cache, separately cache-mounted so re-running
# `gradle assembleRelease` with an unchanged build.gradle doesn't re-pull
# the Android Gradle Plugin / Maven dependencies from the network either.
RUN --mount=type=cache,target=/root/.gradle,sharing=locked \
    cd android && \
    gradle assembleRelease && \
    cp app/build/outputs/apk/release/*.apk ../bin/
