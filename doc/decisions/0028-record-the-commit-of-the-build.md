# 0028. Record the commit of the build

* Status: accepted
* Version: 26.11.14
* Code: `backend/version.go`, `android/build_aar.sh`, `Dockerfile`,
  `local/build.sh`, `backend/internal/status/collect.go`

## Context

The status page showed the version, but not the commit of the build. Two
builds with the same version can come from different commits. Go records the
commit itself only when it builds inside a git checkout, and no OMN-Go build
does that:

* The Docker build context has no `.git`. `.dockerignore` excludes it.
* gomobile compiles a generated main package in its own temporary directory,
  outside the checkout. This is also true on F-Droid.

## Decision

* `-ldflags -X` sets two variables of package `backend`: `buildCommit`, the
  full hash, and `buildCommitTime`, the commit time in RFC 3339. The names
  are not exported, because gomobile makes a Java binding for each exported
  name of that package.
* The Server section of the status page shows `build_commit`,
  `build_commit_time` and `build_commit_url`, a link to the commit on GitHub.
  A build with no commit shows `unknown`. The Git section stays the git sync
  of the notes.
* The Dockerfile takes the commit as the build arguments `OMN_GO_COMMIT` and
  `OMN_GO_COMMIT_TIME`, in the last stage only. A new commit thus starts no
  toolchain stage again.
* `local/build.sh` adds `-dirty` when git shows a changed or a new file.
  GitHub and GitLab give the commit with no `-dirty`.
* `android/build_aar.sh` asks git when the caller gives no commit and the
  checkout is a git repository. That is the F-Droid build. It adds no
  `-dirty`.

## Rejected alternatives

* **Put `.git` into the build context.** Go then records the commit in the
  desktop binaries, but not in the Android library, because gomobile builds
  outside the checkout. Go also marks the build as modified, because the
  build adds the x/mobile tools to `go.mod`.
* **`-dirty` on F-Droid.** fdroidserver changes files of the checkout, for
  example the signing configuration of `build.gradle`. Each F-Droid build
  would thus be `-dirty`, and its library would differ from the library of
  the GitHub build of the same commit.

## Consequences

* The same commit gives the same value on GitHub and on F-Droid. The
  comparison of `libgojni.so` stays possible.
* A local build of changed files differs from the GitHub build, because of
  `-dirty`. This is the purpose of the mark.
* GitLab gives `CI_COMMIT_TIMESTAMP`, and not the output of `git log`. The
  form of the time can differ from the GitHub value.
