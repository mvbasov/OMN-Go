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

**Cache-Control.** Each response carries `Cache-Control: no-cache`. The
header is set in `connectionMiddleware` (`backend/middleware.go`). That
function wraps each route. The header thus applies to a page, to an API
answer and to a static file.

`no-cache` does not stop the cache. The client keeps its copy, but it
asks the server each time. The server answers `304 Not Modified` while
the file does not change. The cost is thus one small request.

Without the header, `http.ServeFile` sends `Last-Modified` only. A
browser then applies its own rule. An old `omn-go-core.js` can stay for
days after an update of the application.

A handler that writes `Cache-Control` later replaces this value.
`/api/logs` does this.

---

## 2. Authentication and authorization

### 2.1 Model

`authMiddleware` (`backend/middleware.go`) wraps the protected endpoints:

1. **A local connection bypasses authentication.** If the peer address is
   `127.0.0.1`, `::1` or `localhost`, the endpoint runs with no further
   check. This lets the WebView of the Android application and the desktop
   browser work without a login.
2. For every other connection, the request must carry a **signed**
   `session_role` cookie with an accepted role. `readSessionRole`
   (`backend/session.go`) is the one reader of that cookie. The server
   registers every protected route with `requireAdmin = true`, so a remote
   caller needs the `admin` role. `authMiddleware` accepts the `guest` role
   only for a route registered with `requireAdmin = false`. No such route
   exists today.
3. A missing, changed or expired cookie gives `401 Unauthorized` with the
   body `Unauthorized`.

**Before 26.09.6 the cookie carried the bare word `admin` and the server
trusted it.** A caller on the network could set that cookie itself and get
the admin role with no password. The signature closes that hole. A cookie
in the old form is refused.

There is no CSRF token, no bearer token, and no rate limiting. Passwords are
stored in `config.json` in cleartext. `handleLogin` compares them with
`subtle.ConstantTimeCompare`, and an empty configured password matches
nothing.

### 2.2 Obtaining the cookie

`POST /login` with the correct password sets **two** cookies:

```
Set-Cookie: session_role=admin.1793404800.9Xr3...; Path=/; Expires=...; HttpOnly; SameSite=Lax
Set-Cookie: session_role_hint=admin; Path=/; Expires=...; SameSite=Lax
```

`session_role` is the only cookie the server reads. Its value has three
parts, separated by a dot:

| Part | Contents |
| --- | --- |
| 1 | The role, `admin` or `guest` |
| 2 | The Unix time at which the cookie stops, as a decimal number |
| 3 | `HMAC-SHA256("role.expiry", key)`, in base64url with no padding |

The key is 32 random bytes at `<StorageDir>/session_secret`. The application
makes the file at the first login and gives it mode 0600. The file is in
`.gitignore`, thus a sync never carries the key of one device to another.
A cookie is therefore valid on the device that made it and on no other.

The cookie lasts **30 days**. It is `HttpOnly` and `SameSite=Lax`. It is not
`Secure`, because this server speaks HTTP.

`session_role_hint` carries the plain role for the page alone. `checkRole`
in `omn-go-sse.js` reads it to disable the `.admin-only` controls for a
guest. **The server never reads it.** A client that changes the hint changes
what its own page shows and gets no permission.

### 2.3 Protection map

| Endpoint | Auth |
| --- | --- |
| `POST /login` | none (it is the login) |
| `GET /api/note` | **none — deliberately open** |
| `GET /api/search` | **none — deliberately open** |
| `GET /api/logs` | **none** |
| `/api/quick`, `/api/bookmark`, `/api/upload`, `/api/upload_json`, `/api/save`, `/api/newpage`, `/api/config`, `/api/restart`, `/api/sql`, `/api/db/backup`, `/api/db/backups`, `/api/db/restore`, `/api/sync`, `/api/sync/preview`, `/api/edit-external`, `/api/status`, `/api/export/note`, `/api/import/note`, `/db_backups` | admin (local bypass applies) |
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
| GET | `/api/export/note` | yes (405 otherwise) | admin | Markdown download |
| POST | `/api/import/note` | yes (405 otherwise) | admin | JSON |
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
| `password` | string | yes | Compared against `admin_password`, then `guest_password` from `config.json`. The comparison is constant-time. An empty configured password matches nothing. |

**Responses**

