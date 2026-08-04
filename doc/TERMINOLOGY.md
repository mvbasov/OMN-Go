# OMN-Go documentation — controlled vocabulary

Adopted 2026-08-02 during the ASD-STE100 (Simplified Technical English) pass over
`README.md`, `full_description.txt`, `UserManual.md`, `Editor.md`,
`AndroidIntents.md`, `ScriptRules.md`, `Database.md` and `API.md`.

Purpose: one name for one thing, across the whole documentation set. Before this
pass the same object was called three or four different names depending on which
file you opened. Use the LEFT column. Never use a term from the RIGHT column as a
stand-in for it. If a right-column word is the only correct word in a spot (a UI
label, a git term, a code identifier), keep it — but do not rotate.

New documentation, and any edit to the files above, should follow this table.

**Standing rule (2026-08-03).** Write every new documentation text for this
project in ASD-STE100 Simplified Technical English. Run the `ste-writing` skill
on the text before you deliver it, then check it against this table. This
covers notes that ship with the application, `README.md`, F-Droid metadata,
release notes and patch commit messages.

**Android first.** Android is the main device for OMN-Go. When a text lists
several ways to do one thing, put the Android ways first, then the way that
works on each device, then the desktop-only ways.

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

Exception: in the search index sections of `API.md`, "document" is the unit the
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
| the note box (the Quick Note panel) | the quick note dialog, the text box, the note popup |
| the bookmark form (the Ingest Bookmark panel) | the bookmark dialog, the bookmark popup, the ingest panel |
| the start page buttons (the two buttons on Welcome) | the big buttons, the cards, the tiles |

`dump` stays correct in `sqlite3 .dump` and `websqldump.js` — those are the real
names of external artifacts, not synonyms for a backup.

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
| the search box (the Bookmarks page field) | the filter field, the bookmark search bar |

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
| local connection (`127.0.0.1`, `::1`, `localhost`) | loopback, same-device connection |
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
- the How to use Bookmarks page (`BookmarksHowTo`)

## Banned words in all files

seamless, robust, powerful, effortless, bloated, aggressive, intelligent,
dynamic (as praise), rich, comprehensive, leverage, utilize, facilitate,
ensure, delve, elevate, unlock, streamline, cutting-edge, modern successor,
world-class, simply (as filler), just (as filler), of course, note that,
it is important to note, in order to, prior to, subsequent to.

## Style rules applied with this vocabulary

STE-flavored mode, not strict mode:

1. Active voice with a named actor. Passive only where the actor is unknown.
2. Max 20 words for an instruction, max 25 for a descriptive sentence.
3. No contractions. No semicolons in prose.
4. An em dash may mark a parenthetical aside, at most one per paragraph. An em
   dash that joins two independent clauses becomes a sentence break.
5. One topic per paragraph, max six sentences.
6. A condition comes before its command.
7. American spelling.

Strict mode (every rule, both length caps, numbered lists for procedures)
applies to the easy-start texts: the first quick note, `BookmarksHowTo.md` and
the "Easy start" part of `UserManual.md`. A first-run reader gets one
instruction per sentence.

## Notes that ship with the application: one line per paragraph

`html.WithHardWraps` is on, so the renderer keeps every line break of the
Markdown source. A paragraph that is wrapped at 75 characters in the source
breaks at those same places on a phone screen. Write each paragraph of a
bundled note on one line. The older bundled notes still carry wrapped
paragraphs.

## What the pass did not touch

Code fences, inline code spans, `intent:` URIs, HTML blocks, `material-icons`
spans, table structure and cell values, link targets, and heading text. Heading
text is locked because `#anchor` links point at it from other files. The one
exception made was `API.md` §5.3, renamed from "Directory index" to "The file
index" after confirming no link targets that anchor.
