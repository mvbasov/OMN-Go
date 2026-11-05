# OMN-Go development rules

These are the standing rules for work on the OMN-Go repository.
Read this document before you change code, tests, documents, or build files.

Source: repository `https://github.com/mvbasov/OMN-Go`. The version of the tree
is in `backend/version.go`.

This document uses ASD-STE100 Simplified Technical English. See section 10.

---

## 1. Fixed constraints

Each constraint has a stated reason in the tree.
Do not remove a constraint without an instruction from the maintainer.

1. **No AndroidX. No AppCompat.** The Android layer uses `android.app.Activity`,
   `android.webkit.WebView`, and `android.app.AlertDialog` only. The one Gradle
   dependency is `fileTree(dir: 'libs', include: ['*.jar','*.aar'])`.
2. **Do not use `html/template`.** It calls `reflect.Value.MethodByName`. That
   call stops linker dead-code elimination for the whole program. The go-git
   method surface then makes the binary large. Use `render.Fill()` with
   `%%PLACEHOLDER%%` tokens. Use the `render.EscapeHTML` and `render.EscapeJS`
   pair in `backend/internal/render/templates.go`. Put the view and the render
   function of a page in the file of that page.
3. **Keep Android storage isolated.** Files stay in
   `/storage/emulated/0/Android/media/<applicationId>/`. This directory needs no
   runtime permission. The Go package cannot read the flavor applicationId.
   `ServerService.storageDir(ctx)` passes the path into `StartServer`. The
   `runtime.GOOS == "android"` branch in `initStorage` is a fallback only.
4. **Do not use bare 64-bit atomics.** Use the `atomic.Int64` and
   `atomic.Uint64` types. Never write `atomic.AddInt64(&field, ...)`. A bare
   64-bit atomic panics on `armeabi-v7a` and on `x86`. F-Droid publishes those
   builds. The test `TestNoBare64BitAtomics` in
   `backend/internal/repocheck/source_rules_test.go` scans the source and
   enforces this rule.
5. **The WebView floor is Chromium 85 and `minSdk 23`.** `html/js/OMN-Go/omn-go-compat.js`
   holds the only ES5 code in the project. Two rules keep it working, and
   `TestCompatScriptIsFirstAndES5` enforces both. **Keep the file in ES5**, and
   **keep it the first script in `templates/index.html`**, with no `defer` and no
   `async`. A `<script src>` element is its own parse unit, thus a SyntaxError in a
   modern script cannot stop it. Do not move this code into `omn-go-core.js`: that
   file is modern, an old WebView drops all of it, and the notice would go with it.
   It is a file and not an inline block, because an inline block goes into the
   compiled page of each note. Other scripts can use `async`, arrow functions, and
   template literals.
6. **Keep F-Droid compatibility.** `metadata/net.basov.omngo.fdroid.yml` is the
   F-Droid recipe. It builds from the committed Gradle configuration. The Docker
   build makes `assembleStandardRelease` only. Do not build the `fdroid` flavor for
   distribution. F-Droid signs the package on its own server. A local build looks
   official, but it is not official.
7. **Give each decision one authority.** Each decision has one implementation.
   `noteheader.Parse` is the only header-block parser.
   `render.Renderer.RenderAndCache` is the only writer of `html/<name>.html`.
   `storage.ResolvePageName` is the only name resolver.
   `hasRole` is the only role check. `systemPages` is the only page-access table.
   `storage.Layout` is the only code that joins a name to `StorageDir`.
   `storage.RelInside` is the only test that a path stays inside a directory.
   `renderPage` is the only shell of a page that the server makes.
   `config.ResolveContentType` is the only MIME resolver.
   `config.UserFileTrees` is the only table of the trees of uploaded files.
   `config.HasKnownAssetExtension` is the only note-or-file test.
   Do not add a second implementation. Extend the first one.
8. **An upgrade never overwrites a user-owned asset.** A version change replaces the
   files in `storage.VersionDependentAssets`
   (`backend/internal/storage/assets.go`). It first copies the old files to
   `asset_backups/<previous>/`. The app creates `md/Welcome.md`,
   `html/json/bookmarker-tags.json`, `omn-go-custom.css`, and `omn-go-custom.js` when
   they are absent. After that it leaves them alone.
9. **The `local-` name rule.** A path segment that starts with `local-` stays on
   the device. Git excludes it. A force pull keeps it. The rule has two
   implementations on purpose. `storage.IsLocalOnlyPath` covers the index.
   `gitsync.GitignoreLocalOnlyPattern` covers new files. A test compares the
   two. The pattern must stay last in `gitsync.GitignorePatterns`, because
   go-git reads patterns from the end.
10. **LAN sharing is off by default.** When `share_lan` is false, the listener binds
    `127.0.0.1`. The socket enforces the limit, not the authentication code. A local
    connection from `127.0.0.1`, `::1`, or `localhost` always counts as admin.
11. **Intent URIs and Termux intents are off by default.** Each one needs
    `enable_intent_uri` or `enable_termux_intent`, a runtime permission, and a
    confirmation for each tap.
