# How OMN-Go is tested

<!-- Summary: A description of the tests of OMN-Go: what each test examines and where it runs. It also tells what no automatic test reaches, and how to read the binary size report and the benchmarks. -->

This document says what is tested, where each test runs, and what no test
reaches. Read it before you add a test or change the build.

The test set has three parts. Each one runs from the same command:

```sh
go vet ./backend/... && go test ./backend/...
```

The `test` stage of the Docker files runs that command. Three builds use
that stage:

| Build | When | What it makes |
| --- | --- | --- |
| `.github/workflows/test.yml` | Each push to `master` and each pull request | A test result only. It needs no Android tool and no secret. |
| `.github/workflows/android-gomobile-release.yml` | Each push to `master` and each tag | The artifacts. `project_builder` waits for the `test` stage. |
| `.gitlab-ci.yml` | Each version tag | The artifacts, the same way. |
| `local/build.sh` | On request | The artifacts, the same way. |

The F-Droid build runs no test. It builds a tag after the GitHub build of
that tag passed the gate.

---

## 1. The three parts

| Part | Where | Language of the test | Needs |
| --- | --- | --- | --- |
| The Go application | `*_test.go` of each package under `backend/internal/` | Go | nothing |
| The Android configuration reader | `android/test/` | Java | a JDK |
| The frontend pure functions | `backend/frontend/test/*.test.js` | JavaScript | Node |

This document gives no count of tests. A count in a document goes out of
date with the next test. `go test -v ./backend/...` lists each test.

Each package holds the tests of its own code. `backend/internal/repocheck`
holds only tests. Each one reads files outside one package: the source
scans, the ports between two languages, the build files and the documents.
The tests of a feature package build their stand-in App with
`backend/internal/testkit`. The tests of `backend/internal/app` use the real
App.

The Java and the JavaScript tests are started BY a Go test. There is no
second command and no second gate.

* `backend/internal/repocheck/java_test.go` compiles and runs the two
  files of `android/test/java/net/basov/omngo/`: `OmnConfigTest.java` and
  `OmnTextTest.java`.
* `backend/internal/repocheck/js_test.go` runs `node --test` over
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
`OmnTextTest.java` has the same form.

The frontend has no build step, and rule 4 of `CLAUDE.md` says why. A test
runner from npm would be the first `package.json` of this project.
`node --test` is part of Node 18 and later, thus the tests need no package
and no `node_modules`.

`TestAndroidGradleHasOneDependency` in `backend/internal/repocheck/java_test.go`
fails when a dependency appears.

### The Java under test imports no Android package

`org.json` is part of the Android framework. A plain Java virtual machine
cannot load it, thus a test of a class that uses it needs an emulator.

`OmnConfig.java` therefore reads `config.json` with `java.io` and parses
it with a small parser of its own. `javac` and `java` alone then run a
test of it. See the banner of that file.

`OmnText.java` holds the text rules of the Android layer, for example the
safe name of a shared file. It reads JSON with `OmnConfig.parseFlat`.
`TestPureJavaClassesImportNoAndroidPackage` holds the rule for the two
classes. Each other class of the layer uses the framework, and only the
Gradle build compiles it.

### The JavaScript under test runs in a stub of a browser

There are TWO stubs, and the difference between them found a fault. A
third file, `mini-dom.js`, gives the page stub a real document. See "The
tests that press a control" below.

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
`SYNC_TITLES` sat in the `if` block of `omn-go-api.js` while
`omn-go-sync.js` read it as a bare name. In a browser, each upload threw
"SYNC_TITLES is not defined", and the "Commit & Push" button did nothing.
In a Node module, the fault did not show. See
`doc/decisions/0015-load-the-click-driven-scripts-on-demand.md`.

`lazy.test.js` uses the page stub. It reads the `omnLazy` and
`omnLazyActions` calls of `omn-go-api.js`, loads each lazy file ALONE, and
calls each name and each action that the calls promise. A ReferenceError is
a failure. A TypeError is not, because the page is a stub and not a browser.
It also fails for an action that the list does not name. A function that a
lazy file puts on `window` with no promise fails it too.