| Status | Content-Type | Body | Notes |
| --- | --- | --- | --- |
| `200` | `text/plain` | `OK` | Sets the signed `session_role` cookie and the `session_role_hint` cookie, for the `admin` or the `guest` role. See §2.2. |
| `401` | `text/plain` | `Invalid` | No cookie set |
| `500` | `text/plain` | `Login unavailable` | This install has no session key and could not make one. See `sessionSecret` in `backend/session.go`. |

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

### 4.3 Note exchange

One note leaves as a file, and one arrives as a file. The transport is not
OMN-Go's business: the Android share sheet reaches Telegram, e-mail,
LocalSend, Bluetooth and every other application on the device, and the
desktop application uses a download and an upload. All of them carry the
same bytes.

Both endpoints are **admin only**. Import writes files. Export is locked as
well, because it is a way out of the note tree. A local connection bypasses
authorization, so the device itself is unaffected.

#### `GET /api/export/note`

The Markdown source of one note, ready to send.

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string | yes | — | The note. `Welcome`, `Welcome.md` and `Welcome.html` all name the same one |

The answer is the note's own source with one line **set** in the header
block:

```
Title: Weekly plan
Date: 2026-08-01 09:00:00
FileName: project/Sub/WeeklyPlan
```

`FileName:` is the note name: the path under `md/`, with no extension. The
path can travel no other way. Each transport delivers a flat file name, so
the folder that the note is in is lost if it is not inside the file.

The line is **set**, not added: a note that was imported one time already
carries a `FileName:` line, and two lines with one key have no meaning.

**The stored note does not change.** An export is a read.

The file name in `Content-Disposition` is the path with each `/` replaced by
`-`:

```
Content-Type: text/markdown; charset=utf-8
Content-Disposition: attachment; filename="project-Sub-WeeklyPlan.md"
```

Two notes with the same name in two folders are then two different
attachments in one mail thread. The name holds only `A-Za-z0-9._-`, so a
recipient can save it on Windows as well as on Android. It is a label for a
person and OMN-Go never reads it back.

**The description**

A note can offer a short text to send **with** the file — a Telegram caption,
a mail body. It is an HTML comment, thus it is invisible on the rendered
page:

```
Tags: Test, Document

<!--- DESCRIPTION:
There is some
description
--->
```

When the note has one, the answer carries this header:

```
X-OMN-Description: VGhlcmUgaXMgc29tZQpkZXNjcmlwdGlvbg==
```

The value is **base64 of UTF-8**. A description is a paragraph: it can hold a
line break, which would end the header field, and it can hold a letter that a
header field cannot carry as it stands. The text is cut to 1000 characters,
which is below Telegram's caption limit of 1024 — Telegram refuses a longer
caption instead of making it shorter.

A note with no description gets **no header**. An empty header is not the
same thing: the Android application opens an empty message body for one, and
attaches and sends for none.

The description **stays in the note that travels**. It is part of the note,
and the receiver must be able to send the note on with it.

`<!--` and `-->` name the same block as `<!---` and `--->`, `DESCRIPTION` is
matched whatever its case, and the colon is optional. One limit comes from
HTML and not from OMN-Go: a comment ends at the first `-->`, thus a
description cannot contain one.

**Errors**

| Status | Body | When |
| --- | --- | --- |
| `400` | `{"status":"error","message":"no note named"}` | `name` is absent |
| `400` | `{"status":"error","message":"… is not a note"}` | `name` is a static asset |
| `404` | `{"status":"error","message":"no note …"}` | No such note |
| `405` | `{"status":"error","message":"GET only"}` | Another method |

#### `POST /api/import/note`

One note arrives. The endpoint accepts two body shapes, because it has two
callers:

- **the raw Markdown**, with any content type that is not
  `multipart/form-data`. The Android application reads the shared file and
  posts the bytes.
- **a form file** in the field `file`, with `multipart/form-data`. This is
  what a browser sends from a file input.

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string (query) | no | — | The name of the file that arrived. Used only when the note carries no `FileName:` line |

The note is larger than the configured upload limit: **413**, and nothing is
written. A truncated note is not an import.

**Where the note lands.** Always under `md/incoming/`, with `FileName:`
resolved relative to that directory:

```
FileName: project/Sub/WeeklyPlan  ->  md/incoming/project/Sub/WeeklyPlan.md
```

