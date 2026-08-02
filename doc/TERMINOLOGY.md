# OMN-Go documentation — controlled vocabulary

One name for one thing. Use the LEFT column. Never use a term from the RIGHT column
as a stand-in for it. If a right-column word is the only correct word in a spot
(a UI label, a git term, a code identifier), keep it — but do not rotate.

## Product and platform

| Use | Do not use instead |
| --- | --- |
| OMN-Go, or "the application" | the app, the program, the software, the tool, the system |
| the Android application | the Android app, the mobile app, the APK app |
| the desktop application | the desktop client, the PC version |
| device | machine, host, box, computer (when it means "a device that runs OMN-Go") |
| the backend | the Go server, the server side, the core |
| the frontend | the web UI, the client side |
| the WebView | the embedded browser, the native view |

## Content model

| Use | Do not use instead |
| --- | --- |
| note (the Markdown file and its content) | document, article, entry, item, record |
| page (the addressable rendered unit, `<name>.html`) | view, screen, document |
| Markdown source | source text, raw markdown, the note text, the file body |
| HTML cache (the compiled `html/<name>.html`) | rendered cache, compiled pages, page cache |
| header block (the Pelican-style `Key: value` lines) | front matter, header, metadata block, preamble |
| storage directory | data directory, workspace, notes directory, storage dir, storage root |
| note script (a `<script>` block inside a note) | page script, inline script, user script |
| static asset | resource, static file, non-page file |

Exception: in the search index sections of API.md, "document" is the unit the
index holds. Keep "document" there and only there.

## Actions and UI

| Use | Do not use instead |
| --- | --- |
| press (a button) | tap, click, hit, select, tap or click |
| open (a page, a link, a file) | navigate to, go to, visit, pull up |
| enable / disable | turn on, switch on, activate, arm, toggle on |
| create a backup | take a backup, make a snapshot, snapshot the database |
| backup (noun: the `.jsonl` file) | snapshot, dump, export, safety copy |
| restore | recover, roll back, reimport |
| save | write out, commit (unless it is a git commit), persist |
| show / hide | display, reveal, surface, conceal |

## Search

| Use | Do not use instead |
| --- | --- |
| global search (UI label: *All notes*) | full search, index search, search everything, site search |
| page search (UI label: *This page*) | local search, in-page search, single-page search |
| search index, or "the index" | the search database, the catalog |
| query | search string, search term (a *term* is one word of a query) |
| term (one whitespace-separated word of a query) | keyword, token, word (in the matching rules) |
| rung (one of the three match levels) | tier, level, stage, pass |
| snippet | excerpt, preview, extract |
| section (the part of a note a hit fell in) | chunk, block, region |
| find bar (the editor's own search) | search bar (reserve "search panel" for the header search) |
| search panel (the header magnifier) | search dialog, search popup |

## Sync and storage

| Use | Do not use instead |
| --- | --- |
| git synchronization, or "git sync" | mirroring, replication, cloud sync |
| pull (UI label: *Download*) | fetch and merge, download changes |
| push (UI label: *Upload*) | send, publish, upload changes |
| remote (the git remote) | server (reserve "server" for the OMN-Go HTTP server) |
| git server slot | profile, git profile, server entry |
| conflict | clash, collision, merge problem |
| LAN sharing (UI label: *Share on LAN*) | network sharing, remote access, sharing on the network |

## Access control

| Use | Do not use instead |
| --- | --- |
| admin (the role, `session_role=admin`) | administrator, superuser, owner |
| guest (the role) | visitor, reader, read-only user |
| admin-only | administrator only, admin-protected, privileged |
| local connection (`127.0.0.1`, `::1`, `localhost`) | loopback, same-device connection, on-device connection |
| remote caller | external client, outside client, LAN client |

## API surface

| Use | Do not use instead |
| --- | --- |
| endpoint | route (only for `ServeMux` registration), API call, method, handler |
| parameter | argument, field (a *field* is a key in a JSON body or config) |
| response | reply, result payload, answer |
| status code | HTTP code, return code |
| request body | payload, post data |

## Named pages (use exactly this form)

- the Welcome page
- the Config page
- the Tags page (`OMNGoTags`)
- the search page (`OMNGoSearch.html`)
- the file index (`OMNGoFiles.html`) — never "directory index", "file browser", "file listing"
- the Database Backups page (`/db_backups`)
- the Quick Notes page, the Bookmarks page

## Banned words in all files

seamless, robust, powerful, effortless, bloated, aggressive, intelligent,
dynamic (as praise), rich, comprehensive, leverage, utilize, facilitate,
ensure, delve, elevate, unlock, streamline, cutting-edge, modern successor,
world-class, simply (as filler), just (as filler), of course, note that,
it is important to note, in order to, prior to, subsequent to.