12. **Each time is UTC.** Go on Android finds no zone file, thus the zone of the
    computer makes the desktop and Android write different text. Use
    `noteheader.Stamp` for a time in a note, and `.UTC().Format` for each other
    time. In a script, use the UTC methods of `Date`.
    `TestEachFormattedTimeIsUTC` reads the Go source. See
    `doc/decisions/0023-write-each-time-in-utc.md`.

---

## 2. Repository map

| Path | Contents |
| --- | --- |
| `main_desktop.go` | The only file in `package main`. It holds the only build tag: `//go:build !android`. |
| `backend/` | `package backend`, the facade for gomobile and for `main_desktop.go`. `backend.go` holds its six functions, and `version.go` holds `APP_VERSION`. |
| `backend/internal/app/` | `package app`, the application. It holds the `App` type, the server, the routes, the handlers and the pages. Each `*_app.go` file gives the values of the App to one package of `backend/internal/`. |
| `backend/internal/repocheck/` | `package repocheck`, which holds only tests. Each test reads the files of the repository and checks a rule that spans more than one package or more than one language. |
| `backend/internal/` | The other packages. `textmatch` holds the search matcher, and `noteheader` holds the header block. `logx` holds the log tags and the log hub. `config` holds the settings and their store. `storage` holds the storage layout, the application files and the plain files beside the notes. `render` holds the page compile, the page shell, the Tags page and the JSON answer. `db` holds the SQLite databases of the notes and their backups. `gitsync` holds the git sync and the host keys. `search` holds the page search and the global search index. `files` holds the Files page. `exchange` holds the export and the import of a note. `status` holds the Status page and /api/status. `testkit` holds the stand-in of the App for the tests of the feature packages. Only test files import it. |
| `backend/frontend/embed.go` | `package frontend`. It embeds `html/` and `md/` as `frontend.Static`, and `templates/` as `frontend.Templates`. |
| `backend/frontend/templates/` | Server-side page fragments. Embedded as `frontend.Templates`. Never extracted to disk. |
| `backend/frontend/html/` | `js/`, `css/`, `css/fonts/`, `json/`, `favicon.ico`. Embedded as `frontend.Static`. Extracted to the storage directory on demand. The user can edit these files with `?edit=true`. |
| `backend/frontend/md/` | The bundled system notes. Examples: `Welcome.md`, `UserManual.md`, `Database.md`, `ScriptRules.md`. Also a `Test/OMN-Go/` demonstration tree. |
| `android/` | Hand-written Gradle files and plain Java. The three components are `MainActivity.java`, `ServerService.java` and `ExportProvider.java`. `MainActivity` holds the life cycle and stays below 800 lines. Each other task is a plain class beside it: `WebViewSetup`, `Fullscreen`, `ShareIn`, `ShareOut`, `IntentBridge` and `Shortcuts`. `OmnConfig` and `OmnText` import no Android package, and `android/test/` holds a test of each. See `doc/decisions/0022-divide-main-activity-into-plain-classes.md`. The build generates `android/app/libs/omngo.aar` with `gomobile bind`. |
| `local/` | Maintainer scripts. The Docker context excludes this directory. The build never ships it. |
| `fastlane/metadata/android/en-US/` | Store metadata and `changelogs/<versionCode>.txt`. Each changelog line starts with `•`. |
| `metadata/` | `net.basov.omngo.fdroid.yml`, the F-Droid build recipe. |
| `backend/frontend/test/` | The JavaScript tests, the DOM stub, the page stub, the small document `mini-dom.js` and the coverage program. Embedded by no `go:embed`, thus no device receives them. |
| `android/test/` | The Java unit test. It sits OUTSIDE the Gradle project on purpose. See `doc/TESTING.md`. |
| `doc/` | Maintainer documents. `API.md` holds the endpoint reference. `TERMINOLOGY.md` holds the controlled vocabulary. `TESTING.md` holds the map of the test set. `decisions/` holds the decision records. `initial_prompt.md` holds the historical origin prompt. The Docker context excludes `doc/`, except `decisions/`. |
| `CLAUDE.md` | This document. The Docker context excludes it. |

The repository does not hold `go.sum`, `output-binaries/`, `data/`, `.env`, or keystores.

The repository holds the offline assets: KaTeX, highlight.js and the web fonts under
`backend/frontend/html/`. Run `local/initial/offline_asset_downloader.sh` only to
update these files.

---

## 3. Go rules

* The module is `net.basov.omngo`. The language version is `go 1.25`. go-git v5.19.2
  needs Go 1.25 or later, thus the line cannot go lower.
* The toolchain image is `golang:1.26.0-bookworm`. The F-Droid recipe builds
  with the srclib `go@go1.26.0`, thus the two builds use the same compiler.
  Change the image, the recipe and
  `backend/internal/repocheck/testdata/binary_size_baseline.json` together.
* Each build fetches the newest `golang.org/x/mobile` with `go get -tool`. That
  version needs Go 1.26.0, thus the real build uses Go 1.26 and not Go 1.25.
* The module has three direct dependencies. Each one has its own `require` line:
  `github.com/yuin/goldmark`, `github.com/go-git/go-git/v5`, and
  `modernc.org/sqlite`. Keep this set small. A new dependency needs a reason and
  maintainer approval.
