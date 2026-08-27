# OMN-Go development rules

These are the standing rules for work on the OMN-Go repository.
Read this document before you change code, tests, documents, or build files.

Source: repository `https://github.com/mvbasov/OMN-Go` at commit `0787bab`, version 26.08.69, plus the four patches of 26.08.70 to 26.08.73.

This document uses ASD-STE100 Simplified Technical English. See section 10.

---

## 1. Fixed constraints

Each constraint has a stated reason in the tree.
Do not remove a constraint without an instruction from the maintainer.

1. **No AndroidX. No AppCompat.** The Android layer uses `android.app.Activity`,
   `android.webkit.WebView`, and `android.app.AlertDialog` only. The one Gradle
   dependency is `fileTree(dir: 'libs', include: ['*.jar','*.aar'])`.
2. **Do not use `html/template`.** It calls `reflect.Value.MethodByName`. That call
   stops linker dead-code elimination for the whole program. The go-git method
   surface then makes the binary large. Use `fill()` with `%%PLACEHOLDER%%` tokens.
   Use the `escapeHTML` and `escapeJS` pair in `backend/templates.go`.
3. **Keep Android storage isolated.** Files stay in
   `/storage/emulated/0/Android/media/<applicationId>/`. This directory needs no
   runtime permission. The Go package cannot read the flavor applicationId.
   `ServerService.storageDir(ctx)` passes the path into `StartServer`. The
   `runtime.GOOS == "android"` branch in `initStorage` is a fallback only.
4. **Do not use bare 64-bit atomics.** Use the `atomic.Int64` and `atomic.Uint64`
   types. Never write `atomic.AddInt64(&field, ...)`. A bare 64-bit atomic panics on
   `armeabi-v7a` and on `x86`. F-Droid publishes those builds. The test
   `TestNoBare64BitAtomics` in `backend/middleware_test.go` scans the source and
   enforces this rule.
5. **The WebView floor is Chromium 85 and `minSdk 23`.** `html/js/omn-go-compat.js`
   holds the only ES5 code in the project. Two rules keep it working, and
   `TestCompatScriptIsFirstAndES5` enforces both. **Keep the file in ES5**, and
   **keep it the first script in `templates/index.html`**, with no `defer` and no
   `async`. A `<script src>` element is its own parse unit, thus a SyntaxError in a
   modern script cannot stop it. Do not move this code into `omn-go-core.js`: that
   file is modern, an old WebView drops all of it, and the notice would go with it.
   It was inline until 26.08.73, and it moved out because the compiled page of each
   note carried a copy. Other scripts can use `async`, arrow functions, and
   template literals.
6. **Keep F-Droid compatibility.** `metadata/net.basov.omngo.fdroid.yml` is the
   F-Droid recipe. It builds from the committed Gradle configuration. The Docker
   build makes `assembleStandardRelease` only. Do not build the `fdroid` flavor for
   distribution. F-Droid signs the package on its own server. A local build looks
   official, but it is not official.
7. **Give each decision one authority.** Each decision has one implementation.
   `parseHeaderBlock` is the only header-block parser. `renderAndCache` is the only
   writer of `html/<name>.html`. `resolvePageName` is the only name resolver.
   `hasRole` is the only role check. `resolveContentType` is the only MIME resolver.
   Do not add a second implementation. Extend the first one.
8. **An upgrade never overwrites a user-owned asset.** A version change replaces the
   files in `versionDependentAssets` (`backend/assets.go`). It first copies the old
   files to `asset_backups/<previous>/`. The app creates `md/Welcome.md`,
   `html/json/bookmarker-tags.json`, `omn-go-custom.css`, and `omn-go-custom.js` when
   they are absent. After that it leaves them alone.
9. **The `local-` name rule.** A path segment that starts with `local-` stays on the
   device. Git excludes it. A force pull keeps it. The rule has two implementations
   on purpose. `isLocalOnlyPath` covers the index. `gitignoreLocalOnlyPattern` covers
   new files. A test compares the two. The pattern must stay last in
   `gitignorePatterns`, because go-git reads patterns from the end.