`actions.test.js` uses the page stub. It loads each application script
that `index.html` names, in the order of `index.html`. It sends a click to
the real click listener. It checks
that a control with `data-action` calls its function. It also checks that a
page from disk does not throw. See
`doc/decisions/0020-name-the-work-of-a-control-in-data-action.md`.

### The tests that press a control

`backend/frontend/test/mini-dom.js` is a small document. It holds elements,
attributes, `classList`, `innerHTML`, `querySelector` and events with the
capture phase and the bubble phase. It has no layout, no CSS and no
network. The project has no `node_modules`, thus no test can use a DOM
library.

`backend/frontend/test/dom-page.js` builds a page from that document and
from the REAL templates: the body of `index.html` and `modals.html`. A
control that a template loses is thus absent in the test too. `notePage()`
loads the scripts that `index.html` names and sends the load events. The
test then sets a server function, presses a control with `h.press(selector)`
and reads `h.requests`, `h.dialogs` and the page.

`configPage()` builds the Config page from `config_page.html` and two git
server cards, and it also loads `omn-go-config.js`. `mini-dom.js` has
`FormData`, thus a test reads the body of a save the same as the server
does.

`editorPage(text, opts)` builds the editor from `editor.html` and loads
`omn-go-editor.js` with `text` as the note. `opts.search` is the query of
the address, `opts.storage` fills `localStorage`, and `opts.server` answers
the requests. The page has the `insertText` command of a browser, and
`opts.noInsertText` takes it away.

`systemPage(template, script, server)` builds a system page: the shell
with one template as its body, and the script of that page.

`bookmarkPage(bookmarks, opts)` builds the bookmark page: a note page that
loads `Bookmarker.js` after the scripts of `index.html`, with `bookmarks`
as the list of the note. `opts.search` and `opts.hash` are the query and
the fragment of the address, and `opts.storage` fills `localStorage`.

Fourteen test files use these pages:

| File | What it holds |
| --- | --- |
| `sync.test.js` | Abort and Cancel send nothing. Each force push has a commit message. A new host key is stored only after OK. |
| `bookmark.test.js` | The panel, the tag list, Save, the share from Android and `omnGoInsertCapture`. |
| `config.test.js` | A save sends no secret that the person did not type. A reveal alone changes nothing. The menu, the Back control and the mark of a change. |
| `search.test.js` | The dialog sends no request while the person types. The two scopes, the keys, and the address of a chosen line. |
| `session.test.js` | The hint cookie, the answer 401 and the login box. |
| `editor-view.test.js` | An expansion keeps the view of the editor, from the Tab key and from the toolbar button. |
| `editor-page.test.js` | A load that failed switches the save off. The save, Cancel, the jump to a line, the selection cycle, the two view toggles and a dropped file. |
| `editor-find.test.js` | The find bar: the count, the steps, the three modes, and the text after Replace and Replace all. |
| `bookmarker.test.js` | The bookmark page of `Bookmarker.js`: the list, the tags, the search, the address of the page and the options of the device. |
| `nav.test.js` | The four answers to a press on a link of a note. The overlay of a slow page shows after 300 ms, and only for a press that leaves the page. |
| `share.test.js` | The metadata panel writes each value as text. Send, the two copy controls, and the two ways to the clipboard. |
| `status.test.js` | The Status page asks for a slow section only on a press. Each kind of value has its form, and each value is text. |
| `logs-page.test.js` | The Log page: the history, the new lines, the boxes of the filter, and a copy of the lines that show. |
| `compat.test.js` | The notice of an old browser: the limit of 85, one notice, and the close mark. |

A script that sets `location.href` leaves the page in a browser. The
document of a test stays, thus `location.href` keeps the address of the
page, and `h.went` holds each address that a script opened. `h.replaced`
holds each address that also took the page out of the history. `h.holdTimers()`
takes the timers of the page away from the clock, thus a test of a wait
ends at once.

**A test here proves what a script does with a document. It does not prove
that a browser agrees.** Check a new control in a real browser too.