A note that arrives can never write over a note that you made. `FileName:`
is text from another device, so OMN-Go refuses an absolute path, a drive
letter and each `..` segment, keeps `A-Za-z0-9 ._-` in each part of the
path, and then makes sure that the result is still in `md/incoming/`.

With no `FileName:` line, OMN-Go uses the name of the file that arrived,
then `Title:`, then the date and the time.

**A name that is in use** gets an index: `WeeklyPlan`, `WeeklyPlan-2`,
`WeeklyPlan-3`.

**The header block.** `FileName:` is removed, because the note is now in a
different place and the line would not be true. `Imported:` is set to the
time of the import. `Date:` and `Modified:` do not change: they are facts
about the note of the person who sent it.

**The incoming index.** `md/incoming/incoming.md` gets one line directly
below `<!-- omn-go-incoming-list -->`, newest first:

```
* <span class="omn-incoming-when">2026-08-09 12:34</span> · [Weekly plan (2)](project/Sub/WeeklyPlan-2)
```

The link text is the note's own `Title:`. The file name is the fallback for a
note that carries no usable one. A collision index is carried into the text,
because two copies of one note otherwise read as the same line twice.

A title arrives from another device, and a Markdown link label is not escaped
by anything after this point, so `` [ ] < > & \ ` * ~ | `` are taken out of it
first. Each becomes a space and the spaces collapse. `_` stays, because it
does not emphasize inside a word and taking it out would damage more titles
than it saved. The destination is percent-encoded, because a note name may
hold a space and a bare Markdown destination may not.

The date is in a `<span>` because Markdown gives no way to set one part of a
line smaller than the rest.

OMN-Go makes this note at the first start. It is yours from that moment: no
version change rewrites it. The note holds nothing but its header block, the
marker and the list - the **Receive a note** box is part of the application
and is spliced in with the modals at serve time, so it is not in the note, not
in the on-disk cache and not on an exported page.

**Response**

```json
{
  "status": "success",
  "name": "incoming/project/Sub/WeeklyPlan-2",
  "base": "WeeklyPlan-2",
  "url": "/incoming/project/Sub/WeeklyPlan-2.html"
}
```

A `warning` field can come with a `200`: the note is on disk, but the line
on the incoming index was not written. The note is not lost, so the answer
is not an error — a second copy is not the repair.

**Errors**

| Status | Body | When |
| --- | --- | --- |
| `400` | `{"status":"error","message":"the note is empty"}` | Nothing but space arrived |
| `400` | `{"status":"error","message":"no file in the upload"}` | `multipart/form-data` with no `file` field |
| `405` | `{"status":"error","message":"POST only"}` | Another method |
| `413` | `{"status":"error","message":"the note is larger than …"}` | Larger than `max_upload_size_mb` |

---

### 4.4 Search

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

A fourth rung, **phrase**, stands above these three. It belongs to a whole
query and never to one term, thus no term can reach it. A document takes it
when one field or one line holds every word of the query in order. One space
separates each pair, in the folded text. That document then ranks above
every document that does not hold the query, whatever the scores say. The
line that holds it takes the rung too, thus that line is the first snippet
of its document.

The rule is narrow on purpose. A comma between two words is not a space. A
query of one term is not a phrase. A query that names a field is not a
phrase, because `title:a b` has no natural reading as one.

**A result can hold fewer snippets than `snippets` asks for.** A long note
answers a common query on hundreds of lines. Few of those lines say anything.
The server takes the first `snippets` lines and then removes each line whose
only matching terms are one rune. Such a line carries no word of the query.
A term of one Han, Hiragana, Katakana or Hangul rune is a whole word, and it
never counts. The order of the two steps is the rule: the server never
replaces a removed line from below, thus a line of a lower rung cannot reach
the answer. A query of one term loses no line. New in 26.08.81.

The reason is arithmetic. A document score is the sum over the terms, and a
field carries a weight. Several query words loose in one title thus outscore
a body line that holds the sentence. A bonus large enough to close that gap
in one query is too large in the next one. See the banner of `scoreDocument`
in `backend/search.go` for the measurements. New in 26.08.80.

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

Every result on this page links to `/<Name>.html?hl=<term>&hl=<term>`. Each
matching **line** under a result is a link of its own, and it adds `?hlt=` with
that line's text, so the note opens at the line that the user selected. See
`?hl=` and `?hlt=` below.

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

#### `?hlt=<line>` — which occurrence to go to

`hl` says which words to mark. `hlt` says which occurrence of them to go to. Its
value is the text of one matching line, as the search result shows it.

A result lists each matching line. Without `hlt`, every line of one result
opens the same URL, and the note opens at its first match — the correct answer
for the first line only.

The client does not use the line NUMBER for this. The number is a position in
the Markdown source, and the page is compiled HTML: a `<script>` block is
absent from it, a link URL is absent from it, and one paragraph can be several
source lines. The client matches the line's TEXT instead (`omnMarkNear` in
`omn-go-core.js`). The text can be a part of the line, because a snippet is a
window of at most 160 runes around the first hit in the line.

A line inside a `<script>` block gets no `hlt`. Its text is in the index but
never on the page.

The client removes `hlt` from the address bar with `hl`.

#### The order of precedence

A URL can carry all three of a fragment, `hl` terms and `hlt`. They are not in
competition — each one is more precise than the last, and the client uses the
most precise one that it can apply:

1. `hlt` — the marked word on that line.
2. The fragment — the first marked word at or after the anchor. This is the
   answer for a link that names a section instead of a line, for example
   `/Bookmarks.html?hl=cats#2026-06-15-200000`. The anchor alone is not the
   answer for a line, because a section continues to the next heading and the
   line that matched can be a screen or more below the anchor.
