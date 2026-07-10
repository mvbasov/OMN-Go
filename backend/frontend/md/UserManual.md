Title: User Manual
Date: 2026-07-07 12:00:00
Category: System

# OMN-Go User Manual

Welcome to the OMN-Go manual. The first half covers everyday use;
the second half covers configuration, synchronization, LAN sharing and
troubleshooting. This page is itself a normal OMN-Go note — open it in
edit mode (pencil button) to see how any example on it is written.

## Table of contents

**Basics**

- [First run and where your data lives](#first-run-and-where-your-data-lives)
- [Login and roles](#login-and-roles)
- [The interface](#the-interface)
- [Creating a new page](#creating-a-new-page)
- [Editing pages](#editing-pages)
- [The editor and Emmet](Editor)
- [Page format: the header block](#page-format-the-header-block)
- [Markdown in a nutshell](#markdown-in-a-nutshell)
- [Links: absolute, relative, external](#links-absolute-relative-external)
- [Edit links for non-page files](#edit-links-for-non-page-files)
- [Math and code highlighting](#math-and-code-highlighting)
- [Material icons in notes](#material-icons-in-notes)
- [Buttons and shortcuts inside a page](#buttons-and-shortcuts-inside-a-page)
- [Quick Notes and Bookmarks](#quick-notes-and-bookmarks)
- [Theme](#theme)

**Advanced**

- [Configuration reference](#configuration-reference)
- [Git synchronization](#git-synchronization)
- [Sharing on the LAN](#sharing-on-the-lan)
- [Raw HTML and JavaScript in pages](#raw-html-and-javascript-in-pages)
- [Troubleshooting](#troubleshooting)

---

# Basics

## First run and where your data lives

OMN-Go keeps everything in one storage directory:

- **Android:** `/storage/emulated/0/Android/media/net.basov.omngo`
- **Desktop:** `./data` next to the executable

Inside it:

- `md/` — your notes as Markdown files. **This is the source of truth**;
  this is what you back up and what git synchronizes.
- `html/` — compiled pages plus static assets (`css/`, `js/`, `images/`,
  `user_json/`). The compiled `.html` files are a regenerable cache: OMN-Go
  rebuilds any of them whenever the matching `.md` file is newer.
- `config.json` — this device's settings (see
  [Configuration reference](#configuration-reference)). It is local to the
  device and is never synchronized by git.

On first start OMN-Go creates the directory, a default `config.json`, and
a few starter pages ([Welcome](Welcome), [QuickNotes](QuickNotes),
[Bookmarks](Bookmarks), [ScriptRules](ScriptRules), [Editor](Editor)).

## Login and roles

There are two passwords, both set on the [Config](Config) page:

- **Admin** — full access: editing, saving, sync, configuration.
- **Guest** — read-oriented access for other people on your network.

Connections from the device OMN-Go runs on (`127.0.0.1` / `localhost`,
which includes the Android app's own view) skip the login entirely; the
passwords matter when [LAN sharing](#sharing-on-the-lan) is enabled and
someone connects from another device.

**Change the default passwords before enabling LAN sharing.** A fresh
install ships with `admin_secret_changeme` / `guest_secret_changeme`, and
anyone on your network who has read this manual knows them.

## The interface

Tap or click the page title to expand the header bar. Its buttons, left to
right:

- <i class="material-icons">home</i> — go to the [Welcome](Welcome) page
- <i class="material-icons">note_add</i> — create a new page
- <i class="material-icons">bolt</i> — add a quick note
- <i class="material-icons">bookmark_add</i> — add a bookmark
- <i class="material-icons">bookmarks</i> — open the [Bookmarks](Bookmarks) page
- <i class="material-icons">refresh</i> — force-recompile the current page
- **Force** checkbox — arms the next sync action as destructive (see
  [Git synchronization](#git-synchronization))
- <i class="material-icons">cloud_download</i> / <i class="material-icons">cloud_upload</i> — git pull / push
- <i class="material-icons">settings</i> — open the [Config](Config) page
- <i class="material-icons">info</i> — show the page's metadata
- <i class="material-icons">save</i> / <i class="material-icons">edit</i> — save / toggle edit mode

Buttons marked admin-only are hidden when logged in as guest.

## Creating a new page

1. Press <i class="material-icons">note_add</i> in the header.
2. Enter a title — a file-safe name is suggested automatically; confirm or
   adjust it.
3. The page opens in edit mode with a header block prefilled.

Where the new page is created depends on the name you confirm:

- a bare name like `Ideas` becomes a **sibling of the current page**
  (creating `Ideas` while viewing `projects/Plan` makes `projects/Ideas`);
- a name with a slash like `work/Ideas` is taken as written;
- a leading slash like `/Ideas` forces a top-level page.

A link to the new page is appended to the page you started from.

## Editing pages

Press <i class="material-icons">edit</i> to open the editor, make your
changes, press <i class="material-icons">save</i>. Saving updates the
`Modified:` header line automatically and recompiles the page. The editor is
a page of its own that loads the note's source when it opens — it has a small
toolbar with an Emmet-style HTML expander and a select-current-line button.
See [The editor and Emmet](Editor) for the toolbar and the abbreviation
syntax.

**Internal vs. external editor.** With *Use Internal Editor* disabled on
the [Config](Config) page, the edit button instead hands the file to an
external editor: on desktop, the command from *Desktop External Cmd* (for
example `subl`); on Android, the system app-chooser. When you return, the
page reloads with your changes.

**Images.** Drag an image file onto the editor area — it is uploaded to
`images/` and a ready-to-use Markdown image reference is inserted at the
cursor.

## Page format: the header block

Every page starts with a Pelican-style header: `Key: value` lines,
terminated by the first blank line. Example:

```
Title: Shopping list
Date: 2026-07-07 10:00:00
Modified: 2026-07-07 12:30:00
Author: Me
Category: Home
Tags: shopping, home

First line of the actual note...
```

- `Title` sets the page title shown in the browser tab and page header.
- `Modified` is maintained automatically on every save — you never edit it.
- `Tags` renders clickable pills in the page header. (Automatic generation
  of the tag overview page is planned.)
- Every header line also becomes a `<meta>` tag in the compiled page.
- If you save a page with no header at all, a minimal one (`Title`,
  `Date`, `Modified`, and `Author` from your config) is added for you.

## Markdown in a nutshell

Pages are written in Markdown with GitHub-flavored extensions (tables,
strikethrough, task lists):

```
# Heading            ## Subheading
**bold**  *italic*  ~~strikethrough~~  `inline code`
- bullet list        1. numbered list
- [ ] open task      - [x] done task
[link text](PageName)
![image alt](/images/photo.png)
> quoted text
| Col A | Col B |    (tables)
|-------|-------|
```

Line breaks are literal: a single newline in the editor is a line break in
the page. For everything else, the
[GitHub Markdown guide](https://docs.github.com/en/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax)
is an excellent reference and matches OMN-Go's dialect closely.

## Links: absolute, relative, external

When a page is compiled, internal links are normalized so you can write
them naturally:

- `[Notes](Ideas)` — **bare page name**: resolves relative to the current
  page (like a file in the same folder) and gets `.html` appended
  automatically.
- `[Notes](Ideas.md)` — same thing; `.md` is rewritten to `.html`.
- `[Up](../Plan)` and `[Here](./Ideas)` — **relative paths** work exactly
  like in a file system.
- `[Top](/Welcome)` — a **leading slash** is absolute from the notes root,
  no matter where the linking page lives.
- `[Section](#some-heading)` and `[Page section](Ideas#some-heading)` —
  anchors are preserved; every heading gets an ID derived from its text
  (lowercased, hyphenated).
- Links with a real file extension (`.png`, `.css`, `.js`, ...) and
  external links (`http://`, `https://`, `mailto:`, `tel:` ...) are left
  untouched. On Android, external links open in the system browser.

## Edit links for non-page files

Any file OMN-Go serves can be opened in the editor by appending
`?edit=true` to its URL. That makes it easy to keep "edit me" shortcuts in
your notes for files that are not Markdown pages:

```
[Edit my stylesheet](/css/custom.css?edit=true)
[Edit shared data](/user_json/inventory.json?edit=true)
```

The link opens the raw file in the built-in editor (or your external
editor, if configured), and saving writes it back in place. If the file
does not exist yet, an empty one is created.

## Math and code highlighting

Formulas are rendered with KaTeX, fully offline. Math rendering is
**opt-in per page**: enable it by placing this line anywhere on the page
(top of the body, right after the header block, is a good habit):

```
<script>var OMN_GO_KATEX=true</script>
```

Without that line, `$...$` stays literal text — so pages about money
(`$5 and $10`) don't accidentally turn into formulas. This manual page
has the flag set, so here is a live example — inline `$E = mc^2$` renders
as $E = mc^2$, and a display block:

<script>var OMN_GO_KATEX=true</script>

- inline: `$E = mc^2$`
- display block:

```
$$
\frac{a}{b} = \sum_{i=1}^{n} x_i
$$
```

Underscores and other Markdown-sensitive characters inside `$...$` /
`$$...$$` are protected — write normal TeX.

Fenced code blocks with a language name are syntax-highlighted:

````
```go
func main() { fmt.Println("hi") }
```
````

## Material icons in notes

The Google Material Icons font ships with OMN-Go, so any icon can be
embedded in a note with a small piece of inline HTML:

```
<i class="material-icons">home</i>
<i class="material-icons">lightbulb</i> An idea!
<i class="material-icons" style="font-size:48px;color:#28a745;">check_circle</i>
```

renders as: <i class="material-icons">home</i>
<i class="material-icons">lightbulb</i> An idea!
<i class="material-icons" style="font-size:48px;color:#28a745;">check_circle</i>

Browse icon names at [fonts.google.com/icons](https://fonts.google.com/icons)
(pick "Material Icons" style; use the snake_case name).

## Buttons and shortcuts inside a page

The header buttons call JavaScript functions that are available on every
page, so you can place your own buttons anywhere in a note:

```
<button onclick="document.getElementById('bmPanel').classList.remove('hidden')">
  <i class="material-icons">bookmark_add</i> Add bookmark
</button>

<button onclick="document.getElementById('quickPanel').classList.remove('hidden')">
  <i class="material-icons">bolt</i> Add quick note
</button>
```

Navigation "buttons" are usually better as plain links. There is no
*Quick notes* button in the header — it is simply a page, so link to it:

```
[Quick notes](/QuickNotes)
[Bookmarks](/Bookmarks)
```

Or dress a link up as a button:

```
<a href="/QuickNotes.html"><button>
  <i class="material-icons">bolt</i> Quick notes
</button></a>
```

Try it: [Quick notes](/QuickNotes) · [Bookmarks](/Bookmarks)

## Quick Notes and Bookmarks

- <i class="material-icons">bolt</i> **Quick note** opens a small text box;
  what you type is appended, with a timestamp, to the
  [QuickNotes](QuickNotes) page. Use it for capture-first-sort-later notes.
- <i class="material-icons">bookmark_add</i> **Add bookmark** asks for a
  URL, title, tags and notes, and stores the entry in the
  [Bookmarks](Bookmarks) page's data block, which renders as your bookmark
  collection.
- **Android sharing:** share text or a link from any app to OMN-Go and it
  arrives pre-filled in the bookmark form.

## Theme

On the [Config](Config) page, *Theme* selects **Auto** (follow the
system's light/dark setting), **Light**, or **Dark**. The choice applies
immediately to every page after saving.

---

# Advanced

## Configuration reference

The [Config](Config) page edits `config.json`. Fields:

| Setting | Meaning |
|---------|---------|
| Server Port | TCP port of the built-in server (default `8080`). Takes effect after restart. |
| Admin Password | Full-access password for remote connections. |
| Guest Password | Read-oriented password for remote connections. |
| Author Name | Written into the `Author:` header of new pages. |
| Theme | Auto / Light / Dark, see [Theme](#theme). |
| Use Internal Editor | Off = hand files to an external editor instead. |
| Desktop External Cmd | Editor command used on desktop (e.g. `subl`, `code`). |
| Share on LAN | Serve other devices, see [Sharing on the LAN](#sharing-on-the-lan). Changing it restarts the application. |
| Git Servers | Up to five remote slots, see [Git synchronization](#git-synchronization). |

Two settings exist only in the file itself: `mime_types` (extension →
content-type overrides) and `force_pull_one_time` (arms a single forced
pull on next sync; normally managed by the app).

`config.json` is per-device: it is never committed or synchronized, so
each device keeps its own passwords, port and git keys.

## Git synchronization

OMN-Go synchronizes the whole storage directory with a git remote over
SSH.

**Setup.** On the [Config](Config) page fill one of the five server slots:
a name, the git URL (`git@host:user/repo.git`), the SSH **private** key
(paste the key body), and the key's password if it has one. Select the
slot's radio button to make it the active server, then save.

**Everyday use.**

- <i class="material-icons">cloud_download</i> **Download (pull)** fetches
  and merges the remote's changes into your local notes.
- <i class="material-icons">cloud_upload</i> **Upload (push)** shows the
  list of changed files and asks for a commit message, then commits and
  pushes. If the remote has newer commits, the push is rejected — pull
  first, then push again.

**Conflicts.** If a pull meets changes that cannot be merged automatically,
a dialog offers three choices:

- **Force Pull (Reset to Remote)** — makes local match the remote exactly.
  **Destructive:** local modifications to tracked files are overwritten,
  and local files that are neither tracked by git nor listed in
  `.gitignore` are deleted. `config.json` is never touched.
- **Mark Conflicts in Files** — writes both versions into the affected
  files between `<<<<<<<` / `=======` / `>>>>>>>` markers for manual
  resolution; edit the files, then upload.
- **Abort** — cancels; nothing has been changed yet.

**The Force checkbox** in the header arms the *next* pull or push as
forced (a forced pull behaves like Force Pull above; a forced push
overwrites remote history). It always asks for confirmation and disarms
itself after one use. Treat it as the emergency lever it is.

## Sharing on the LAN

By default the server answers **only this device** — with sharing off,
other machines cannot even open a connection.

To share your notes on the local network:

1. Set proper **admin and guest passwords** first.
2. Enable **Share on LAN** on the [Config](Config) page and save.
3. Confirm the restart prompt. The application restarts to re-bind the
   server (on Android the app closes — reopen it; on desktop the page
   reloads by itself).

On Android, while sharing is active a **persistent notification** shows
the exact address other devices should open (for example
`http://192.168.1.5:8080`) and offers a **Stop** button. The first start
with sharing enabled asks for notification permission and for an exemption
from battery optimization — grant both, or the server may stop answering
when the screen has been locked for a while. With sharing off, no
notification is shown and no permissions are requested.

Other devices open the shown address in a browser and log in with the
guest (read) or admin (full) password. **Security note:** anyone on your
network with a password can access your notes; there is no HTTPS, so treat
it as trusted-home-network sharing, not internet publishing.

## Raw HTML and JavaScript in pages

Markdown pages may contain raw HTML — that is how the
[icon](#material-icons-in-notes) and
[button](#buttons-and-shortcuts-inside-a-page) examples above work — and
`<script>` blocks, which is how the [Bookmarks](Bookmarks) page stores its
data. Two rules keep scripts well-behaved (see
[ScriptRules](ScriptRules) for details and examples):

- wrap code in a block scope (`{ ... }` or an IIFE) so variables from one
  page cannot collide with another page's scripts or with OMN-Go's own;
- remember the page is server-compiled once and then cached — scripts run
  on every view, so keep them idempotent.

Scripts run with full access to the page. Only put script blocks in your
notes that you understand and trust.

## Troubleshooting

- **Page shows stale content** — press
  <i class="material-icons">refresh</i> in the header; it recompiles the
  page from its Markdown source (it simply reloads with `?refresh=1`).
- **Server logs** — open the browser's developer console: backend log
  lines are streamed live and printed with a `[GO]` prefix. On Android,
  connect the device to a desktop Chrome via `chrome://inspect` to get the
  same console.
- **"Failed to bind" / port busy** — another program (or a leftover
  OMN-Go instance) occupies the port. Close it or change *Server Port* in
  [Config](Config) and restart.
- **Other devices cannot connect** — check, in order: *Share on LAN* is
  enabled **and** the app was restarted after enabling it; both devices
  are on the same network; the address and port match the Android
  notification; the password is correct.
- **Sharing stops when the phone screen is locked** — the battery
  optimization exemption was not granted. Grant it in system Settings →
  Apps → OMN-Go → Battery ("Unrestricted"), or disable and re-enable LAN
  sharing to be asked again.
- **No sharing notification on Android 13+** — notification permission
  was denied. Allow notifications for OMN-Go in system settings; sharing
  itself works either way, but without the notification there is no
  visible address or Stop button.
