# OMN-Go Server API

Reference for every HTTP endpoint exposed by the Go backend
(`backend/server.go`, `backend/logger.go`).

Applies to OMN-Go **26.07.45** (`backend/version.go`, `APP_VERSION`).

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

The listening socket is bound **once at process start**. Changing
`server_port` or `share_lan` requires a restart (see
[`POST /api/restart`](#412-post-apirestart)); with sharing off no remote
device can even complete a TCP handshake, regardless of any auth logic.

If the port is busy the server retries `net.Listen` ten times at 300 ms
intervals (~3 s) before giving up — this covers the window during a
self-restart where the replacement process races the old socket teardown.

### 1.2 Routing model

Routing uses the Go standard library `http.ServeMux`. Consequences worth
knowing:

* Paths registered **without** a trailing slash (`/api/note`, `/login`, …)
  match that exact path only.
* Paths registered **with** a trailing slash (`/js/`, `/css/`, `/json/`,
  `/images/`, `/user_json/`) match the whole subtree.
* `/` is the catch-all: everything not matched above lands in
  `serveFrontend`.
* **`ServeMux` does not dispatch on method.** Unless an endpoint explicitly
  checks `r.Method` (the table in §3 says which do), any method reaches the
  handler. `r.FormValue` reads the URL query string *and* an
  `application/x-www-form-urlencoded` / `multipart/form-data` body, so most
  form-style endpoints accept parameters either way.

### 1.3 Request encodings

| Encoding | Used by |
| --- | --- |
| URL query string | `/api/note`, `/api/edit-external`, `/api/sync/preview`, `/api/db/*`, page-level `?edit`/`?refresh` |
| `application/x-www-form-urlencoded` | `/login`, `/api/save`, `/api/newpage`, `/api/quick`, `/api/bookmark`, `/api/config`, `/api/sync` |
| `multipart/form-data` | `/api/upload`, `/api/upload_json` |
| `application/json` | `/api/sql` |

### 1.4 Response encodings

There is no single envelope — three shapes are in use:

1. **Plain text** — short status words (`Saved`, `OK`, `Restarting`) or a
   fragment to splice into a note. Returned by the legacy note/upload
   endpoints.
2. **JSON** — `/api/config` (GET), `/api/sql`, `/api/db/*`, `/api/sync`,
   `/api/sync/preview`.
3. **`text/event-stream`** — `/api/logs` only.

Errors from `http.Error` are `text/plain; charset=utf-8` with the message
in the body. Errors from the JSON endpoints keep their JSON shape and carry
`"status": "error"`.

---

## 2. Authentication and authorization

### 2.1 Model

`authMiddleware` (`backend/middleware.go`) wraps the protected endpoints:

1. **Local connections bypass auth entirely.** If the peer address is
   `127.0.0.1`, `::1` or `localhost`, the handler runs with no further
   checks. This is what lets the app's own WebView / desktop browser work
   without ever logging in.
2. Otherwise a `session_role` cookie must be present and hold an accepted
   role. Every currently protected route is registered with
   `requireAdmin = true`, so remote callers need `session_role=admin`.
   The `guest` role is accepted by the middleware only for routes
   registered with `requireAdmin = false` — none exist today.
3. A missing or insufficient cookie yields `401 Unauthorized` with the body
   `Unauthorized`.

There is no CSRF token, no bearer token, and no rate limiting. Passwords
are stored in `config.json` in cleartext and compared with `==`.

### 2.2 Obtaining the cookie

`POST /login` with the correct password sets:

```
Set-Cookie: session_role=admin; Path=/
```

It is a session cookie (no `Max-Age`/`Expires`), not `HttpOnly`, not
`Secure`, and has no `SameSite` attribute.

### 2.3 Protection map

| Endpoint | Auth |
| --- | --- |
| `POST /login` | none (it is the login) |
| `GET /api/note` | **none — deliberately open** |
| `GET /api/search` | **none — deliberately open** |
| `GET /api/logs` | **none** |
| `/api/quick`, `/api/bookmark`, `/api/upload`, `/api/upload_json`, `/api/save`, `/api/newpage`, `/api/config`, `/api/restart`, `/api/sql`, `/api/db/backup`, `/api/db/backups`, `/api/db/restore`, `/api/sync`, `/api/sync/preview`, `/api/edit-external`, `/db_backups` | admin (local bypass applies) |
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
| GET | `/db_backups` | no | admin | HTML |
| GET | `/OMNGoSearch.html` | no | none | HTML (404 when global search is off) |
| GET | `/`, `/<name>.html`, `/<asset>` | no | none | HTML / asset |
| GET | `/js/…`, `/css/…`, `/json/…` | no | none | asset |
| GET | `/images/…`, `/user_json/…` | no | none | asset |

---

## 4. Endpoint reference

### 4.1 Authentication

#### `POST /login`

Exchange a password for a role cookie. Only needed by remote (non-loopback)
clients.

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

Return the **source text** of a note or of an arbitrary static asset. This
is what the internal editor fetches on load.

**Parameters** (query string)

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string | no | `Welcome` | Page name or asset path |

`name` is resolved by `resolvePageName` (`backend/paths.go`):

| Shape of `name` | Treated as | Source read |
| --- | --- | --- |
| `Welcome` (no dot) | markdown page | `md/Welcome.md` |
| `Welcome.md` | markdown page | `md/Welcome.md` |
| `Welcome.html` | markdown page | `md/Welcome.md` |
| `notes/Trip` | markdown page in a subdirectory | `md/notes/Trip.md` |
| `js/omn-go-core.js`, `css/x.css`, any other extension | static asset | `html/js/omn-go-core.js` |

**Behaviour when a page does not exist yet**

The endpoint **never 404s for a markdown page**. It falls back, in order,
to the embedded default (`frontend/md/<name>.md`) or to a synthesized
front-matter stub:

```
Title: <name>
Date: 2026-07-27 14:05:00
Category: Notes
Author: <config.author, omitted when empty>

```

The fallback is written to disk before being returned, so it only ever
happens once per page.

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

* `\r\n` is normalized to `\n`.
* For a markdown page, `ensureHeaderModified` stamps/updates
  `Modified: YYYY-MM-DD HH:MM:SS` in the front matter.
* The markdown source is written **first**; only then is the compiled HTML
  cache (`html/<name>.html`) regenerated by `renderAndCache`. A cache
  failure is logged but still reports success — the next page view
  recompiles it.
* For a non-page asset the bytes are written straight to
  `html/<path>`, no rendering.

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

Create a new note **and** insert a link to it into the note it was created
from.

**Parameters** (form body or query string)

| Name | Type | Required | Description |
| --- | --- | --- | --- |
| `target` | string | yes | Name of the page to create, without `.md` |
| `title` | string | yes | Title written into the new page's front matter, and the link text |
| `source` | string | no | Page the link is inserted into |

**Target resolution** (`resolveNewPageTarget`) mirrors how a bare relative
link on `source` would resolve in the browser:

| `target` | `source` | Created as |
| --- | --- | --- |
| `test` | `local/local` | `local/test` |
| `test` | `Welcome` | `test` |
| `/test` | anything | `test` (storage root) |
| `sub/test` | anything | `sub/test` (storage root) |

The link written into `source` uses the **raw** target for the bare case
(so the browser resolves it relative to `source` the same way) and an
explicit leading `/` when the target carried its own directory.

An existing `target` file is never overwritten; only the link insertion
runs. New pages get:

```
Title: <title>
Date: <now>
Modified: <now>
Category: Notes
Author: <config.author, omitted when empty>

```

The link `* [<title>](<href>)` is inserted just below `source`'s front
matter (or prepended to a headerless note), `source` is re-stamped with
`Modified:`, and its HTML cache is recompiled immediately.

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

The entry is inserted immediately after the first blank line (the end of
the Pelican-style header) as:

```

---
##### 2026-07-27 14:05:00
<note>

```

The page title is re-stamped as `Quick Notes` and the HTML cache is
recompiled.

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

The record is inserted directly after the marker line
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

Values are JSON-encoded (`<`, `>`, `&` become `\u`-escapes), so a note can
never break out of the surrounding `<script>` block.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `Saved` — also returned when `Bookmarks.md` is missing or lacks the marker, in which case nothing was written |

---

### 4.3 Search

#### `GET /api/search`

Fuzzy search over the notes. Two scopes, one matcher: the response shape,
the scoring and the meaning of a result are identical either way — only the
haystack differs.

| Scope | Searches | Needs the index | Needs configuration |
| --- | --- | --- | --- |
| `page` | the one file named by `on` | no | **no — always available** |
| `all` | every indexed document | yes | yes (`search_enabled`) |

Page scope reads that single file per request and keeps nothing, which is why
it has no setting: there is no standing cost to opt out of. Global scope is
answered from the in-memory index (`backend/search_index.go`), which is built
only when `search_enabled` is on.

**Parameters** (query string)

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `q` | string | yes | — | The query. Empty is an empty result, not an error |
| `scope` | string | no | see below | `page` or `all` |
| `on` | string | no | — | Page name or asset path for `scope=page`, resolved by `resolvePageName` |
| `kind` | string | no | the configured kinds | Comma list of `md,bookmarks,js,json,user_json` — narrows, never widens |
| `limit` | int | no | `50` | Max results (hard cap `200`); `scope=page` returns at most one |
| `snippets` | int | no | `3` | Max snippet lines per result (hard cap `10`) |

The default `scope` is the configured `search_scope`, except that it falls back
to `page` whenever global search cannot answer — defaulting a caller who
expressed no preference into a scope that can only fail would be a strange
reading of silence. An **explicit** `scope=all` is still refused in that state,
because hiding it would misreport what the server did.

**Query syntax**

Terms are whitespace-separated and combined with **AND**: every term must match
somewhere in a document. A term may carry a field prefix — `title:`, `tag:`,
`path:` — or `kind:` to filter. An unknown prefix is not a prefix, so
`https://example.com` stays a search for that text.

Each term is matched by the first of three rungs that hits, and matches compare
by **(rung, score)** — never score alone, so a document containing the word
outranks one that merely suggests it:

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
| `url` | Where the result lives: `/<Name>.html` for a note, the served path for an asset. Always the plain URL — see `highlight` |
| `highlight` | The query's terms as typed, for the client to mark in whatever page it opens. Present on both scopes, and on a query that matched nothing |
| `matches[].line` | 1-based line in the file **as stored** — the markdown source for a note |
| `matches[].context` | `script` or `code` when the hit is inside a `<script>` or a fenced block; absent in prose |
| `matches[].text` | The snippet, whitespace-trimmed and windowed to ~160 runes around the first hit, with `…` markers |
| `matches[].spans` | `[start, length]` pairs in **rune** offsets into `text` |

`spans` are rune offsets, not byte offsets and not UTF-16 units: slicing
`text` by anything else cuts Cyrillic and emoji in half. In JavaScript,
`Array.from(text)` gives the right units.

`highlight` is not the same thing as `spans`. `spans` are offsets into a
snippet of the **source**; `highlight` is literal text to look for in the
**rendered** page, which is a different document — the source line
`**fetch** the json` renders as `fetch the json`, and no span from one survives
into the other. Field prefixes are stripped (`tag:hydro` → `hydro`), terms
shorter than 2 runes are dropped, and the text is **not** folded: the client
marks literal occurrences, and folding maps `ё` to `е`, so the folded form of a
term may appear nowhere on the page. A term that only matched fuzzily will not
be found and nothing is marked — which is the honest outcome.

**Files larger than 500 KiB** are searched up to that point, cut at a line
boundary, and the result carries `"truncated": true` — "found nothing in the
part I looked at" rather than silence.

#### `GET /OMNGoSearch.html`

The results page: the same search as `scope=all`, rendered as a shareable,
JavaScript-free page. Special-cased in `serveHTMLPage` beside `Config` and
`OMNGoTags`, and **dynamic like `Config`** - there is no `md/OMNGoSearch.md`,
nothing is written to the `html/` cache, and `?refresh` means nothing here.

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `q` | string | no | — | The query. Absent or empty renders just the form |

Every result on this page links to `/<Name>.html?hl=<term>&hl=<term>` — see
`?hl=` below.

Global only: with `search_enabled` off it answers **404** through
`serveNotFound`, because a page that could only ever be empty is worse than an
honest miss. Page search lives in the dialog.

#### `?hl=<term>` — highlight on arrival

Any page accepts repeated `hl` parameters. On load the client marks every
literal occurrence of those terms in `#preview`, scrolls to the first, and then
removes the parameters from the address bar with `history.replaceState` — so
the URL that gets copied, bookmarked or reloaded is the plain one, and a
refresh does not re-apply the highlight.

Repeated parameters rather than one comma-joined value: a term may itself
contain a comma. Terms shorter than 2 runes are ignored (`OMN_HL_MIN` in
`omn-go-core.js`, `highlightMinRunes` in `search.go` — the two ends agree).

Handled entirely in `omn-go-core.js`, so it works on a page opened from disk
with no server running, and on pages the search dialog never loads on.

**Errors**

| Status | Body | When |
| --- | --- | --- |
| `200` | normal response with `"results": []` | Nothing matched; `on` names something that does not exist; `q` is empty |
| `400` | `{"status":"error","error":"unknown scope …"}` | `scope` is neither `page` nor `all` |
| `503` | `{"status":"disabled","error":"global search is off …"}` | `scope=all` with `search_enabled` false — the user can act on this |
| `503` | `{"status":"unavailable","error":"the search index is not ready"}` | `scope=all`, enabled, but no index yet — the user cannot |

A miss is always `200` with no results. `on` pointing outside the storage
directory, or at a file that does not exist, is a miss — never a way to probe
the filesystem.

---

### 4.4 Uploads

Both upload endpoints share `saveUploadedFile`:

* the multipart form is parsed with a 10 MB in-memory threshold (this is
  *not* the size cap);
* the extension is checked case-insensitively against a whitelist;
* `header.Size` is checked against `max_upload_size_mb` (default **3 MB**);
* the file is written to its destination directory under its **original
  filename** — an existing file with the same name is overwritten.

#### `POST /api/upload`

**Body**: `multipart/form-data`

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `image` | file | yes | Allowed: `.png .jpg .jpeg .gif .webp .svg` |

Stored in `html/images/`, served from `/images/<filename>`.

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

Stored in `html/user_json/`, served from `/user_json/<filename>`.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `\n[<name>](/user_json/<name>)\n` |
| `400` / `500` | Same shapes as `/api/upload` |

---

### 4.5 Configuration

#### `GET /api/config`

Return the entire live configuration as JSON. **This includes
`admin_password`, `guest_password` and every git server's SSH private key
and password in cleartext.**

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

`enable_intent_uri`, `enable_termux_intent`, `android_fullscreen` and
`max_upload_size_mb` are read natively by `MainActivity` out of
`config.json`, not through this API.

#### `POST /api/config`

Update the configuration and persist it to `config.json`.
Content type: `application/x-www-form-urlencoded` (or query string).

**Parameters** — every field is optional; an absent field leaves the stored
value unchanged, *except* the checkbox and select fields noted below, where
"absent" is meaningful.

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

A git slot `i` is rewritten **only if at least one** of
`git_name_<i>` / `git_url_<i>` / `git_key_<i>` / `git_pass_<i>` is
non-empty; when that holds, all four fields of the slot are replaced with
the submitted values (so a single field can be cleared, but not all four at
once).

**Responses**

| Status | Body | Meaning |
| --- | --- | --- |
| `200` | `Saved` | Written to `config.json` |
| `200` | `RestartRequired` | Written, but `share_lan` changed — the frontend reacts to this exact string by calling `/api/restart` |
| `405` | `Method Not Allowed` | Any method other than GET/POST |
| `500` | `Failed to save configuration` | Marshal or write failure |

---

#### `POST /api/restart`

Restart the whole process so start-up-bound state (above all the listen
address) is rebuilt from the saved config.

No parameters.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `Restarting` — written *before* the restart, so the caller actually receives it |
| `405` | `Method Not Allowed` |

The actual restart happens ~500 ms later on a background goroutine:

* **Android** — `os.Exit(0)`. `ServerService` is `START_STICKY`, so the
  system recreates it; the UI closes and the user reopens the app.
* **Desktop** — spawns a fresh copy of the executable with
  `OMN_GO_RESTARTED=1` in the environment, then exits. If the spawn fails
  the current process keeps running (a working old instance beats none).

The client should expect the connection to drop.

---

### 4.6 External editor

#### `GET /api/edit-external`

Open a file in the platform's external editor. Reached by redirect from
`/<path>?edit=true` when `use_internal_editor` is `false`.

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

The editor command is `desktop_ext_cmd` if set (split on whitespace, the
file path appended as the last argument), otherwise `xdg-open` on Linux,
`open` on macOS, `rundll32 url.dll,FileProtocolHandler` on Windows. A
failure to launch is logged only — the wait page is still returned `200`.

---

### 4.7 Server log stream

#### `GET /api/logs`

Server-Sent Events stream of everything the Go backend writes through the
standard `log` package. Consumed by the frontend progress overlay
(`omn-go-sse.js`) so sync progress reflects real backend stages.

No parameters. **No authentication** — every log line the server emits is
readable by any client that can reach the port.

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

Each subscriber gets a 10-slot buffered channel; when it is full further
messages for that subscriber are **dropped**, never blocked. The stream
ends when the client disconnects (`r.Context().Done()`). If the
`ResponseWriter` does not implement `http.Flusher` the handler returns
immediately with an empty `200`.

```js
const es = new EventSource('/api/logs');
es.onmessage = e => console.log(e.data);
```

---

### 4.8 SQLite

#### `POST /api/sql`

Run one atomic batch of SQL statements against one named server-side
SQLite database, stored at `<storage>/db/<name>.sqlite`. Replaces the
removed WebSQL API; the browser-side wrapper is `omnGoOpenDatabase()` in
`omn-go-core.js`.

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

Whole request body is capped at **1 MB** (`http.MaxBytesReader`).

**Execution semantics**

* All statements run inside **one transaction**. Any failure rolls the
  whole batch back — this is what gives the JS shim's `batch()` atomicity.
* `Query` vs `Exec` is chosen per statement by first-keyword sniffing:
  `SELECT`, `WITH`, `PRAGMA`, `EXPLAIN`, `VALUES` return rows; everything
  else returns counters.
* `[]byte` column values are returned as strings, not base64.
* A stale-file-handle error (`SQLITE_READONLY_DBMOVED`, code 1032 — caused
  by a git pull swapping the file underneath) is self-healed **once**: the
  cached handle is evicted, the database reopened, and the whole batch
  retried from scratch.
* Opening a database that has backups but **no** `.sqlite` file at all
  triggers the one automatic restore in the app (see
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

`results` is index-aligned with the statements that ran. `columns` and
`rows` are omitted for non-row statements.

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

Backups are **JSON Lines** (`.jsonl`) dumps of one user database, stored
at `html/db_backup/<db>/<timestamp>_<hostname>.jsonl` — under `html/`, so
they travel through git sync like any other file.

Filename grammar (also the traversal guard for `file`):

```
^[0-9]{8}T[0-9]{6}Z(_[0-9]+)?_[A-Za-z0-9_-]{1,64}\.jsonl$
```

e.g. `20260727T140500Z_pixel7.jsonl`. Lexicographic order equals
chronological order, so listings are sorted newest-first.

Each file starts with a header line, followed by one line per schema object
and per row:

```json
{"format":"omngo-db-backup","version":1,"database":"mydata",
 "created":"2026-07-27T14:05:00Z","hostname":"pixel7","objects":3,"rows":42}
```

Backups are **manual** — there is exactly one automatic case:
`bootstrapIfMissing` restores the newest backup when a database has backups
but no `.sqlite` file at all (a fresh device right after a pull), because
there is no local state that could be destroyed.

#### `POST /api/db/backup`

**Parameters** (query string)

| Name | Type | Required | Constraints |
| --- | --- | --- | --- |
| `db` | string | yes | `^[A-Za-z0-9_-]{1,64}$` |

Creates a backup and prunes older ones beyond `backup_prune_depth`.

**Responses**

| Status | Body |
| --- | --- |
| `200` | `{"status":"success","file":"20260727T140500Z_pixel7.jsonl","pruned":["html/db_backup/mydata/20260101T…jsonl"]}` |
| `400` | `{"status":"error","message":"invalid db name \"…\""}` |
| `405` | `{"status":"error","message":"POST only"}` |
| `500` | `{"status":"error","message":"<reason>"}` |

`pruned` holds storage-relative paths of the files removed.

#### `GET /api/db/backups`

Everything the `/db_backups` page needs, in one read-only call — it never
opens a database (opening would trigger the bootstrap restore).

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

`databases` is the union of names that have a `.sqlite` file and names that
only have a backup directory, sorted alphabetically. `backups` is sorted
newest-first and is always an array (never `null`).

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

Restores destructively into `<storage>/db/<db>.sqlite`. Serialized against
the bootstrap restore by `dbRestoreMu`; the open handle is evicted so the
next `/api/sql` call reopens the new file. The restored `.sqlite` mtime is
set equal to the backup's, so the page's state dot reads `insync`
immediately.

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
All repository mutation is serialized by `GitMutex`.

**Parameters** — read with `r.FormValue`, so a POST body **or** a query
string works (the frontend uses both).

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `action` | string | no | `pull` | See table below |
| `force` | `"true"` | no | — | Promotes `pull*`→`pull_force` and `push*`→`push_force` |
| `message` | string | no | — | Commit message; used by `push`/`push_force` only |

**Actions**

| `action` | Aliases | Behaviour |
| --- | --- | --- |
| `pull` | `pull_ff`, `download` | Fast-forward if possible; `conflict` if history diverged or local changes are in the way |
| `pull_mark` | — | After a `pull` conflict: write 3-way conflict markers for manual resolution |
| `pull_abort` | `abort` | After a `pull` conflict: discard it, restore local state |
| `pull_force` | `download_force` | Reset local to exactly match remote; also deletes files that are neither tracked nor gitignored |
| `push` | `upload` | Commit local changes (requires `message`) then push; stops on conflict |
| `push_force` | `upload_force` | Commit if needed, then force-push, resetting the remote |

**Responses** — always `200 OK` with `application/json`; the outcome is in
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

Dry-run listing of what a `push` would commit.

**Parameters** (query string)

| Name | Type | Required | Value |
| --- | --- | --- | --- |
| `action` | string | yes | Must be `upload` — nothing else is supported |

Files are taken from `git status`, minus anything matched by `.gitignore`
and minus the root `config.json` (which is never synced — it holds
per-device secrets). Database backups need no special handling: they are
ordinary files under `html/db_backup/` and show up in the same scan.

**Responses**

| Status | Content-Type | Body |
| --- | --- | --- |
| `200` | `application/json` | JSON array of storage-relative paths, e.g. `["md/Welcome.md","html/Welcome.html"]`. **`null` when nothing changed** (a nil slice) — clients must handle that |
| `400` | `text/plain` | `Only upload preview supported` |
| `405` | `text/plain` | `Method Not Allowed` |
| `500` | `text/plain` | `Repo init failed: …`, `Worktree error: …`, `Status error: …` |

---

## 5. Page and asset routes

Not JSON endpoints, but part of the server's URL surface.

### 5.1 Markdown pages

| URL | Behaviour |
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
| `/OMNGoTags.html` | `serveTagsPage` | Auto-generated tag index; staleness is checked against the newest mtime of **all** notes, not one source. Honours `?refresh` |
| `/db_backups` | `serveDBBackupsPage` | Admin page; all data comes from `GET /api/db/backups`. **Admin-protected**, unlike other pages |

Served pages have `injectRuntimeVars` applied, which splices in:

```html
<script>var APP_VERSION = "1.11.32"; var USE_INTERNAL_ED = true; var OMN_THEME = "auto";
document.documentElement.setAttribute('data-theme', OMN_THEME);</script>
```

### 5.2 Static assets

| Prefix | Source | Extraction |
| --- | --- | --- |
| `/js/`, `/css/`, `/json/` | `html/js`, `html/css`, `html/json` | Lazily extracted from the embedded frontend on first request, then user-editable via `?edit=true` |
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

`.jsonl` resolves to `application/jsonl`.

---

## 6. 404 handling

Every miss funnels through `serveNotFound`, which **content-negotiates on
the `Accept` header**:

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

`Linked from` appears only when the `Referer` points at this same server
and contains no `..`; only its path is echoed. `Did you mean` appears when
the failing path has no extension but a note of that name exists — the
`[text](name)` instead of `[text](name.html)` mistake.

Every miss also emits one log line, visible on the `/api/logs` stream:

```
[404] GET /js/missing.js (referer "/Welcome.html")
```

---

## 7. Status codes in use

| Code | Where |
| --- | --- |
| `200 OK` | Success on every endpoint; also the JSON error responses of `/api/sync` |
| `303 See Other` | `/` → `/Welcome.html`; `?edit=true` → `/api/edit-external`; Android `omngo://edit` handoff |
| `400 Bad Request` | Missing/invalid parameters, rejected uploads, SQL errors |
| `401 Unauthorized` | `/login` with a wrong password; `authMiddleware` for a remote caller without an admin cookie |
| `404 Not Found` | Missing static asset or `/api/note` for a missing non-page file |
| `405 Method Not Allowed` | `/api/config`, `/api/restart`, `/api/sql`, `/api/db/*`, `/api/sync/preview` |
| `500 Internal Server Error` | Disk/permission failures, git repo errors, restore failures |

---

## 8. Notes for API clients

* **Do not expose this server to an untrusted network.** `share_lan` plus
  the admin password is the only barrier, there is no TLS, `GET
  /api/config` hands out every stored secret in cleartext, `POST /api/sql`
  is arbitrary SQL, and `/api/note` and `/api/logs` are unauthenticated
  outright.
* From `127.0.0.1` no authentication is needed at all — scripts on the same
  device can drive the whole API directly.
* Success is not always signalled by the status code: `/api/sync` returns
  `200` with `{"status":"error"}`, and `/api/quick` returns `200` with an
  empty body when it wrote nothing.
* Plain-text endpoints return bare words without a trailing newline
  (`Saved`, `OK`, `Restarting`); compare after trimming.
* `/api/newpage` returns the *resolved* target, which may differ from what
  was submitted — use the response, not the request, to build the follow-up
  URL.
* Two endpoints return content designed to be spliced into a note verbatim,
  newlines included: `/api/upload` and `/api/upload_json`.
* `/api/sync/preview` returns JSON `null`, not `[]`, when there is nothing
  to push.