3. The first marked word in the page.

Three conditions move down this list:

- The text of `hlt` is not in the page, or it is above the anchor. The same
  line can occur two times, and the copy above the anchor is not the one that
  the user selected. Rule 2 applies.
- The anchor names an element, but no marked word is at or after it. The
  client keeps the scroll to the anchor. Each hit above it is in a section
  that the user did not select.
- The anchor names no element. Rule 3 applies.

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

### 4.5 Uploads

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

### 4.6 Configuration

#### `GET /api/config`

Return the whole live configuration as JSON. **The response includes
`admin_password`, `guest_password`, and the SSH private key and password of
every git server slot, all in cleartext.**

The Config page reads this endpoint for that reason. Since 26.09.7 the page
renders each password box and each SSH key box empty, thus the HTML of the
page holds no secret. The **Show passwords** button calls this endpoint and
fills the boxes. See `omnGoRevealSecrets` in `omn-go-sse.js`.

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
  "search_enabled": false,
  "search_kinds": ["md", "bookmarks"],
  "search_bundled": false,
  "search_scope": "all",
  "max_upload_size_mb": 3,
  "enable_intent_uri": false,
  "enable_termux_intent": false,
  "android_fullscreen": "fullscreen",
  "log_debug": false,
  "log_info": false,
  "log_tags": ["404", "assets", "config", "db", "db-backup", "db-bootstrap",
               "db-restore", "edit", "exchange", "note-files", "page",
               "precompile", "restart", "search", "server", "status",
               "storage", "sync", "tags", "templates", "upload"]
}
```

**Field reference**

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `force_pull_one_time` | bool | `false` | One-shot force-pull flag |
| `server_port` | int | `8080` | Listen port; applied at next start |
| `admin_password` | string | `admin_secret_changeme` | Grants the `admin` role at `/login`. An empty value grants nothing. |
| `guest_password` | string | `guest_secret_changeme` | Grants the `guest` role at `/login`. An empty value grants nothing. |
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

**A field the request does not carry is left as it is.** Send one field to
change one setting. The rest of the configuration is not touched. A field
that IS carried is applied even when its value is empty, which is how the
Config page clears a text box.

> **Changed in 26.09.7.** The four git-server fields of one slot now follow
> that rule one field at a time. Until that version the handler wrote each
> of the four when a minimum of one was not empty. A request that named
> `git_name_0` alone therefore wrote an empty SSH key and an empty key
> password over the stored ones.

> **Changed in 26.08.43.** Before that version this endpoint rebuilt the
> whole configuration from the request, so a request that named one field
> emptied every field it did not name. A note that saved the theme also
> emptied the author name, both passwords, the external-editor command and
> the device label.

**Checkboxes, and `config_fields`.** A browser sends nothing at all for an
unticked checkbox, so "unticked" and "not my business" arrive identically.
A form therefore declares the fields it governs in one hidden field:

```
config_fields=use_internal_editor,share_lan,enable_intent_uri,enable_termux_intent,search_enabled,search_bundled,search_kinds,log_debug,log_info,log_tags
```

A name in that list counts as carried even when the request holds no value
for it, which is what an unticked box means. A caller that sends no
`config_fields` governs only the fields it actually names. A caller with no
form behind it does not need the list at all: `share_lan=false` turns one
setting off and touches nothing else.

**Parameters** — every field is optional.

| Name | Type | Applied when | Notes |
| --- | --- | --- | --- |
| `config_fields` | string | — | Comma-separated field names this request governs |
| `server_port` | int | parses `> 0` | Otherwise ignored |
| `admin_password` | string | carried | Written verbatim, including empty |
| `guest_password` | string | carried | |
| `author` | string | carried | |
| `use_internal_editor` | `"true"` | carried | Any other value → `false` |
| `desktop_ext_cmd` | string | carried | |
| `max_upload_size_mb` | int | non-empty and `> 0` | |
| `theme` | string | carried | Through `normalizeTheme`; unknown → `auto` |
| `share_lan` | `"true"` | carried | **Flipping this changes the response body** |
| `enable_intent_uri` | `"true"` | carried | |
| `enable_termux_intent` | `"true"` | carried | |
| `android_fullscreen` | string | carried | Through `normalizeFullscreen`; unknown → `fullscreen` |
| `search_enabled` | `"true"` | carried | |
| `search_bundled` | `"true"` | carried | |
| `search_scope` | string | carried | Through `normalizeSearchScope` |
| `search_kinds` | string | carried | Repeated field. Every value carried is the whole new set |
| `log_debug` | `"true"` | carried | Any other value → `false` |
| `log_info` | `"true"` | carried | Any other value → `false` |
| `log_tags` | string | carried | Repeated field. Every value carried is the whole new set. Through `normalizeLogTags`. An unknown tag goes away |
| `hostname` | string | carried | Sanitized; carried-but-empty resets to the OS-derived default |
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

### 4.7 External editor

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

### 4.8 Server log stream

#### `GET /api/logs`

Server-Sent Events stream of every log line the backend writes. The
progress overlay of the frontend (`omn-go-sse.js`) reads this stream, so the
sync progress shows the real stages of the backend.

No parameters. **No authentication.** Any client that can reach the port can
read every log line that the server writes.

**The stream always carries every line.** The `log_debug`, `log_info` and
`log_tags` settings of `config.json` control what the server prints to
stdout. They also control what `omn-go-sse.js` prints to the browser
console. They do not control this stream. The sync overlay is built on `[sync] (debug)` lines,
and it must work when a reader asks for less noise.

**Response headers**

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

**Event format** — unnamed events, one `data:` field per log write. A line
is `<stamp> [tag] (level) message`. `tag` is the subsystem, from the
constant block in `backend/log_levels.go`. `level` is `debug`, `info` or
`error`.

```
data: 2026/07/27 14:05:00 [sync] (info) Pull: fetching origin

