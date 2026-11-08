# 0027. Build the Android library with one script

* Status: accepted
* Version: 26.11.11, 26.11.12
* Code: `android/build_aar.sh`, `Dockerfile`,
  `backend/internal/repocheck/pipelines_test.go`

## Context

OMN-Go has three builds of the Android package: the local build, the GitHub
and GitLab build, and the F-Droid build. The maintainer keeps them as similar
as possible. A change of the F-Droid recipe needs a merge request to
fdroiddata and the work of an F-Droid maintainer.

Each build added `golang.org/x/mobile` with
`go get -tool golang.org/x/mobile/cmd/gobind golang.org/x/mobile/cmd/gomobile`,
with no version. That command takes the newest x/mobile. A check on
2026-11-08 found that it also raises three modules that go into
`libgojni.so` and into the desktop binaries:

| Module | Without x/mobile | With the newest x/mobile |
| --- | --- | --- |
| `golang.org/x/crypto` | v0.53.0 | v0.58.0 |
| `golang.org/x/net` | v0.56.0 | v0.61.0 |
| `golang.org/x/sys` | v0.46.0 | v0.49.0 |

Two builds of one commit on two days thus gave two different libraries.
GitHub and F-Droid build on different days.

A new x/mobile can also ask for a newer Go. With `GOTOOLCHAIN=auto`, Go then
downloads a second compiler and reports no error. That breaks the rule that
the image and the recipe use the same compiler.

gomobile writes a `go.mod` in its temporary directory, with
`replace net.basov.omngo => <the directory of the module>`. Go records that
path in `libgojni.so`, also with `-trimpath`. A check on 2026-11-09 showed
that. The Docker build uses `/app`, and F-Droid uses its own build
directory. The two libraries thus could not be the same.

The gomobile steps were in three files: `Dockerfile.base`, `Dockerfile`, and
`Dockerfile.ci`, and a fourth copy was in the F-Droid recipe. The recipe copy
had no `-trimpath`.

## Decision

* `android/build_aar.sh` holds each gomobile step: the tools, `gomobile init`
  and the one `gomobile bind` line.
* `XMOBILE_VERSION` in that script is the only pin of x/mobile. The script
  sets `GOTOOLCHAIN=local`.
* The bind step runs from the fixed path `/tmp/omn-go-src`, a symbolic link
  to the checkout. Go reads the path from `PWD`, thus it records the link
  and not the real directory.
* One `Dockerfile` builds the local, GitHub and GitLab artifacts. It calls
  the script with `tools`, `init` and `bind`, one step for each image layer.
* The F-Droid recipe can call the script with no argument. That is a
  separate merge request to fdroiddata.
* The image is `golang:1.26.0-trixie` with JDK 21, the Debian and the JDK of
  the F-Droid server.
* `android/app/build.gradle` pins `buildToolsVersion "34.0.0"`. That is the
  default of AGP 8.2.2, thus the F-Droid build does not change.

## Rejected alternatives

* **Pin x/mobile in `go.mod`.** The pin then needs `tool` lines and a
  `require` line in `go.mod`, and `go.mod` has three dependencies on purpose.
  It does not help the recipe either: a `go get -tool` with no version
  upgrades a pinned x/mobile. A check on 2026-11-08 showed that.
* **Keep the gomobile steps in the recipe.** Each change of a flag or of the
  version then needs a merge request to fdroiddata.

## Consequences

* A change of a flag or of the x/mobile version is a change in this tree.
  When the recipe calls the script, F-Droid takes the change with the next
  `f` tag, with no new merge request.
* A change of the script or of `go.mod` starts `go_env` and the last step of
  `base_env` again. The Android SDK and the NDK are in `android_env`, which
  reads no file of the repository, thus they do not download again.
* `TestXMobileHasOnePinnedVersion` holds the pin, `GOTOOLCHAIN=local`, and
  the rule that the `Dockerfile` has no gomobile step of its own.
* The recipe still installs build-tools 33.0.1, and it has no `-trimpath`.
  The tests do not compare these values until the recipe calls the script.