* **The build generates `go.sum` inside the container.** `Dockerfile.base` writes it
  to `/root/lockfiles`. The `test` and `project_builder` stages restore it. The host has no Go
  toolchain. Remember this before you change `go.mod`.
* The driver is `modernc.org/sqlite`, because it is pure Go and works with
  `CGO_ENABLED=0`.
* **Use one package for each component under `backend/internal/`.** A package
  imports only packages of a lower layer. `importLayers` in
  `backend/internal/repocheck/import_layers_test.go` is the table, and
  `TestImportLayers` holds it. Give each new package a row. `backend` is the
  gomobile and desktop facade. It holds no logic, and it exports only functions
  of simple types. The application is `package app` in `backend/internal/app/`.
  The App side of a package is one file there, for example `config_app.go`,
  `storage_app.go`, `render_app.go`, `db_app.go`, `gitsync_app.go`,
  `search_app.go`, `files_app.go`, `exchange_app.go` and `status_app.go`. Its
  methods give the values of the App to the package.
* **Keep the groups of `package app` apart.** Each production file there belongs
  to one group of `fileGroups` in `backend/internal/app/group_links_test.go`. A
  group uses only the groups of a lower layer. `TestGroupsUseOnlyLowerLayers`
  holds the rule, with no exception. When a group must call a higher group, set
  a hook in `connectGroups`.
* **Keep the exported surface small.** Export only what the Android layer or
  the desktop entry point calls. The Android layer calls `StartServer`,
  `AssetsRefreshed`, `SetAndroidPackage` and `SetLANAddresses`.
  `main_desktop.go` calls `StartServer`, `WaitUntilReady` and `ServerPort`.
  These six functions are the facade in `backend/backend.go`. Each one calls
  the function of the same name in `package app`. Write everything else as a
  lowercase method on `*App`.
* `package app` exports the type `App`, the six functions and `SetVersion`.
  Only `backend/backend.go` calls them. `SetVersion` gives `APP_VERSION` to
  the app, because `package app` cannot import `backend`. Do not add an
  exported name.
* **Names.** Use `handleXxx` for an API endpoint. Use `serveXxx` for a page or an
  asset. Use `renderXxxPage` with an `xxxView` struct. Use `normalizeXxx` for value
  repair. Write a predicate as a question: `storage.IsLocalOnlyPath`,
  `storage.FileExists`, `hasRole`.
  Compile each regular expression once into a package-level `xxxRe` variable.
* **Branch on `runtime.GOOS`, not on a build tag.** The tree holds one build tag.
* **Errors.** Wrap an error with `fmt.Errorf("...: %w", err)`. Send the HTTP failure
  of a plain-text endpoint with `http.Error(w, msg, code)`.
* **JSON answers.** Send each JSON answer with `a.writeJSON(w, code, v)`. Send a JSON
  failure with `a.writeJSONError(w, code, msg)`. Do not write a second JSON encoder.
  `TestOnlyWriteJSONEncodesAnAnswer` enforces this. `doc/API.md` section 1.4 fixes
  the response shapes. A response is plain-text status words, or JSON with
  `"status":"success"` or `"status":"error"`, or `text/event-stream` for `/api/logs`.
* **Logging.** Do not call `log.Printf`. Write a step with
  `a.log(tag).Debugf(format, ...)`, an outcome with `a.log(tag).Infof(format, ...)`
  and a fault with `a.log(tag).Errf(format, ...)`. Give a function that has no
  `*App` a `logx.Logger` value. `TestNoDirectLogPrintf` enforces
  this.
  * `tag` is a typed constant from `backend/internal/logx/levels.go`, for example
    `logx.Sync` or `logx.Assets`. That file is the only authority for the tag set. Add a new tag
    to the constant block and to `logx.AllTags` together. The helper writes the
    brackets and the parentheses, thus a format string never carries them.
  * The emitted text is `[tag] (level) message`. Do not write `Error:`,
    `Warning:` or `FATAL:` in the message. The level word says it.
  * The Config page has two switches, `log_debug` and `log_info`, and one
    checkbox for each tag. A debug or info line prints when its level is on and
    its tag is ticked. **An error line ignores both.** A person who turns a level
    off asks for less noise, and never for fewer faults.
  * There are three levels and no more. The project has no leveled logger
    library and no structured logger.
  * `backend/internal/logx/hub.go` sends each line to stdout and to the SSE
    subscribers on `/api/logs`. **The SSE stream always carries every line.** The switches
    control stdout, and they control what `omn-go-api.js` mirrors into the
    browser console. The sync progress overlay reads `[sync] (debug)` lines off
    the raw stream, and it must work when debug is off.
  * **`/api/logs` is admin only**, the same as `/api/logs/history`, and the
    local bypass applies. A remote caller with no admin cookie reads no log
    line, live or held. See `doc/decisions/0013-send-each-log-line-to-three-places-and-to-the-admin-only.md`.
  * `applySyncLogLine` in `omn-go-api.js` removes the level word before it
    matches a sync stage. Keep the two in agreement, or the progress overlay
    loses a stage. That file exports it as `window.applySyncLogLine`, because
    `omn-go-sync.js` is the only caller and it is a separate file.
    `TestEverySyncLineReachesTheOverlay` runs a whole sync and sends each
    line that it wrote through the real JavaScript.
  * `logLinePrints` in `omn-go-api.js` and `logLineEnabled` in `log_app.go`
    are two implementations of one decision. The page needs the answer
    without the server, thus rule 7 of section 1 allows the pair with a test.
    `TestLogFilterPortAgreesWithTheRealJavaScript` compares them.
  * A log line must never take the config lock. `loadConfig` holds the write
    lock and writes a line, and a Go RWMutex is not reentrant. `applyLogFilter`
    keeps an atomic copy of the three switches for that reason.
  * Two call sites keep `log.Printf`, because no `*App` can reach them:
    `render.LoadTemplate` in `internal/render/templates.go` runs at package
    init, and `internal/search/sections.go` logs from a `sync.Once` and from a
    method on `searchDocument`. Both files write `[tag] (error) ` into the text
    by hand.
