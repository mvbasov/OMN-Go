# STAGE 2: Dependency Lock
FROM omn-go-base:latest AS project_builder

ARG KEYSTORE_PASSWORD
ARG KEY_ALIAS
ARG KEY_PASSWORD

# Set to 1 to skip the test gate for an emergency build:
#   docker build --build-arg SKIP_TESTS=1 ...
ARG SKIP_TESTS=0

COPY . .
RUN go get -tool golang.org/x/mobile/cmd/gobind && \
    go mod tidy

# Quality Gate: vet + unit tests must pass before ANY artifact is built.
# Runs after `go mod tidy` (modules resolved) and before the desktop/APK
# steps, so a red test aborts the build before minutes of gomobile/gradle
# work. Tests run as a plain native-linux `go test` - deliberately WITHOUT
# the release GOFLAGS (-s -w -trimpath), which are only exported inside the
# desktop build step below and strip debug info tests don't want anyway.
RUN if [ "$SKIP_TESTS" = "1" ]; then \
        echo "WARNING: SKIP_TESTS=1 - test gate bypassed"; \
    else \
        go vet ./backend/... && \
        go test ./backend/...; \
    fi

# Desktop Binary (OMN-Go naming convention)
RUN VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    export GOFLAGS="-ldflags=-s -w -trimpath" && \
    GOOS=linux GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

# Android APK - Webview Wrapper via Gradle & gomobile bind (strictly zero AndroidX/AppCompat)
RUN mkdir -p android/app/libs && \
    gomobile bind -target=android -androidapi 24 -javapkg net.basov.omngo -ldflags="-s -w" -o android/app/libs/omngo.aar ./backend

RUN cd android && \
    gradle assembleRelease && \
    cp app/build/outputs/apk/release/*.apk ../bin/
