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
RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init

# Intercept AAPT to guarantee SDK Target 34 at the lowest compiler level
RUN printf '#!/bin/bash\n\
MANIFEST_PATH=""\n\
get_next=0\n\
for arg in "$@"; do\n\
    if [ $get_next -eq 1 ]; then\n\
        MANIFEST_PATH="$arg"\n\
        get_next=0\n\
    elif [[ "$arg" == "-M" ]] || [[ "$arg" == "--manifest" ]]; then\n\
        get_next=1\n\
    elif [[ "$arg" == --manifest=* ]]; then\n\
        MANIFEST_PATH="${arg#*=}"\n\
    fi\n\
done\n\
if [ -n "$MANIFEST_PATH" ] && [ -f "$MANIFEST_PATH" ]; then\n\
    sed -i '\''s/<uses-sdk[^>]*>/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\\/>/g'\'' "$MANIFEST_PATH"\n\
    if ! grep -q "uses-sdk" "$MANIFEST_PATH"; then\n\
        sed -i '\''s/<application/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\\/> <application/g'\'' "$MANIFEST_PATH"\n\
    fi\n\
fi\n\
COMMAND_NAME=$(basename "$0")\n\
exec /opt/android/build-tools/33.0.2/${COMMAND_NAME}.real "$@"\n' > /tmp/aapt_wrapper.sh && \
    chmod +x /tmp/aapt_wrapper.sh && \
    mv /opt/android/build-tools/33.0.2/aapt /opt/android/build-tools/33.0.2/aapt.real && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt && \
    mv /opt/android/build-tools/33.0.2/aapt2 /opt/android/build-tools/33.0.2/aapt2.real && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt2

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
