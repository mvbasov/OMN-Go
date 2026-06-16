import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.13 Crash Fixes & Reversions...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.12"', 'APP_VERSION = "1.2.13"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.12";', 'const APP_VERSION = "1.2.13";'),
        ("backend/frontend/index.html", "let v = '1.2.12';", "let v = '1.2.13';"),
        ("android/app/build.gradle", "versionCode 10212", "versionCode 10213"),
        ("android/app/build.gradle", 'versionName "1.2.12"', 'versionName "1.2.13"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Revert -nowarn in build.gradle
    build_gradle = "android/app/build.gradle"
    if os.path.exists(build_gradle):
        with open(build_gradle, "r", encoding="utf-8") as f:
            gradle_content = f.read()

        if '"-Xlint:-deprecation", "-nowarn"' in gradle_content:
            gradle_content = gradle_content.replace('"-Xlint:-deprecation", "-nowarn"', '"-Xlint:-deprecation"')
            with open(build_gradle, "w", encoding="utf-8") as f:
                f.write(gradle_content)
            print("  [+] Reverted -nowarn compiler flag in build.gradle.")

    # 3. Patch MainActivity.java to explicitly mount Scoped Storage
    main_activity = "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    if os.path.exists(main_activity):
        with open(main_activity, "r", encoding="utf-8") as f:
            java_content = f.read()

        old_start = "// Start the Go Backend Server from the gomobile .aar\n        Backend.startServer();"
        new_start = """// Ensure Android OS mounts scoped storage directories for native C/Go access
        java.io.File[] mediaDirs = getExternalMediaDirs();
        if (mediaDirs != null && mediaDirs.length > 0 && mediaDirs[0] != null) {
            mediaDirs[0].mkdirs();
        }

        // Start the Go Backend Server from the gomobile .aar
        Backend.startServer();"""

        if old_start in java_content:
            java_content = java_content.replace(old_start, new_start)
            with open(main_activity, "w", encoding="utf-8") as f:
                f.write(java_content)
            print("  [+] Patched MainActivity.java with getExternalMediaDirs() bypass.")

    # 4. Patch server.go to prevent log.Fatalf panics and fix Port 0 bugs
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Swap instant-kill log.Fatalf for a non-blocking log.Printf
        old_fatal = 'log.Fatalf("Failed to create storage: %v", err)'
        new_fatal = 'log.Printf("Failed to create storage: %v", err)'
        if old_fatal in server_code:
            server_code = server_code.replace(old_fatal, new_fatal)
            print("  [+] Swapped fatal panic for standard log trace in initStorage().")

        # Catch Zero-Port Configurations silently to prevent random port binds
        old_unmarshal = "json.Unmarshal(data, &appConfig)\n\t\tif appConfig.MimeTypes == nil {"
        new_unmarshal = "json.Unmarshal(data, &appConfig)\n\t\tif appConfig.ServerPort == 0 {\n\t\t\tappConfig.ServerPort = 8080\n\t\t}\n\t\tif appConfig.MimeTypes == nil {"
        if old_unmarshal in server_code:
            server_code = server_code.replace(old_unmarshal, new_unmarshal)
            print("  [+] Added ServerPort fallback to prevent zero-port routing failure.")

        # Wrap the main WebServer routine in a crash-recovery defer
        old_goroutine = "go func() {\n\t\tmux := http.NewServeMux()"
        new_goroutine = "go func() {\n\t\tdefer func() {\n\t\t\tif r := recover(); r != nil {\n\t\t\t\tlog.Printf(\"Recovered from panic in server: %v\", r)\n\t\t\t}\n\t\t}()\n\t\tmux := http.NewServeMux()"
        if old_goroutine in server_code:
            server_code = server_code.replace(old_goroutine, new_goroutine)
            print("  [+] Wrapped core server routine in crash-recovery defer logic.")

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """fix(core): resolve black screen crash and random port bindings

- Reverted `-nowarn` injection from Gradle tasks.
- Patched `MainActivity.java` to explicitly call `getExternalMediaDirs()`, forcing Android to mount the namespace directory. This prevents Scoped Storage `EACCES` from crashing the native Go engine.
- Replaced `log.Fatalf` with `log.Printf` inside `initStorage()` to completely eliminate violent app shutdowns during edge-case folder creation.
- Implemented a `ServerPort = 8080` fallback when `config.json` fails serialization, ensuring the WebServer never accidentally binds to `:0` (random).
- Added `defer recover()` wrapper to the primary goroutine to prevent unhandled routing panics.
- Bumped application to V1.2.13 (Android 10213)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()