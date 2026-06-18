# Android Gomobile CI/CD Workflow Instructions

## Prerequisites
1. **Go Module**: Ensure your Go code is in a subdirectory (e.g., `go/`) and is a valid module.
2. **Android Project**: Ensure your Android `app/build.gradle` is configured to load th`.aar` from `libs/`.
   ```groovy
   // app/build.gradle
   repositories {
        flatDir { dirs 'libs' }
   }
   dependencies {
        implementation(name: 'your-lib-name', ext: 'aar')
   }
   ```
3. **Secrets**: Configure these in GitHub Repo Settings > Secrets > Actions:
   - `ANDROID_KEYSTORE_BASE64`: Run `openssl base64 -A -in your-keystore.jks` and paste output.
   - `KEYSTORE_PASSWORD`: Your keystore password.
   - `KEY_ALIAS`: Your key alias.
   - `KEY_PASSWORD`: Your key password.

## Workflow File
Create `.github/workflows/android-gomobile-release.yml`:

```yaml
name: Android Gomobile Release

on:
  push:
    tags:
      - 'v*' # Triggers on tags like v1.0.0

jobs:
  build:
    runs-on: ubuntu-latest
    env:
      GO_PACKAGE_PATH: "./go" # UPDATE: Path to your Go package
      AAR_NAME: "your-lib-name" # UPDATE: Name of your .aar (matches gradle)
      ANDROID_DIR: "." # UPDATE: Root of Android project if different

    steps:
      - name: Checkout Code
        uses: actions/checkout@v4

      # --- Setup Go ---
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install Gomobile
        run: |
          go install golang.org/x/mobile/cmd/gomobile@latest
          go install golang.org/x/mobile/cmd/gobind@latest
          gomobile init

      # --- Setup Java & Android ---
      - name: Set up IDK
        uses: actions/setup-java@v4
        with:
          distribution: 'temurin'
          java-version: '17'

      - name: Grant Execute Permission for gradlew
        run: chmod +x ./gradlew

      # --- Build AAR ---
      - name: Build Go Library (AAR)
        run: |
          cd $GO_PACKAGE_PATH
          # Output to Android app/libs directory
          gomobile bind -target=android -o ../app/libs/${{ env.AAR_NAME }}.aar ./...
        env:
          ANDROID_HOME: /usr/local/lib/android/sdk
          ANDROID_NDK_HOME: /usr/local/lib/android/sdk/ndk-bundle

      # --- Build APK ---
      - name: Build Release APK
        run: |
          cd $ANDROID_DIR
          ./gradlew assembleRelease

      # --- Sign APK ---
      - name: Sign APK
        uses: r0adkll/sign-android-release@v1
        id: sign_app
        with:
          releaseDirectory: ${{ env.ANDROID_DIR }}/app/build/outputs/apk/release
          signingKeyBase64: ${{ secrets.ANDROID_KEYSTORE_BASE64 }}
          alias: ${{ secrets.KEY_ALIAS }}
          keyStorePassword: ${{ secrets.KEYSTORE_PASSWORD }}
          keyPassword: ${{ secrets.KEY_PASSWORD }}
        env:
          BUILD_TOOLS_VERSION: "34.0.0"

      # --- Create Release ---
      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: ${{ steps.sign_app.outputs.signedReleaseFile }}
          generate_release_notes: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Usage
1. Save the YAML above to `.github/workflows/android-gomobile-release.yml`.
2. Update `GO_PACKAGE_PATH`, `AAR_NAME`, and `ANROID_DIR in the env section to match your project structure.
3. Push a tag: `git tag v1.0.0 && git push origin v1.0.0`.
4. GitHub Actions will build the `.aar`, compile the APK, sign it, and publish a release.

## For split `.apk`
```
      # --- Sign Split APKs ---
      - name: Sign APKs
        uses: r0adkll/sign-android-release@v1
        id: sign_app
        with:
          releaseDirectory: ${{ env.ANDROID_DIR }}/app/build/outputs/apk/release
          signingKeyBase64: ${{ secrets.ANDROID_KEYSTORE_BASE64 }}
          alias: ${{ secrets.KEY_ALIAS }}
          keyStorePassword: ${{ secrets.KEYSTORE_PASSWORD }}
          keyPassword: ${{ secrets.KEY_PASSWORD }}
        env:
          BUILD_TOOLS_VERSION: "34.0.0"

      # --- Split File List ---
      # The action outputs files separated by ':' (e.g., file1.apk:file2.apk)
      # We split them into an array to upload individually
      - name: Split Signed Files
        uses: jungwinter/split@v2
        id: split_files
        with:
          msg: ${{ steps.sign_app.outputs.signedReleaseFiles }}
          separator: ':'

      # --- Create Release with All APKs ---
      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          # Upload all split APKs (indexed 0 to N)
          files: |
            ${{ steps.split_files.outputs._0 }}
            ${{ steps.split_files.outputs._1 }}
            ${{ steps.split_files.outputs._2 }}
            ${{ steps.split_files.outputs._3 }}
            ${{ steps.split_files.outputs._4 }}
          generate_release_notes: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}   
````

### Multiplatform binaries
```
     # --- Build Go Binaries (Linux/Windows) ---
      - name: Build Go Binaries (Cross-Compile)
        run: |
          mkdir -p dist
          # Build Linux AMD64
          GOOS=linux GOARCH=amd64 go build -o dist/myapp-linux-amd64 ./cmd/myapp
          # Build Windows AMD64
          GOOS=windows GOARCH=amd64 go build -o dist/myapp-windows-amd64.exe ./cmd/myapp
          # (Optional) Build Mac
          # GOOS=darwin GOARCH=amd64 go build -o dist/myapp-darwin-amd64 ./cmd/myapp
        working-directory: ${{ env.GO_PACKAGE_PATH }}

      # --- Collect All Artifacts ---
      # The AAR is already in app/libs, APK in build/outputs, binaries in dist/
      # We move/copy everything to a single 'release-assets' folder for easy uploading
      - name: Prepare Release Assets
        run: |
          mkdir -p release-assets
          # Copy APKs (handle split APKs if necessary, or just copy all apks)
          cp ${{ env.ANDROID_DIR }}/app/build/outputs/apk/release/*.apk release-assets/ || true
          # Copy AAR
          cp ${{ env.ANDROID_DIR }}/app/libs/*.aar release-assets/ || true
          # Copy Binaries
          cp ${{ env.GO_PACKAGE_PATH }}/dist/* release-assets/
          ls -R release-assets

      # --- Create Release with All Assets ---
      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          # Upload everything in the release-assets folder
          files: |
            release-assets/*
          generate_release_notes: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```