* **Configuration.** Read the configuration with `a.config.Get()`. It returns a
  copy under `RLock`. Change the configuration with
  `a.config.Update(func(c *config.Config){...})`. It reads and writes under
  `Lock`. Give a function that has no `*App` the `*config.Store`. `config.Store`
  in `backend/internal/config/store.go` is the only holder of `config.Config`.
  The `config.NormalizeXxx` functions repair an unknown enum value. The loader, the POST
  handler, and the renderer then always agree. A request that omits a field leaves
  that field alone. See `configFieldSent` in `config_handlers.go`.
* **Routes.** Register every route in `registerRoutes` in
  `backend/internal/app/server.go`. `StartServer` calls it with `a.Router`, a
  plain `http.ServeMux`. The parameter is the small `routeTable` interface, thus
  `TestBaseline_RouteSet` can pass a recorder and read the real table. Do not
  register a route anywhere else. Use the form `a.route(mux, "/api/x", admin,
  "JSON", post(a.handleX))`. The third argument is the role: `open`, `admin` or
  `adminPage`. The fourth argument says what the route answers. Give a route
  that reads the method GET. Give a route that writes the method POST. `route`
  also registers the bare path, and that path answers 405 for another method.
  Do not check `r.Method` in a handler. A protected route needs the admin role.
  The route table of `doc/API.md` section 3 comes from these calls. After a
  change, run `OMN_WRITE_API_TABLE=1 go test -run TestAPIRouteTable
  ./backend/internal/app/`. Add a system page as a row of
  `systemPages` in `backend/internal/app/page_access.go`. Do not check the role
  in a page handler. A row with a `pageMenu` also gives a link in the Config
  page menu. Do not write that link in `config_page.html`.
* **Comments say what the code does now, and why, one time.** Many files start
  with a `// ---` banner. The banner gives the design decision and the rejected
  alternative. Write the same kind of justification for new code that is not
  obvious. Keep each fact that the code cannot show. Examples are a constraint
  of a platform, a lock order, a rule that a "simplification" would break, and
  the test that holds a rule. Do not repeat what the next line of code says.
  Do not repeat a reason that another comment or a decision record gives.
  Point to it.
  `TestCommentShare` keeps the share of comments in the Go production code
  at 20 percent or less. A new long comment needs a shorter one elsewhere.
* **A comment tells no history.** Do not write a version number, "until", "used
  to" or the story of a past fault. The git log holds the history. When a past
  fault is the reason for a rule, write one sentence of the reason. Then put the
  full account in a decision record under `doc/decisions/`, and name its path in
  the comment. `doc/decisions/README.md` gives the format.
  `TestNoVersionNumberInComments` counts the version numbers.

### Add a feature

A feature is a package with a page, its API routes, a script and a style. The
Status page is the model: `backend/internal/status`, `status_app.go`,
`status_page.html`, `omn-go-status.js` and `omn-go-status.css`. Do the steps
in this order. The tests after the list hold steps 1, 8, 9 and 11, and the
harness of step 12.

1. Make the package `backend/internal/<name>/`. Give it a row in
   `importLayers` in `backend/internal/repocheck/import_layers_test.go`. A
   feature is in layer 4. A feature that reads another feature is in layer 5.
2. Write a `Service` struct. It holds the values of the App that one request
   needs. These are the `storage.Layout`, a copy of the settings or of one
   setting, and the `Log` and `RenderPage` functions. The App makes one
   Service for each request.
3. Put the state that lives longer than one request, for example a lock or an
   index, in a type of the package. The App holds a field of that type, and
   the Service holds a pointer to it.
4. Write the page in `backend/frontend/templates/<name>_page.html`. Load it one
   time with `render.LoadTemplate`. Escape each value with `render.EscapeHTML`,
   and put it in with `render.Fill`.
5. Write `ServePage` on the Service. It calls `RenderPage` with
   `render.PageHeader(title, "System")` and the filled page.
6. Write each API handler on the Service. Answer a JSON value with
   `render.WriteJSON`.
7. Write `backend/frontend/html/js/OMN-Go/omn-go-<name>.js` and
   `backend/frontend/html/css/OMN-Go/omn-go-<name>.css`. Load both with a
   `<script src>` and a `<link>` at the end of the page. Put no style and no
   script in the template.
