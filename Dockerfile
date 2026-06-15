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
# Android APK - Built normally, then post-processed with Apktool to inject targetSdkVersion and bump versionCode
RUN gomobile build -target=android -androidapi 21 -o bin/goomn.apk . && \
    wget -q https://github.com/iBotPeaches/Apktool/releases/download/v2.9.3/apktool_2.9.3.jar -O /tmp/apktool.jar && \
    java -jar /tmp/apktool.jar d bin/goomn.apk -o /tmp/apk_decoded && \
    perl -0777 -pi -e 's/<uses-sdk[^>]*>/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\/>/gs' /tmp/apk_decoded/AndroidManifest.xml && \
    sed -i -E 's/android:versionCode="[0-9]+"/android:versionCode="10021"/g' /tmp/apk_decoded/AndroidManifest.xml && \
    sed -i -E "s/versionCode: '[0-9]+'/versionCode: '10021'/g" /tmp/apk_decoded/apktool.yml && \
    java -jar /tmp/apktool.jar b /tmp/apk_decoded -o /tmp/goomn_unsigned.apk && \
    /opt/android/build-tools/33.0.2/zipalign -v -p 4 /tmp/goomn_unsigned.apk /tmp/goomn_aligned.apk && \
    keytool -genkey -v -keystore /tmp/debug.keystore -storepass android -alias androiddebugkey -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Android Debug,O=Android,C=US" && \
    /opt/android/build-tools/33.0.2/apksigner sign --ks /tmp/debug.keystore --ks-pass pass:android --key-pass pass:android --out bin/goomn.apk /tmp/goomn_aligned.apk && \
    rm -rf /tmp/apktool.jar /tmp/apk_decoded /tmp/goomn_unsigned.apk /tmp/goomn_aligned.apk /tmp/debug.keystore
