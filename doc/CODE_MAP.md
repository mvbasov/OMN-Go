# OMN-Go code map

<!-- Summary: A map from each feature of OMN-Go to the files that make it. It starts with the three layers, and then shows each package, file and class. Use it to find the place for a change. -->

This document shows where each feature of OMN-Go is in the code. Use it to find the files that a change must touch.

The document goes from the top to the details. Sections 1 to 4 give the whole picture. Section 5 is the map of the features. Sections 6 to 9 are a reference for each layer. Section 10 holds the procedures for common changes.

State of the tree: version 26.11.29. When the code changes, a file name in this document can become wrong. Section 11 shows how to find the place without this document.

## Contents

1. [How to use this map](#1-how-to-use-this-map)
2. [The three layers](#2-the-three-layers)
3. [The storage directory](#3-the-storage-directory)
4. [What occurs at the start, at a view and at a press](#4-what-occurs-at-the-start-at-a-view-and-at-a-press)
5. [The map of the features](#5-the-map-of-the-features)
6. [The backend package by package](#6-the-backend-package-by-package)
7. [The frontend file by file](#7-the-frontend-file-by-file)
8. [The Android layer class by class](#8-the-android-layer-class-by-class)
9. [How to read Go when you know Java and JavaScript](#9-how-to-read-go-when-you-know-java-and-javascript)
10. [Procedures for common changes](#10-procedures-for-common-changes)
11. [How to find a place without this map](#11-how-to-find-a-place-without-this-map)
12. [Tests, build and other documents](#12-tests-build-and-other-documents)

## 1. How to use this map

1. Find the feature in section 5. Each entry names the files of that feature in each layer.
2. Open the files. Most files start with a comment that tells what the file holds.
3. Read the procedure in section 10 that is nearest to your change.
4. Make the change. Then run the tests of section 12. A test that fails usually names the file that you did not change.

The last step is important. The project has many tests that read the source files. They know which files go together. When you add an endpoint and do not describe it in the document of the endpoints, a test fails and tells you.

### Short names in this document

A file name in this document is a link to the file. The text of the link is a short name. This list gives the directory that comes before each short name.

- `app/server.go` and each name of the form `<package>/<file>.go`: `backend/internal/`
- `omn-go-core.js`: `backend/frontend/html/js/OMN-Go/`
- `omn-go-core.css`: `backend/frontend/html/css/OMN-Go/`
- `templates/index.html`: `backend/frontend/`
- `md/UserManual.md`: `backend/frontend/`
- `MainActivity.java`: `android/app/src/main/java/net/basov/omngo/`

## 2. The three layers

OMN-Go has three layers. Each layer uses a different language.

```text
+--------------------------------------+
| Android layer (Java)                 |
| one Activity, one WebView, one       |
| Service. Directory: android/         |
+------------------+-------------------+
                   | starts, and shows
                   v
+--------------------------------------+
| Frontend (HTML, CSS, JavaScript)     |
| runs in the WebView or a browser.    |
| Directory: backend/frontend/         |
+------------------+-------------------+
                   | HTTP on localhost
                   v
+--------------------------------------+
| Backend (Go)                         |
| a web server. It reads and writes    |
| the storage directory.               |
| Directory: backend/internal/         |
+--------------------------------------+
```

| Layer | Its work | Where |
| --- | --- | --- |
| The Android layer | It starts the backend as a service. It shows the pages in a WebView. It takes a share from another application. | [`android/`](../android/) |
| The frontend | It is each page that a person sees: the markup, the styles and the scripts of the controls. | [`backend/frontend/`](../backend/frontend/) |
| The backend | It is a web server on the device. It compiles the notes, saves them, searches them and synchronizes them. | [`backend/internal/`](../backend/internal/) |

On the desktop there is no Android layer. [`main_desktop.go`](../main_desktop.go) starts the backend and opens the default browser.

### How the layers speak to each other

There are five paths. Before you add a feature, find which path it uses.

1. **Frontend to backend.** A script sends an HTTP request to an endpoint with `fetch`. Example: `fetch('/api/save')` in [`omn-go-editor.js`](../backend/frontend/html/js/OMN-Go/omn-go-editor.js).
2. **Backend to frontend.** The backend sends the HTML of the page, with some values in a `<script>`. Example: `APP_VERSION` and `OMN_THEME` from `InjectRuntimeVars` in [`render/pages.go`](../backend/internal/render/pages.go).
3. **Frontend to Android.** The page opens an address that starts with `omngo://` or `intent:`. Example: `omngo://share` in [`omn-go-share.js`](../backend/frontend/html/js/OMN-Go/omn-go-share.js). `shouldOverrideUrlLoading` in [`WebViewSetup.java`](../android/app/src/main/java/net/basov/omngo/WebViewSetup.java) reads it.
4. **Android to backend.** Java calls a Go function through the `Backend` class, or it sends an HTTP request. Examples: `Backend.startServer` in [`ServerService.java`](../android/app/src/main/java/net/basov/omngo/ServerService.java), and `POST /api/quick` in [`ShareIn.java`](../android/app/src/main/java/net/basov/omngo/ShareIn.java).
5. **Android to frontend.** Java opens a page with a parameter in the address, or it calls a script function. Examples: `?quicknote=1` in [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java), and `omnGoInsertCapture` in [`IntentBridge.java`](../android/app/src/main/java/net/basov/omngo/IntentBridge.java).

Android also reads four settings directly from the file `config.json`. See [`OmnConfig.java`](../android/app/src/main/java/net/basov/omngo/OmnConfig.java).

The Go functions that Java can call are all in one file: [`backend/backend.go`](../backend/backend.go). The build tool gomobile makes the Java class `Backend` from that file.

## 3. The storage directory

The backend keeps all data as files in one directory. On Android it is `/storage/emulated/0/Android/media/net.basov.omngo/`. On the desktop it is `./data/`.

| Path | Contents | Who writes it |
| --- | --- | --- |
| `md/` | The Markdown source of each note. This is the data of the person. | The editor, the note box, the bookmark form, git sync |
| `md/incoming/` | The notes that the device received from another person. | Note exchange |
| `html/` | The HTML cache: one compiled page for each note. The backend makes it again when a note changes. | The backend |
| `html/js/`, `html/css/` | The scripts and styles of the application. The backend writes them from the binary at each new version. | The backend |
| `html/images/` | The images of the notes. | The editor, a share from Android |
| `html/user_json/`, `html/user_contacts/`, `html/user_calendars/` | The data files of the person. | The editor, a share from Android |
| `db/` | The SQLite databases of the note scripts. Git sync does not carry them. | A note script |
| `config.json` | The settings. | The Config page |
| `.git/` | The repository of git sync. | Git sync |
| `asset_backups/` | The old application files of the version before. | The backend |

Only [`storage/layout.go`](../backend/internal/storage/layout.go) builds these paths. In Go, `a.layout().MD("Welcome.md")` gives the path of a note. Do not join a path by hand. A test refuses it.

The files that the binary carries are in [`backend/frontend/`](../backend/frontend/). [`backend/frontend/embed.go`](../backend/frontend/embed.go) puts them into the binary. At the start, the backend copies them into the storage directory. See `RefreshEmbeddedAssets` in [`storage/assets.go`](../backend/internal/storage/assets.go).

## 4. What occurs at the start, at a view and at a press

These four sequences are the skeleton of the application. Most changes touch one step of one sequence.

### 4.1 The start

1. Android starts `MainActivity`. See `onCreate` in [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java).
2. `onCreate` starts `ServerService` and builds the WebView with `WebViewSetup.build`.
3. `ServerService` calls `Backend.setAndroidPackage` and `Backend.startServer`. See `onStartCommand` in [`ServerService.java`](../android/app/src/main/java/net/basov/omngo/ServerService.java).
4. The call arrives in Go at `StartServer` in [`backend/backend.go`](../backend/backend.go), and then at `startServer` in [`app/server.go`](../backend/internal/app/server.go).
5. `initStorage` in [`app/storage.go`](../backend/internal/app/storage.go) makes the directories. It writes the application files, reads `config.json` and writes the starter notes.
6. `registerRoutes` in [`app/server.go`](../backend/internal/app/server.go) gives each address its function. Then the server listens.
7. `MainActivity` waits with `Backend.waitUntilReady`. It shows the text of `Backend.startStage` under the spinner.
8. `loadStartPage` in [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java) reads the intent and opens the first page in the WebView.

On the desktop, [`main_desktop.go`](../main_desktop.go) does steps 4 to 6 and then opens the browser.

### 4.2 A person opens a note

1. The WebView asks for `/Welcome.html`.
2. `serveFrontend` in [`app/handlers.go`](../backend/internal/app/handlers.go) takes each address that is not an API endpoint. It calls `serveHTMLPage`.
3. `serveHTMLPage` finds the two files of the note with `resolvePageName`: `md/Welcome.md` and `html/Welcome.html`.
4. When the HTML cache is absent or older than the Markdown source, `recompileMarkdownPage` compiles the note again.
5. The compile is in package `render`. `RenderMarkdown` in [`render/markdown.go`](../backend/internal/render/markdown.go) makes the HTML of the body. `CompilePage` reads the header block and puts the body into the page shell [`templates/index.html`](../backend/frontend/templates/index.html). `RenderAndCache` in [`render/cache.go`](../backend/internal/render/cache.go) writes the result to `html/`.
6. `InjectRuntimeVars` in [`render/pages.go`](../backend/internal/render/pages.go) adds the values that can change between two views, for example the theme. It also adds the panels of [`templates/modals.html`](../backend/frontend/templates/modals.html).
7. The browser loads the scripts that [`templates/index.html`](../backend/frontend/templates/index.html) names. The load function in [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js) then starts highlight.js and KaTeX.

### 4.3 A person presses a control

Each control of the application has an attribute `data-action`. It has no `onclick`.

1. The markup names the action. Example from [`templates/index.html`](../backend/frontend/templates/index.html): `<button data-action="refresh-page">`.
2. One listener in [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js) takes each press. It finds the function of the action in a table.
3. A script puts its function into that table with `OMN.action('refresh-page', function ...)`.
4. When the action needs the backend, the function calls an endpoint with `fetch`.
5. `registerRoutes` in [`app/server.go`](../backend/internal/app/server.go) has one line for each endpoint. The line names the Go function.

Thus the chain is: `data-action` in a template, `OMN.action` in a script, `fetch` of an endpoint, `a.route` in [`app/server.go`](../backend/internal/app/server.go), the Go function. Section 11 follows one chain as an example.

### 4.4 A person saves a note

1. The editor page is [`templates/editor.html`](../backend/frontend/templates/editor.html) with the script [`omn-go-editor.js`](../backend/frontend/html/js/OMN-Go/omn-go-editor.js).
2. The script sends the text to `POST /api/save`.
3. `handleSaveNote` in [`app/note_handlers.go`](../backend/internal/app/note_handlers.go) sets the `Modified:` line of the header block. `writeNoteSource` writes `md/<name>.md` through a temporary file (`storage.ReplaceFile`) and compiles the page.
4. It calls `renderAndCache`, which compiles the page and writes `html/<name>.html`.
5. `renderAndCache` also tells the search index that a page changed. See `connectGroups` in [`app/server.go`](../backend/internal/app/server.go).

## 5. The map of the features

Each entry has the same lines. "Page" is what a person sees. "Endpoint" is the address that the script calls. "Go" is the path of the request in the backend. A line is absent when the layer has no part.

### 5.1 Notes

#### View a note

- Page: `/<Name>.html`. Shell: [`templates/index.html`](../backend/frontend/templates/index.html). Panels: [`templates/modals.html`](../backend/frontend/templates/modals.html).
- Scripts: [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js) (the header controls and the load), [`omn-go-nav.js`](../backend/frontend/html/js/OMN-Go/omn-go-nav.js) (the links of the note), [`omn-go-share.js`](../backend/frontend/html/js/OMN-Go/omn-go-share.js) (the metadata panel).
- Style: [`omn-go-core.css`](../backend/frontend/html/css/OMN-Go/omn-go-core.css).
- Go: `serveFrontend`, `serveHTMLPage` in [`app/handlers.go`](../backend/internal/app/handlers.go). `renderAndCache`, `injectRuntimeVars` in [`app/render_app.go`](../backend/internal/app/render_app.go).
- Compile: [`render/markdown.go`](../backend/internal/render/markdown.go), [`render/pages.go`](../backend/internal/render/pages.go), [`render/cache.go`](../backend/internal/render/cache.go). Header block: [`noteheader/noteheader.go`](../backend/internal/noteheader/noteheader.go). Name to path: `ResolvePageName` in [`storage/paths.go`](../backend/internal/storage/paths.go).
- A page that does not exist: `serveNotFound` in [`app/serving.go`](../backend/internal/app/serving.go), [`templates/not_found.html`](../backend/frontend/templates/not_found.html).

#### Edit a note

- Page: `/<Name>.html?edit=true`. Template: [`templates/editor.html`](../backend/frontend/templates/editor.html).
- Script: [`omn-go-editor.js`](../backend/frontend/html/js/OMN-Go/omn-go-editor.js). Style: [`omn-go-editor.css`](../backend/frontend/html/css/OMN-Go/omn-go-editor.css).
- Endpoints: `GET /api/note`, `POST /api/save`.
- Go: `serveEditor`, `renderInternalEditor` in [`app/handlers.go`](../backend/internal/app/handlers.go). `handleGetNote`, `handleSaveNote` in [`app/note_handlers.go`](../backend/internal/app/note_handlers.go). Page: [`render/editor_page.go`](../backend/internal/render/editor_page.go).
- External editor: `handleEditExternal` in [`app/note_handlers.go`](../backend/internal/app/note_handlers.go), [`templates/external_edit.html`](../backend/frontend/templates/external_edit.html). On Android, the address `omngo://edit` in [`WebViewSetup.java`](../android/app/src/main/java/net/basov/omngo/WebViewSetup.java).
- Settings: `use_internal_editor`, `desktop_ext_cmd`.
- Manual for the person: [`md/Editor.md`](../backend/frontend/md/Editor.md).

#### Create a page

- Page: the *note_add* button of the page header.
- Script: action `new-page` in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js). It asks for the title and the file name in one form of `omnDialog` in [`omn-go-dialog.js`](../backend/frontend/html/js/OMN-Go/omn-go-dialog.js). The file name follows the title until the person types one.
- Endpoint: `POST /api/newpage`.
- Go: `handleNewPage`, `resolveNewPageTarget` in [`app/note_handlers.go`](../backend/internal/app/note_handlers.go).

#### Quick Notes

- Page: the note box, `#omn-quick-panel` in [`templates/modals.html`](../backend/frontend/templates/modals.html). The page `QuickNotes`. Each way to open or close a panel goes through `omnShowPanel`, `omnHidePanel` and `omnTogglePanel` in [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js). Only one panel shows at a time.
- Script: actions `quick-note-save` in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js) and `quick-note-copy` in [`omn-go-share.js`](../backend/frontend/html/js/OMN-Go/omn-go-share.js).
- Endpoint: `POST /api/quick`.
- Go: `handleQuickNote` in [`app/note_handlers.go`](../backend/internal/app/note_handlers.go). It adds the text to `md/QuickNotes.md` with `writeNoteSource`. A failed write answers 500, and the page says "The note was not saved."
- Android: the second launcher icon is `QuickNoteAlias` in [`android/app/src/main/AndroidManifest.xml`](../android/app/src/main/AndroidManifest.xml). `isQuickNoteAliasLaunch` in [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java) opens the page with `?quicknote=1`.

#### Bookmarks

- Page: the bookmark form, `#omn-bm-panel` in [`templates/modals.html`](../backend/frontend/templates/modals.html). The page `Bookmarks`.
- Scripts: [`omn-go-bookmark.js`](../backend/frontend/html/js/OMN-Go/omn-go-bookmark.js) (the form), action `bookmark-save` in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js). The page itself: [`Bookmarker.js`](../backend/frontend/html/js/OMN-Go/Bookmarker.js) and [`Bookmarker.css`](../backend/frontend/html/css/OMN-Go/Bookmarker.css).
- Endpoint: `POST /api/bookmark`.
- Go: `handleBookmark` in [`app/note_handlers.go`](../backend/internal/app/note_handlers.go). It adds one line to the script block of `md/Bookmarks.md` with `writeNoteSource`. A failed write answers 500, and the page says "The bookmark was not saved."
- Android: a shared link arrives in `loadStartPage` of [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java). It opens the page with `?share_text=`. The load function of [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js) then opens the form.
- Manual for the person: [`md/BookmarksHowTo.md`](../backend/frontend/md/BookmarksHowTo.md).

#### The Tags page

- Page: `/OMNGoTags.html`, and the tag marks in the page header.
- Go: `serveTagsPage` in [`app/render_app.go`](../backend/internal/app/render_app.go). `BuildTagIndex`, `GenerateTagsPage` in [`render/tags.go`](../backend/internal/render/tags.go).
- The backend writes the page as a note. It has no script and no template of its own.

### 5.2 Search

- Page: the search panel that the magnifier opens. The search page `/OMNGoSearch.html`, [`templates/search_page.html`](../backend/frontend/templates/search_page.html).
- Scripts: [`omn-go-search.js`](../backend/frontend/html/js/OMN-Go/omn-go-search.js) (the panel), [`omn-go-highlight.js`](../backend/frontend/html/js/OMN-Go/omn-go-highlight.js) (the marks in the page after a hit). Style: [`omn-go-search.css`](../backend/frontend/html/css/OMN-Go/omn-go-search.css).
- Endpoint: `GET /api/search`.
- Go: `handleSearch` in [`app/search_app.go`](../backend/internal/app/search_app.go), then `HandleSearch` in [`search/http.go`](../backend/internal/search/http.go).
- Page search and the query: [`search/search.go`](../backend/internal/search/search.go). Score of a document: `scoreDocument` in [`search/score.go`](../backend/internal/search/score.go). Match of one term: [`textmatch/textmatch.go`](../backend/internal/textmatch/textmatch.go).
- Global search index: [`search/index.go`](../backend/internal/search/index.go), `RebuildIndex` in [`search/index_build.go`](../backend/internal/search/index_build.go). Parts of a note: [`search/sections.go`](../backend/internal/search/sections.go), [`search/document.go`](../backend/internal/search/document.go).
- Settings: `search_enabled`, `search_kinds`, `search_bundled`, `search_scope`.

### 5.3 Settings

- Page: `/Config.html`. Templates: [`templates/config_page.html`](../backend/frontend/templates/config_page.html), [`templates/git_server_card.html`](../backend/frontend/templates/git_server_card.html), [`templates/config_menu_link.html`](../backend/frontend/templates/config_menu_link.html).
- Script: [`omn-go-config.js`](../backend/frontend/html/js/OMN-Go/omn-go-config.js). Style: [`omn-go-config.css`](../backend/frontend/html/css/OMN-Go/omn-go-config.css).
- Endpoints: `GET /api/config`, `POST /api/config`, `POST /api/restart`.
- Go: `serveConfigPage`, `handleConfigPost`, `applyConfigChange` in [`app/config_handlers.go`](../backend/internal/app/config_handlers.go). The page: [`app/config_page.go`](../backend/internal/app/config_page.go).
- The settings themselves: the type `Config` in [`config/config.go`](../backend/internal/config/config.go). `config.Load` reads `config.json`, and `config.Save` is its only writer. The table of the fields: `configFields` in [`config/fields.go`](../backend/internal/config/fields.go). The values of the page: [`config/page_values.go`](../backend/internal/config/page_values.go).
- Android: [`OmnConfig.java`](../android/app/src/main/java/net/basov/omngo/OmnConfig.java) reads `config.json` directly for four settings.

### 5.4 Git sync

- Page: the two cloud buttons of the page header. The dialogs Commit, Merge Conflict and Push Rejected come from `omnDialog`: `askCommitMessage`, `askConflict` and `askPushConflict` in [`omn-go-sync.js`](../backend/frontend/html/js/OMN-Go/omn-go-sync.js).
- Script: [`omn-go-sync.js`](../backend/frontend/html/js/OMN-Go/omn-go-sync.js). The progress indicator: `OMNProgress` in [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js).
- Endpoints: `POST /api/sync`, `GET /api/sync/preview`, `POST /api/sync/trust-host-key`.
- Go: [`app/gitsync_app.go`](../backend/internal/app/gitsync_app.go), then `HandleSync` in [`gitsync/handlers.go`](../backend/internal/gitsync/handlers.go), then `SyncRepo` in [`gitsync/sync.go`](../backend/internal/gitsync/sync.go).
- Pull: [`gitsync/pull.go`](../backend/internal/gitsync/pull.go). Push: [`gitsync/push.go`](../backend/internal/gitsync/push.go). Commit: [`gitsync/commit.go`](../backend/internal/gitsync/commit.go).
- The repository, the remotes, the ignore rules and the SSH key: [`gitsync/repo.go`](../backend/internal/gitsync/repo.go). `WriteExcludeFile` writes the ignore rules of the app to `.git/info/exclude`, for command line git. `Worktree` gives the same rules to go-git, which cannot read that file. The app never writes `.gitignore`. See `doc/decisions/0029-keep-the-ignore-patterns-of-the-app-in-git-info-exclude.md`. Host keys: [`gitsync/host_keys.go`](../backend/internal/gitsync/host_keys.go).
- Settings: `git_servers`, `active_git_index`. The card of one git server slot: [`templates/git_server_card.html`](../backend/frontend/templates/git_server_card.html).

### 5.5 Database

#### SQL for a note script

- Script: `omnGoOpenDatabase` in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js). A note script calls it.
- Endpoint: `POST /api/sql`.
- Go: `handleSQL` in [`app/db_app.go`](../backend/internal/app/db_app.go), then `HandleSQL`, `RunBatch`, `Open` in [`db/sqlite.go`](../backend/internal/db/sqlite.go).
- Files: `db/<name>.sqlite` in the storage directory.
- Manuals for the person: [`md/Database.md`](../backend/frontend/md/Database.md), [`md/SQLImport.md`](../backend/frontend/md/SQLImport.md).

#### Database backups

- Page: `/db_backups`. Template: [`templates/db_backups.html`](../backend/frontend/templates/db_backups.html). Script: [`omn-go-db.js`](../backend/frontend/html/js/OMN-Go/omn-go-db.js). Backup and Restore ask with `omnDialog`. Style: [`omn-go-db.css`](../backend/frontend/html/css/OMN-Go/omn-go-db.css).
- Endpoints: `POST /api/db/backup`, `GET /api/db/backups`, `POST /api/db/restore`.
- Go: [`app/db_app.go`](../backend/internal/app/db_app.go), then [`db/backup_http.go`](../backend/internal/db/backup_http.go). Create: [`db/backup_create.go`](../backend/internal/db/backup_create.go). Restore: [`db/backup_restore.go`](../backend/internal/db/backup_restore.go).

### 5.6 Files

#### Images and data files

- Page: paste or drop a file into the editor.
- Script: [`omn-go-editor.js`](../backend/frontend/html/js/OMN-Go/omn-go-editor.js).
- Endpoints: `POST /api/upload` (image), `POST /api/upload_json`, `POST /api/upload_contacts`, `POST /api/upload_calendars`.
- Go: `handleUpload`, `handleUploadUserFile` in [`app/upload_handlers.go`](../backend/internal/app/upload_handlers.go).
- The table of the data file kinds: `UserFileTrees` in [`config/user_files.go`](../backend/internal/config/user_files.go). The content types: [`config/content_types.go`](../backend/internal/config/content_types.go).
- The backend serves the files with `serveStorageSubdir` in [`app/serving.go`](../backend/internal/app/serving.go).
- Android: `handleSharedFile` in [`ShareIn.java`](../android/app/src/main/java/net/basov/omngo/ShareIn.java). The kind of a shared file: `userFileTree` in [`OmnText.java`](../android/app/src/main/java/net/basov/omngo/OmnText.java).
- Setting: `max_upload_size_mb`.

#### The file index

- Page: `/OMNGoFiles.html`. Template: [`templates/files_page.html`](../backend/frontend/templates/files_page.html). Style: [`omn-go-files.css`](../backend/frontend/html/css/OMN-Go/omn-go-files.css).
- Go: `serveFilesPage` in [`app/files_app.go`](../backend/internal/app/files_app.go). Then [`files/index.go`](../backend/internal/files/index.go), [`files/state.go`](../backend/internal/files/state.go), [`files/page.go`](../backend/internal/files/page.go).

### 5.7 Note exchange

- Send a note. Script: `omnGoSendNote` in [`omn-go-share.js`](../backend/frontend/html/js/OMN-Go/omn-go-share.js). Endpoint: `GET /api/export/note`.
- Send on Android: the address `omngo://share`, then `handleShareOut` in [`ShareOut.java`](../android/app/src/main/java/net/basov/omngo/ShareOut.java). [`ExportProvider.java`](../android/app/src/main/java/net/basov/omngo/ExportProvider.java) gives the file to the share sheet.
- Receive a note on Android: `importSharedNote`, `importSharedText` in [`ShareIn.java`](../android/app/src/main/java/net/basov/omngo/ShareIn.java). They call `POST /api/import/note`.
- Receive a note on the desktop: the receive box, `#omn-incoming` in [`templates/modals.html`](../backend/frontend/templates/modals.html), with its code in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js).
- Go: [`app/exchange_app.go`](../backend/internal/app/exchange_app.go), then [`exchange/http.go`](../backend/internal/exchange/http.go). `ExportNoteSource`, `ImportNote`, `AddIncomingFile` in [`exchange/exchange.go`](../backend/internal/exchange/exchange.go).
- The Incoming notes page: [`templates/incoming_index.md`](../backend/frontend/templates/incoming_index.md).

### 5.8 Status and log

#### The Status page

- Page: `/OMNGoStatus.html`. Template: [`templates/status_page.html`](../backend/frontend/templates/status_page.html).
- Script: [`omn-go-status.js`](../backend/frontend/html/js/OMN-Go/omn-go-status.js). Style: [`omn-go-status.css`](../backend/frontend/html/css/OMN-Go/omn-go-status.css).
- Endpoint: `GET /api/status`.
- Go: [`app/status_app.go`](../backend/internal/app/status_app.go), then `HandleStatus` in [`status/status.go`](../backend/internal/status/status.go). The values: [`status/collect.go`](../backend/internal/status/collect.go). The text form: [`status/markdown.go`](../backend/internal/status/markdown.go).

#### The log

- Page: `/OMNGoLogs.html`. Template: [`templates/logs_page.html`](../backend/frontend/templates/logs_page.html).
- Script: [`omn-go-logs.js`](../backend/frontend/html/js/OMN-Go/omn-go-logs.js). Style: [`omn-go-logs.css`](../backend/frontend/html/css/OMN-Go/omn-go-logs.css).
- Endpoints: `GET /api/logs` (a stream), `GET /api/logs/history`.
- Go: [`app/log_handlers.go`](../backend/internal/app/log_handlers.go), [`app/log_app.go`](../backend/internal/app/log_app.go). The log tags: [`logx/levels.go`](../backend/internal/logx/levels.go). The stream and the history: [`logx/hub.go`](../backend/internal/logx/hub.go).
- Settings: `log_debug`, `log_info`, `log_tags`.
- To write a log line in Go: `a.log(logx.Sync).Infof("text %v", value)`.
- The console of the page in the footer: [`omn-go-console.js`](../backend/frontend/html/js/OMN-Go/omn-go-console.js).

### 5.9 Access from another device

- Settings: `share_lan`, `admin_password`.
- The server address: `startServer` in [`app/server.go`](../backend/internal/app/server.go). With `share_lan` off, the server listens on `127.0.0.1` only.
- Who is admin: `isLocalConnection`, `hasRole`, `authMiddleware` in [`app/middleware.go`](../backend/internal/app/middleware.go).
- Login: `handleLogin` and the session cookie in [`app/session.go`](../backend/internal/app/session.go). The login dialog: `checkSession` and `askLogin` in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js). Only the answer 401 of `/api/config` shows it. It is an `omnDialog` with no Cancel.
- A request from another site: [`app/request_guard.go`](../backend/internal/app/request_guard.go).
- The role of each endpoint: the third value of each `a.route` line in [`app/server.go`](../backend/internal/app/server.go). The role of each system page: `systemPages` in [`app/page_access.go`](../backend/internal/app/page_access.go).

### 5.10 Look of the pages

- The colors: section 1 of [`omn-go-core.css`](../backend/frontend/html/css/OMN-Go/omn-go-core.css). Each color value is there and in no other place.
- The theme: the setting `theme`. The backend sets `data-theme` on the page through `OMN_THEME`.
- The files of the person: `css/omn-go-custom.css` and `js/omn-go-custom.js` in the storage directory. An update does not replace them.
- The icons: the Material Icons font, `<i class="material-icons">name</i>`.
- Document: [`doc/COLOR_SCHEMES.md`](../doc/COLOR_SCHEMES.md).

### 5.11 Features of the Android layer only

- A note on the home screen: action `add-shortcut` in [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js). On Android 8 and later it asks for Reliable or Quick with `omnDialog`. Then the address `omngo://shortcut` with `mode`, then `createNoteShortcut` in [`Shortcuts.java`](../android/app/src/main/java/net/basov/omngo/Shortcuts.java).
- An `intent:` link and Termux: `handleIntentUri` in [`IntentBridge.java`](../android/app/src/main/java/net/basov/omngo/IntentBridge.java). The result comes back through `omnGoInsertCapture` in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js). Settings: `enable_intent_uri`, `enable_termux_intent`. Manual: [`md/AndroidIntents.md`](../backend/frontend/md/AndroidIntents.md).
- The Back key: `onBackPressed` in [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java). It asks `omnCloseTopLayer` of [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js) first. That function closes a dialog, then the search overlay, then a panel. Escape calls it too. With no open layer, the WebView goes back.
- The system bars: [`Fullscreen.java`](../android/app/src/main/java/net/basov/omngo/Fullscreen.java). Setting: `android_fullscreen`.
- The spinner of the start: `showStartStage` in [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java). The text comes from [`app/start_stage.go`](../backend/internal/app/start_stage.go).
- The background service and its notification: [`ServerService.java`](../android/app/src/main/java/net/basov/omngo/ServerService.java).
- The permissions and the intent filters: [`android/app/src/main/AndroidManifest.xml`](../android/app/src/main/AndroidManifest.xml).

## 6. The backend package by package

A Go package is one directory. All packages are in [`backend/internal/`](../backend/internal/). A package can use only a package of a lower layer. A test holds this rule.

| Layer | Package | Its work |
| --- | --- | --- |
| 0 | `logx` | The log: tags, levels, the stream to the Log page. |
| 0 | `noteheader` | The header block of a note: read a key, set a key. |
| 0 | `textmatch` | The match of one search term with a text. |
| 1 | `config` | The settings: the type `Config`, the table of the fields, the content types. |
| 2 | `storage` | The paths of the storage directory and the application files. |
| 3 | `render` | Markdown to HTML, the page shell, the Tags page, the templates. |
| 4 | `db` | The SQLite databases and their backups. |
| 4 | `gitsync` | Git sync. |
| 4 | `search` | Page search and global search. |
| 4 | `files` | The file index. |
| 4 | `exchange` | Note exchange. |
| 5 | `status` | The Status page. |
| 6 | `app` | The application: the server, the endpoints, the access control. |
| 7 | `backend` | Seven functions for Android and the desktop. See [`backend/backend.go`](../backend/backend.go). |

Two packages hold no application code. `testkit` helps the tests. `repocheck` holds the tests that read the whole repository.

### The files of package app

Package `app` connects all other packages. Start here when you follow a request.

| File | Contents |
| --- | --- |
| [`app/server.go`](../backend/internal/app/server.go) | The type `App` with all state. `startServer`. `registerRoutes`: the list of all endpoints. |
| [`app/routes.go`](../backend/internal/app/routes.go) | The function `a.route` that registers one endpoint with its role. |
| [`app/page_access.go`](../backend/internal/app/page_access.go) | `systemPages`: the list of the pages that are not notes. |
| [`app/handlers.go`](../backend/internal/app/handlers.go) | `serveFrontend`: each page and each file that is not an endpoint. The editor page. |
| [`app/note_handlers.go`](../backend/internal/app/note_handlers.go) | Save, read, create a page, the note box, the bookmark form, the external editor. |
| [`app/upload_handlers.go`](../backend/internal/app/upload_handlers.go) | Upload of an image or a data file. |
| [`app/serving.go`](../backend/internal/app/serving.go) | Static assets, files of the storage directory, the "not found" page. |
| [`app/config_handlers.go`](../backend/internal/app/config_handlers.go), [`app/config_page.go`](../backend/internal/app/config_page.go) | The Config page, save of the settings, restart. |
| [`app/log_handlers.go`](../backend/internal/app/log_handlers.go) | The Log page and the log stream. |
| [`app/middleware.go`](../backend/internal/app/middleware.go), [`app/session.go`](../backend/internal/app/session.go), [`app/request_guard.go`](../backend/internal/app/request_guard.go) | Access control. |
| [`app/storage.go`](../backend/internal/app/storage.go) | `initStorage`: the work of the start. |
| [`app/start_stage.go`](../backend/internal/app/start_stage.go) | The text under the spinner of the start. |
| `app/<name>_app.go` | The connection to one other package. Example: [`app/search_app.go`](../backend/internal/app/search_app.go) connects package `search`. |

### How a feature package connects to package app

Each feature package has a type `Service`. See [`search/service.go`](../backend/internal/search/service.go) for an example. The `Service` holds the values that one request needs. Package `app` makes a new `Service` for each request in its `<name>_app.go` file.

Thus a request for `/api/search` goes this way: `registerRoutes` in [`app/server.go`](../backend/internal/app/server.go), `handleSearch` in [`app/search_app.go`](../backend/internal/app/search_app.go), `HandleSearch` in [`search/http.go`](../backend/internal/search/http.go). The first two steps are one line each. The work is in the third.

## 7. The frontend file by file

### Templates

A template is a page fragment with tokens such as `%%TITLE_HTML%%`. The Go function `render.Fill` replaces each token. The directory is [`backend/frontend/templates/`](../backend/frontend/templates/).

| Template | Page, and the Go file that fills it |
| --- | --- |
| [`templates/index.html`](../backend/frontend/templates/index.html) | The shell of each page: header, buttons, footer, script list. Go: [`render/pages.go`](../backend/internal/render/pages.go) |
| [`templates/modals.html`](../backend/frontend/templates/modals.html) | The panels: note box, bookmark form, receive box. The dialogs come from `omnDialog`. Go: [`render/pages.go`](../backend/internal/render/pages.go) |
| [`templates/editor.html`](../backend/frontend/templates/editor.html) | The editor. Go: [`render/editor_page.go`](../backend/internal/render/editor_page.go) |
| [`templates/config_page.html`](../backend/frontend/templates/config_page.html) | The Config page. Go: [`app/config_page.go`](../backend/internal/app/config_page.go) |
| [`templates/search_page.html`](../backend/frontend/templates/search_page.html) | The search page. Go: [`search/page.go`](../backend/internal/search/page.go) |
| [`templates/files_page.html`](../backend/frontend/templates/files_page.html) | The file index. Go: [`files/page.go`](../backend/internal/files/page.go) |
| [`templates/status_page.html`](../backend/frontend/templates/status_page.html) | The Status page. Go: [`status/status.go`](../backend/internal/status/status.go) |
| [`templates/logs_page.html`](../backend/frontend/templates/logs_page.html) | The Log page. Go: [`app/log_handlers.go`](../backend/internal/app/log_handlers.go) |
| [`templates/db_backups.html`](../backend/frontend/templates/db_backups.html) | The Database Backups page. Go: [`db/backup_http.go`](../backend/internal/db/backup_http.go) |
| [`templates/not_found.html`](../backend/frontend/templates/not_found.html), [`templates/not_editable.html`](../backend/frontend/templates/not_editable.html) | The two refusal pages. Go: [`app/serving.go`](../backend/internal/app/serving.go) |

### Scripts

The directory is [`backend/frontend/html/js/OMN-Go/`](../backend/frontend/html/js/OMN-Go/). No script uses a framework or a build step. A top-level name of a script is a global name.

Scripts that each page loads, in this order:

| Script | Contents |
| --- | --- |
| [`omn-go-compat.js`](../backend/frontend/html/js/OMN-Go/omn-go-compat.js) | The notice for a WebView that is too old. The only ES5 file. |
| [`omn-go-console.js`](../backend/frontend/html/js/OMN-Go/omn-go-console.js) | The console of the page. |
| [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js) | `OMN.action` and the press listener. The header controls. The panels and `omnCloseTopLayer`. The notice (`omnNotice`, `omnNoticeAfterLoad`). The progress indicator. The work at the load of a page. |
| [`omn-go-highlight.js`](../backend/frontend/html/js/OMN-Go/omn-go-highlight.js) | The marks of the search terms in the page. |
| [`omn-go-nav.js`](../backend/frontend/html/js/OMN-Go/omn-go-nav.js) | A press on a link of a note. |
| [`omn-go-share.js`](../backend/frontend/html/js/OMN-Go/omn-go-share.js) | Send a note, copy, the page link, the metadata panel. |
| [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js) | Each call of the backend from a note page: login, new page, note box, `omnGoOpenDatabase`, the log stream, the receive box. |

Scripts that a page loads at the first press of a control. `omnLazy` and `omnLazyActions` in [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js) hold the list.

| Script | Contents |
| --- | --- |
| [`omn-go-sync.js`](../backend/frontend/html/js/OMN-Go/omn-go-sync.js) | Git sync: the buttons and the dialogs. |
| [`omn-go-bookmark.js`](../backend/frontend/html/js/OMN-Go/omn-go-bookmark.js) | The bookmark form. |
| [`omn-go-search.js`](../backend/frontend/html/js/OMN-Go/omn-go-search.js) | The search panel. |
| [`omn-go-dialog.js`](../backend/frontend/html/js/OMN-Go/omn-go-dialog.js) | The dialogs of the application: `omnAlert`, `omnConfirm`, `omnPrompt`, `omnDialog`, and `omnCloseDialog` for the Back key. The editor page loads it with a script element. |

Scripts of one page:

| Script | Page |
| --- | --- |
| [`omn-go-editor.js`](../backend/frontend/html/js/OMN-Go/omn-go-editor.js) | The editor. |
| [`omn-go-config.js`](../backend/frontend/html/js/OMN-Go/omn-go-config.js) | The Config page. |
| [`omn-go-status.js`](../backend/frontend/html/js/OMN-Go/omn-go-status.js) | The Status page. |
| [`omn-go-db.js`](../backend/frontend/html/js/OMN-Go/omn-go-db.js) | The Database Backups page. |
| [`omn-go-logs.js`](../backend/frontend/html/js/OMN-Go/omn-go-logs.js) | The Log page. |
| [`Bookmarker.js`](../backend/frontend/html/js/OMN-Go/Bookmarker.js) | The Bookmarks page. |

Three files come from other projects. Do not edit them: `katex.min.js`, `auto-render.min.js`, `highlight.min.js`.

### Styles

The directory is [`backend/frontend/html/css/OMN-Go/`](../backend/frontend/html/css/OMN-Go/). [`omn-go-core.css`](../backend/frontend/html/css/OMN-Go/omn-go-core.css) holds the palette and the style of each note page. Each other page has one file with its name, for example [`omn-go-search.css`](../backend/frontend/html/css/OMN-Go/omn-go-search.css). Each class, id and color token starts with `omn-`.

### Bundled notes

The directory is [`backend/frontend/md/`](../backend/frontend/md/). It holds the notes that the binary carries, for example [`md/UserManual.md`](../backend/frontend/md/UserManual.md) and [`md/ScriptRules.md`](../backend/frontend/md/ScriptRules.md).

## 8. The Android layer class by class

The directory is [`android/app/src/main/java/net/basov/omngo/`](../android/app/src/main/java/net/basov/omngo/). The code is plain Java with no AndroidX and no library.

| Class | Contents |
| --- | --- |
| [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java) | The one Activity. The life cycle. It reads each intent in `loadStartPage` and `onNewIntent` and gives the work to another class. |
| [`ServerService.java`](../android/app/src/main/java/net/basov/omngo/ServerService.java) | The foreground service that runs the backend. |
| [`WebViewSetup.java`](../android/app/src/main/java/net/basov/omngo/WebViewSetup.java) | The WebView and the spinner. `shouldOverrideUrlLoading` takes each address that is not a page: `omngo://share`, `omngo://shortcut`, `omngo://edit`, `intent:`. |
| [`ShareIn.java`](../android/app/src/main/java/net/basov/omngo/ShareIn.java) | A note, an image or a data file from another application. |
| [`ShareOut.java`](../android/app/src/main/java/net/basov/omngo/ShareOut.java) | A note to another application. |
| [`ExportProvider.java`](../android/app/src/main/java/net/basov/omngo/ExportProvider.java) | The content provider that gives a sent note to the other application. |
| [`IntentBridge.java`](../android/app/src/main/java/net/basov/omngo/IntentBridge.java) | An `intent:` link, a capture result, Termux. |
| [`Shortcuts.java`](../android/app/src/main/java/net/basov/omngo/Shortcuts.java) | A note on the home screen. |
| [`Fullscreen.java`](../android/app/src/main/java/net/basov/omngo/Fullscreen.java) | The system bars. |
| [`OmnConfig.java`](../android/app/src/main/java/net/basov/omngo/OmnConfig.java) | The reader of `config.json`. |
| [`OmnText.java`](../android/app/src/main/java/net/basov/omngo/OmnText.java) | Text rules that need no Android class, thus a plain test can run them. |

Other files of the Android layer:

- [`android/app/src/main/AndroidManifest.xml`](../android/app/src/main/AndroidManifest.xml): the permissions, the intent filters, the launcher icons.
- [`android/app/build.gradle`](../android/app/build.gradle): the version, the two flavors `standard` and `fdroid`, the default port of each flavor.
- [`android/build_aar.sh`](../android/build_aar.sh): the script that makes the Go library for Android.

## 9. How to read Go when you know Java and JavaScript

You do not need to know all of Go to find a place and change it. This list covers most of what the backend uses.

- `package app` at the top of a file: All files of one directory are one package. They see the names of each other with no import.
- `func (a *App) handleSync(w, r)`: A method `handleSync` of the class `App`. `a` is `this`.
- `func RenderMarkdown(...)`: A plain function. Java: a static method.
- A name that starts with a capital letter: The name is public, thus another package can use it. A name with a small first letter is private to its package.
- `type Config struct { ... }`: A class with fields only. There is no inheritance.
- `x := f()`: Declare `x` and give it the result. The compiler finds the type.
- `data, err := os.ReadFile(p)` then `if err != nil { ... }`: Go has no exceptions. A function gives the error as a second result. The caller tests it.
- `go f()`: Run `f` in a new thread.
- `defer f()`: Run `f` when this function ends. Java: `finally`.
- `w http.ResponseWriter, r *http.Request`: The response and the request of an endpoint. `r.FormValue("name")` reads a parameter. `w.Write(...)` sends the body.
- `map[string]string`: Java: `Map<String, String>`.
- `[]string`: A list of strings.
- `a.layout().MD("x.md")`: The path of a file in the storage directory.
- `a.config.Get()`: A copy of the settings. Example: `a.config.Get().ShareLAN`.
- `a.log(logx.Page).Errf(...)`: Write a log line with the tag `page`.
- A file `x_test.go`: The tests of `x.go`. The build does not put them into the binary.

The comment above a function tells what the function does and why. Read it before the code.

## 10. Procedures for common changes

Each procedure names the files and the tests that hold the rule. Read section 3 of [`CLAUDE.md`](../CLAUDE.md) for the full rules.

### 10.1 Change the text or the place of a control

1. Find the control in [`templates/index.html`](../backend/frontend/templates/index.html), [`templates/modals.html`](../backend/frontend/templates/modals.html) or the template of the page.
2. Change the markup. Keep the `data-action` value.
3. Change the style in the CSS file of the page.

### 10.2 Add a button to the page header

1. Add the `<button data-action="my-action">` to [`templates/index.html`](../backend/frontend/templates/index.html).
2. Add `OMN.action('my-action', function (el) { ... })` to [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js), or to [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js) when the action calls the backend.
3. Add the style to [`omn-go-core.css`](../backend/frontend/html/css/OMN-Go/omn-go-core.css). Use a class name that starts with `omn-`.
4. Add a test of the control in [`backend/frontend/test/`](../backend/frontend/test/).

`TestEachDataActionHasAFunction` fails when step 2 is absent. The classes `omn-server-only` and `omn-android-only` hide a control where it cannot work.

### 10.3 Add an endpoint

1. Write the Go function in the file of the topic, for example in [`app/note_handlers.go`](../backend/internal/app/note_handlers.go): `func (a *App) handleMyThing(w http.ResponseWriter, r *http.Request)`.
2. Add one line to `registerRoutes` in [`app/server.go`](../backend/internal/app/server.go): `a.route(mux, "/api/mything", admin, "JSON", post(a.handleMyThing))`.
3. Use the role `admin` when the endpoint writes data or shows private data.
4. Add the endpoint to `TestBaseline_RouteSet` in [`app/baseline_test.go`](../backend/internal/app/baseline_test.go).
5. Describe the endpoint in [`doc/API.md`](../doc/API.md).
6. Call the endpoint from a script with `fetch('/api/mything', { method: 'POST', body: ... })`.

`TestBaseline_RouteSet` and `TestAPIRouteTable` fail when step 4 or 5 is absent.

### 10.4 Add a setting

1. Add the field to the type `Config` in [`config/config.go`](../backend/internal/config/config.go), with its JSON key.
2. Add one row to `configFields` in [`config/fields.go`](../backend/internal/config/fields.go).
3. Add the control to [`templates/config_page.html`](../backend/frontend/templates/config_page.html).
4. Read the setting in Go with `a.config.Get().MyField`.
5. When Android must read it, add a method to [`OmnConfig.java`](../android/app/src/main/java/net/basov/omngo/OmnConfig.java).

`TestEveryConfigFieldIsInTheTable` and `TestEachTableValueHasAPlaceOnTheConfigPage` fail when step 2 or 3 is absent.

### 10.5 Add a value that a script reads from the backend

1. Add the value to `InjectRuntimeVars` in [`render/pages.go`](../backend/internal/render/pages.go). The script then reads it as a global name.
2. Add it to the test that holds the list of these names in [`app/baseline_test.go`](../backend/internal/app/baseline_test.go).

Use this path for a value that can change between two views. A value that never changes for a page goes into [`templates/index.html`](../backend/frontend/templates/index.html) as a token.

### 10.6 Change how Markdown becomes HTML

1. The Markdown rules: `RenderMarkdown` in [`render/markdown.go`](../backend/internal/render/markdown.go). The library is goldmark.
2. The links between notes: `RewriteInternalLink` in the same file.
3. The page around the body: `RenderIndexPage` in [`render/pages.go`](../backend/internal/render/pages.go) and [`templates/index.html`](../backend/frontend/templates/index.html).
4. The work in the browser after the load (KaTeX, highlight.js): the load function in [`omn-go-core.js`](../backend/frontend/html/js/OMN-Go/omn-go-core.js).

After such a change, each page must compile again. The backend does that at each new version. For a test on your device, open a page with `?refresh=1`.

### 10.7 Add a new kind of data file

1. Add one row to `UserFileTrees` in [`config/user_files.go`](../backend/internal/config/user_files.go).
2. Add the content type to [`config/content_types.go`](../backend/internal/config/content_types.go).
3. Add the same kind to `userFileTree` in [`OmnText.java`](../android/app/src/main/java/net/basov/omngo/OmnText.java) and to the upload list in [`omn-go-editor.js`](../backend/frontend/html/js/OMN-Go/omn-go-editor.js).
4. Decide if git sync carries the directory. `GitignorePatterns` in [`gitsync/repo.go`](../backend/internal/gitsync/repo.go) holds the paths that git sync does not carry.

`TestUserFileTreesHaveTheirCopies_JavaScript` and `TestUserFileTreesHaveTheirCopies_Java` compare the three copies.

### 10.8 Add a bundled note

1. Put the file into [`backend/frontend/md/`](../backend/frontend/md/). Write each paragraph on one line.
2. When each new version must replace the note, add it to `VersionDependentAssets` in [`storage/assets.go`](../backend/internal/storage/assets.go).

### 10.9 Add a script or a style file

1. Put the file into the script directory or the style directory. Give it the name `omn-go-<name>.js` or `omn-go-<name>.css`.
2. Add it to `VersionDependentAssets` in [`storage/assets.go`](../backend/internal/storage/assets.go) and to `GitignorePatterns` in [`gitsync/repo.go`](../backend/internal/gitsync/repo.go).
3. Load the script from the template of its page.

`TestEachAppScriptAndStyleIsVersionDependent` and `TestVersionDependentAssetsAreGitignored` fail when step 2 is absent.

### 10.10 Add an Android feature that a page starts

1. In the script, open an address: `window.location.href = 'omngo://mything?name=' + encodeURIComponent(value)`. Do it only when `IS_ANDROID` is true.
2. In `shouldOverrideUrlLoading` of [`WebViewSetup.java`](../android/app/src/main/java/net/basov/omngo/WebViewSetup.java), add a branch for `omngo://mything`.
3. Put the work into a class of its own, as [`Shortcuts.java`](../android/app/src/main/java/net/basov/omngo/Shortcuts.java) does. `MainActivity` must stay short.
4. Put each text rule into [`OmnText.java`](../android/app/src/main/java/net/basov/omngo/OmnText.java), and add a test in [`android/test/`](../android/test/).

### 10.11 Take a new kind of share from another application

1. Add the intent filter to [`android/app/src/main/AndroidManifest.xml`](../android/app/src/main/AndroidManifest.xml).
2. Read the intent in `loadStartPage` and in `onNewIntent` of [`MainActivity.java`](../android/app/src/main/java/net/basov/omngo/MainActivity.java). Both need the same branch.
3. Put the work into [`ShareIn.java`](../android/app/src/main/java/net/basov/omngo/ShareIn.java). Send the data to an endpoint of the backend.

### 10.12 Add a whole new page with its own endpoints

This is the largest change. The Status page is the model. Section 3 of [`CLAUDE.md`](../CLAUDE.md), "Add a feature", holds the 13 steps and the tests that hold them.

### 10.13 Give a change its version

Each commit has a new version. Change two files:

1. `APP_VERSION` in [`backend/version.go`](../backend/version.go).
2. `versionName` and `versionCode` in [`android/app/build.gradle`](../android/app/build.gradle).

## 11. How to find a place without this map

### Follow a control to its Go code

This example follows the *Upload* button of the page header.

1. Find the control in the templates. Search for its icon name or its title text.

   ```sh
   grep -rn 'cloud_upload' backend/frontend/templates
   ```

   Result: [`templates/index.html`](../backend/frontend/templates/index.html) holds `data-action="sync" data-arg="upload"`.
2. Find the function of the action.

   ```sh
   grep -rn "'sync'" backend/frontend/html/js/OMN-Go
   ```

   Result: [`omn-go-api.js`](../backend/frontend/html/js/OMN-Go/omn-go-api.js) names the action in `omnLazyActions`, thus the first press loads [`omn-go-sync.js`](../backend/frontend/html/js/OMN-Go/omn-go-sync.js). That file holds `OMN.action('sync', ...)`.
3. Find the endpoint that the function calls. Search the script for `fetch(`.

   Result: `fetch('/api/sync', ...)`.
4. Find the Go function of the endpoint.

   ```sh
   grep -n '"/api/sync"' backend/internal/app/server.go
   ```

   Result: `a.route(mux, "/api/sync", admin, "JSON", post(a.handleSync))`.
5. Find the function.

   ```sh
   grep -rn 'func (a \*App) handleSync' backend/internal/app
   ```

   Result: [`app/gitsync_app.go`](../backend/internal/app/gitsync_app.go). The function calls `HandleSync` of package `gitsync`.
6. Read [`gitsync/handlers.go`](../backend/internal/gitsync/handlers.go). `HandleSync` calls `SyncRepo` in [`gitsync/sync.go`](../backend/internal/gitsync/sync.go). `SyncRepo` selects the work by the action name. For `upload` it calls `syncPush` in [`gitsync/push.go`](../backend/internal/gitsync/push.go).

The same six steps work for each control.

### Other searches

| You know | Search for |
| --- | --- |
| A text that the page shows | The text, in [`backend/frontend/`](../backend/frontend/) and in [`backend/internal/`](../backend/internal/) |
| The address of a page, for example `/OMNGoStatus.html` | The address, in [`app/page_access.go`](../backend/internal/app/page_access.go) |
| An endpoint | The endpoint, in [`app/server.go`](../backend/internal/app/server.go) and in [`doc/API.md`](../doc/API.md) |
| A setting key, for example `share_lan` | The key, in the whole repository. Go, Java and JavaScript use the same key. |
| A text of the log | The text, in [`backend/internal/`](../backend/internal/) |
| A CSS class | The class, in the style directory and the templates |
| A word of the user manual | The word, in [`doc/TERMINOLOGY.md`](../doc/TERMINOLOGY.md). It gives the one name of each thing. |

Without a computer, use the search box of the repository page on GitHub. It searches the code of the repository.

## 12. Tests, build and other documents

### Run the tests

The build runs the tests before it compiles. Thus `bash local/build.sh` is also the test run. With Go on the device, the command is:

```sh
go vet ./backend/... && go test ./backend/...
```

This one command also runs the JavaScript tests and the Java tests. Go starts them. See `TestJavaScriptUnitTests` and `TestJavaUnitTests`.

| Tests | Where |
| --- | --- |
| Go tests of one package | The `*_test.go` files in the directory of the package |
| Tests that read the whole repository | [`backend/internal/repocheck/`](../backend/internal/repocheck/) |
| JavaScript tests | [`backend/frontend/test/`](../backend/frontend/test/) |
| Java tests | [`android/test/`](../android/test/) |

### Build

- `bash local/build.sh` builds the Android APK and the desktop binaries in Docker. See [`local/build.sh`](../local/build.sh).
- [`Dockerfile`](../Dockerfile) holds the steps. [`android/build_aar.sh`](../android/build_aar.sh) makes the Go library for Android.
- [`.github/workflows/`](../.github/workflows/) holds the build of a release on GitHub.
- [`metadata/net.basov.omngo.fdroid.yml`](../metadata/net.basov.omngo.fdroid.yml) is a copy of the F-Droid recipe.

### Other documents

| Document | Contents |
| --- | --- |
| [`CLAUDE.md`](../CLAUDE.md) | The rules of the development. Section 2 is a short map of the repository. Section 3 holds "Add a feature" and "Add a setting". |
| [`doc/API.md`](../doc/API.md) | Each endpoint: parameters, response, role. |
| [`doc/TESTING.md`](../doc/TESTING.md) | The map of the tests. |
| [`doc/TERMINOLOGY.md`](../doc/TERMINOLOGY.md) | The one name of each thing in the documents. |
| [`doc/COLOR_SCHEMES.md`](../doc/COLOR_SCHEMES.md) | The palette and the color tokens. |
| [`doc/decisions/`](../doc/decisions/) | One record for each large decision, with the reason. |
| [`md/UserManual.md`](../backend/frontend/md/UserManual.md) | The manual for the person who uses OMN-Go. |
| [`md/ScriptRules.md`](../backend/frontend/md/ScriptRules.md) | The rules for a note script. |