8. Add the two files to `storage.VersionDependentAssets` and to
   `gitsync.GitignorePatterns`. Add the two lines to the text of
   `TestEnsureGitignoreFreshInstall`.
9. Write `backend/internal/app/<name>_app.go`. It holds one method that makes
   the Service, and one method for each route. Give the file a group in
   `fileGroups`, and give the group a layer in `groupLayers`. Both are in
   `backend/internal/app/group_links_test.go`.
10. Register the routes. The page is one row of `systemPages` in
    `page_access.go`. Give the row a `pageMenu` when the Config page menu must
    link to the page. Each API route is one `a.route` call in
    `registerRoutes` in `server.go`. Give a route that writes, or that shows
    private data, the role `admin`.
11. Add each new route to `TestBaseline_RouteSet` in `baseline_test.go`. Write
    the route table of `doc/API.md` again with `TestAPIRouteTable`.
12. Write the tests of the package. The `testApp` in `harness_test.go` embeds
    `testkit.App` and makes the Service. Test the routes and the role check
    in `backend/internal/app`.
13. Document each new endpoint in `doc/API.md`. Name the package in the
    repository map of section 2.

`TestImportLayers`, `TestEachFileHasAGroup`, `TestGroupsUseOnlyLowerLayers`,
`TestEachAppScriptAndStyleIsVersionDependent`,
`TestVersionDependentAssetsAreGitignored`, `TestEnsureGitignoreFreshInstall`,
`TestBaseline_RouteSet`, `TestAPIRouteTable` and `TestEachFeatureHasItsParts`
fail when you skip one of these steps.

### Add a setting

1. Add the field to `config.Config`, with its JSON key.
2. Add one row to `configFields` in `backend/internal/config/fields.go`. Give
   an enumeration or a set of checkboxes its `Options` and its `Mark`.
3. Put the control in `config_page.html`, with the placeholder that
   `config.PageValues` gives. The name is the key in upper case. A checkbox
   uses `KEY_CHECKED`, and each option uses `KEY_OPTION`.
4. When the Status page must show the value, give the row a `Status` key.
   The key is the name of the value in the config section of `/api/status`.

`TestEveryConfigFieldIsInTheTable` and
`TestEachTableValueHasAPlaceOnTheConfigPage` fail when you skip step 2 or 3.

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
* Write CSS by hand. `css/OMN-Go/omn-go-core.css` declares the design tokens as `:root`
  custom properties. The theme is CSS only. It uses `data-theme` on `<html>` with a
  `prefers-color-scheme` fallback.
* **Module pattern.** Use an IIFE with an explicit `window.*` export.
* **A control names its work in `data-action`.** Write no inline `onclick` in a
  template. Call `OMN.action('name', fn)` in a script, and put
  `data-action="name"` on the control. `data-arg` gives one value. See
  `doc/decisions/0020-name-the-work-of-a-control-in-data-action.md`.
  `TestTemplatesHoldNoInlineHandler` fails for an inline handler in a template.
  An action of one page goes into the script of that page, for example
  `omn-go-config.js`.
* **A note page loads seven application scripts, and the order is a rule.**
  `templates/index.html` names `omn-go-compat.js`, `omn-go-console.js`,
  `omn-go-core.js`, `omn-go-highlight.js`, `omn-go-nav.js`, `omn-go-share.js` and
  `omn-go-api.js`. The three vendored libraries and `omn-go-custom.js` follow.
  It names no other file of the project. The lazy files load on demand. See
  the lazy loading rule below, and
  `doc/decisions/0021-divide-the-page-script-into-parts.md`.
* **A new name of an app script or style needs four lists.** Change the path in
  `storage.VersionDependentAssets` and in `gitsync.GitignorePatterns`. Add the old
  path to `storage.RetiredAssets` and to `storage.RenamedAssets`. `legacyAssetURL`
  then answers the old name, which a note of the user can hold.