data: 2026/07/27 14:05:01 [sync] (info) Pull: fast-forward complete

data: 2026/07/27 14:05:02 [edit] (error) cannot run "subl": no such file

```

Three call sites cannot reach an application object and thus write the
level into the text by hand. Each one is an `(error)`. A reader that finds
no level in a line must print it, because a fault always prints.

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

### 4.9 SQLite

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
  [§4.10](#410-database-backups)).

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

### 4.10 Database backups

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

### 4.11 Git synchronization

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

**The local-only name rule.** A path stays on one device when a name in
it starts with `local-`. This is the name of the file, or the name of a
directory above it. The `.gitignore` pattern is the single line
`local-*`, and it is the last line of the file. go-git reads the patterns from the end and stops at the
first match, thus a local-only name wins over a `!` negation, for example
`html/images/local-map.svg`.

The rule started as `/html/db_backup/local-*/`, which kept the backups of
a `local-` database out of git. `ensureGitignore` deletes that older line
from an existing install and writes the general one.

A `.gitignore` pattern does not remove a file from the git index. A file
that git tracked before it got the name would thus keep its old behavior.
A commit takes no new content, and a force pull writes the copy of the
repository over the local file. `commitLocalChanges` thus reads
the index before it reads the status, and it removes each local-only path
from the index. The result is one commit that says "stop to track this".
The file stays on the device. A pull on another device deletes the copy
there, which is the meaning of the name.

Only the local-only rule removes a path from the index. The other
patterns do not. A file such as `md/UserManual.md`, or a compiled
`*.html` page, can be in the index of an old repository. A removal of
each of those at one time would delete many files on the other
devices.

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

The endpoint then reads the git index and adds each local-only path that
git still tracks, with the note ` (local-only: git stops to track it)`
after the path. `git status` reports no such file when its content did
not change, but the next commit removes it. See **The local-only name
rule** below.

**Response** `200`

```json
{ "files": [], "unpushed": true, "remote": "slot1", "verified": true }
```

| Field | Meaning |
| --- | --- |
| `files` | Storage-relative paths that would be committed. Always an array, never `null`. A local-only path that git still tracks carries the note ` (local-only: git stops to track it)` after the path |
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

### 4.12 Status

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
| `config` | none | `internal_editor`, `theme`, `max_upload_mb`, `search_enabled`, `search_kinds`, `search_scope`, `search_bundled`, `intent_uri`, `termux_intent`, `android_fullscreen`, `backup_prune_depth`, `hostname`, `author`, `log_debug`, `log_info`, `log_tags` |
| `git` | two object reads | `repo_exists`, `configured`, `branch`, `head{hash,short,subject,author,date}`, `remote{name,url}`, `remote_ref`, `remote_head{…}` |
| `search` | one pass over the index | `enabled`, `docs`, `lines`, `bytes`, `index_bytes_estimate`, `built`, `checked`, `dirty`, `kinds`, `scope` |
| `runtime` | none | `go_version`, `goroutines`, `heap_alloc`, `sys`, `assets_version`, `assets_refreshed` |
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

`remote_head` is the commit that the remote-tracking ref names, for example
`refs/remotes/gitserver0/master`, and `remote_ref` names that ref in the short
form `gitserver0/master`. The last pull or push wrote it. This endpoint asks
no remote server, so the value is what this device knows, and it can be old. A
reader compares `head.hash` with `remote_head.hash` to see whether the two ends
agree. A branch that this device never fetched leaves both fields absent, and
so does a detached HEAD.

`assets_refreshed` tells if this start installed or replaced a
version-dependent asset. This occurs one time, after an update of the
application. Android reads the same fact through the `AssetsRefreshed`
binding, immediately before the WebView loads the first page. If the
answer is yes, Android clears the cache of the WebView. An old script in
that cache made some new pages operate incorrectly. A user then had to
stop the application and clear the cache by hand.

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
    "remote": { "name": "home", "url": "https://user@example.com/notes.git" },
    "remote_ref": "gitserver0/master",
    "remote_head": {
      "hash": "3d5a1c0b7e8f9021a3b4c5d6e7f8091a2b3c4d5e",
      "short": "3d5a1c0",
      "subject": "Tag every page that comes with OMN-Go",
      "author": "Mikhail Basov",
      "date": "2026-08-03T19:40:00Z"
    }
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
| `/OMNGoFiles.html` | `serveFilesPage` | The file index: the Bundled, Served and Source trees. **Admin-only**. See §5.3 |

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

The file index shows the files that OMN-Go holds, in the shape that a browser
uses for `file:///`: **one directory at a time**, with a breadcrumb up and
links down.

