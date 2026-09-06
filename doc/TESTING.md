# How OMN-Go is tested

This document says what is tested, where each test runs, and what no test
reaches. Read it before you add a test or change the build.

The test set has three parts. Each one runs from the same command, and the
Docker build runs that command before it makes any artifact.

```sh
go vet ./backend/... && go test ./backend/...
```

---

## 1. The three parts

| Part | Count | Language of the test | Needs |
| --- | --- | --- | --- |
| The Go application | about 380 tests | Go | nothing |
| The Android configuration reader | 1 test class | Java | a JDK |
| The frontend pure functions | 7 tests | JavaScript | Node |

The Java and the JavaScript tests are started BY a Go test. There is no
second command and no second gate.

* `backend/java_test.go` compiles and runs
  `android/test/java/net/basov/omngo/OmnConfigTest.java`.
* `backend/js_test.go` runs `node --test` over
  `backend/frontend/test/*.test.js`.

**Each one skips when its tool is absent.** A machine with no JDK and no
Node still runs the Go set and reports a pass. The Docker image holds both
tools, thus the full set runs there.

---

## 2. Why the Java and the JavaScript tests are shaped this way

### No test framework, in either language

The Android build has ONE Gradle dependency, and rule 1 of `CLAUDE.md`
section 1 says why. JUnit would be a second one. `OmnConfigTest.java` is
therefore a plain `main` method with three check helpers of ten lines.

The frontend has no build step, and rule 4 of `CLAUDE.md` says why. A test
runner from npm would be the first `package.json` of this project.
`node --test` is part of Node 18 and later, thus the tests need no package
and no `node_modules`.

`TestAndroidGradleHasOneDependency` in `backend/java_test.go` fails when a
dependency appears.

### The Java under test imports no Android package

`org.json` is part of the Android framework. A plain Java virtual machine
cannot load it, thus a test of a class that uses it needs an emulator.

`OmnConfig.java` therefore reads `config.json` with `java.io` and parses
it with a small parser of its own. `javac` and `java` alone then run a
test of it. See the banner of that file.

### The JavaScript under test runs in a stub of a browser

There are TWO stubs, and the difference between them found a fault.

`backend/frontend/test/dom-stub.js` makes the `window` and the `document`
that a shipped script touches WHILE IT LOADS, and nothing more. It loads
a script with `require`, thus each one runs as a Node module. Each shipped
script carries a short export tail behind a check of `module`, and a
browser never sees that export.

`backend/frontend/test/page-stub.js` builds a `vm` context whose GLOBAL is
the window stub, then runs a script the way a `<script src>` element does.
That is how a browser works: `window` IS the global object, thus
`window.runSync = ...` makes a global name and a bare name resolves
against the same object.

**A Node module has its own scope, and that difference hid a real fault.**
`SYNC_TITLES` sat in the `if` block of `omn-go-sse.js` while
`omn-go-sync.js` read it as a bare name. Every upload from 26.09.24 to
26.09.40 threw "SYNC_TITLES is not defined", and the "Commit & Push"
button did nothing.

`lazy.test.js` uses the page stub. It reads the `omnLazy` call of
`omn-go-sse.js`, loads each lazy file ALONE, and calls each name that the
call promises. A ReferenceError is a failure. A TypeError is not, because
the page is a stub and not a browser.

`editor.test.js` uses the DOM stub. Each case of it quotes
`backend/frontend/md/Editor.md`, which is the note that a person reads
before they type. A failure therefore reads in one of two ways. The
editor broke, or the note is now wrong.

---

## 3. What the F-Droid build sees

**Nothing of the test set.** F-Droid builds the committed Gradle
configuration on its own server, from the recipe at
`metadata/net.basov.omngo.fdroid.yml`. That recipe is hard to change,
because `AutoUpdateMode: Version` copies the last build block for each new
tag.

Four rules keep it correct with no edit, and a test holds each one:

| Rule | Test |
| --- | --- |
| The Android build keeps one Gradle dependency. | `TestAndroidGradleHasOneDependency` |
| No `test` or `androidTest` source set under `android/app/src`. | `TestNoAndroidTestSourceSet` |
| The `standard` and `fdroid` flavors keep their names. | `TestFdroidFlavorStillExists` |
| No file of `backend/frontend/test` reaches a device. | `TestFrontendTestsAreNotShipped` |

**The Java test lives at `android/test/`**, outside the Gradle project.
Gradle reads a source set only under `android/app/src`, thus it never
compiles this directory and the F-Droid build cannot see it.

**Node is in `Dockerfile.base` and `Dockerfile.ci` only.** Those two build
the GitHub artifacts. The F-Droid build server installs what the recipe
names, and the recipe names no Node.

`OmnConfig.java` IS in `src/main`, thus F-Droid compiles it. It adds no
dependency and no import outside `java.io` and `java.util`.

---

## 4. The rules that exist in two languages

Some rules of this application exist two times on purpose. The Go side
answers for the server, and a copy answers for the page or for the Android
layer. `backend/ports_test.go` holds a test for each pair.

Most of those tests read the other language and compare a VALUE in its
source. That finds a rule that MOVED. It cannot find a copy that was wrong
the day a person wrote it, and `ports_test.go` says so.

**One pair is tested by running both.** `isHeaderFirstLine` and
`firstLineAfterHeader` in `omn-go-editor.js` are a port of
`header_block.go`. `TestHeaderPortAgreesWithTheRealJavaScript` in
`backend/js_test.go` runs the real JavaScript through Node and compares
each answer against `parseHeaderBlock`.

The cases live in `backend/frontend/test/header-cases.json`, and both
languages read that one file. Add a case there when you find a note shape
that the two might read differently.

That pair had already moved apart when it first got a test. Version
26.09.14 repaired it, and four of eight note shapes had disagreed.

---

## 5. What no automatic test reaches

Three parts of this application have no test, and none of them can get one
without a change that this project has refused.

**The Android WebView itself.** The Chromium 85 floor, the three
fullscreen modes, the intent dispatch and the Termux path each need a
device or an emulator. An emulator needs a test framework, and a test
framework is a Gradle dependency.

**The git remote over SSH.** `backend/git_sync_test.go` drives the real
sync code against a BARE REPOSITORY ON DISK, which needs no server and no
network. It cannot test the SSH transport, and it cannot test a network
failure. `getSSHAuth` runs in each of those tests, and the code that
speaks SSH does not.

**The F-Droid build.** Only F-Droid runs it. The tests in section 3 read
the files that the recipe depends on and check that the rules still hold.
That is the most that a test here can do.

A browser test is a fourth thing that this gate does not do. Chromium in
the build image would add about 300 MB against about 50 MB for Node, and
each cold build would pay it. A session or a separate job can drive a real
browser without touching the release build, and one did for 26.09.24.
