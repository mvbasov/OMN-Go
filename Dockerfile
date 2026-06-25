# STAGE 1: Toolchains & Cache
FROM golang:1.26-bookworm AS builder

ARG KEYSTORE_PASSWORD
ARG KEY_ALIAS
ARG KEY_PASSWORD

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    openjdk-17-jdk wget unzip cmake ninja-build \
    && rm -rf /var/lib/apt/lists/*

# Install Android CMD Line Tools
RUN wget https://dl.google.com/android/repository/commandlinetools-linux-10406996_latest.zip -O /tmp/cmd.zip && \
    mkdir -p /opt/android/cmdline-tools && \
    unzip /tmp/cmd.zip -d /opt/android/cmdline-tools && \
    mv /opt/android/cmdline-tools/cmdline-tools /opt/android/cmdline-tools/latest && \
    rm /tmp/cmd.zip

ENV ANDROID_HOME=/opt/android
ENV PATH=$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools

# Accept licenses and install platform dependencies
RUN yes | sdkmanager --licenses && \
    sdkmanager "platforms;android-34" "build-tools;33.0.2" "ndk;25.2.9519653"

# Install Gradle
RUN wget -q https://services.gradle.org/distributions/gradle-8.5-bin.zip -O /tmp/gradle.zip && \
    mkdir -p /opt/gradle && \
    unzip -q /tmp/gradle.zip -d /opt/gradle && \
    rm /tmp/gradle.zip
ENV PATH=$PATH:/opt/gradle/gradle-8.5/bin

# Install GoMobile
RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init

# STAGE 2: Dependency Lock
WORKDIR /app
COPY go.mod ./
RUN go mod download || true

# STAGE 3: Build & Pack
COPY . .
RUN go get github.com/yuin/goldmark@latest && go get golang.org/x/mobile@latest && go mod tidy

# Desktop Binary (OMN-Go naming convention)
RUN VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/version.go) && \
    export GOFLAGS="-ldflags=-s -w -trimpath" && \
    GOOS=linux GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

# Android APK - Webview Wrapper via Gradle & gomobile bind (strictly zero AndroidX/AppCompat)
RUN go get -tool golang.org/x/mobile/cmd/gobind && \
    go mod tidy && \
    mkdir -p android/app/libs && \
    gomobile bind -target=android -androidapi 24 -javapkg net.basov.omngo -ldflags="-s -w" -o android/app/libs/omngo.aar ./backend

RUN cd android && \
    gradle assembleRelease && \
    cp app/build/outputs/apk/release/*.apk ../bin/