The address with no parameter is the **first screen**: three buttons, one for
each tree, and nothing else. Each tree answers one question:

| Tree | `?tree=` | Root | The question |
| --- | --- | --- | --- |
| Bundled | `bundled` | `staticFS` (`frontend/html`, `frontend/md`) | What does this build carry? |
| Served | `served` | `StorageDir/html` | What does a URL find, and where did it come from? |
| Source | `source` | `StorageDir/md` | What did you write, and how large is it? |

Until 26.08.53 two of these were sections of one screen and a name in both was
printed two times. Each tree is a screen of its own now, and inside a tree each
**name has one row**.

**Silence is the ordinary case.** On a real installation most files are the
user's: the notes, the pages OMN-Go compiled from them, the uploads. 26.08.54
gave each of those a word (`yours`, `compiled`, `as shipped`) and the words
that mattered drowned in them. Since 26.08.55 a row speaks only when the
application is involved.

**Two channels in a row.** The word says what the file **is**. The colour says
what **happens** to it. The colour tokens are `--file-app`, `--file-alert`,
`--file-keep`, `--file-derived` and `--file-plain`, and each has a value for
each theme (`omn-go-core.css`, section 1). Colour is never the only carrier:
`app-owned` is a word on the second line of each row that it applies to, in
each of the three trees.

