# OMN-Go Server API

Reference for every HTTP endpoint that the backend exposes
(`backend/server.go`, `backend/logger.go`).

Applies to OMN-Go **26.08.2** (`backend/version.go`, `APP_VERSION`).

---

## 1. Overview

### 1.1 Base URL

```
http://<host>:<port>/
```

| Aspect | Value |
| --- | --- |
| Scheme | `http` only — there is no TLS listener |
| Bind address | `127.0.0.1` by default; `0.0.0.0` when `share_lan` is `true` in `config.json` |
| Port | `config.json` → `server_port` if `> 0`; otherwise the per-platform default |
| Default port (desktop) | `8080` |
| Default port (Android, standard flavor) | `8080` |
| Default port (Android, F-Droid flavor) | `8081` |

The server binds the listening socket **once at process start**. A change to
`server_port` or `share_lan` needs a restart (see
[`POST /api/restart`](#412-post-apirestart)). With LAN sharing off, no remote
device can complete a TCP handshake, whatever the authentication logic does.

If the port is busy, the server retries `net.Listen` ten times at 300 ms
intervals (~3 s) before it stops. This covers the window during a self-restart
when the replacement process races the teardown of the old socket.

### 1.2 Routing model

Routing uses the Go standard library `http.ServeMux`. This has four effects:

* A route **without** a trailing slash (`/api/note`, `/login`, …)
  matches that exact path only.
* A route **with** a trailing slash (`/js/`, `/css/`, `/json/`,
  `/images/`, `/user_json/`) matches the whole subtree.
* `/` is the catch-all. `serveFrontend` receives every request that the
  routes above do not match.
* **`ServeMux` does not dispatch on method.** Any method reaches the
  endpoint, unless that endpoint checks `r.Method` (the table in §3 says
  which endpoints do). `r.FormValue` reads the URL query string *and* an
  `application/x-www-form-urlencoded` / `multipart/form-data` body. Most
  form-style endpoints therefore accept parameters either way.

### 1.3 Request encodings

| Encoding | Used by |
| --- | --- |
| URL query string | `/api/note`, `/api/edit-external`, `/api/sync/preview`, `/api/db/*`, page-level `?edit`/`?refresh` |
| `application/x-www-form-urlencoded` | `/login`, `/api/save`, `/api/newpage`, `/api/quick`, `/api/bookmark`, `/api/config`, `/api/sync` |
| `multipart/form-data` | `/api/upload`, `/api/upload_json` |
| `application/json` | `/api/sql` |

### 1.4 Response encodings

There is no single envelope. The server uses three shapes:

1. **Plain text** — short status words (`Saved`, `OK`, `Restarting`) or a
   fragment to splice into a note. The legacy note and upload endpoints
   return this shape.
2. **JSON** — `/api/config` (GET), `/api/sql`, `/api/db/*`, `/api/sync`,
   `/api/sync/preview`.
3. **`text/event-stream`** — `/api/logs` only.

`http.Error` sends `text/plain; charset=utf-8` with the message in the body.
The JSON endpoints keep their JSON shape for an error and add
`"status": "error"`.

---

## 2. Authentication and authorization

### 2.1 Model

`authMiddleware` (`backend/middleware.go`) wraps the protected endpoints:

1. **A local connection bypasses authentication.** If the peer address is
   `127.0.0.1`, `::1` or `localhost`, the endpoint runs with no further
   check. This lets the WebView of the Android application and the desktop
   browser work without a login.
2. For every other connection, the request must carry a `session_role`
   cookie with an accepted role. The server registers every protected route
   with `requireAdmin = true`, so a remote caller needs `session_role=admin`.
   `authMiddleware` accepts the `guest` role only for a route registered
   with `requireAdmin = false`. No such route exists today.
3. A missing or insufficient cookie gives `401 Unauthorized` with the body
   `Unauthorized`.

There is no CSRF token, no bearer token, and no rate limiting. Passwords
are stored in `config.json` in cleartext and compared with `==`.

### 2.2 Obtaining the cookie

`POST /login` with the correct password sets:

```
Set-Cookie: session_role=admin; Path=/
```

This is a session cookie. It has no `Max-Age` and no `Expires`. It is not
`HttpOnly`, it is not `Secure`, and it has no `SameSite` attribute.

### 2.3 Protection map

| Endpoint | Auth |
| --- | --- |
| `POST /login` | none (it is the login) |
| `GET /api/note` | **none — deliberately open** |
| `GET /api/search` | **none — deliberately open** |
| `GET /api/logs` | **none** |
| `/api/quick`, `/api/bookmark`, `/api/upload`, `/api/upload_json`, `/api/save`, `/api/newpage`, `/api/config`, `/api/restart`, `/api/sql`, `/api/db/backup`, `/api/db/backups`, `/api/db/restore`, `/api/sync`, `/api/sync/preview`, `/api/edit-external`, `/api/status`, `/db_backups` | admin (local bypass applies) |
| `GET /OMNGoFiles.html` | admin (local bypass applies) — answers a **page**, not a 401 |
| `GET /OMNGoStatus.html` | admin (local bypass applies) — answers a **page**, not a 401 |
| All page and static routes (`/`, `*.html`, `/js/`, `/css/`, `/json/`, `/images/`, `/user_json/`) | none |

---

## 3. Endpoint index

| Method(s) | URL | Enforces method? | Auth | Response |
| --- | --- | --- | --- | --- |
| any | `/login` | no | none | text |
| GET | `/api/note` | no | none | raw file |
| GET | `/api/search` | no | **none — deliberately open** | JSON |
| any | `/api/save` | no | admin | text |
| any | `/api/newpage` | no | admin | text |
| any | `/api/quick` | no | admin | text |
| any | `/api/bookmark` | no | admin | text |
| POST | `/api/upload` | no | admin | text (HTML fragment) |
| POST | `/api/upload_json` | no | admin | text (Markdown fragment) |
| GET, POST | `/api/config` | yes (405 otherwise) | admin | JSON / text |
| POST | `/api/restart` | yes (405 otherwise) | admin | text |
| POST | `/api/sql` | yes (405 otherwise) | admin | JSON |
| POST | `/api/db/backup` | yes (405 otherwise) | admin | JSON |
| GET | `/api/db/backups` | yes (405 otherwise) | admin | JSON |
| POST | `/api/db/restore` | yes (405 otherwise) | admin | JSON |
| any | `/api/sync` | no | admin | JSON |
| GET | `/api/sync/preview` | yes (405 otherwise) | admin | JSON |
| GET | `/api/edit-external` | no | admin | HTML or 303 |
| GET | `/api/logs` | no | none | SSE |
| GET | `/api/status` | yes (405 otherwise) | admin | JSON / Markdown |
| GET | `/db_backups` | no | admin | HTML |
| GET | `/OMNGoSearch.html` | no | none | HTML (explains how to turn global search on when it is off; used to 404) |
| GET | `/OMNGoFiles.html` | no | admin | HTML (a page for a guest, not a 401) |
| GET | `/OMNGoStatus.html` | no | admin | HTML (a page for a guest, not a 401) |
| GET | `/`, `/<name>.html`, `/<asset>` | no | none | HTML / asset |
| GET | `/js/…`, `/css/…`, `/json/…` | no | none | asset |
| GET | `/images/…`, `/user_json/…` | no | none | asset |

---

## 4. Endpoint reference

### 4.1 Authentication

#### `POST /login`

Exchange a password for a role cookie. Only a remote caller needs this
endpoint.

**Parameters** (query string or form body)

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `password` | string | yes | Compared against `admin_password`, then `guest_password` from `config.json` |

**Responses**

| Status | Content-Type | Body | Notes |
| --- | --- | --- | --- |
| `200` | `text/plain` | `OK` | Sets `session_role=admin` or `session_role=guest` |
| `401` | `text/plain` | `Invalid` | No cookie set |

```bash
curl -i -c jar.txt -d 'password=admin_secret_changeme' http://host:8080/login
```

---

### 4.2 Notes

#### `GET /api/note`

Return the **Markdown source** of a note, or the bytes of any static asset.
The internal editor reads this endpoint when it loads.

**Parameters** (query string)

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string | no | `Welcome` | Page name or asset path |

`resolvePageName` (`backend/paths.go`) resolves `name`:

| Shape of `name` | Treated as | Source read |
| --- | --- | --- |
| `Welcome` (no dot) | markdown page | `md/Welcome.md` |
| `Welcome.md` | markdown page | `md/Welcome.md` |
| `Welcome.html` | markdown page | `md/Welcome.md` |
| `notes/Trip` | markdown page in a subdirectory | `md/notes/Trip.md` |
| `js/omn-go-core.js`, `css/x.css`, any other extension | static asset | `html/js/omn-go-core.js` |

**Behavior when a page does not exist yet**

The endpoint **never answers 404 for a page**. It falls back, in order, to
the embedded default (`frontend/md/<name>.md`), or to a synthesized header
block stub:

```
Title: <name>
Date: 2026-07-27 14:05:00
Category: Notes
Author: <config.author, omitted when empty>

```

The endpoint saves the fallback to disk before it answers, so this happens
only once for each page.

**Responses**

| Status | Content-Type | Body |
| --- | --- | --- |
| `200` | (not set — sniffed) | Raw markdown source, or the raw asset bytes |
| `404` | `text/plain` or `text/html` | Only for a non-page asset that does not exist; see [§6](#6-404-handling) |

```bash
curl 'http://127.0.0.1:8080/api/note?name=Welcome'
```

---

#### `POST /api/save`

Write a note or asset back to disk.

**Parameters** (form body or query string)

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Same resolution rules as `/api/note` |
| `content` | string | yes (may be empty) | Full replacement text |

**Side effects**

* The endpoint changes `\r\n` to `\n`.
* For a note, `ensureHeaderModified` writes or updates
  `Modified: YYYY-MM-DD HH:MM:SS` in the header block.
* The endpoint saves the Markdown source **first**. Only then does
  `renderAndCache` compile the HTML cache (`html/<name>.html`). The server
  logs a cache failure, but the endpoint still reports success. The next
  page view compiles the cache again.
* For a static asset, the endpoint saves the bytes straight to
  `html/<path>` and renders nothing.

**Responses**

| Status | Body | Cause |
| --- | --- | --- |
| `200` | `Saved` | |
| `400` | `Missing name` | `name` empty |
| `500` | `Failed to save` | `mkdir` or write of the source failed |

```bash
curl -X POST http://127.0.0.1:8080/api/save \
     --data-urlencode 'name=Welcome' \
     --data-urlencode 'content=Title: Welcome

Hello.'
```

---

#### `POST /api/newpage`

Create a note **and** insert a link to the new note into the source note.

**Parameters** (form body or query string)

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `target` | string | yes | Name of the page to create, without `.md` |
| `title` | string | yes | Title written into the header block of the new note, and the link text |
| `source` | string | no | Page the link is inserted into |

**Target resolution** (`resolveNewPageTarget`) follows the way the browser
resolves a bare relative link on `source`:

| `target` | `source` | Created as |
| --- | --- | --- |
| `test` | `local/local` | `local/test` |
| `test` | `Welcome` | `test` |
| `/test` | anything | `test` (storage directory) |
| `sub/test` | anything | `sub/test` (storage directory) |

The endpoint writes the link into `source`. For the bare case the link uses
the **raw** target, so the browser resolves it against `source` in the same
way. When the target carries its own directory, the link gets an explicit
leading `/`.

The endpoint never overwrites an existing `target` file. In that case it
only inserts the link. A new note gets this header block:

```
Title: <title>
Date: <now>
Modified: <now>
Category: Notes
Author: <config.author, omitted when empty>

```

The endpoint inserts the link `* [<title>](<href>)` below the header block
of `source`. If `source` has no header block, the endpoint puts the link at
the top. It then updates `Modified:` in `source` and compiles the HTML cache
of `source` again at once.

**Responses**

| Status | Body |
| --- | --- |
| `200` | The **resolved** target name, e.g. `local/test` — plain text, no trailing newline |
| `400` | `Missing fields` when `target` or `title` is empty |

---

#### `POST /api/quick`

Prepend a timestamped entry to `md/QuickNotes.md`.

**Parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `note` | string | yes | Entry text |

The endpoint inserts the entry directly after the first blank line, which
is the end of the header block:

```

---
##### 2026-07-27 14:05:00
<note>

```

The endpoint sets the title to `Quick Notes` and compiles the HTML cache
again.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `Saved` |
| `200` | *(empty body, nothing written)* when `note` is empty |

---

#### `POST /api/bookmark`

Append a bookmark record to `md/Bookmarks.md`.

**Parameters**

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `url` | string | no | Bookmarked URL |
| `title` | string | no | Bookmark title |
| `tags` | string | no | Comma-separated; split on `,`, trimmed, empties dropped |
| `notes` | string | no | Semicolon-separated; split on `;`, trimmed, empties dropped |

The endpoint inserts the record directly after the marker line
`<!-- Don't edit body below this line -->` as an indented JSON object:

```json
  {
    "date": "2026-07-27 14:05:00",
    "url": "https://example.org",
    "title": "Example",
    "tags": ["ref", "web"],
    "notes": ["read later"]
  },
```

The endpoint encodes the values as JSON. `<`, `>` and `&` become
`\u`-escapes, so a note can never break out of the `<script>` block around
it.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `Saved` — also returned when `Bookmarks.md` is missing or lacks the marker, in which case nothing was written |

---

### 4.3 Search

#### `GET /api/search`

Fuzzy search over the notes. There are two scopes and one matcher. The
response shape, the scoring and the meaning of a result are the same for
both scopes. Only the searched content differs.

| Scope | Searches | Needs the index | Needs configuration |
| --- | --- | --- | --- |
| `page` | the one file named by `on` | no | **no — always available** |
| `all` | every indexed document | yes | yes (`search_enabled`) |

Page scope reads that one file for each request and keeps nothing. This is why
page scope has no setting. There is no continuing cost that a setting could
remove. The server answers global scope from the in-memory index
(`backend/search_index.go`). The server builds that index only when
`search_enabled` is on.

**Parameters** (query string)

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `q` | string | yes | — | The query. Empty is an empty result, not an error |
| `scope` | string | no | see below | `page` or `all` |
| `on` | string | no | — | Page name or asset path for `scope=page`, resolved by `resolvePageName` |
| `kind` | string | no | the configured kinds | Comma list of `md,bookmarks,js,json,user_json` — narrows, never widens |
| `limit` | int | no | `50` | Max results (hard cap `200`); `scope=page` returns at most one |
| `snippets` | int | no | `3` | Max snippet lines per result (hard cap `10`) |

The default `scope` is the configured `search_scope`. When global search cannot
answer, the default falls back to `page`. A caller that sends no `scope` states
no preference. A default into a scope that can only fail would be a poor
reading of that silence. The server still refuses an **explicit** `scope=all`
in that state. To answer it with page scope would misreport what the server
did.

**Query syntax**

Whitespace separates the terms, and the matcher combines them with **AND**.
Every term must match somewhere in a document. A term can carry a field prefix:
`title:`, `tag:` or `path:`. A term can also carry `kind:` to filter. An
unknown prefix is not a prefix, so `https://example.com` stays a search for
that text.

The first of the three rungs that hits matches the term. Matches compare by
**(rung, score)** and never by score alone. A document that contains the word
therefore ranks above a document that only suggests it:

1. **exact substring** (case- and diacritic-folded)
2. **subsequence**, fzf-style — `andint` finds `Android Intents`
3. **bounded edit distance** (Damerau/OSA) for terms of 4+ runes — `fecth`
   finds `fetch`

**Response** `200`

```json
{
  "query": "fetch json",
  "scope": "page",
  "took_ms": 9,
  "total": 1,
  "truncated": false,
  "highlight": ["fetch", "json"],
  "results": [
    {
      "path": "md/Test/OMN-Go/Fetch.md",
      "kind": "md",
      "name": "Test/OMN-Go/Fetch",
      "title": "Test/OMNGo/Fetch",
      "tags": ["Test"],
      "score": 570,
      "url": "/Test/OMN-Go/Fetch.html",
      "matches": [
        {
          "line": 15,
          "context": "script",
          "section": { "id": "fetching-json", "label": "Fetching JSON" },
          "text": "const response = await fetch('/json/test.json');",
          "spans": [[23,5],[31,4],[41,4]]
        }
      ]
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `scope` | The scope actually used — a client that sent none can read its default from here |
| `total` | Documents matched, before `limit` |
| `truncated` | Results were capped by `limit`, or a file was only partly searched |
| `score` | Higher is better; comparable only within one response |
| `url` | Where the result lives: `/<Name>.html` for a note, the served path for an asset, plus `#<section.id>` when the best hit fell in an addressable section. Never carries `hl` — see `highlight` |
| `highlight` | The query's terms as typed, for the client to mark in whatever page it opens. Present on both scopes, and on a query that matched nothing |
| `matches[].line` | 1-based line in the file **as stored** — the markdown source for a note |
| `matches[].context` | `script` or `code` when the hit is inside a `<script>` or a fenced block; absent in prose |
| `matches[].section` | The part of the document the hit fell in — a bookmark entry, a timestamped quick note, a heading's section. Absent for a flat document and for the header block of a note |
| `matches[].section.id` | The anchor in the compiled HTML. May be absent while `label` is present: a section that can be named but not linked |
| `matches[].section.label` | What to show — the heading text, the bookmark's title (or its URL) |
| `matches[].text` | The snippet, whitespace-trimmed and windowed to ~160 runes around the first hit, with `…` markers |
| `matches[].spans` | `[start, length]` pairs in **rune** offsets into `text` |

`spans` are rune offsets. They are not byte offsets and not UTF-16 units. If
you cut `text` by any other unit, you cut Cyrillic letters and emoji in half.
In JavaScript, `Array.from(text)` gives the correct units.

#### Sections

The index holds some notes as sections. This applies to a note whose body is a
run of entries, such as QuickNotes and anything else built from `---` plus
`##### <timestamp>`. It also applies to a note with headings. A result then
opens **at** the entry that matched, not at the top of the page. The server
parses `Bookmarks.md` instead of a scan line by line, so each bookmark is also
its own section.

The server predicts the anchor. It does not observe it. goldmark assigns
heading ids at compile time, and `Bookmarker.js` assigns bookmark ids at render
time. The server must name these ids without a read of either result. This has
two effects:

- **An id can be absent while a label is present.** A heading can contain a
  link, inline code, math, an entity, inline HTML, an underscore or a closing
  `##` run. Such a heading renders as text that differs from its source, so
  the server predicts no id and emits none. A heading that is wholly
  non-ASCII gets the degenerate `heading` fallback of goldmark, and the server
  does not emit that id either. After one heading in a document is unreadable
  in this way, **no** later heading in that document gets an id. The collision
  counter of goldmark has moved on by an unknown amount.
- **Anchors switch off if the renderer changes.** At first use the server
  compiles a probe document. It then checks that the returned ids are the ids
  that it predicted. After a mismatch it logs
  `[search] section anchors disabled`, and every result links at the page
  instead. The alternative is to send the reader to the wrong section of the
  right page, which looks like a fault in the note.

Page scope reports sections, but it never anchors a result. The reader is
already on the page, and the search panel highlights in place instead of
opening another page.

`highlight` is not the same thing as `spans`. `spans` are offsets into a
snippet of the **Markdown source**. `highlight` is literal text for the client
to find in the **rendered** page. The rendered page is a different document.
The source line `**fetch** the json` renders as `fetch the json`, so no span
from the source is valid in the rendered page.

The server strips a field prefix from each `highlight` term (`tag:hydro` →
`hydro`) and drops a term shorter than 2 runes. The server does **not** fold
the text. The client marks literal occurrences, and folding maps `ё` to `е`,
so the folded form of a term can appear nowhere on the page. If a term matched
only fuzzily, the client finds nothing and marks nothing. This reports the true
result.

**For a file larger than 500 KiB**, the server searches up to that point and
cuts at a line boundary. The result then carries `"truncated": true`. This
tells the client that the server found nothing in the part that it read,
instead of saying nothing at all.

#### `GET /OMNGoSearch.html`

The search page runs the same search as `scope=all` and renders it as a page
that you can share and that needs no JavaScript. `serveHTMLPage` handles it as
a special case beside `Config` and `OMNGoTags`. It is **dynamic like `Config`**
- there is no `md/OMNGoSearch.md`, the server writes nothing to the `html/`
cache, and `?refresh` has no effect here.

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `q` | string | no | — | The query. Absent or empty renders just the form |

Every result on this page links to `/<Name>.html?hl=<term>&hl=<term>`. See
`?hl=` below.

The search page is global only. With `search_enabled` off, it answers **200**
with an explanation of what global search costs and a link to
`/Config.html#cfg-search`. It shows no search form, because a submitted form
would return to this same page. In that state the page ignores the query
parameter. An empty result list would read as "nothing matched".

The page answered 404 before. The reason was that a permanently empty page is
worse than a true miss. That reason was wrong about who opens this address. The
address is linkable, and users put a "Search" link on their own notes. The 404
was therefore a dead end that named neither the cause nor the cure.

#### `?hl=<term>` — highlight on arrival

Any page accepts repeated `hl` parameters. At load the client marks every
literal occurrence of those terms in `#preview` and scrolls to the first one.
The client then removes the parameters from the address bar with
`history.replaceState`. The URL that the user copies, bookmarks or reloads is
therefore the plain one, and a refresh does not apply the highlight again.

The parameter repeats instead of one comma-joined value, because a term can
contain a comma. The client ignores a term shorter than 2 runes (`OMN_HL_MIN`
in `omn-go-core.js`, `highlightMinRunes` in `search.go` — the two ends agree).

`omn-go-core.js` handles all of this. The highlight therefore works on a page
opened from disk with no server running, and on a page that the search panel
never loads.

When the URL also carries a fragment, for example
`/Bookmarks.html?hl=cats#2026-06-15-200000`, the client still marks the terms.
It leaves the **scroll to the anchor**, which is the more precise target.

**Errors**

| Status | Body | When |
| --- | --- | --- |
| `200` | normal response with `"results": []` | Nothing matched; `on` names something that does not exist; `q` is empty |
| `400` | `{"status":"error","error":"unknown scope …"}` | `scope` is neither `page` nor `all` |
| `503` | `{"status":"disabled","error":"global search is off …"}` | `scope=all` with `search_enabled` false — the user can act on this |
| `503` | `{"status":"unavailable","error":"the search index is not ready"}` | `scope=all`, enabled, but no index yet — the user cannot |

A miss is always `200` with no results. If `on` points outside the storage
directory, or at a file that does not exist, the answer is a miss. The endpoint
is never a way to probe the filesystem.

---

### 4.4 Uploads

Both upload endpoints share `saveUploadedFile`:

* It parses the multipart form with a 10 MB in-memory threshold. This
  threshold is *not* the size cap.
* It checks the extension against a whitelist and ignores case.
* It checks `header.Size` against `max_upload_size_mb` (default **3 MB**).
* It saves the file to the destination directory under the **original
  filename**. It overwrites an existing file with the same name.

#### `POST /api/upload`

**Body**: `multipart/form-data`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `image` | file | yes | Allowed: `.png .jpg .jpeg .gif .webp .svg` |

The server saves the file in `html/images/` and serves it from
`/images/<filename>`.

**Responses**

| Status | Content-Type | Body |
| --- | --- | --- |
| `200` | `text/plain` | `\n<img src="/images/<name>" alt="<name>" class="omn-imported-image" />\n` (filename HTML-escaped) — ready to splice into a note |
| `400` | `text/plain` | `file type ".exe" is not allowed (allowed: .png, .jpg, …)` or `file too large (5.21 MB, limit is 3.00 MB)` |
| `500` | `text/plain` | `Upload failed` (disk/permission error) |

```bash
curl -F 'image=@shot.png' http://127.0.0.1:8080/api/upload
```

#### `POST /api/upload_json`

**Body**: `multipart/form-data`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `file` | file | yes | Allowed: `.json .jsonl` |

The server saves the file in `html/user_json/` and serves it from
`/user_json/<filename>`.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `\n[<name>](/user_json/<name>)\n` |
| `400` / `500` | Same shapes as `/api/upload` |

---

### 4.5 Configuration

#### `GET /api/config`

Return the whole live configuration as JSON. **The response includes
`admin_password`, `guest_password`, and the SSH private key and password of
every git server slot, all in cleartext.**

**Response** `200`, `application/json`:

```json
{
  "force_pull_one_time": false,
  "server_port": 8080,
  "admin_password": "admin_secret_changeme",
  "guest_password": "guest_secret_changeme",
  "author": "Anonymous",
  "use_internal_editor": true,
  "desktop_ext_cmd": "subl",
  "theme": "auto",
  "share_lan": false,
  "hostname": "pixel7",
  "backup_prune_depth": 3,
  "mime_types": { ".css": "text/css", ".js": "application/javascript" },
  "active_git_index": 0,
  "git_servers": [
    { "name": "Server 1", "url": "", "ssh_key_data": "", "password": "" }
  ],
  "max_upload_size_mb": 3,
  "enable_intent_uri": false,
  "enable_termux_intent": false,
  "android_fullscreen": "fullscreen"
}
```

**Field reference**

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `force_pull_one_time` | bool | `false` | One-shot force-pull flag |
| `server_port` | int | `8080` | Listen port; applied at next start |
| `admin_password` | string | `admin_secret_changeme` | Grants `session_role=admin` |
| `guest_password` | string | `guest_secret_changeme` | Grants `session_role=guest` |
| `author` | string | `Anonymous` | `Author:` line on newly created pages, git commit author |
| `use_internal_editor` | bool | `true` | `false` routes `?edit=true` to `/api/edit-external` |
| `desktop_ext_cmd` | string | `subl` | External editor command line |
| `theme` | string | `auto` | `auto` \| `light` \| `dark`; anything else normalizes to `auto` |
| `share_lan` | bool | `false` | `true` binds `0.0.0.0`; applied at next start |
| `hostname` | string | OS hostname | Device label embedded in backup filenames; sanitized to `[A-Za-z0-9_-]`, max 64 chars |
| `backup_prune_depth` | int | `3` | Backups kept per database |
| `mime_types` | object | see below | Extension → content-type overrides, highest precedence |
| `active_git_index` | int | `0` | Index into `git_servers` |
| `git_servers` | array | 5 empty slots | Always padded to 5 entries |
| `max_upload_size_mb` | int | `3` | Upload size cap |
| `enable_intent_uri` | bool | `false` | Android only — allow `intent:` URIs from the WebView |
| `enable_termux_intent` | bool | `false` | Android only — allow Termux `RUN_COMMAND`; requires `enable_intent_uri` too |
| `android_fullscreen` | string | `fullscreen` | `off` \| `fullscreen` \| `immersive` |

`MainActivity` reads `enable_intent_uri`, `enable_termux_intent`,
`android_fullscreen` and `max_upload_size_mb` natively from `config.json`,
not through this API.

#### `POST /api/config`

Update the configuration and save it to `config.json`.
Content type: `application/x-www-form-urlencoded` (or query string).

**Parameters** — every field is optional. An absent field leaves the stored
value unchanged. The exceptions are the checkbox and select fields below,
where an absent field has a meaning.

| Name | Type | Applied when | Notes |
| --- | --- | --- | --- |
| `server_port` | int | parses `> 0` | Otherwise ignored |
| `admin_password` | string | always | Written verbatim, including empty |
| `guest_password` | string | always | |
| `author` | string | always | |
| `use_internal_editor` | `"true"` | always | Any other value (incl. absent) → `false` |
| `desktop_ext_cmd` | string | always | |
| `max_upload_size_mb` | int | non-empty and `> 0` | |
| `theme` | string | always | Through `normalizeTheme`; unknown → `auto` |
| `share_lan` | `"true"` | always | Absent → `false`. **Flipping this changes the response body** |
| `enable_intent_uri` | `"true"` | always | Absent → `false` |
| `enable_termux_intent` | `"true"` | always | Absent → `false` |
| `android_fullscreen` | string | always | Through `normalizeFullscreen`; unknown → `fullscreen` |
| `hostname` | string | always | Sanitized; empty resets to the OS-derived default |
| `backup_prune_depth` | int | parses `> 0` | |
| `active_git_index` | int | in range `[0, len(git_servers))` | |
| `git_name_<i>` | string | *(see below)* | `i` in `0..4` |
| `git_url_<i>` | string | | |
| `git_key_<i>` | string | | SSH private key text |
| `git_pass_<i>` | string | | |

The endpoint rewrites git server slot `i` **only if at least one** of
`git_name_<i>`, `git_url_<i>`, `git_key_<i>` or `git_pass_<i>` is non-empty.
It then replaces all four fields of the slot with the submitted values. You
can therefore clear one field, but you cannot clear all four at the same
time.

**Responses**

| Status | Body | Meaning |
| --- | --- | --- |
| `200` | `Saved` | Written to `config.json` |
| `200` | `RestartRequired` | Written, but `share_lan` changed — the frontend reacts to this exact string by calling `/api/restart` |
| `405` | `Method Not Allowed` | Any method other than GET/POST |
| `500` | `Failed to save configuration` | Marshal or write failure |

---

#### `POST /api/restart`

Restart the whole process. The process then builds its start-up state again
from the saved configuration. The listen address is the main part of that
state.

No parameters.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `Restarting` — written *before* the restart, so the caller actually receives it |
| `405` | `Method Not Allowed` |

The restart itself happens about 500 ms later on a background goroutine:

* **Android** — `os.Exit(0)`. `ServerService` is `START_STICKY`, so the
  system creates it again. The user interface closes, and the user opens the
  Android application again.
* **Desktop** — the process starts a new copy of the executable with
  `OMN_GO_RESTARTED=1` in the environment, then exits. If that start fails,
  the current process continues to run, because a working old instance is
  better than none.

The client must expect the connection to drop.

---

### 4.6 External editor

#### `GET /api/edit-external`

Open a file in the external editor of the platform. When
`use_internal_editor` is `false`, `/<path>?edit=true` redirects here.

**Parameters** (query string)

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | yes | Page name or asset path; resolved by `resolvePageName` |

**Responses**

| Platform | Status | Body / headers |
| --- | --- | --- |
| Android | `303 See Other` | `Location: omngo://edit?name=<name>` — the name is normalized to `<base>.md` for a real page, left untouched for a plain asset, so `MainActivity` opens the markdown source rather than the compiled HTML |
| Desktop | `200`, `text/html` | A full "editing externally" wait page pointing back at the view URL (`<base>.html` for a page, the raw name otherwise) |
| any | `400` | `Missing name` |

If `desktop_ext_cmd` is set, it is the editor command. The server splits it
on whitespace and adds the file path as the last argument. If it is not set,
the command is `xdg-open` on Linux, `open` on macOS, and
`rundll32 url.dll,FileProtocolHandler` on Windows. The server only logs a
failure to start the editor. It still returns the wait page with `200`.

---

### 4.7 Server log stream

#### `GET /api/logs`

Server-Sent Events stream of everything the backend writes through the
standard `log` package. The progress overlay of the frontend
(`omn-go-sse.js`) reads this stream, so the sync progress shows the real
stages of the backend.

No parameters. **No authentication.** Any client that can reach the port can
read every log line that the server writes.

**Response headers**

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

**Event format** — unnamed events, one `data:` field per log write:

```
data: 2026/07/27 14:05:00 [sync] fetching from origin

data: 2026/07/27 14:05:01 [sync] fast-forward complete

```

Each subscriber gets a buffered channel with 10 slots. When that channel is
full, the server **drops** further messages for that subscriber and never
blocks. The stream ends when the client disconnects (`r.Context().Done()`).
If the `ResponseWriter` does not implement `http.Flusher`, the endpoint
returns an empty `200` at once.

```js
const es = new EventSource('/api/logs');
es.onmessage = e => console.log(e.data);
```

---

### 4.8 SQLite

#### `POST /api/sql`

Run one atomic batch of SQL statements against one named SQLite database on
the server. The server stores the database at `<storage>/db/<name>.sqlite`.
This endpoint replaces the removed WebSQL API. The wrapper in the browser is
`omnGoOpenDatabase()` in `omn-go-core.js`.

**Headers**: `Content-Type: application/json`

**Request body**

```json
{
  "db": "mydata",
  "statements": [
    { "sql": "CREATE TABLE IF NOT EXISTS t(a,b)", "args": [] },
    { "sql": "INSERT INTO t VALUES(?,?)", "args": [1, "x"] },
    { "sql": "SELECT * FROM t WHERE a > ?", "args": [0] }
  ]
}
```

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `db` | string | yes | `^[A-Za-z0-9_-]{1,64}$` — used as a filename, this is the traversal guard |
| `statements` | array | yes | 1 … 500 entries |
| `statements[].sql` | string | yes | Arbitrary SQL |
| `statements[].args` | array | no | Positional `?` bindings; JSON scalars |

`http.MaxBytesReader` caps the whole request body at **1 MB**.

**Execution semantics**

* All statements run inside **one transaction**. Any failure rolls the whole
  batch back. This gives the `batch()` function of the JavaScript shim its
  atomicity.
* The endpoint chooses `Query` or `Exec` for each statement from the first
  keyword. `SELECT`, `WITH`, `PRAGMA`, `EXPLAIN` and `VALUES` return rows.
  Every other statement returns counters.
* The endpoint returns a `[]byte` column value as a string, not as base64.
* The endpoint repairs a stale-file-handle error
  (`SQLITE_READONLY_DBMOVED`, code 1032) **once**. It removes the cached
  handle, opens the database again, and runs the whole batch again from the
  start. A git pull that replaces the file under the open handle causes this
  error.
* If a database has backups but **no** `.sqlite` file, an open of that
  database starts the one automatic restore in OMN-Go (see
  [§4.8](#48-database-backups)).

**Success response** — `200`, `application/json`:

```json
{
  "status": "success",
  "results": [
    { "rows_affected": 0, "last_insert_id": 0 },
    { "rows_affected": 1, "last_insert_id": 7 },
    { "columns": ["a", "b"], "rows": [[1, "x"]],
      "rows_affected": 0, "last_insert_id": 0 }
  ]
}
```

`results` has the same index order as the statements that ran. A statement
that returns no rows has no `columns` field and no `rows` field.

**Error response**

```json
{
  "status": "error",
  "message": "no such table: t",
  "failed_statement": 1
}
```

| Status | `message` | Cause |
| --- | --- | --- |
| `400` | `bad request: <json error>` | Body is not valid JSON, or exceeds 1 MB |
| `400` | `no statements` | Empty `statements` |
| `400` | `too many statements (501 > 500)` | Over the batch limit |
| `400` | `invalid database name "…"` | `db` fails the whitelist |
| `400` | *driver message* | A statement failed; `failed_statement` holds its 0-based index |
| `405` | `POST only` | Wrong method |

---

### 4.9 Database backups

A backup is a **JSON Lines** (`.jsonl`) copy of one user database. The
server stores it at `html/db_backup/<db>/<timestamp>_<hostname>.jsonl`. This
path is under `html/`, so a backup travels through git sync like any other
file.

Filename grammar (also the traversal guard for `file`):

```
^[0-9]{8}T[0-9]{6}Z(_[0-9]+)?_[A-Za-z0-9_-]{1,64}\.jsonl$
```

An example is `20260727T140500Z_pixel7.jsonl`. Lexicographic order is the
same as chronological order, so the server sorts a listing newest-first.

Each file starts with a header line. After that line there is one line for
each schema object and one line for each row:

```json
{"format":"omngo-db-backup","version":1,"database":"mydata",
 "created":"2026-07-27T14:05:00Z","hostname":"pixel7","objects":3,"rows":42}
```

Backups are **manual**. There is exactly one automatic case.
`bootstrapIfMissing` restores the newest backup when a database has backups
but no `.sqlite` file at all. This is a new device directly after a pull. In
that state there is no local data that the restore could destroy.

#### `POST /api/db/backup`

**Parameters** (query string)

| Name | Type | Required | Constraints |
| --- | --- | --- | --- |
| `db` | string | yes | `^[A-Za-z0-9_-]{1,64}$` |

Creates a backup and deletes the older backups beyond `backup_prune_depth`.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `{"status":"success","file":"20260727T140500Z_pixel7.jsonl","pruned":["html/db_backup/mydata/20260101T…jsonl"]}` |
| `400` | `{"status":"error","message":"invalid db name \"…\""}` |
| `405` | `{"status":"error","message":"POST only"}` |
| `500` | `{"status":"error","message":"<reason>"}` |

`pruned` holds the paths of the removed files, relative to the storage
directory.

#### `GET /api/db/backups`

One read-only call that returns everything the Database Backups page needs.
The endpoint never opens a database, because an open would start the
bootstrap restore.

No parameters.

**Response** `200`, `application/json`:

```json
{
  "status": "success",
  "hostname": "pixel7",
  "prune_depth": 3,
  "databases": [
    {
      "name": "mydata",
      "sqlite_exists": true,
      "size": 20480,
      "mtime": "2026-07-27T14:05:00Z",
      "state": "insync",
      "backups": [
        {
          "file": "20260727T140500Z_pixel7.jsonl",
          "size": 8123,
          "mtime": "2026-07-27T14:05:00Z",
          "created": "2026-07-27T14:05:00Z",
          "hostname": "pixel7",
          "objects": 3,
          "rows": 42,
          "valid": true
        }
      ]
    }
  ]
}
```

`databases` is the union of the names that have a `.sqlite` file and the
names that only have a backup directory. The server sorts that union
alphabetically. It sorts `backups` newest-first. `backups` is always an
array, never `null`.

**`state` values**

| Value | Meaning |
| --- | --- |
| `none` | No backups at all |
| `invalid` | Newest backup has an unreadable or mismatched header |
| `missing` | Backups exist but there is no `.sqlite` file |
| `backup_newer` | Newest backup is newer than the `.sqlite` file |
| `dirty` | `.sqlite` file is newer than the newest backup |
| `insync` | mtimes are equal |

A backup entry with `"valid": false` carries `"error": "<reason>"`.

| Status | Body |
| --- | --- |
| `405` | `{"status":"error","message":"GET only"}` |

#### `POST /api/db/restore`

**Parameters** (query string)

| Name | Type | Required | Constraints |
| --- | --- | --- | --- |
| `db` | string | yes | `^[A-Za-z0-9_-]{1,64}$` |
| `file` | string | yes | Must match the backup filename grammar above |

Restores into `<storage>/db/<db>.sqlite` and destroys the current content.
`dbRestoreMu` serializes this endpoint against the bootstrap restore. The
endpoint removes the open handle, so the next `/api/sql` call opens the new
file. It sets the mtime of the restored `.sqlite` file to the mtime of the
backup. The state dot on the page therefore reads `insync` at once.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `{"status":"success"}` |
| `400` | `{"status":"error","message":"invalid db name \"…\""}` or `invalid backup filename "…"` |
| `405` | `{"status":"error","message":"POST only"}` |
| `500` | `{"status":"error","message":"<reason>"}` |

---

### 4.10 Git synchronization

#### `POST /api/sync`

Run one git action against the active remote (`git_servers[active_git_index]`).
`GitMutex` serializes every change to the repository.

**Parameters** — the endpoint reads them with `r.FormValue`, so a POST body
**or** a query string works. The frontend uses both.

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `action` | string | no | `pull` | See table below |
| `force` | `"true"` | no | — | Promotes `pull*`→`pull_force` and `push*`→`push_force` |
| `message` | string | no | — | Commit message; used by `push`/`push_force` only |

**Actions**

| `action` | Aliases | Behavior |
| --- | --- | --- |
| `pull` | `pull_ff`, `download` | Fast-forward if possible; `conflict` if history diverged or local changes are in the way |
| `pull_mark` | — | After a `pull` conflict: write 3-way conflict markers for manual resolution |
| `pull_abort` | `abort` | After a `pull` conflict: discard it, restore local state |
| `pull_force` | `download_force` | Reset local to exactly match remote; also deletes files that are neither tracked nor gitignored |
| `push` | `upload` | Commit local changes (requires `message`) then push; stops on conflict |
| `push_force` | `upload_force` | Commit if needed, then force-push, resetting the remote |

**Responses** — always `200 OK` with `application/json`. The outcome is in
`status`.

| `status` | `message` | Extra |
| --- | --- | --- |
| `success` | `""` | |
| `conflict` | `Fast-forward not possible. Choose abort or 3-way merge.` | `files`: array of conflicting paths (always an array, never `null`) |
| `push_conflict` | `Remote has new commits. Pull before pushing.` | |
| `needs_commit_message` | `Please provide a commit message.` | |
| `error` | The raw error text (bad request, unknown action, SSH/auth failure, …) | |

```json
{ "status": "conflict",
  "message": "Fast-forward not possible. Choose abort or 3-way merge.",
  "files": ["md/Welcome.md", "md/notes/Trip.md"] }
```

```bash
curl -X POST http://127.0.0.1:8080/api/sync \
     -d 'action=push' -d 'message=notes from phone'
```

#### `GET /api/sync/preview`

Dry run of an upload. It reports what there is to commit, **and** whether
there is anything to push. It commits nothing.

**Parameters** (query string)

| Name | Type | Required | Value |
| --- | --- | --- | --- |
| `action` | string | yes | Must be `upload` — nothing else is supported |

The endpoint takes the files from `git status`. It removes anything that
`.gitignore` matches, and it removes the root `config.json`. The
`config.json` file is never synced, because it holds the secrets of one
device. Database backups need no special handling. They are ordinary files
under `html/db_backup/`, and the same scan finds them.

**Response** `200`

```json
{ "files": [], "unpushed": true, "remote": "slot1", "verified": true }
```

| Field | Meaning |
| --- | --- |
| `files` | Storage-relative paths that would be committed. Always an array, never `null` |
| `unpushed` | Local HEAD is, or may be, ahead of the active remote — see below |
| `remote` | The active remote the answer is about |
| `verified` | The remote itself was contacted, not just the local remote-tracking ref. Only the local-view-says-level path goes to the network, so this is absent whenever the local refs alone settled the answer |
| `remote_error` | Why the remote could not be reached, when it could not |

**`unpushed` is the reason this endpoint is more than a file list.** An empty
`files` does not mean that there is nothing to upload. A commit with a failed
push leaves commits that the active remote has never seen. A change of git
server slot after a successful push does the same. A client that reads
`files.length === 0` as "nothing to do" strands those commits.

The server computes `unpushed` from local data first, and it prefers `true`
when it is not sure:

1. No local HEAD → `false`.
2. `refs/remotes/<active>/master` missing, or different from HEAD → `true`.
   Each git server slot owns its own named remote. A slot that the server has
   never contacted has no such ref. This is the case of a changed slot.
3. In every other case the local view says that HEAD is level with the
   remote. This is the one answer that would stop a client from a push. The
   server therefore confirms it against the remote with a refs listing
   (`ls-remote`) before it reports the answer. This listing downloads no
   objects. If the listing fails, the server sets `remote_error` and leaves
   `verified` absent.

The server computes `unpushed` only when `files` is empty. When there is
something to commit, the upload runs anyway, so the answer would change
nothing.

A wrong `true` costs one round trip, because `push` then reports
already-up-to-date and does no harm. A wrong `false` costs the user their
commits. The asymmetry is deliberate for this reason.

| Status | Content-Type | Body |
| --- | --- | --- |
| `400` | `text/plain` | `Only upload preview supported` |
| `405` | `text/plain` | `Method Not Allowed` |
| `500` | `text/plain` | `Repo init failed: …`, `Worktree error: …`, `Status error: …` |

---

### 4.11 Status

#### `GET /api/status`

Reports what this process does now. It gives the address that the server
listens on, the git commit of the notes, the size of the search index and
the Android package. The endpoint is admin only, because the answer
carries LAN addresses, absolute paths and a commit subject. The local
bypass applies, so the Android WebView and a desktop browser reach it with
no login.

Nothing in this endpoint opens a network connection. Nothing in it creates
or changes a file. The git section opens the repository read-only, so a
status request on an installation that never synchronized reports
`repo_exists: false` and creates no repository.

**Parameters**

| Parameter | Values | Default |
| --- | --- | --- |
| `sections` | `server`, `config`, `git`, `search`, `runtime`, `android`, `storage`, `git_dirty`, `all` | the six cheap sections |
| `format` | `json`, `md` | `json` |

Two sections cost real work, so they are never in the default answer:
`storage` walks the storage directory, and `git_dirty` walks the git
worktree. Ask for them by name. A page can therefore paint the cheap facts
at once and run a progress bar over the slow request alone. `git_dirty`
writes one `[status]` line to the log stream before it starts.

`sections=all` selects each section, the two slow ones included. An unknown
section name answers `400`.

`format=md` answers with the same facts as a Markdown document, with the
content type `text/plain; charset=utf-8`. A browser paints `text/plain`,
and the Android WebView paints nothing else.

**Sections**

| Section | Cost | Fields |
| --- | --- | --- |
| `server` | none | `app_version`, `started`, `uptime_s`, `bind_port`, `share_lan`, `lan_urls[]`, `active_conns`, `hostname`, `goos`, `goarch` |
| `config` | none | `internal_editor`, `theme`, `max_upload_mb`, `search_enabled`, `search_kinds`, `search_scope`, `search_bundled`, `intent_uri`, `termux_intent`, `android_fullscreen`, `backup_prune_depth`, `hostname`, `author` |
| `git` | one object read | `repo_exists`, `configured`, `branch`, `head{hash,short,subject,author,date}`, `remote{name,url}` |
| `search` | one pass over the index | `enabled`, `docs`, `lines`, `bytes`, `index_bytes_estimate`, `built`, `checked`, `dirty`, `kinds`, `scope` |
| `runtime` | none | `go_version`, `goroutines`, `heap_alloc`, `sys`, `assets_version` |
| `android` | none | `package`, `default_port`, `fullscreen` (the section is absent off Android) |
| `storage` | a walk of the storage directory | `dir` and one `{files,bytes}` group each for `notes`, `pages`, `images`, `user_json`, `databases`, `backups`, `asset_backups`, `total` |
| `git_dirty` | a walk of the worktree | `dirty`, `changed`, `untracked` |

`bind_port` is what the listener bound, not what the configuration asked for.
The two differ when the retry loop in `StartServer` ends on another port.
There is no listen address in the answer: the listener binds `::` or
`0.0.0.0`, and `[::]:8080` answers no question that a person asks.

`lan_urls` is empty when `share_lan` is off. With sharing on it holds the
address of the default route first, then the addresses that the Android layer
sent, then each other interface address in sorted order. Each address appears
one time.

The address of the default route comes from a UDP dial that sends no packet.
Android needs that: `net.InterfaceAddrs` asks the kernel over `NETLINK_ROUTE`,
and Android denies that socket to an application since Android 11, so the list
was empty on a phone.

`ServerService.java` also hands its own list to the backend through
`SetLANAddresses`, a comma-separated string. Java reads the addresses through
`getifaddrs()`, which an application may call, and it is the same list that
the LAN notification shows. The notification and this endpoint therefore
cannot name different addresses.

`index_bytes_estimate` is an estimate, and the name says so. Go cannot
report the true size of a live object graph. The number counts the line
masks, the trigram signature and the strings of each indexed document,
with a flat allowance for the structure.

The answer never carries a password, an SSH key, or a remote URL with a
password in it. The user name of a remote URL stays: it is part of the
address, and it is not a secret.

**Responses**

| Status | Body |
| --- | --- |
| `200` | the status document, as JSON or as Markdown |
| `400` | `unknown section: <name>` |
| `401` | `Unauthorized` |
| `405` | `GET only` |

A section that fails does not fail the answer. The section is absent, and
`errors` names it with the reason.

**Example**

```
GET /api/status?sections=server,git&format=json
```

```json
{
  "generated": "2026-08-04T10:00:00Z",
  "server": {
    "app_version": "26.08.14",
    "started": "2026-08-04T09:30:00Z",
    "uptime_s": 1800,
    "bind_port": 8080,
    "share_lan": true,
    "lan_urls": ["http://192.168.1.5:8080"],
    "active_conns": 1,
    "hostname": "pixel7",
    "goos": "android",
    "goarch": "arm64"
  },
  "git": {
    "repo_exists": true,
    "configured": true,
    "branch": "master",
    "head": {
      "hash": "9f1c0f1d2b3a4c5d6e7f8091a2b3c4d5e6f70819",
      "short": "9f1c0f1",
      "subject": "Add user-owned omn-go-custom.css and omn-go-custom.js",
      "author": "Mikhail Basov",
      "date": "2026-08-04T08:12:00Z"
    },
    "remote": { "name": "home", "url": "https://user@example.com/notes.git" }
  }
}
```

---

## 5. Page and asset routes

These are not JSON endpoints, but they are part of the URL surface of the
server.

### 5.1 Markdown pages

| URL | Behavior |
| --- | --- |
| `/` , `/index.html` | `303 See Other` → `/Welcome.html` |
| `/<name>.html` | Rendered page. If the `.html` cache is missing or older than `md/<name>.md`, it is recompiled first. A missing `.md` is created from the embedded default or a stub — **a page URL does not 404** |
| `/<dir>/<name>.html` | Same, for pages in subdirectories |

**Query parameters accepted on any page URL**

| Name | Values | Effect |
| --- | --- | --- |
| `edit` | `true` | Opens the editor. With `use_internal_editor` → the standalone editor page (which then calls `/api/note` and `/api/save`); otherwise `303` → `/api/edit-external?name=<path>` |
| `refresh` | `1`, `true` | Force recompilation of the HTML cache from markdown |

**Special pages**

| URL | Served by | Notes |
| --- | --- | --- |
| `/Config.html` | `serveConfigPage` | Rendered server-side; posts to `/api/config` |
| `/OMNGoTags.html` | `serveTagsPage` | Auto-generated tag index; staleness is checked against the newest mtime of **all** notes, not one source. Honors `?refresh` |
| `/db_backups` | `serveDBBackupsPage` | Admin page. All data comes from `GET /api/db/backups`. **Admin-only**, unlike other pages |
| `/OMNGoFiles.html` | `serveFilesPage` | The file index of the embedded and on-disk file trees. **Admin-only**. See §5.3 |

`injectRuntimeVars` adds this block to every served page:

```html
<script>var APP_VERSION = "1.11.32"; var USE_INTERNAL_ED = true; var OMN_THEME = "auto";
document.documentElement.setAttribute('data-theme', OMN_THEME);</script>
```

### 5.2 Static assets

| Prefix | Source | Extraction |
| --- | --- | --- |
| `/js/`, `/css/`, `/json/` | `html/js`, `html/css`, `html/json` | Lazily extracted from the embedded frontend on first request, then user-editable via `?edit=true` (text files only: a picture, a font, an audio file or a video file answers `415`) |
| root catch-all (`/favicon.ico`, `/robots.txt`, …) | `html/` | Same lazy extraction |
| `/images/` | `html/images/` | Pure user content, never embedded |
| `/user_json/` | `html/user_json/` | Pure user content, never embedded |

**Content-type resolution** (`resolveContentType`), highest precedence
first:

1. `config.json` → `mime_types[ext]`
2. the built-in table: `.html .css .js .mjs .json .jsonl .md .svg .png
   .jpg .jpeg .gif .webp .ico .woff .woff2 .ttf`
3. Go's `mime.TypeByExtension`
4. nothing set → `net/http` sniffs the content

`.jsonl` resolves to `text/plain; charset=utf-8`. A database backup is JSON
Lines, not one JSON document, and this type shows the file in the browser and
in the Android WebView. `application/json` would send a browser JSON viewer to
a parse error, and `application/jsonl` would start a download that the WebView
cannot do.

### 5.3 The file index

#### `GET /OMNGoFiles.html`

The file index lists everything that OMN-Go can serve, in the shape that a
browser uses for `file:///`. It shows **one directory at a time**, with a
breadcrumb up and links down. Each directory has two sections, because a file
can exist in one section, in the other, or in both, and the difference matters:

| Section | Source | What a row means |
| --- | --- | --- |
| Embedded in the application | `staticFS` (`frontend/html`, `frontend/md`) | What this build ships. A row says whether the file has been extracted to disk yet, and whether it is app-owned |
| On this device | `StorageDir/html` | What is actually on the device, with its size and modification time |

**Parameters**

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `dir` | string | no | root | Directory to show, relative to `html/`, e.g. `js/`, `Test/OMN-Go/` |
| `all` | `1`, `true` | no | — | Show every file in the directory instead of the first 200 |

`normalizeFilesDir` normalizes `dir`. It resolves `..` segments, and a result
that would climb above the root collapses to the root. The page uses `dir` only
as a **string prefix over the paths already collected** from the two roots. It
never joins `dir` onto a filesystem path. A value that names nothing therefore
matches nothing and renders an empty directory with a working breadcrumb.

**Authorization.** The page is admin-only, with the usual local connection
bypass. The server registers it as its own exact route, and does not dispatch
it from `serveHTMLPage`. The reason is that the catch-all that serves every
other page needs no authentication. `/db_backups` is a separate route for the
same reason.

The page does **not** wrap `authMiddleware`. A guest gets a **200** and a page
that says the listing is admin-only and how to log in. A guest does not get
the one plain-text line of the middleware. This is the same lesson as the 404
of the search page (26.08.2). The address is linkable, so a refusal must name
the cause and the cure. No filename appears anywhere in the refusal.

**What is never listed**

* `frontend/templates` — this directory lives in `templatesFS`, a separate
  embed. It cannot appear for that reason, not because of an exclusion rule.
* `db_backup/` — the page excludes the database backups by name. This is a
  listing decision and not an access control. Anyone who can reach the server
  can still fetch the files, exactly as before this page existed.

**Row extras**

* An **edit** link appears only where an edit in place makes sense. The page
  decides this from the resolved content type and shows the link for text
  content (`.js`, `.json`, `.css`, `.md`, …). There is no edit link on an
  image, a font, audio or video. There is also no edit link on an `.html`
  file. To edit a page, open it and press the Edit button of that page.
* An embedded row carries `on disk` or `not yet`, and `app-owned` or `yours`.
  `app-owned` means that the file is in `versionDependentAssets`. At the next
  change of `APP_VERSION`, `refreshEmbeddedAssets` creates a backup of your
  copy and replaces it. The server extracts a `yours` file once and then
  leaves it alone.

Directory totals are **recursive**. The file count and size in a directory row
cover the whole subtree. The number therefore answers "how big is this" and not
"how many rows are directly inside".

**The 200-file cap** applies to files only. The page never caps directory rows,
so navigation always works. When the page trims the list, it says how many
files it held back and offers `all=1` with the true total. The page is dynamic
like `Config` and `OMNGoSearch`. There is no `md/` source, the server writes
nothing to the `html/` cache, and the page itself writes nothing at all.

---

### 5.4 The Status page

#### `GET /OMNGoStatus.html`

The page of the status endpoint. It holds no facts of its own. It reads
`/api/status` and draws each section that comes back. Admin only, and the
handler asks `hasRole` itself, so a guest gets a page and not a line of plain
text.

The page loads the cheap sections at once. The *Storage* and *Git worktree*
buttons ask for one slow section each, and the shared progress overlay runs
while the request is open. *Copy as Markdown* asks for `format=md` and puts
the answer on the clipboard with `select` and `execCommand`, because
`navigator.clipboard` is unusable in the Android WebView.

The Config menu carries the link to this page, after the buttons of the
settings screens.

---

## 6. 404 handling

Every miss goes through `serveNotFound`. This function **negotiates the
content type on the `Accept` header**:

* `Accept` contains `text/html` (a browser navigation) → the full themed
  404 page, `text/html; charset=utf-8`.
* anything else (`fetch`, XHR, `<img>`, `<script>` — `Accept: */*`) →
  `text/plain; charset=utf-8`:

```
404 Not Found

Requested: /js/missing.js?x=1
Method:    GET
Time:      2026-07-27 14:05:00
Linked from: /Welcome.html
Did you mean: /Notes.html
```

`Linked from` appears only when the `Referer` points at this same server and
contains no `..`. The server echoes the path of the `Referer` and nothing
else. `Did you mean` appears when the failing path has no extension and a
note with that name exists. This is the `[text](name)` mistake in place of
`[text](name.html)`.

Every miss also writes one log line, which is visible on the `/api/logs`
stream:

```
[404] GET /js/missing.js (referer "/Welcome.html")
```

---

## 7. Status codes in use

| Code | Where |
| --- | --- |
| `200 OK` | Success on every endpoint; also the JSON error responses of `/api/sync` |
| `303 See Other` | `/` → `/Welcome.html`; `?edit=true` → `/api/edit-external`; Android `omngo://edit` handoff |
| `415 Unsupported Media Type` | `?edit=true`, `/api/edit-external`, `/api/note` or `/api/save` on a file that is not text (picture, font, audio, video) |
| `400 Bad Request` | `/api/status` with an unknown `sections` name |
| `400 Bad Request` | Missing/invalid parameters, rejected uploads, SQL errors |
| `401 Unauthorized` | `/login` with a wrong password; `authMiddleware` for a remote caller without an admin cookie |
| `404 Not Found` | Missing static asset or `/api/note` for a missing non-page file |
| `405 Method Not Allowed` | `/api/config`, `/api/restart`, `/api/sql`, `/api/db/*`, `/api/sync/preview` |
| `500 Internal Server Error` | Disk/permission failures, git repo errors, restore failures |

---

## 8. Notes for API clients

* **Do not expose this server to an untrusted network.** `share_lan` and the
  admin password are the only barrier. There is no TLS. `GET /api/config`
  returns every stored secret in cleartext. `POST /api/sql` runs arbitrary
  SQL. `/api/note` and `/api/logs` need no authentication at all.
* From `127.0.0.1` no authentication is needed. A script on the same device
  can drive the whole API directly.
* The status code does not always signal success. `/api/sync` returns `200`
  with `{"status":"error"}`. `/api/quick` returns `200` with an empty body
  when it wrote nothing.
* A plain-text endpoint returns a bare word with no trailing newline
  (`Saved`, `OK`, `Restarting`). Trim the response before you compare it.
* `/api/newpage` returns the *resolved* target, which can differ from the
  submitted target. Build the follow-up URL from the response, not from the
  request.
* Two endpoints return content for a verbatim splice into a note, with the
  newlines: `/api/upload` and `/api/upload_json`.
* `/api/sync/preview` returned a bare JSON array before, and `null` in place
  of `[]` when nothing had changed. It now returns an object whose `files`
  field is always an array. See §4 for the reason for the extra fields.