### The line coverage of the scripts

`TestJavaScriptUnitTests` runs the tests with `NODE_V8_COVERAGE`, and
`backend/frontend/test/coverage.js` makes one number for each shipped
script. A line with only a comment and an empty line do not count, thus the
numbers are lower than the report of `node --experimental-test-coverage`.

The target is 60 percent of the code lines of each script.
`jsLineCoverageFloor` in `backend/internal/repocheck/js_test.go` lists the
scripts below the target, with the number of each. The list only shrinks,
and a number only rises. **The list is empty: each script is at the target.**
Do not add a row for a new script. Write its tests. The test fails in four
cases:

1. A script that is not in the list is below the target.
2. A script is below its floor.
3. A script is 5 points or more above its floor. Raise the floor.
4. A script of the list is at the target. Remove its row.

Run the measure by hand:

```sh
NODE_V8_COVERAGE=/tmp/cov node --test backend/frontend/test/*.test.js
node backend/frontend/test/coverage.js /tmp/cov
```

Two tests run BOTH languages against one another. Each one starts on the Go
side, because the Go side is what writes the input.

`TestEverySyncLineReachesTheOverlay` in
`backend/internal/gitsync/js_port_test.go` runs a whole sync, reads the lines
that it really wrote out of the log ring, and sends each one through the
real `applySyncLogLine`. A line that moves no stage is the failure. The
overlay fails quietly, thus nothing else would report it.

`TestLogFilterPortAgreesWithTheRealJavaScript` in
`backend/internal/app/js_port_test.go` compares `logx.Logger.Enabled` in
`backend/internal/logx/hub.go` with `logLinePrints` in `omn-go-api.js` over
120 states. It builds the value of `OMN_LOG_TAGS` the way
`render.Renderer.InjectRuntimeVars` does. A test that builds it another way
compares a state that no page ever holds.

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

**Node is in the `Dockerfile` only.** It builds the local, GitHub and GitLab
artifacts. The F-Droid build server installs what the recipe
names, and the recipe names no Node.

`OmnConfig.java` and `OmnText.java` ARE in `src/main`, thus F-Droid
compiles them. They add no dependency and no import outside `java.io` and
`java.util`.

---

## 4. The rules that exist in two languages

Some rules of this application exist two times on purpose. The Go side
answers for the server, and a copy answers for the page or for the Android
layer. `backend/internal/repocheck/ports_test.go` holds a test for each pair.

Most of those tests read the other language and compare a VALUE in its
source. That finds a rule that MOVED. It cannot find a copy that was wrong
the day a person wrote it, and `ports_test.go` says so.

**One pair is tested by running both.** `isHeaderFirstLine` and
`firstLineAfterHeader` in `omn-go-editor.js` are a port of
`backend/internal/noteheader/noteheader.go`.
`TestHeaderPortAgreesWithTheRealJavaScript` in
`backend/internal/repocheck/js_test.go` runs the real JavaScript through Node
and compares each answer against `noteheader.Parse`.

The cases live in `backend/frontend/test/header-cases.json`, and both
languages read that one file. Add a case there when you find a note shape
that the two might read differently.

**The table of user files is tested the same way.** `config.UserFileTrees`
in `backend/internal/config/user_files.go` names each tree of uploaded
files: `user_json`, `user_contacts` and `user_calendars`. `omn-go-editor.js`
and `OmnText.java` hold a copy each. The two tests
`TestUserFileTreesHaveTheirCopies` in
`backend/internal/repocheck/user_files_test.go` run the real JavaScript and
the real Java, and compare each answer with the Go table.

A copy of a rule that no test runs drifts from the original. This pair
drifted. Before this test existed, the two read four of eight note shapes
differently.

---

## 5. What no automatic test reaches

Three parts of this application have no test, and none of them can get one
without a change that this project has refused.

**The Android WebView itself.** The Chromium 85 floor, the three
fullscreen modes, the intent dispatch and the Termux path each need a
device or an emulator. An emulator needs a test framework, and a test
framework is a Gradle dependency.

