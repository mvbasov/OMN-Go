# STAGE 1: Toolchains & Cache
FROM golang:1.25-bookworm AS builder
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

# Install GoMobile
RUN git clone --depth 1 https://github.com/golang/mobile.git /tmp/mobile && \
    sed -i 's/<uses-sdk.*/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\/>/g' /tmp/mobile/cmd/gomobile/build_androidapp.go && \
    sed -i 's/uses-permission.*INTERNET.*/&\n    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" \/>\n    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" \/>\n    <uses-permission android:name="android.permission.MANAGE_EXTERNAL_STORAGE" \/>/g' /tmp/mobile/cmd/gomobile/build_androidapp.go && \
    cd /tmp/mobile/cmd/gomobile && \
    go install . && \
    gomobile init

# STAGE 2: Dependency Lock
WORKDIR /app
COPY go.mod ./
RUN go mod download || true

# STAGE 3: Build & Pack
COPY . .
RUN go get golang.org/x/mobile@latest && go mod tidy

# Desktop Binary (Linux example)
RUN GOOS=linux GOARCH=amd64 go build -o bin/goomn-desktop server.go main_desktop.go

# Android APK (Under 5MB, No AppCompat)
RUN gomobile build -target=android -androidapi 21 -o bin/goomn.apk .
