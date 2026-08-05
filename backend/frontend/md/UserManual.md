Title: User Manual
Date: 2026-07-07 12:00:00
Category: System
Author: Mikhail Basov
Tags: Document, OMN-Go, OMN-Go app

# OMN-Go User Manual

Welcome to the OMN-Go manual. [Easy start](#easy-start) covers the two functions that need no editor: quick notes and bookmarks. Basics covers the everyday use of notes and pages. Advanced covers configuration, git synchronization, LAN sharing and troubleshooting. This page is a normal OMN-Go note. Open it in edit mode (<i class="material-icons">edit</i>) to see how each example on it is written.

## Table of contents

**Easy start**

- [The start page](#the-start-page)
- [Quick notes](#quick-notes)
- [Bookmarks](#bookmarks)

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
- [Android intents and Termux](AndroidIntents)
- [The Tags page](#the-tags-page)
- [Searching](#searching)
- [Theme](#theme)

**Advanced**

- [Configuration reference](#configuration-reference)
- [Git synchronization](#git-synchronization)
- [Database backups](#database-backups)
- [The file index](#the-file-index)
- [The Status page](#the-status-page)
- [Sharing on the LAN](#sharing-on-the-lan)
- [Raw HTML and JavaScript in pages](#raw-html-and-javascript-in-pages)
- [Your own CSS and JavaScript](#your-own-css-and-javascript)
- [Troubleshooting](#troubleshooting)

---

# Easy start

This part is for a user who only writes short notes and keeps links. This user does not open the editor. Most users run OMN-Go on Android, so each list gives the Android ways first.

## The start page

The [Welcome](Welcome) page is the start page. The <i class="material-icons">home</i> button of the header opens it. The Android application opens it at start.

A new installation shows two large buttons at the top of the page:

- **My Quick Notes** opens the [QuickNotes](QuickNotes) page. OMN-Go puts each note that you write with the <i class="material-icons">insert_comment</i> button on that page, with a timestamp.
- **My Bookmarks** opens the [Bookmarks](Bookmarks) page. OMN-Go puts each link that you save with the <i class="material-icons">bookmark_add</i> button on that page, with its tags and its notes.

The [QuickNotes](QuickNotes) page starts with one note. That note tells you how to write a quick note. The [Bookmarks](Bookmarks) page starts with one bookmark. That bookmark opens [How to use Bookmarks](BookmarksHowTo). Both texts are short. Both point to this part of the manual.

The buttons are two links in the Markdown source of the note. They are not part of the header. The start page is your page. Open it in edit mode (<i class="material-icons">edit</i>) to change the buttons, to move them, or to delete the `<div class="omn-start-buttons">` block.

An installation from an older version keeps its own start page. To put the buttons on that page, copy this block into it:

```
<div class="omn-start-buttons">
<a class="omn-start-button" href="QuickNotes">
<i class="material-icons omn-start-icon">insert_comment</i>
<span class="omn-start-text"><span class="omn-start-label">My Quick Notes</span><span class="omn-start-hint">Write it down now. Sort it later.</span></span>
</a>
<a class="omn-start-button omn-start-button-bookmarks" href="Bookmarks">
<i class="material-icons omn-start-icon">bookmark</i>
<span class="omn-start-text"><span class="omn-start-label">My Bookmarks</span><span class="omn-start-hint">Keep a link. Find it again.</span></span>
</a>
</div>
```

Do not put an empty line in the block. An empty line ends a raw HTML block. The rest of the block then shows as text.

## Quick notes

A quick note is one short text with a timestamp. OMN-Go puts every quick note on the [QuickNotes](QuickNotes) page. The newest note is at the top. OMN-Go writes over no note, and you give no name to a note.

**With the Android icon.** The Android application installs a second icon, *OMN-Go Quick Note*. This icon opens the start page with the note box open.

**With Android sharing.** Press *Share* in a different application and select OMN-Go. A text without a link goes to the note box. A text with a link goes to the bookmark form. See [Bookmarks](#bookmarks).

**With the button.** This method works on each device.

1. Press the title of the page to expand the header.
2. Press <i class="material-icons">insert_comment</i>.
3. Write the text and press *Save*.

*Copy* puts the text on the clipboard and saves nothing. Use *Copy* when a text came into the box from a share or from a scan and you only want to paste it into a different application.

**With a browser bookmark on a desktop.** A query in the address also opens the note box. Make a bookmark of this address in your browser. Open that bookmark when you want to write a note. Use the port of your server:

```
http://localhost:8080/Welcome.html?quicknote=1
```

The [QuickNotes](QuickNotes) page is a normal note. Open it in edit mode (<i class="material-icons">edit</i>) to change or to delete a note on it. You can also move a note from it into a note of its own.

## Bookmarks

A bookmark is a link with a title, tags and notes. OMN-Go keeps your bookmarks in the [Bookmarks](Bookmarks) note, not in a browser. The bookmarks stay with your notes. They go to each device that you synchronize.

### To save a link

**With Android sharing.** Press *Share* in your browser or in a different application. Select OMN-Go. The bookmark form opens with the address and the title in it.

**With the button.** This method works on each device.

1. Press the title of the page to expand the header.
2. Press <i class="material-icons">bookmark_add</i>.
3. Write the address. A title, tags and notes are not necessary.
4. Press *Save*.

Write a comma between two tags (`work, recipe`). Write a semicolon between two notes. The *Tags* field shows the tags that you used before.

**With drag and drop on a desktop.** Drag a link from your browser and drop it on an OMN-Go page. The bookmark form opens with the address in it. If the browser sends a title, the form also shows the title.

**With a browser bookmark on a desktop.** A bookmark in your browser can send the page that you read to OMN-Go. Make a bookmark and write this text in its address field. Use the port of your server:

```
javascript:window.open('http://localhost:8080/Welcome.html?share_text='+encodeURIComponent(location.href)+'&share_subject='+encodeURIComponent(document.title));
```

### To find a link

Open the [Bookmarks](Bookmarks) page:

- Write a word in the search box and press *Search*. The page shows only the bookmarks with that word.
- Press a tag button to show only the bookmarks with that tag. Press *All* to show all bookmarks again.
- Press the number in the top right corner to show or hide the list of tags.
- Press **ⓘ** before a bookmark to show its tags and its notes. *Expand* and *Collapse* do this for all bookmarks.

If you enable global search in the [settings](#configuration-reference), the header search (<i class="material-icons">search</i>) also finds bookmarks together with your notes.

# Basics

## First run and where your data lives

OMN-Go keeps everything in one storage directory:

- **Android:** `/storage/emulated/0/Android/media/net.basov.omngo`
- **Desktop:** `./data` next to the executable

Inside it:

- `md/` — your notes as Markdown files. **This is the source of truth.**
  You back up this directory, and git synchronizes it.
- `html/` — compiled pages plus static assets (`css/`, `js/`, `images/`,
  `user_json/`). The compiled `.html` files are the HTML cache. OMN-Go
  rebuilds a page when the matching `.md` file is newer.
- `config.json` — the settings of this device (see
  [Configuration reference](#configuration-reference)). The file stays
  local to the device. Git never synchronizes it.

On first start, OMN-Go creates the storage directory, a default
`config.json`, and a few starter pages ([Welcome](Welcome),
[QuickNotes](QuickNotes), [Bookmarks](Bookmarks),
[BookmarksHowTo](BookmarksHowTo), [ScriptRules](ScriptRules),
[Editor](Editor)).

## Login and roles

You set two passwords on the [Config](Config) page:

- **Admin** — full access. The admin can edit and save notes, use git
  synchronization, and change the configuration.
- **Guest** — read-oriented access for other people on your network.

A local connection (`127.0.0.1` or `localhost`) skips the login. The
WebView of the Android application also makes a local connection. The
passwords apply when you enable [LAN sharing](#sharing-on-the-lan) and a
remote caller connects from another device.

**Change the default passwords before enabling LAN sharing.** A fresh
install ships with `admin_secret_changeme` and
`guest_secret_changeme`. Anyone on your network who read this manual
knows these two passwords.

## The interface

Press the page title to expand the header bar. The header bar has these
buttons, from left to right:

- <i class="material-icons">home</i> — open the [Welcome](Welcome) page
- <i class="material-icons">note_add</i> — create a new page
- <i class="material-icons">insert_comment</i> — add a quick note
- <i class="material-icons">bookmark_add</i> — add a bookmark
- <i class="material-icons">refresh</i> — force-recompile the current page
- <i class="material-icons">settings</i> — open the [Config](Config) page
- <i class="material-icons">cloud_download</i> / <i class="material-icons">cloud_upload</i> — git pull / push
  - **Force** checkbox — makes the next git sync action destructive (see [Git synchronization](#git-synchronization))
- <i class="material-icons">info</i> — show the metadata of the page
- <i class="material-icons">save</i> / <i class="material-icons">edit</i> — save the note / enable or disable edit mode

OMN-Go hides the admin-only buttons when you log in as guest.

## Creating a new page

1. Press <i class="material-icons">note_add</i> in the header.
2. Enter a title. OMN-Go suggests a file-safe name.
3. Confirm the name, or adjust it.

The page then opens in edit mode with a prefilled header block.

The name you confirm sets where OMN-Go creates the new page:

- A bare name like `Ideas` becomes a **sibling of the current page**. If
  you create `Ideas` while you view `projects/Plan`, OMN-Go makes
  `projects/Ideas`.
- OMN-Go uses a name with a slash, like `work/Ideas`, as written.
- A leading slash, like `/Ideas`, forces a top-level page.

OMN-Go adds a link to the new page at the end of the page you started
from.

## Editing pages

1. Press <i class="material-icons">edit</i> to open the editor.
2. Make your changes.
3. Press <i class="material-icons">save</i>.

When you save, OMN-Go updates the `Modified:` header line and recompiles
the page. The editor is a page of its own. It loads the Markdown source of
the note when it opens. Its small toolbar has an Emmet-style HTML expander
and a select-current-line button. See
[The editor and Emmet](Editor) for the toolbar and the abbreviation
syntax.

**Internal vs. external editor.** If you disable *Use Internal Editor* on
the [Config](Config) page, the edit button sends the file to an external
editor. On the desktop application, OMN-Go runs the command from *Desktop
External Cmd*, for example `subl`. On the Android application, OMN-Go
opens the system app-chooser. When you come back, the page reloads with
your changes.

**Images.** Drag an image file onto the editor area. OMN-Go uploads the
file to `images/`. OMN-Go then puts a Markdown image reference at the
cursor.

**Find and replace.** Press <i class="material-icons">search</i> in the
editor toolbar, or press **Ctrl-F**. To start with the replace field
shown, press **Ctrl-H**. The find bar opens between the toolbar and the
text. It pushes the note down and does not cover it.

The editor highlights every match. It puts a ring around the current match
and shows a **3 / 17** counter. Press **Enter** to move to the next match.
Press **Shift-Enter** to move to the previous match. Press **Esc** to
close the find bar.

Three switches enable case sensitivity, whole-word matching and regular
expressions. **Replace** changes one match, and **All** changes every
match. One **Ctrl-Z** undoes either change. The find bar searches the
Markdown source, which is the text you edit. It finds header lines and the
contents of code blocks like any other text. For full details, see
[The editor and Emmet](Editor#find-and-replace).

## Page format: the header block

Every page starts with a header block. The header block contains
`Key: value` lines, and the first blank line ends it. Example:

```
Title: Shopping list
Date: 2026-07-07 10:00:00
Modified: 2026-07-07 12:30:00
Author: Me
Category: Home
Tags: shopping, home

First line of the actual note...
```

- `Title` sets the page title. OMN-Go shows this title in the browser tab
  and in the page header.
- OMN-Go updates `Modified` on every save. You never edit this line.
- `Tags` renders each tag as a pill in the page header. Press a pill to open
  that tag on the [Tags page](#the-tags-page), which OMN-Go generates
  automatically.
- OMN-Go also makes a `<meta>` tag in the compiled page from each header
  line.
- If you save a page with no header block, OMN-Go adds a minimal one. It
  contains `Title`, `Date`, `Modified`, and `Author` from your
  configuration.

## Markdown in a nutshell

You write pages in Markdown with GitHub-flavored extensions (tables,
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

Line breaks are literal. One newline in the editor makes a line break in
the page. For all other syntax, use the
[GitHub Markdown guide](https://docs.github.com/en/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax).
It is close to the Markdown dialect of OMN-Go.

## Links: absolute, relative, external

When OMN-Go compiles a page, it normalizes the internal links. You can
write them in a natural form:

- `[Notes](Ideas)` — **bare page name**. OMN-Go resolves the name relative
  to the current page, like a file in the same folder, and adds `.html`.
- `[Notes](Ideas.md)` — the same result. OMN-Go rewrites `.md` to `.html`.
- `[Up](../Plan)` and `[Here](./Ideas)` — **relative paths** work exactly
  like in a file system.
- `[Top](/Welcome)` — a **leading slash** makes the link absolute from the
  notes root. The location of the linking page does not change this.
- `[Section](#some-heading)` and `[Page section](Ideas#some-heading)` —
  OMN-Go keeps the anchors. Each heading gets an ID from its text, in
  lowercase and with hyphens.
- OMN-Go does not change links with a real file extension (`.png`, `.css`,
  `.js`, ...) or external links (`http://`, `https://`, `mailto:`,
  `tel:` ...). On the Android application, an external link opens in the
  system browser.

## Edit links for non-page files

To open any file that OMN-Go serves in the editor, add `?edit=true` to its
URL. With this URL you can keep "edit me" shortcuts in your notes for
static assets:

```
[Edit my stylesheet](/css/custom.css?edit=true)
[Edit shared data](/user_json/inventory.json?edit=true)
```

The link opens the raw file in the internal editor. If you configured an
external editor, the link opens the file there. When you save, OMN-Go
writes the file back in place. A file that comes with OMN-Go opens with
its content, also when the storage directory does not hold that file yet.
If the file does not exist, the editor opens an empty page. Your first
save makes the file.

If the editor cannot read the file, it shows a red message and turns the
*Save* button off. This keeps an empty editor from replacing a file that
has content.

OMN-Go does not open a picture, a font, an audio file or a video file in
an editor. An editor writes text, and a save would damage such a file.
The link answers with a short page and a link to the file. The
[file index](OMNGoFiles) gives no edit link for these files.

If you do not know the name of a file, open
[the file index](OMNGoFiles). It lists every file that OMN-Go serves and
gives the same link ready-made.

## Math and code highlighting

OMN-Go renders formulas with KaTeX, fully offline. Math rendering is
**opt-in per page**. To enable it, put this line anywhere on the page. A
good habit is to put the line at the top of the body, directly after the
header block:

```
<script>var OMN_GO_KATEX=true</script>
```

Without that line, `$...$` stays literal text. Pages about money
(`$5 and $10`) thus do not change into formulas. This manual page has the
flag set. Here is a live example. The inline text `$E = mc^2$` renders as
$E = mc^2$. A display block follows:

<script>var OMN_GO_KATEX=true</script>

- inline: `$E = mc^2$`
- display block:

```
$$
\frac{a}{b} = \sum_{i=1}^{n} x_i
$$
```

OMN-Go protects underscores and other Markdown-sensitive characters inside
`$...$` and `$$...$$`. Write normal TeX.

OMN-Go highlights the syntax of a fenced code block that has a language
name:

````
```go
func main() { fmt.Println("hi") }
```
````

## Material icons in notes

OMN-Go ships with the Google Material Icons font. You can put any icon in
a note with a small piece of inline HTML:

```
<i class="material-icons">home</i>
<i class="material-icons">lightbulb</i> An idea!
<i class="material-icons" style="font-size:48px;color:#28a745;">check_circle</i>
```

renders as: <i class="material-icons">home</i>
<i class="material-icons">lightbulb</i> An idea!
<i class="material-icons" style="font-size:48px;color:#28a745;">check_circle</i>

Find icon names at [fonts.google.com/icons](https://fonts.google.com/icons).
Select the "Material Icons" style. Use the snake_case name.

## Buttons and shortcuts inside a page

The header buttons call JavaScript functions. These functions are
available on every page. You can put your own buttons anywhere in a note:

```
<button onclick="document.getElementById('bmPanel').classList.remove('hidden')">
  <i class="material-icons">bookmark_add</i> Add bookmark
</button>

<button onclick="document.getElementById('quickPanel').classList.remove('hidden')">
  <i class="material-icons">insert_comment</i> Add quick note
</button>
```

For navigation, plain links are usually better than buttons. The header
has no *Quick notes* button. The Quick Notes page is a normal page, so
make a link to it:

```
[Quick notes](/QuickNotes)
[Bookmarks](/Bookmarks)
```

You can also make a link look like a button:

```
<a href="/QuickNotes.html"><button>
  <i class="material-icons">insert_comment</i> Quick notes
</button></a>
```

Try it: [Quick notes](/QuickNotes) · [Bookmarks](/Bookmarks)

**Android intents (Android only).** On the Android application, a link or
a button can do more. It can open a system screen, start another Android
application, run a Termux command, or scan a barcode into the Quick Notes
page. OMN-Go disables this function by default. See
[Android Intents & Termux](AndroidIntents).

## The Tags page

The Tags page (the note `OMNGoTags`) indexes every note that has a `Tags:`
header line. The page shows an alphabetical cloud of all tags
at the top. Below the cloud it shows one section per tag, with links to
the notes that use the tag. To open the Tags page, press a tag pill in a
page header. The pill opens the section of that tag.

OMN-Go **generates the page automatically**. It rebuilds the page at
startup and each time the tags of a note change. Do not edit the page by
hand. A "do not edit" comment at the top gives this warning, and the next
rebuild overwrites your changes. The page is plain static HTML with no
note scripts, so it also works when you open the compiled `html/` tree
offline. OMN-Go omits the notes that have no tags.

### The tags of the pages that come with OMN-Go

Each page that comes with OMN-Go has the tag `OMN-Go`. A second tag says who owns the page:

- **`OMN-Go app`** — the application owns the page. An update writes the shipped text over your version of it. OMN-Go first copies your version to `asset_backups/<previous version>/md/`, and writes the path in the log. This manual and the other documentation pages have this tag.
- **`OMN-Go user`** — you own the page. OMN-Go writes the page one time: at the first start, or at the first view. After that an update keeps your text. The [Welcome](Welcome) page, the [Quick Notes](QuickNotes) page, the [Bookmarks](Bookmarks) page and the test pages have this tag.

Press a tag pill in the header of such a page to see the full list. The [Tags](OMNGoTags) page also holds the three sections.

Before you write in a page with the tag `OMN-Go app`, copy the text into a page of your own. Your notes are safe: OMN-Go touches only the pages that come with it.

The other files use the same two words. The [file index](OMNGoFiles) marks each file *app-owned* or *user-owned*. `css/omn-go-custom.css` and `js/omn-go-custom.js` are user-owned, like a page with the tag `OMN-Go user`.

## Searching

To open the search panel, press the magnifier in the page header. On a
keyboard, press **Ctrl-K** or **/**.

The search panel searches your notes as OMN-Go publishes them. To search
the text of the note that you edit, use the find bar of the editor. The
find bar also searches the header lines and the contents of the code
blocks. See [Find and replace](Editor#find-and-replace).

There are two searches. The chips at the top of the panel show which
search you use:

- **This page** — this is page search. It searches the note you have open.
  It always works and needs no setting. It costs nothing, because OMN-Go
  reads the file for that one query and then forgets it.
- **All notes** — this is global search. It searches all notes at one
  time. Global search needs the search index. OMN-Go shows this chip only after you
  enable **Enable global search** under
  [Config → Search](Config#cfg-search). Until then, the
  [search page](OMNGoSearch) explains this and links directly to the
  setting. A *Search* link on one of your own notes thus works in both
  cases.

### What it matches

Your query does not have to be exact. OMN-Go tries each term of the query
on three rungs, best first:

1. the term as written — `json` finds `json`
2. the letters in order — `andint` finds **And**roid **Int**ents
3. a near miss — `fecth` finds `fetch`. A swapped pair of letters is the
   most common typing error.

All terms must match. The query `fetch json` finds the notes that contain
both terms. To limit a term to one part of a note, use `title:`, `tag:` or
`path:`. An example is `tag:recipe bread`.

OMN-Go ignores case and accents in every script. The query `елка` finds
`Ёлка`.

### Reading the results

Each line shows where the match is, and highlights the match. A `‹/›` mark
tells you that the line is in a note script or in a code block, and not in
prose. OMN-Go does not rank such a line lower, because code in notes is a
normal thing to look for.

If a note has sections, the result names the section and not the line. A
quick note shows its timestamp. A bookmark shows its title. A note with
headings shows the heading. When you open the result, OMN-Go opens that
section, and not the top of a long page.

"Line 1842" does not answer the question *where*. "27 Jul, 07:23" answers
it.

OMN-Go searches bookmarks by what you see: the title, the address, the
tags and the notes. It does not search the stored form. This is important.
OMN-Go writes a bookmark with the title *Cats & Dogs* to disk with the `&`
encoded. Before this change, you could not find that bookmark by its own
name.

When you press a result in **This page**, OMN-Go closes the search panel.
It marks every occurrence in the page and moves to the first one. When you
press a result in **All notes**, OMN-Go opens the note of the match. The
note is already scrolled to the match, and the terms you typed are marked.
You do not have to find them again by eye.

The full [search page](OMNGoSearch) behaves the same way. The *See all
results* link opens that page. The search page is easier to read for a
long list. It is also an ordinary URL that you can bookmark or send to
another person.

OMN-Go removes the highlight from the address bar directly after it
applies the marks. The URL that you copy or bookmark is thus the plain
one. A reload of the page does not put the marks back.

In one case OMN-Go marks nothing. A term that matched only on a lower
rung, such as `fecth` for `fetch`, is not in the note in the form you
typed. There is thus nothing to mark. The result list has already shown
you the lines that matched.

### What it costs

Page search costs nothing. Global search keeps the search index in memory.
The index is approximately half the size of the text it covers, which is
about 3 MB for 2000 notes. The Config page shows the current figure. On a
device with little memory, keep global search disabled. Page search works
in both cases.

There are two limits. First, OMN-Go searches only the first 500 KiB of a
very large file. The results say so when this happens. Second, OMN-Go
cannot link directly to some sections. Examples are a heading in a
non-Latin script, and a heading that contains a link, code or a formula.

These results still name the section. They open the page at the top. This
behavior is deliberate. OMN-Go does not make a link when the link can send
you to the wrong place.

## Theme

On the [Config](Config) page, *Theme* selects **Auto**, **Light**, or
**Dark**. **Auto** follows the light or dark setting of the system. When
you save, the choice applies immediately to every page.

---

# Advanced

## Configuration reference

The [Config](Config) page edits `config.json`. It has these fields:

| Setting | Meaning |
|---------|---------|
| Server Port | TCP port of the OMN-Go server (default `8080`). It takes effect after a restart. |
| Admin Password | Full-access password for remote callers. |
| Guest Password | Read-oriented password for remote callers. |
| Author Name | OMN-Go writes this name into the `Author:` line of the header block of a new page. |
| Theme | Auto / Light / Dark, see [Theme](#theme). |
| Use Internal Editor | If you disable it, OMN-Go sends the files to an external editor. |
| Desktop External Cmd | Editor command for the desktop application (for example `subl` or `code`). |
| Share on LAN | Serve other devices, see [Sharing on the LAN](#sharing-on-the-lan). A change of this setting restarts the application. |
| Hostname | Device label in the database backup filenames (see [Database backups](#database-backups)). The default is the OS hostname. On Android, set a short name like `phone`. |
| Backup Prune Depth | How many backups to keep per database (default `3`). When you create a backup, OMN-Go deletes the oldest backups above this count. |
| Max Upload Size (MB) | Largest image or JSON file that you can drag into the editor or share into the Quick Notes page (default `3`). |
| Git Servers | Up to five git server slots, see [Git synchronization](#git-synchronization). |
| Fullscreen mode (Android) | Which system bars the Android application hides. *Off* shows the status bar. *Fullscreen* hides the status bar and is the default. *Immersive* hides the status bar and the navigation bar. Swipe from an edge to show them for a short time. The setting applies as soon as you save. Android only. |
| Enable intent: links (Android) | Lets a note start Android `intent:` URIs when you press the link. Disabled by default. Android only. See [Android Intents & Termux](AndroidIntents). |
| Enable Termux commands (Android) | Also lets notes run Termux shell commands. You must enable *Enable intent: links* too. Disabled by default. See [Android Intents & Termux](AndroidIntents). |

Two settings exist only in the file. `mime_types` holds the
extension-to-content-type overrides. `force_pull_one_time` makes the next
git sync do one forced pull. OMN-Go normally manages
`force_pull_one_time`.

`config.json` belongs to one device. OMN-Go never commits or synchronizes
it. Each device keeps its own passwords, port and git keys.

## Git synchronization

OMN-Go synchronizes the whole storage directory with a git remote over
SSH.

**Setup.**

1. Open the [Config](Config) page.
2. Enter a name and the git URL (`git@host:user/repo.git`) in one of the
   five git server slots.
3. Paste the body of the SSH **private** key into the same slot.
4. Enter the password of the key, if the key has one.
5. Select the radio button of the slot to make it the active git server
   slot.
6. Save the configuration.

**Everyday use.**

- <i class="material-icons">cloud_download</i> **Download (pull)** pulls
  the changes of the remote into your local notes.
- <i class="material-icons">cloud_upload</i> **Upload (push)** shows the
  list of changed files and asks for a commit message. It then commits and
  pushes. If the remote has newer commits, the remote rejects the push.
  Pull first, then push again.

**Conflicts.** If a pull finds changes that git cannot merge
automatically, a dialog gives three choices:

- **Force Pull (Reset to Remote)** — makes the local notes match the
  remote exactly. **Destructive:** OMN-Go overwrites your local changes to
  tracked files. It deletes the local files that git does not track and
  `.gitignore` does not list. OMN-Go never touches `config.json`.
- **Mark Conflicts in Files** — writes both versions into the affected
  files, between the `<<<<<<<`, `=======` and `>>>>>>>` markers. Edit the
  files, then upload them.
- **Abort** — cancels the pull. Nothing has changed.

**The Force checkbox** in the header makes the *next* pull or push forced.
A forced pull behaves like Force Pull above. A forced push overwrites the
history on the remote. The checkbox always asks for confirmation. It
disables itself after one use. Use it only in an emergency.

### Files that stay on this device

Give a file or a directory a name that starts with `local-`. OMN-Go then
keeps it out of git. The file stays on this device, and it does not go to
the other devices.

| Name | Result |
| --- | --- |
| `html/user_json/local-data.json` | The data file stays here |
| `md/local-drafts/Monday.md` | Each note in the directory stays here |
| a database with the name `local-notes` | Each backup of it stays here |

The rule looks at the name of the file and at the name of each directory
above it. The name must **start** with `local-`, and the capitals count.
A file with the name `mylocal-data.json` is thus a normal file that goes
to the other devices.

A file with such a name is also safe from **Force Pull**. That command
deletes a local file that git does not track, but it keeps a file that
`.gitignore` matches.

If you give the `local-` name to a file that the other devices already
have, the next upload removes that file from git. The file stays on this
device. The next download deletes the copy on each other device. The
upload dialog shows the name of such a file before you write the commit
message.

The older rule `/md/local/` stays. Each note in that one directory also
stays on this device.

## Database backups

A note script can store data in a real SQL database. See the
[Database](Database) page for the API. These databases are outside git.
OMN-Go does **not** synchronize them with your notes automatically. To
move the contents of a database between devices, or to keep a copy, create
a **backup**.

1. Open the [Config](Config) page.
2. Select **DB Backups**.
3. Press **Database Backups**.

For each database you can then:

- **Backup now** — writes a backup file under `html/db_backup/`. The file
  is beside your notes, so the normal
  <i class="material-icons">cloud_upload</i> Upload commits and pushes it.
  The backup reaches your other devices on their next pull.
- **Restore** — replaces a full database with a backup that you select. A
  confirmation dialog shows what you overwrite.

A colored dot shows the state of each database (in sync, not backed up,
backup newer, no backups, ...). A new device has no database file. On such
a device, OMN-Go restores the newest backup automatically the first time a
note opens the database. Two related settings are on the
[Config](Config) page under **DB Backups**. **Hostname** labels this
device in the backup filenames. **Backup Prune Depth** sets how many
backups to keep per database.

OMN-Go backs up a database whose name starts with `local-` on the device,
but keeps it out of git.

To load data from an existing SQL dump, use the [SQL Import](SQLImport)
note. Such a dump is a `sqlite3 .dump` file or old `websqldump.js` output.
Then press **Backup now**. See the [Database](Database) page for the full
reference.

## The file index

The [file index](OMNGoFiles) lists every file that OMN-Go can serve. It
shows one directory at a time, like a browser that shows a `file:///`
folder. A trail of links leads up, and the subdirectories are links down.
The page is **admin-only**. From another device on the network, you must
log in as admin. From the device itself, you are always admin.

OMN-Go shows each directory two times, because these are two different
questions:

- **Embedded in the application** — the files that this build of OMN-Go
  ships. Each row shows whether OMN-Go wrote that file to disk (*on disk* /
  *not yet*). This section has no **edit** link, because an edit always
  operates on the copy on the device.
- **On this device** — the files that the device stores. Each row shows
  the size, the day when OMN-Go last wrote the file, and an **edit** link
  where an edit makes sense. Point at the date to see the time also.

A row of either section shows **app-owned** when the next version of
OMN-Go replaces that file. OMN-Go backs up your copy first, but it stops
using your changes. The stylesheets and scripts of the frontend are
app-owned. A row with no such word is your file: OMN-Go extracted it one
time and then never changes it, and you can edit it safely.

The size, the state and the **edit** link stay together on the right of a
row. OMN-Go shortens no name. A name that is longer than the space that is
left takes two lines, and the rest of the row stays where it is.

The count and the size of a directory cover all files below it, and not
only the rows you see. The numbers thus answer the question "how big is
this whole folder". A very large directory shows its first 200 files. It
also shows how many files it holds back, with a link to show all of them.

A file on the device that you can edit as text has an **edit** link. The
link uses the `?edit=true` form from
[Edit links for non-page files](#edit-links-for-non-page-files), with the
URL already filled in. Images, fonts, sounds and video have no edit link,
because the editor would only damage them. Pages with the `.html`
extension also have no edit link. Open such a page and use the normal Edit
button.

A shipped file that says *not yet* is in no device row, thus it has no
edit link. Open the file with its own link first. OMN-Go writes it to the
device at that moment, and the row with the edit link comes after a
reload of the index.

Two things never appear in the file index. The page templates are part of
OMN-Go and not part of your data. The `db_backup/` folder appears on the
[Database backups](#database-backups) screen instead.

The page only reads. Nothing on it deletes, moves, or creates a file.

## The Status page

The Status page tells what OMN-Go does now. Open it from the last line of the [Config](Config) menu, or open [Status](OMNGoStatus) here. The page is for the admin of the device. A guest sees a short note instead.

The page reads the `/api/status` endpoint and shows what comes back:

- **Server** — the port of the server, and the addresses that another device on your network can open. It also gives the time since the start.
- **Configuration** — the settings that change behavior, for example the editor, the theme and the upload limit.
- **Git** — the branch, the commit that your notes are at, and the remote. It also gives the commit of the remote from your last synchronization. OMN-Go asks no server for it, so it can be older than the server. The password of a remote never appears.
- **Search** — how many notes the index holds, how large it is, and when OMN-Go built it.
- **Runtime** and **Android** — the Go version, the memory in use, and the package name of the Android application.

Two parts cost time, so the page loads them when you ask:

- **Storage** counts the files in the storage directory.
- **Git worktree** reads the state of each tracked file.

Press the button of the part that you want. A progress bar runs while OMN-Go reads.

*Copy as Markdown* puts the full text on the clipboard. Use it in a bug report. *Open as text* and *Open as JSON* show the same facts in the browser.

## Sharing on the LAN

By default, the server answers **only this device**. With LAN sharing
disabled, other devices cannot open a connection.

To share your notes on the local network:

1. Set your own **admin and guest passwords** first.
2. Enable **Share on LAN** on the [Config](Config) page.
3. Save the configuration.
4. Confirm the restart prompt.

The application restarts to re-bind the server. The Android application
closes. Open it again. On the desktop application, the page reloads by
itself.

On Android, a **persistent notification** shows while LAN sharing is
active. It gives the exact address that other devices must open, for
example `http://192.168.1.5:8080`. It also has a **Stop** button. The
first start with LAN sharing enabled asks for the notification permission
and for an exemption from battery optimization. Grant both. If you do not,
the server may not answer when the screen stays locked for some time.

With LAN sharing disabled, OMN-Go shows no notification and asks for no
permissions.

On another device, open the address in a browser. Log in with the guest
password for read access, or with the admin password for full access.
**Security note:** Any person on your network who has a password can
access your notes. OMN-Go has no HTTPS. Use LAN sharing only on a trusted
home network. Do not use it to publish on the internet.

## Raw HTML and JavaScript in pages

Markdown pages can contain raw HTML. The
[icon](#material-icons-in-notes) and
[button](#buttons-and-shortcuts-inside-a-page) examples above use raw
HTML. Pages can also contain note scripts. The [Bookmarks](Bookmarks) page
stores its data in a note script. Two rules keep note scripts correct. See
[ScriptRules](ScriptRules) for details and examples:

- Put your code in a block scope (`{ ... }` or an IIFE). Then the
  variables of one page cannot collide with the note scripts of another
  page, or with the scripts of OMN-Go.
- The backend compiles the page one time and then caches it. The note
  scripts run on every view, so make them idempotent.

Note scripts run with full access to the page. Put only note scripts that
you understand and trust in your notes.

## Your own CSS and JavaScript

Two files hold your own changes to the application:

- `/css/omn-go-custom.css`
- `/js/omn-go-custom.js`

Each page loads the two files. The stylesheet is the last one in the head. The script is the last one before the end of the body. This order gives you control. Your rule wins over a rule of the application that has the same specificity. Your code can use each function that the application scripts define.

To open a file, use its edit link. Example: `[My styles](/css/omn-go-custom.css?edit=true)`. The [Edit not .md](/Test/OMN-Go/EditNot-md) test note has a link to each of the two files.

OMN-Go writes the two files one time. Each file gets a comment that says what the file is for. After that the files are yours. An update keeps your version. Git synchronization copies the files to your other devices.

To change one color everywhere, set the design token in your stylesheet. The tokens are at the top of `css/omn-go-core.css`:

```
:root { --accent: #8e44ad; }
```

Put your code in a block scope, as in a note script (see [Scripting Rules](ScriptRules)). A `var` at the top level of the file becomes a global name on each page.

One page is different. The [Bookmarks](Bookmarks) page loads `css/Bookmarker.css` from the note. The browser reads that stylesheet after the head. To change that page, give your rule a higher specificity.

**The editor page does not load the two files.** This is deliberate. A rule that hides a control cannot keep you out of the editor. An error in your code cannot keep you out of the editor. The editor always opens, and you repair the file there.

## Troubleshooting

- **Page shows stale content** — press
  <i class="material-icons">refresh</i> in the header. The button
  recompiles the page from its Markdown source. It reloads the page with
  `?refresh=1`.
- **Server logs** — open the developer console of the browser. The backend
  streams its log lines live and prints them with a `[GO]` prefix. On
  Android, connect the device to a desktop Chrome with `chrome://inspect`
  to get the same console.
- **"Failed to bind" / port busy** — another program, or an old OMN-Go
  instance, occupies the port. Close that program. As an alternative,
  change *Server Port* in [Config](Config) and restart the application.
- **Other devices cannot connect** — check these points in order. *Share
  on LAN* is enabled **and** you restarted the application after you
  enabled it. Both devices are on the same network. The address and the
  port agree with the Android notification. The password is correct.
- **Sharing stops when the phone screen is locked** — you did not grant
  the battery optimization exemption. Grant it in system Settings → Apps →
  OMN-Go → Battery ("Unrestricted"). As an alternative, disable LAN
  sharing and enable it again to get the question a second time.
- **No sharing notification on Android 13+** — you denied the notification
  permission. Allow notifications for OMN-Go in the system settings. LAN
  sharing works in both cases. Without the notification there is no
  visible address and no Stop button.
