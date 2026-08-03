# STAGE 2: Dependency Lock
FROM omn-go-base:latest AS project_builder

ARG KEYSTORE_PASSWORD
ARG KEY_ALIAS
ARG KEY_PASSWORD

# Set to 1 to skip the test gate.
ARG SKIP_TESTS=0

COPY . .

# The host go.mod from the COPY above has no go.sum.
RUN cp /root/lockfiles/go.mod /root/lockfiles/go.sum ./

# Safety net, not a resolution step. No network while go.mod matches
# the source imports.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod tidy

# Quality gate before the desktop and APK steps, so a failing test aborts
# early. The tests need the debug info the release GOFLAGS strip.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    if [ "$SKIP_TESTS" = "1" ]; then \
        echo "WARNING: SKIP_TESTS=1 - test gate bypassed"; \
    else \
        go vet ./backend/... && \
        go test ./backend/...; \
    fi

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    export GOFLAGS="-ldflags=-s -w -trimpath" && \
    GOOS=linux GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

# Strictly no AndroidX or AppCompat.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    mkdir -p android/app/libs && \
    gomobile bind -target=android -androidapi 24 -javapkg net.basov.omngo -ldflags="-s -w" -o android/app/libs/omngo.aar ./backend

# Standard flavor only. F-Droid builds and signs its own fdroid APK,
# which an APK built here would not match.
RUN --mount=type=cache,target=/root/.gradle,sharing=locked \
    cd android && \
    gradle assembleStandardRelease && \
    cp app/build/outputs/apk/standard/release/*.apk ../bin/