* File roles:
  * `omn-go-console.js` holds the console of the page: the hooks of the console
    methods and of the error events. It loads before each other modern script.
  * `omn-go-core.js` holds what each other script uses: the KaTeX start code,
    `OMN.action`, the progress API, the controls of the page header, the load
    listener and the version footer.
  * `omn-go-highlight.js` holds the marks of a search and the fold table.
  * `omn-go-nav.js` holds link interception and the slow-navigation guard.
  * `omn-go-share.js` holds send and copy of a note, the one clipboard writer,
    the page link and the metadata panel.
  * Each of these five files works on a page from disk too.
  * `omn-go-api.js` holds everything that calls the backend. The file body sits
    inside `if (window.location.protocol !== 'file:')`. The `else` branch replaces
    the same globals and actions with stubs, so an exported page degrades quietly. It also
    holds `omnLoadModule`, `omnLazy` and `omnLazyActions`, the three functions of
    the lazy loading.
  * `omn-go-sync.js`, `omn-go-bookmark.js` and `omn-go-search.js` hold the parts
    that a tap starts. `omnLazyActions` writes a stub for each action of such a
    file, and `omnLazy` writes a stub for each exported name. The first call to a
    stub fetches the file one time and then calls the real function. The body of
    each file sits inside the `file:` guard. Write each function there as a
    `const`. A `function` declaration of the block becomes a global name.
  * `omn-go-config.js` holds the whole Config page. Only
    `templates/config_page.html` names it. A note page never loads it.
  * `omn-go-editor.js` holds the standalone editor page. It uses `var` in an
    ES5 style. It reads `OMN_EDIT_NAME`, `OMN_EDIT_EXT`, and `OMN_EDIT_VIEW`.
  * `omn-go-compat.js` holds the too-old-WebView notice, and nothing else. It is
    the only ES5 file. See section 1, rule 5.
  * `Bookmarker.js` holds the bookmark page of the bundled note. See
    `frontend/md/BookmarksHowTo.md`. **A value of a bookmark is text from a
    foreign page.** Write it with `textContent`, and never with `innerHTML`.
    `TestBookmarkerWritesNoValueAsHTML` holds the rule.
  * `omn-go-custom.js` and `omn-go-custom.css` are user files. They are empty on
    purpose. `omn-go-custom.js` stays independent. It keeps its own plain
    `<script>` element, and it loads last.
* **A lazy file gives its work through `omnLazyActions`.** Call `OMN.action` in
  the lazy file. Add the action to the `omnLazyActions` call of that file in
  `omn-go-api.js`, and to the stub list of the `else` branch. Use
  `omnLazy` only for a name that Android, the User Manual or another script
  calls. A name or an action that is absent from its list does nothing until
  something else loads the file. `printDebug` sits above the `file:` guard,
  because a stub needs it.
* **A lazy file must read NO bare name of `omn-go-api.js`.** The body of that
  file sits inside an `if` block, thus a `const` of the block reaches no other
  file. A `function` of the block reaches one by accident, through Annex B of
  the standard. Both are traps. Put the value in the lazy file, or export it as
  a property of `window`. See
  `doc/decisions/0015-load-the-click-driven-scripts-on-demand.md`.
  `backend/frontend/test/lazy.test.js` runs each exported function and each
  action of each lazy file and fails on a free variable.
* **The fold table has two implementations on purpose.** `textmatch.FoldTable` in
  `backend/internal/textmatch/textmatch.go` folds before the server matches.
  `OMN_FOLD_TABLE` in `omn-go-highlight.js` folds again in the page. The server sends the term unfolded in
  `?hl=`, because the reader has to see the word as typed, thus the page cannot
  match on a lowercase alone. `TestFoldTableHasAFrontendCopy` compares the two
  tables and checks that the three call sites use them. Each row maps one
  character to one character. A row that changes the length moves every span
  after it, and the marks land on the wrong words. Without the table, a search
  for `елка` opens `Ёлка` with nothing marked.
* **Load order is a feature.** The custom files load last. A user rule then wins
  against an app rule at equal specificity. The editor page loads neither custom
  file. A broken custom file can never lock the user out of the editor that repairs
  it.
* **The two embed trees are separate on purpose.** `frontend.Static` holds
  `frontend/html` and `frontend/md`. The app extracts these files, and the user can
  edit them. `frontend.Templates` holds `frontend/templates`. Templates are render
  logic. Never make them extractable.
* **Escape by hand and by context.** Use `render.EscapeHTML(v)` for HTML text
  and for an attribute. Use `render.EscapeJS(v)` for a JS string literal. Use
  `render.EscapeHTML(render.EscapeJS(v))` for a JS literal inside an HTML
  attribute. Splice trusted pre-rendered HTML raw.
* The server injects the runtime variables **when it serves the page**. It replaces
  the marker `<meta id="omn-go-runtime-vars-marker">`. A change of a setting
  therefore needs no new compile. Do not "repair" this.
* **A new version compiles each page again.** The compiled page holds the page
  shell of the templates. At a start with a new version, `storage.DropCompiledPages` removes
  each compiled page. See
  `doc/decisions/0026-compile-each-page-again-after-a-new-version.md`.

---

## 5. Note and page rules

* Call the metadata a **header block**. Do not call it front matter. See
  `doc/TERMINOLOGY.md`.
* **A name is a note or a file, and the LAST extension decides.**
  `config.HasKnownAssetExtension` in `backend/internal/config/content_types.go`
  is the only authority for that question. It reads `Config.MimeTypes` and then
  `config.BuiltinMIME`. It must
  never call `mime.TypeByExtension`. The stdlib reads `/etc/mime.types`, thus
  the same name would mean one thing on a desktop and another on Android.
  * `.md` is the source of a note. `.html` is a compiled note, and an
    `.html` always has a `.md` source.
  * A known extension, for example `.js` or `.txt`, is a file under `html/`.
  * An unknown extension or no extension is a note. **A note name may hold a
    dot.** `Report.2026` is a note with the source `md/Report.2026.md`.
  * A note named `Draft.txt` has the source `md/Draft.txt.md` and compiles to
    `html/Draft.txt.html`. The file `html/Draft.txt` is a different thing.
    The two never collide, because each name carries each of its extensions.
  * Send the `.md` form to `/api/note`, `/api/save`, `/api/export/note` and
    `/api/search` for the note on screen. A bare name is ambiguous when it
    ends in a real file extension. `renderInternalEditor` does this, and
    `omnGoCurrentNoteName` in `omn-go-share.js` does it for the frontend.
