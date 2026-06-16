import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.11 Compiler Warning Suppression...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.10"', 'APP_VERSION = "1.2.11"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.10";', 'const APP_VERSION = "1.2.11";'),
        ("backend/frontend/index.html", "let v = '1.2.10';", "let v = '1.2.11';"),
        ("android/app/build.gradle", "versionCode 10210", "versionCode 10211"),
        ("android/app/build.gradle", 'versionName "1.2.10"', 'versionName "1.2.11"')
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

    # 2. Patch build.gradle to suppress Java compiler deprecation warnings
    build_gradle = "android/app/build.gradle"
    if os.path.exists(build_gradle):
        with open(build_gradle, "r", encoding="utf-8") as f:
            gradle_content = f.read()

        old_deps = """dependencies {
    implementation fileTree(dir: 'libs', include: ['*.jar', '*.aar'])
}"""

        new_deps = """dependencies {
    implementation fileTree(dir: 'libs', include: ['*.jar', '*.aar'])
}

// Suppress deprecated API usage notes during compilation (e.g. WebViewClient URL loading)
tasks.withType(JavaCompile).configureEach {
    options.compilerArgs += ["-Xlint:-deprecation"]
}"""

        if old_deps in gradle_content:
            gradle_content = gradle_content.replace(old_deps, new_deps)
            with open(build_gradle, "w", encoding="utf-8") as f:
                f.write(gradle_content)
            print(f"  [+] Injected compiler warning suppression into {build_gradle}")
        elif "-Xlint:-deprecation" in gradle_content:
            print(f"  [=] Compiler warnings already suppressed in {build_gradle}")
        else:
            print(f"  [-] Could not find the dependencies block to patch inside {build_gradle}")

    commit_msg = """build(android): suppress javac deprecation warnings

- Added `-Xlint:-deprecation` to `JavaCompile` tasks in app/build.gradle.
- Silenced compilation notes regarding legacy `shouldOverrideUrlLoading` without altering the pristine MainActivity.java source code.
- Bumped application to V1.2.11 (Android 10211)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()