10. **LAN sharing is off by default.** When `share_lan` is false, the listener binds
    `127.0.0.1`. The socket enforces the limit, not the authentication code. A local
    connection from `127.0.0.1`, `::1`, or `localhost` always counts as admin.
11. **Intent URIs and Termux intents are off by default.** Each one needs
    `enable_intent_uri` or `enable_termux_intent`, a runtime permission, and a
    confirmation for each tap.

---

## 2. Repository map

| Path | Contents |
| --- | --- |
| `main_desktop.go` | The only file in `package main`. It holds the only build tag: `//go:build !android`. |
| `backend/` | The full Go application. One flat `package backend`. About 30 files, not counting tests. |
| `backend/frontend/templates/` | Server-side page fragments. Embedded as `templatesFS`. Never extracted to disk. |
| `backend/frontend/html/` | `js/`, `css/`, `css/fonts/`, `json/`, `favicon.ico`. Embedded as `staticFS`. Extracted to the storage directory on demand. The user can edit these files with `?edit=true`. |
| `backend/frontend/md/` | The bundled system notes. Examples: `Welcome.md`, `UserManual.md`, `Database.md`, `ScriptRules.md`. Also a `Test/OMN-Go/` demonstration tree. |
| `android/` | Hand-written Gradle files and plain Java: `MainActivity.java`, `ServerService.java`, `ExportProvider.java`. The build generates `android/app/libs/omngo.aar` with `gomobile bind`. |
| `local/` | Maintainer scripts. The Docker context excludes this directory. The build never ships it. |
| `fastlane/metadata/android/en-US/` | Store metadata and `changelogs/<versionCode>.txt`. Each changelog line starts with `•`. |
| `metadata/` | `net.basov.omngo.fdroid.yml`, the F-Droid build recipe. |
| `doc/` | Maintainer documents. `API.md` holds the endpoint reference. `TERMINOLOGY.md` holds the controlled vocabulary. `initial_prompt.md` holds the historical origin prompt. |
| `CLAUDE.md` | This document. The Docker context excludes it. |

The repository does not hold `go.sum`, `output-binaries/`, `data/`, `.env`, or keystores.

Two statements in the tree are wrong. Do not trust them.

* The README says that the repository does not hold the offline assets. That text is
  old. The repository holds KaTeX, highlight.js, and the web fonts under
  `backend/frontend/html/`. Run `local/initial/offline_asset_downloader.sh` only to
  update these files.
* Some comments name `CODE_REVIEW.md`, `claude/note-exchange-plan.md`, and
  `claude/tags-page-plan.md`. These files do not exist, and git history has no record
  of them. They were AI session notes. Do not look for them.

---

## 3. Go rules

* The module is `net.basov.omngo`. The language version is `go 1.25`. The toolchain
  image is `golang:1.26-bookworm`.
* The module has three direct dependencies. Each one has its own `require` line:
  `github.com/yuin/goldmark`, `github.com/go-git/go-git/v5`, and
  `modernc.org/sqlite`. Keep this set small. A new dependency needs a reason and
  maintainer approval.
* **The build generates `go.sum` inside the container.** `Dockerfile.base` writes it
  to `/root/lockfiles`. Stage 2 of `Dockerfile` restores it. The host has no Go
  toolchain. Remember this before you change `go.mod`.
* The driver is `modernc.org/sqlite`, because it is pure Go and works with
  `CGO_ENABLED=0`.
* **Use one package.** Split the code by file and by concern, not by package.
* **Keep the exported surface small.** Export only what gomobile or the desktop entry
  point calls: `StartServer`, `AssetsRefreshed`, `GetServerPort`, and
  `WaitUntilReady`. Write everything else as a lowercase method on `*App`.
* **Names.** Use `handleXxx` for an API endpoint. Use `serveXxx` for a page or an
  asset. Use `renderXxxPage` with an `xxxView` struct. Use `normalizeXxx` for value
  repair. Write a predicate as a question: `isLocalOnlyPath`, `fileExists`, `hasRole`.
  Compile each regular expression once into a package-level `xxxRe` variable.