* `noteheader.Parse` in `backend/internal/noteheader/noteheader.go` is the only parser.
  A header block exists only if the first line holds a colon and does not start with a space, `#`,
  or `<`. The header block ends at the first empty line, which the parser drops. It
  also ends at the first non-header line, which the parser keeps.
* `isHeaderFirstLine` and `firstLineAfterHeader` in `omn-go-editor.js` are a direct
  port of the Go code. **Keep the two versions the same.**
* Read or write a single key with `noteheader.SetKey` or `noteheader.SplitRegion`.
  These functions splice into the original string. The result
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
* **SQL API.** `window.omnGoOpenDatabase(name)` returns a handle at once. The
  handle has `exec`, `batch`, `transaction`, and `readTransaction`.
  `window.openDatabase(...)` is the legacy WebSQL entry point. The Go side is
  `POST /api/sql` in `backend/internal/db/sqlite.go`. Rules: admin only. All
  statements of one request run in one transaction. A database name must match
  `^[A-Za-z0-9_-]{1,64}$`. That pattern is the path-traversal guard. The body
  limit is 1 MiB. The statement limit is 500. Each database file lives at
  `<StorageDir>/db/<name>.sqlite`.
* **Database backups.** A backup holds the full database as JSONL. The user starts
  each backup by hand. File names are immutable:
  `html/db_backup/<db>/<UTCtimestamp>_<hostname>.jsonl`. The format version is 2.
  The app restores a backup automatically in one case only. That case is a bootstrap
  restore, when backups exist and no `.sqlite` file exists. The state of a
  database compares the mtime of the `.sqlite` file with the `created` time in
  the header of the newest backup. It never reads the mtime of the backup file.
  See `doc/decisions/0024-read-the-backup-state-from-the-created-time.md`.

---

## 6. Version and release

* `backend/version.go` holds the version as `const APP_VERSION = "YY.MM.S"`.
  YY is the year, and MM is the month, with two digits each. S is the sequence
  number of the change in that month. It starts at 1, and it has one, two or
  three digits, with no leading zero. Examples: `26.10.1`, `26.09.37`,
  `26.09.100`.
* **Bump the version in every commit.** The maintainer can ask for an exception.
* A bump changes **two files**:
  1. `backend/version.go`.
  2. `android/app/build.gradle`. Set `versionName`. Compute `versionCode` as
     `YY*100000 + MM*1000 + S`. Example: 26.09.100 gives 2609100.
* The F-Droid flavor multiplies `versionCode` by 10 and adds an offset for each ABI.
  See `android/app/fdroid-abi-versioncode.gradle`.
* **The maintainer creates every tag.** Do not create a tag.
* A tag without the `f` suffix starts the GitHub CI release build. That build
  publishes the desktop binaries and the APK.
* A tag with the `f` suffix starts the F-Droid build. Example: `v26.08.51f`.
* The `v1.x` tags use the old scheme. The `YY.MM.S` scheme starts at `v26.07.36`.
* An F-Droid release also needs a changelog file at
  `fastlane/metadata/android/en-US/changelogs/<versionCode>.txt`. Write each entry as
  a `•` bullet line. Commit it as `release(f-droid): ...`.

---

## 7. Commits and branches

### Commit message format

A commit message has three parts: a subject line, one empty line, and a list of
the changes.

1. Subject line: `type(scope): Sentence. vYY.MM.S`. Write **at most 80
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

* Each Go package holds its own tests. The tests of the application live in
  `backend/internal/app/`. The tests in `backend/internal/repocheck/` read the
  whole repository, for example `ports_test.go` and `pipelines_test.go`. Put a
  new test that reads files outside its own package there. The one exception is
  `TestAPIRouteTable` in `backend/internal/app`, because only package `app` can
  run `registerRoutes`. Most production files have a test file of the same name
  beside them. Some test files hold one topic across many files, for example
  `baseline_test.go`. Each test uses the package of its directory, so the tests
  are white-box tests.
* **Go is the one gate, and it is not the only language.**
  `backend/internal/repocheck/js_test.go` runs the JavaScript tests of
  `backend/frontend/test/` with `node --test`. It also measures the lines of each
  script that the tests ran. **A new script, or new code in a script, needs a
  test that runs it.** The target is 60 percent of the code lines.
  Each script is at the target, and `jsLineCoverageFloor` is empty. For a
  control, write a test with `notePage()` of `dom-page.js`, which presses the
  control on the markup of the real templates.
  `backend/internal/repocheck/java_test.go` compiles and runs `android/test/`
  with `javac` and `java`. Each one skips when the tool is absent, and the build
  image holds both. `doc/TESTING.md` maps the whole set.
* **A test that reads source text proves what a file SAYS. A test that runs the
  code proves what the code DOES.** Prefer the second. A test that runs the code
  finds faults that a test of the source text cannot see.