**The git remote over SSH.** `backend/internal/gitsync/sync_test.go` drives the
real sync code against a BARE REPOSITORY ON DISK, which needs no server and no
network. It cannot test the SSH transport, and it cannot test a network failure.
`gitsync.Service.GetSSHAuth` runs in each of those tests, and the code that
speaks SSH does not.

**The F-Droid build.** Only F-Droid runs it. The tests in section 3 read
the files that the recipe depends on and check that the rules still hold.
That is the most that a test here can do.

A browser test is a fourth thing that this gate does not do. Chromium in
the build image would add about 300 MB against about 50 MB for Node, and
each cold build would pay it. A session or a separate job can drive a real
browser, and the release build stays as it is.

---

## 6. The binary size report

`TestBinarySize` in `backend/internal/repocheck/binary_size_test.go` builds the
release binaries and compares their size with a baseline build. It reports the
change in bytes and in percent for each target.

**The normal gate skips it.** Five builds take minutes on a cold cache.
Set `OMN_BINARY_SIZE=1` to run it:

```sh
OMN_BINARY_SIZE=1 go test -v -run 'TestBinarySize$' -timeout 30m ./backend/internal/repocheck/
```

The targets use the release flags of the Dockerfile and `CGO_ENABLED=0`.
A Linux build stands for the Android ABI of the same CPU.

| Target | Stands for |
| --- | --- |
| `linux-amd64` | the desktop application on Linux, and Android x86_64 |
| `windows-amd64` | the desktop application on Windows |
| `linux-arm64` | Android arm64-v8a |
| `linux-arm7` | Android armeabi-v7a |
| `linux-386` | Android x86 |

**The baseline.**
`backend/internal/repocheck/testdata/binary_size_baseline.json` holds the git
reference of the baseline build, its sizes, and the Go version that made them. A
different Go version also changes the size. The report then says so, and the
growth limit does not apply.

| Setting | Effect |
| --- | --- |
| `OMN_BINARY_SIZE=1` | Runs the test. |
| `OMN_BINARY_SIZE_BASE=<git reference>` | Builds that reference now, with the same Go version. Use the `ref` of the JSON file for the same baseline. This needs the `.git` directory. |
| `OMN_BINARY_SIZE_MAX_GROWTH=1.5` | Fails the test when a target grows by more than 1.5 percent. |
| `OMN_BINARY_SIZE_WRITE=1` | Writes the sizes of the baseline reference to the JSON file. |

Use `OMN_BINARY_SIZE_WRITE=1` after the build image gets a new Go version.

Two small tests run in the normal gate. `TestBinarySizeReport` checks the
report. `TestBinarySizeBaselineFile` checks the JSON file.

---

## 7. The benchmarks

`backend/internal/app/bench_test.go` holds seven benchmarks. Each one measures a
path that a person waits for.

| Benchmark | What it measures |
| --- | --- |
| `BenchmarkCompileBundledNotes` | The compile of each bundled note to HTML. |
| `BenchmarkServeCachedPage` | The answer for `/UserManual.html` from the cache. |
| `BenchmarkSearchPage` | The search of one note. |
| `BenchmarkSearchGlobal` | The global search of about 200 notes, with a typo in the query. |
| `BenchmarkSearchIndexBuild` | The build of the global index of about 200 notes. |
| `BenchmarkSQLQuery` | Two SELECT statements over 2000 rows through `/api/sql`. |
| `BenchmarkDBBackupRestore` | A backup of 2000 rows and its restore. |

**The normal gate does not run a benchmark.** It runs
`TestBenchmarkFixtures`. That test checks that each benchmark still
measures real data, and not an empty result or a fault page.

Use the benchmarks before and after a refactor patch:

1. Check out `master`.
2. Run the benchmarks and keep the result:

   ```sh
   go test -run '^$' -bench . -benchmem -count 5 ./backend/internal/app/ > before.txt
   ```

3. Apply the patch.
4. Run the same command again. Write the result to `after.txt`.
5. Compare the two files. A refactor patch must not make a path slower.

Run both measurements on the same device. A number from one device does
not compare with a number from another device.