* **Branch on `runtime.GOOS`, not on a build tag.** The tree holds one build tag.
* **Errors.** Wrap an error with `fmt.Errorf("...: %w", err)`. Send an HTTP failure
  with `http.Error(w, msg, code)`. `doc/API.md` section 1.4 fixes the response
  shapes. A response is plain-text status words, or JSON with
  `"status":"success"` or `"status":"error"`, or `text/event-stream` for `/api/logs`.
* **Logging.** Do not call `log.Printf`. Write a step with
  `a.logDebugf(tag, format, ...)`, an outcome with `a.logInfof(tag, format, ...)`
  and a fault with `a.logErrf(tag, format, ...)`. `TestNoDirectLogPrintf` enforces
  this.
  * `tag` is a typed constant from `backend/log_levels.go`, for example `logSync`
    or `logAssets`. That file is the only authority for the tag set. Add a new tag
    to the constant block and to `allLogTags` together. The helper writes the
    brackets and the parentheses, thus a format string never carries them.
  * The emitted text is `[tag] (level) message`. Do not write `Error:`,
    `Warning:` or `FATAL:` in the message. The level word says it.
  * The Config page has two switches, `log_debug` and `log_info`, and one
    checkbox for each tag. A debug or info line prints when its level is on and
    its tag is ticked. **An error line ignores both.** A person who turns a level
    off asks for less noise, and never for fewer faults.
  * There are three levels and no more. The project has no leveled logger
    library and no structured logger.
  * `backend/logger.go` sends each line to stdout and to the SSE subscribers on
    `/api/logs`. **The SSE stream always carries every line.** The switches
    control stdout, and they control what `omn-go-sse.js` mirrors into the
    browser console. The sync progress overlay reads `[sync] (debug)` lines off
    the raw stream, and it must work when debug is off.
  * `applySyncLogLine` in `omn-go-sse.js` removes the level word before it
    matches a sync stage. Keep the two in agreement, or the progress overlay
    loses a stage.
  * A log line must never take the config lock. `loadConfig` holds the write
    lock and writes a line, and a Go RWMutex is not reentrant. `applyLogFilter`
    keeps an atomic copy of the three switches for that reason.
  * Two call sites keep `log.Printf`, because no `*App` can reach them:
    `loadTemplate` in `templates.go` runs at package init, and
    `search_sections.go` logs from a `sync.Once` and from a method on
    `searchDocument`. Both files write `[tag] (error) ` into the text by hand.
* **Configuration.** Read the configuration with `a.GetConfig()`. It returns a copy
  under `RLock`. Change the configuration with `a.WithConfig(func(c *Config){...})`.
  It reads and writes under `Lock`. Never touch `App.Config` directly. The
  `normalizeXxx` functions repair an unknown enum value. The loader, the POST
  handler, and the renderer then always agree. A request that omits a field leaves
  that field alone. See `configFieldSent` in `handlers.go`.
* **Routes.** Register every route in one block inside `StartServer`, on a plain
  `http.ServeMux`. Use the form
  `a.Router.HandleFunc("/api/x", a.authMiddleware(a.handleX, true))`. The boolean is
  `requireAdmin`. Add a comment to any registration that differs from this form.
* **Comments say why, at length.** Most files start with a `// ---` banner of 20 to
  60 lines. The banner gives the design decision, the rejected alternative, and often
  the bug that forced the change. Write the same kind of justification for new code
  that is not obvious. This is the strongest convention in the codebase.

---

## 4. Frontend rules

* **The frontend has no build step.** There is no `package.json`, no bundler, no
  PostCSS, and no `node_modules`.
* **Keep `templates/index.html` small.** The server copies its shell into
  `html/<name>.html` for each note. An inline script or an inline `style`
  attribute there costs its bytes again for each note, on disk and in each git
  sync. Put the code in an asset under `frontend/html/` and load it with a `src`
  or a `link`. `TestCompiledPageShellStaysSmall` guards the size.
