# STAGE 1: Toolchains & Cache
FROM golang:1.22-bookworm AS builder
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
    sdkmanager "platforms;android-33" "build-tools;33.0.2" "ndk;25.2.9519653"

# Install GoMobile
RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init

# STAGE 2: Dependency Lock
WORKDIR /app
COPY go.mod ./
RUN go mod tidy && go mod download

# STAGE 3: Build & Pack
COPY . .

# Desktop Binary (Linux example)
RUN GOOS=linux GOARCH=amd64 go build -o bin/goomn-desktop server.go main_desktop.go

# Android APK (Under 5MB, No AppCompat)
RUN gomobile build -target=android -androidapi 21 -javapkg net.basov.goomn.fdroid -o bin/goomn.apk server.go main_android.go