* The tests use the standard library `testing` package, `net/http/httptest`, and
  `t.TempDir()`. The project uses no assertion library and no mock library.
* In `package app`, build the application under test with `newTestApp(t)` from
  `handlers_test.go`. A feature package has a `testApp` in `harness_test.go`.
  It embeds `testkit.App` of `backend/internal/testkit`, and it adds the fields
  and the Service of that package. `newTestApp(t)` there builds it. The tests
  of `storage` and `render` have a `testApp` of their own, because `testkit`
  imports both packages.
* Write a helper with a lowercase name. Take `t *testing.T` as the first parameter.
  Call `t.Helper()`.
* Add a file prefix to a helper name that can collide across files.
  `internal/db/backup_test.go` uses `dbbApp`, `dbbExec`, and `dbbBackup`.
* Write table-driven tests with anonymous structs. Use `t.Run` rarely.
* **Give each test a comment that says why it exists.** A failure must then read
  either as "you broke it" or as "you changed it on purpose, so update the golden
  value".
* `baseline_test.go` pins behavior with golden sets. When you add a route, update
  `TestBaseline_RouteSet`, with one comment that says why the route is there. The
  same rule covers a new injected runtime variable and a change to the page
  dispatch.
* Some tests scan the source and act as lint rules. Respect them.
* Run the tests with `go vet ./backend/... && go test ./backend/...`.

---

## 9. Build and CI

* The Docker build is the reference build. The host needs no Go, no Android Studio,
  and no Gradle.
* The build has two files. `Dockerfile.base` makes the toolchain image and stores
  `go.sum`. `Dockerfile` then runs the gate and builds the artifacts. `local/build.sh`
  runs both files and copies the artifacts to `output-binaries/`. `Dockerfile.ci`
  holds the same stages in one file for GitHub and GitLab. Keep the three in step.
* The stages:
  * `go_env`: Go, the JDK, Node and go.sum.
  * `base_env`: `go_env` plus the Android SDK, the NDK, Gradle and gomobile.
  * `test`: the quality gate.
  * `project_builder`: the artifacts.
  * `export`: the artifacts alone. Only `Dockerfile.ci` has this stage.
* **Quality gate.** The `test` stage runs `go vet ./backend/... && go test ./backend/...`
  after `go mod tidy`. The JDK and Node of `go_env` also run the Java test and the
  JavaScript tests. `project_builder` copies `/gate-passed` from the `test` stage, thus
  no artifact comes from a build with a failed gate. `--build-arg SKIP_TESTS=1` skips
  the gate and prints a warning. Do not use that argument for work that you push.
* **The three builds must agree.**
  `backend/internal/repocheck/pipelines_test.go` compares the Android API level,
  the Go version and the NDK. It reads the Docker files and
  `android/app/build.gradle`. It also reads the last version in
  `metadata/net.basov.omngo.fdroid.yml`. `-androidapi` must be the same as
  `minSdk`. When the recipe on the F-Droid server changes, copy it into
  `metadata/` first. The tests then show each difference.
* **Two builds of one commit give the same APK.** `gomobile bind` has `-trimpath`, and
  `build.gradle` removes the dependency list from the APK. `TestAndroidBuildIsTheSameOnEachHost`
  holds both rules. The F-Droid recipe has no `-trimpath` yet.
* **Two GitHub workflows.** `test.yml` builds the `test` stage alone on each push to
  `master` and on each pull request. It needs no secret. `android-gomobile-release.yml`
  builds the artifacts, and a tag push makes the release. Run the gate alone on a
  device with `docker buildx build --target test .` after `local/build.sh` made the
  base image one time.
* Desktop targets are `linux/amd64` and `windows/amd64`. The binary name is
  `omn-go-v${VERSION}-desktop-<os>-<arch>`. A release build uses `-trimpath` and
  `-ldflags="-s -w"`.
* Pass `-ldflags` on the `go build` command line. Keep `GOFLAGS` for single-word
  flags only. The `go` command splits `GOFLAGS` on spaces and drops an unknown flag
  without a message.
* CI reads the version from `backend/version.go`. It uses `grep` or `awk` anchored on
  `APP_VERSION =`. Do not change the shape of that line.
* **The GitLab pipeline runs for a version tag and for nothing else.** The
  `workflow:` rule at the top of `.gitlab-ci.yml` holds that condition, and each of
  the three jobs repeats it. A workflow rule stops the pipeline before GitLab makes
  it. A job rule alone leaves a pipeline that holds no job, and GitLab reports that
  as a fault. `.github/workflows/sync-gitlab.yml` mirrors each branch push and each
  tag, thus a branch push reaches GitLab and must make no pipeline.
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

**`TestNoCommentStyleFault` in
`backend/internal/repocheck/comment_style_test.go` counts them.** It reads each
whole line comment of every Go, JavaScript and Java file, and it demands zero. A
comment that breaks a rule fails the gate.

**`TestNoVersionNumberInComments` in the same file** demands zero comment
lines with a version number. It reads the same files, and in a JavaScript and a
Java file also each line of a block comment. See `doc/decisions/README.md`.

**`TestEveryGoFileIsGofmtClean` in the same file** checks the formatting that
`go vet` does not read.

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