* **Do not add Tailwind, React, or marked.js.** goldmark renders the markdown on the
  server. The word "Tailwind" stays only in the historical `doc/initial_prompt.md`.
* Write CSS by hand. `css/omn-go-core.css` declares the design tokens as `:root`
  custom properties. The theme is CSS only. It uses `data-theme` on `<html>` with a
  `prefers-color-scheme` fallback.
* **Module pattern.** Use an IIFE with an explicit `window.*` export. Attach anything
  that an inline `onclick=` calls to `window`.
* File roles:
  * `omn-go-core.js` holds the offline-safe part: render helpers, the KaTeX start
    code, the progress API, link interception, and the version footer.
  * `omn-go-sse.js` holds everything that calls the backend. The file body sits
    inside `if (window.location.protocol !== 'file:')`. The `else` branch replaces
    the same globals with stubs, so an exported page degrades quietly.
  * `omn-go-editor.js` holds the standalone editor page. It uses `var` in an
    ES5 style. It reads `OMN_EDIT_NAME`, `OMN_EDIT_EXT`, and `OMN_EDIT_VIEW`.
  * `omn-go-compat.js` holds the too-old-WebView notice, and nothing else. It is
    the only ES5 file. See section 1, rule 5.
  * `omn-go-custom.js` and `omn-go-custom.css` are user files. They are empty on
    purpose.
* **Load order is a feature.** The custom files load last. A user rule then wins
  against an app rule at equal specificity. The editor page loads neither custom
  file. A broken custom file can never lock the user out of the editor that repairs
  it.
* **The two embed trees are separate on purpose.** `staticFS` holds
  `frontend/html` and `frontend/md`. The app extracts these files, and the user can
  edit them. `templatesFS` holds `frontend/templates`. Templates are render logic.
  Never make them extractable.
* **Escape by hand and by context.** Use `escapeHTML(v)` for HTML text and for an
  attribute. Use `escapeJS(v)` for a JS string literal. Use `escapeHTML(escapeJS(v))`
  for a JS literal inside an HTML attribute. Splice trusted pre-rendered HTML raw.
* The server injects the runtime variables **when it serves the page**. It replaces
  the marker `<meta id="omn-go-runtime-vars-marker">`. A version bump therefore does
  not invalidate the HTML cache on disk. Do not "repair" this.

---

## 5. Note and page rules

* Call the metadata a **header block**. Do not call it front matter. See
  `doc/TERMINOLOGY.md`.
* `parseHeaderBlock` in `backend/header_block.go` is the only parser. A header block
  exists only if the first line holds a colon and does not start with a space, `#`,
  or `<`. The header block ends at the first empty line, which the parser drops. It
  also ends at the first non-header line, which the parser keeps.
* `isHeaderFirstLine` and `firstLineAfterHeader` in `omn-go-editor.js` are a direct
  port of the Go code. **Keep the two versions the same.**
* Read or write a single key with `setHeaderKey` or `splitHeaderRegion`. These
  functions splice into the original string. The result
  keeps `header + separator + body` equal to the content, byte for byte. Never
  rebuild a header block with a hard-coded `"\n\n"`.
* Write one line for each paragraph in a bundled note. `html.WithHardWraps()` is on.
  A line break in the source becomes a line break in the output.
* `html.WithUnsafe()` is on, so a raw script in a note can run. This is a deliberate
  trade, not a mistake.
* **Note scripts.** See `backend/frontend/md/ScriptRules.md`. A plain `<script>` runs
  during parsing. The elements below it do not exist yet. Use `window.onload` or
  `<script type="module">` when you need the full page. Keep state inside a block
  scope or an IIFE. Do not write a top-level `const` or `let`. Attach anything that
  an `onclick` calls to `window`. The server compiles a page once and caches it, so
  a note script must be idempotent.
