# OMN-Go instructions for Codex

Read `CLAUDE.md` for the project rules. Read `doc/TERMINOLOGY.md` before you write prose.
Use `doc/CODE_MAP.md` to find the files for a feature.

## Changes

- Preserve user changes in the working tree.
- For each commit, update the version in `backend/version.go`, `android/app/build.gradle`, and `doc/CODE_MAP.md`.
- Calculate the Android version code as `YY * 100000 + MM * 1000 + S`.
- Give the maintainer a unified diff for `git apply` and a commit message.
- Leave commits, tags, and pushes to the maintainer.

## Native builds on Linux

The `Dockerfile` defines the reference build and toolchain versions.
If Docker storage is limited, use `bash local/codex/build.sh` with one target.

| Target | Result |
| --- | --- |
| `all` | Run the test gate. Build the desktop executables and the standard release APKs. |
| `test` | Run `go vet` and the Go, Java, and JavaScript tests. |
| `desktop` | Run the test gate. Build the Linux and Windows x86-64 executables. |

1. Add the pinned Go compiler, Node.js, and the JDK to `PATH`.
2. For `all`, install the SDK, NDK, build tools, and Gradle from the `Dockerfile`.
3. For `all`, set `ANDROID_HOME` and add Gradle to `PATH`.
4. For `all`, set `KEYSTORE_FILE`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, and `KEY_PASSWORD`.

Use a local test key for test APKs. Keep the key and its passwords outside the source files.
Use a full JDK with `javac` and `jlink`. Set `JAVA_HOME` to that JDK for Gradle.

The script copies the files that Git tracks with their current working tree contents.
For a new source file, use `git add -N <path>` before the build.
The test gate includes repository checks that use the documentation.

Set `TMPDIR` to a writable directory with space for the build.
Set `GOPATH`, `GOCACHE`, and `GRADLE_USER_HOME` if their defaults are outside writable paths.
Keep the caches between builds to reuse the dependencies.
The default output directory is `output-binaries/native/`. Set `OMN_GO_OUTPUT_DIR` to change it.

Report the test results, the targets that built, and the platforms that ran.
