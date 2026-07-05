# STAGE 2: Dependency Lock
FROM omn-go-base:latest AS project_builder

ARG KEYSTORE_PASSWORD
ARG KEY_ALIAS
ARG KEY_PASSWORD

# Set to 1 to skip the test gate for an emergency build:
#   docker build --build-arg SKIP_TESTS=1 ...
ARG SKIP_TESTS=0

COPY . .

# Restore the fully-resolved go.mod/go.sum stashed by Dockerfile.base at
# /root/lockfiles, undoing whatever the host's (go.sum-less, x/mobile-less)
# copies the COPY above just brought in. No .dockerignore trickery needed.
RUN cp /root/lockfiles/go.mod /root/lockfiles/go.sum ./

# Safety net, not a resolution step: reconciles go.mod against the now-
# fully-present source. Should be a no-op - and touch no network - as
# long as go.mod hasn't drifted from what the source actually imports.
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

RUN --mount=type=cache,target=/root/.gradle,sharing=locked \
    cd android && \
    gradle assembleRelease && \
    cp app/build/outputs/apk/release/*.apk ../bin/