* **SQL API.** `window.omnGoOpenDatabase(name)` returns a handle at once. The handle
  has `exec`, `batch`, `transaction`, and `readTransaction`. `window.openDatabase(...)`
  is the legacy WebSQL entry point. The Go side is `POST /api/sql` in
  `backend/sqlite.go`. Rules: admin only. All statements of one request run in one
  transaction. A database name must match `^[A-Za-z0-9_-]{1,64}$`. That pattern is
  the path-traversal guard. The body limit is 1 MiB. The statement limit is 500.
  Each database file lives at `<StorageDir>/db/<name>.sqlite`.
* **Database backups.** A backup holds the full database as JSONL. The user starts
  each backup by hand. File names are immutable:
  `html/db_backup/<db>/<UTCtimestamp>_<hostname>.jsonl`. The format version is 2.
  The app restores a backup automatically in one case only. That case is a bootstrap
  restore, when backups exist and no `.sqlite` file exists.

---

## 6. Version and release

* `backend/version.go` holds the version as `const APP_VERSION = "YY.MM.NN"`.
* **Bump the version in every commit.** The maintainer can ask for an exception.
* A bump changes **two files**:
  1. `backend/version.go`.
  2. `android/app/build.gradle`. Set `versionName`. Compute `versionCode` as
     `YY*100000 + MM*1000 + NN`.
* The F-Droid flavor multiplies `versionCode` by 10 and adds an offset for each ABI.
  See `android/app/fdroid-abi-versioncode.gradle`.
* **The maintainer creates every tag.** Do not create a tag.
* A tag without the `f` suffix starts the GitHub CI release build. That build
  publishes the desktop binaries and the APK.
* A tag with the `f` suffix starts the F-Droid build. Example: `v26.08.51f`.
* The `v1.x` tags use the old scheme. The `YY.MM.NN` scheme starts at `v26.07.36`.
* An F-Droid release also needs a changelog file at
  `fastlane/metadata/android/en-US/changelogs/<versionCode>.txt`. Write each entry as
  a `•` bullet line. Commit it as `release(f-droid): ...`.

---

## 7. Commits and branches

### Commit message format

A commit message has three parts: a subject line, one empty line, and a list of
the changes.

1. Subject line: `type(scope): Sentence. vYY.MM.NN`. Write **at most 80
   characters**. Count the version in that limit.
2. One empty line.
3. One `-` bullet for each change. Write one change in one bullet. Put a period
   at the end of each bullet.

Write the subject line and each bullet in Simplified Technical English. See
section 10.

```
docs(manual): Fix and improve manual. v26.08.60

- Fix four wrong statements, add the script empty-line rule.
- Group the configuration table into the same six parts as the Config page.
- Reflow every bundled note to one line for each paragraph.
```

A commit that makes one small change can have one bullet. Each commit needs the
subject line, also when it has no list.

### Types, scopes and branches

* Types in use, most frequent first: `fix`, `feat`, `build`, `refactor`, `release`,
  `chore`, `doc`, `tool`.
* Common scopes: `android`, `sync`, `git`, `core`, `ui`, `f-droid`, `build`,
  `search`, `test`, `editor`, `frontend`, `gitlab`, `github`, `markdown`, `exchange`,
  `config`, `db`, `ai`.
* Commit messages follow the vocabulary in `doc/TERMINOLOGY.md`.
* `master` is the working branch.
* The maintainer commits to `master` directly.
* Another contributor sends a pull request on GitHub against `master`.
* Ignore the `DS` branch. It is not active work.
* GitHub is the primary remote. `.github/workflows/sync-gitlab.yml` mirrors each push
  to GitLab. Do not push to GitLab by hand.

---

## 8. Tests

* Tests live in `backend/`. Each production file has one `_test.go` file beside it.
  All tests use `package backend`, so they are white-box tests. The suite holds about
  336 `Test*` functions. No tests exist outside `backend/`.
* The tests use the standard library `testing` package, `net/http/httptest`, and
  `t.TempDir()`. The project uses no assertion library and no mock library.
* Build the application under test with `newTestApp(t)` from `handlers_test.go`.
* Write a helper with a lowercase name. Take `t *testing.T` as the first parameter.
  Call `t.Helper()`.