| Word | Colour | Meaning |
| --- | --- | --- |
| *(none)* | — | The file is the user's, or OMN-Go makes it again when it needs to. Nothing is at stake |
| `not extracted` | app when app-owned, else plain | The build carries the file, and no request has asked for it yet |
| `changed here` | alert when app-owned, else keep | The build carries it and the copy on the device differs |
| `edited outside` | alert | A `.txt` copy in `html/` is newer than the file in `md/`. One save in the editor copies it back |
| `waits for restart` | derived | ... or older, and `syncNoteFilesToHTML` repairs it at the next start |
| `same size` | plain | Larger than `filesCompareMax`, and the two sizes agree |

A directory row carries `<n> from the app`, or `<n> from the app, none
extracted`, and nothing at all when the application delivered no file into it.

**The key** (`filesLegend`) is built from the pairs `(word, colour)` that the
rendered rows and directory rows actually use, and it renders as a `<details>`
element that is **closed** by default. The pair is the key, not the word: the
same `changed here` is green on a file OMN-Go keeps and red on a file the next
version replaces. A directory that uses no word gets no key.

**Parameters**

| Name | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `tree` | `bundled`, `served`, `source` | no | — | Which tree to show. With no value the page shows the three buttons |
| `dir` | string | no | root | Directory to show, relative to the root of that tree, e.g. `js/`, `Test/OMN-Go/` |
| `all` | `1`, `true` | no | — | Show every file in the directory instead of the first 200 |

A `dir` with no `tree` selects the Served tree. Each link that the page of
26.08.53 produced has that shape, and it still lands where it did.

`normalizeFilesDir` normalizes `dir`. It resolves `..` segments, and a result
that would climb above the root collapses to the root. The page uses `dir` only
as a **string prefix over the paths already collected** by the walk. It never
joins `dir` onto a filesystem path. A value that names nothing therefore
matches nothing and renders an empty directory with a working breadcrumb.

**How the state is decided.** The two sides of a name are paired by the walk
(`treeEntries`). A file on one side only gives `not extracted` or one of the
device-only words. A file on both sides is compared: the sizes first, which
answers most pairs for free, then the bytes when the sizes are equal. The byte
read is bounded by `filesCompareMax` (2 MB), and a larger pair of equal size
reads `same size`. Reading an embedded file is not writing, so this does not
break the rule below.

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
* Your notes do not appear in the Served tree. A note is not served from
  `html/`. Its compiled page is, and that page says `compiled`.

**Row extras**

* An **edit** link appears where an edit in place makes sense. The page decides
  this from the resolved content type and shows the link for text content
  (`.js`, `.json`, `.css`, `.md`, …). There is no edit link on an image, a
  font, audio or video. There is also no edit link on an `.html` file: to edit
  a page, open it and press the Edit button of that page.
* A row that says `not extracted` still has an edit link. `handleGetNote`
  extracts the file and then opens the editor on it.
* The **Bundled** tree has no edit link at all. An edit always operates on the
  copy on the device, and that copy has a row in the Served tree or in the
  Source tree.
* A row of the **Source** tree carries `local only` when the path stays on this
  device (`md/local/`, or a `local-` segment).
* The row is **two lines**. The name and the state word are on the first line,
  and the facts are on the second. The name thus always has the width of the
  screen. Before 26.08.54 the row was one line and a long name broke one
  character to a line on a phone.
* The date is on the row of a file that is on the device. The row shows the
  day, and the full time is in a `title`.

Directory totals are **recursive**. The file count and size in a directory row
cover the whole subtree, and one name counts one time. The number therefore
answers "how big is this" and not "how many rows are directly inside". A
directory of one kind carries one word: `not extracted`, `not shipped` or
`yours`.

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
| `401 Unauthorized` | `/login` with a wrong password. `authMiddleware` for a remote caller with no valid admin cookie, which covers a missing, a changed and an expired one. |
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
