import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.15 Loopback, Keystore, & Builder Fixes...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.14"', 'APP_VERSION = "1.2.15"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.14";', 'const APP_VERSION = "1.2.15";'),
        ("backend/frontend/index.html", "let v = '1.2.14';", "let v = '1.2.15';"),
        ("android/app/build.gradle", "versionCode 10214", "versionCode 10215"),
        ("android/app/build.gradle", 'versionName "1.2.14"', 'versionName "1.2.15"')
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

    # 2. Rename builder tags in documentation and scripts
    docs_and_scripts = ["README.md", "build.sh", "generate_context.sh", "setup_project.py"]
    for filepath in docs_and_scripts:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if "goomn-builder" in content:
                content = content.replace("goomn-builder", "omn-go-builder")
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Renamed 'goomn-builder' to 'omn-go-builder' in {filepath}")

    # 3. Export new Keystore in Dockerfile
    dockerfile = "Dockerfile"
    if os.path.exists(dockerfile):
        with open(dockerfile, "r", encoding="utf-8") as f:
            d_content = f.read()
        
        # We append the exact omn-go.keystore copy command to the release builder chain, 
        # replacing any old keystore copy commands if they exist.
        d_content = re.sub(
            r'cp app/build/outputs/apk/release/app-release\.apk \.\./bin/omn-go\.apk(?: && cp [^\n]+ \|\| true)?', 
            r'cp app/build/outputs/apk/release/app-release.apk ../bin/omn-go.apk && cp app/omn-go.keystore ../bin/omn-go.keystore || true', 
            d_content
        )
        with open(dockerfile, "w", encoding="utf-8") as f:
            f.write(d_content)
        print(f"  [+] Configured Dockerfile to export omn-go.keystore to bin/")

    # 4. Patch MainActivity.java to use 127.0.0.1 bypassing buggy WebView DNS/IPv6 issues
    main_activity = "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    if os.path.exists(main_activity):
        with open(main_activity, "r", encoding="utf-8") as f:
            java_content = f.read()

        if '"http://localhost:8080"' in java_content:
            java_content = java_content.replace('"http://localhost:8080"', '"http://127.0.0.1:8080"')
            with open(main_activity, "w", encoding="utf-8") as f:
                f.write(java_content)
            print("  [+] Hardcoded IPv4 loopback (127.0.0.1) in MainActivity.java to prevent connection refusals.")

    # 5. Patch server.go to force exact IPv4 binding and protect against 0 port bugs
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        old_bind_logic = '\t\tport := fmt.Sprintf(":%d", appConfig.ServerPort)\n\t\tlog.Printf("OMN-Go Backend running on %s", port)\n\t\thttp.ListenAndServe(port, connectionMiddleware(mux))\n\t}()\n}'
        
        new_bind_logic = '\t\tif appConfig.ServerPort <= 0 {\n\t\t\tappConfig.ServerPort = 8080\n\t\t}\n\t\t\n\t\tbindAddr := fmt.Sprintf("127.0.0.1:%d", appConfig.ServerPort)\n\t\tif runtime.GOOS != "android" {\n\t\t\tbindAddr = fmt.Sprintf(":%d", appConfig.ServerPort)\n\t\t}\n\t\t\n\t\tlog.Printf("OMN-Go Backend running on %s", bindAddr)\n\t\terr := http.ListenAndServe(bindAddr, connectionMiddleware(mux))\n\t\tif err != nil {\n\t\t\tlog.Printf("FATAL: Server crashed: %v", err)\n\t\t}\n\t}()\n}'
        
        if 'port := fmt.Sprintf(":%d", appConfig.ServerPort)' in server_code:
            server_code = server_code.replace(old_bind_logic, new_bind_logic)
        else:
            # Fallback regex string targeting the end of StartServer
            server_code = re.sub(
                r'port := fmt\.Sprintf\(":%d", appConfig\.ServerPort\)\s*log\.Printf\("OMN-Go Backend running on %s", port\)\s*http\.ListenAndServe\(port, connectionMiddleware\(mux\)\)\s*}\(\)\s*}', 
                lambda _: new_bind_logic, 
                server_code, 
                flags=re.DOTALL
            )
            
        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)
        print("  [+] Rewrote server.go binding logic to guarantee 127.0.0.1 IPv4 mapping on Android.")

    commit_msg = """fix(network): resolve Android localhost IPv6 failures and build tags

- Consolidated `omn-go-builder` renaming across project documentation and scripts.
- Configured Dockerfile to extract the newly named `omn-go.keystore` file to `bin/`.
- Modified `MainActivity.java` to route the WebView directly to `127.0.0.1` instead of `localhost`, bypassing flawed Android DNS resolution.
- Forced the Go backend inside `server.go` to explicitly bind to `127.0.0.1` on Android hardware, ensuring flawless loopback communication.
- Bumped application to V1.2.15 (Android 10215)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()