* Add a file prefix to a helper name that can collide across files.
  `db_backup_test.go` uses `dbbApp`, `dbbExec`, and `dbbBackup`.
* Write table-driven tests with anonymous structs. Use `t.Run` rarely.
* **Give each test a comment that says why it exists.** A failure must then read
  either as "you broke it" or as "you changed it on purpose, so update the golden
  value".
* `baseline_test.go` pins behavior with golden sets. When you add a route, update
  `TestBaseline_RouteSet`. Also add a line to the changelog in that file header, and
  key the line to the version. The same rule covers a new injected runtime variable
  and a change to the `serveHTMLPage` dispatch table.
* Some tests scan the source and act as lint rules. Respect them.
* Run the tests with `go vet ./backend/... && go test ./backend/...`.

---

## 9. Build and CI

* The Docker build is the reference build. The host needs no Go, no Android Studio,
  and no Gradle.
* The build has two stages. `Dockerfile.base` makes the toolchain image and stores
  `go.sum`. `Dockerfile` then builds the artifacts. `local/build.sh` runs both stages
  and copies the artifacts to `output-binaries/`.
* **Quality gate.** `go vet ./backend/... && go test ./backend/...` runs after
  `go mod tidy` and before any artifact build. A failed test stops the build before
  the gomobile and Gradle work. `--build-arg SKIP_TESTS=1` skips the gate and prints
  a warning. Do not use that argument for work that you push.
* Desktop targets are `linux/amd64` and `windows/amd64`. The binary name is
  `omn-go-v${VERSION}-desktop-<os>-<arch>`. A release build uses `-trimpath` and
  `-ldflags="-s -w"`.
* Pass `-ldflags` on the `go build` command line. Keep `GOFLAGS` for single-word
  flags only. The `go` command splits `GOFLAGS` on spaces and drops an unknown flag
  without a message.
* CI reads the version from `backend/version.go`. It uses `grep` or `awk` anchored on
  `APP_VERSION =`. Do not change the shape of that line.
* The Android build tools stay pinned at 34.0.0. **The image follows the build. The
  build does not follow the image.** Never change the Gradle configuration to suit
  the Docker image.

---

## 10. Documentation style

`doc/TERMINOLOGY.md` binds the README, the bundled notes, the F-Droid metadata, the
release notes, and the commit messages.

**The rule also covers each code comment and each commit message that you write.**
`doc/TERMINOLOGY.md` does not name them, and that gap let text through unchecked.
It is closed here: apply the `ste-writing` skill to every word that a person
reads, and not only to a document. Code, identifiers and command syntax stay as
they are.

A code comment keeps the long form that section 3 asks for. Length is not the
question. The question is the shape of each sentence: one idea in one sentence,
at most 25 words, active voice, no semicolon, no contraction.

Check your own text before you give a patch. A sentence over the limit and a
banned word are both easy to find with a search, and both are easy to miss by
eye.

* Write each new document in ASD-STE100 Simplified Technical English. The
  `ste-writing` skill does this.
* Use the controlled vocabulary table. It has a "do not use" column.
* Do not use these words: seamless, robust, powerful, leverage, ensure, delve,
  streamline, simply, just, in order to.
* Put Android first in any list of platforms.
* Use American spelling.
* Do not use contractions.
* Do not use a semicolon in prose.
* Write at most 20 words in one instruction.

---

## 11. Working agreement for Claude sessions

1. Read this document and `doc/TERMINOLOGY.md` before you propose a change.
2. Search for an existing authority before you add a function. See section 1, rule 7.
3. Ask before you add a dependency, a build step, or a framework.
4. Report the `go vet` and `go test` result for each Go change.
5. Bump the version in each commit. See section 6.
6. Do not create a tag.
7. Write each document, code comment and commit message in Simplified Technical
   English. Check your own text before you give the patch. See section 10.
8. **Never push to the repository. Never commit.** The maintainer applies each change.
9. Give each change as a unified diff for `git apply`.
10. Give a short commit message together with the patch. Use the format in section 7.
