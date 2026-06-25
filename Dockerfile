# STAGE 2: Dependency Lock
FROM omn-go-base:latest AS project_builder

ARG KEYSTORE_PASSWORD
ARG KEY_ALIAS
ARG KEY_PASSWORD

COPY . .
RUN go get -tool golang.org/x/mobile/cmd/gobind && \
    go mod tidy

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
