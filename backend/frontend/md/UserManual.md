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
- [Text files beside your notes](#text-files-beside-your-notes)
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
- [Send and receive one note](#send-and-receive-one-note)
- [Database backups](#database-backups)
- [The file index](#the-file-index)
- [The Status page](#the-status-page)
- [Sharing on the LAN](#sharing-on-the-lan)
- [Raw HTML and JavaScript in pages](#raw-html-and-javascript-in-pages)
- [Your own CSS and JavaScript](#your-own-css-and-javascript)
- [Troubleshooting](#troubleshooting)
- [Disclaimer](#disclaimer)

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

On first start, OMN-Go creates the storage directory, a default `config.json`, and a few starter pages ([Welcome](Welcome), [QuickNotes](QuickNotes), [Bookmarks](Bookmarks), [BookmarksHowTo](BookmarksHowTo), [ScriptRules](ScriptRules), [Editor](Editor)).

## Login and roles

You set two passwords on the [Config](Config) page:

- **Admin** — full access. The admin can edit and save notes, use git
  synchronization, and change the configuration.
- **Guest** — read-oriented access for other people on your network.

A local connection (`127.0.0.1` or `localhost`) skips the login. The WebView of the Android application also makes a local connection. The passwords apply when you enable [LAN sharing](#sharing-on-the-lan) and a remote caller connects from another device.

**Change the default passwords before enabling LAN sharing.** A fresh
install ships with `admin_secret_changeme` and `guest_secret_changeme`. Anyone on your network who read this manual knows these two passwords.

## The interface

Press the page title to expand the header bar. The header bar has these buttons, from left to right:

- <i class="material-icons">home</i> — open the [Welcome](Welcome) page
- <i class="material-icons">note_add</i> — create a new page
- <i class="material-icons">insert_comment</i> — add a quick note
- <i class="material-icons">bookmark_add</i> — add a bookmark
- <i class="material-icons">refresh</i> — force-recompile the current page
- <i class="material-icons">settings</i> — open the [Config](Config) page
- <i class="material-icons">cloud_download</i> / <i class="material-icons">cloud_upload</i> — git pull / push (see [Git synchronization](#git-synchronization))
- <i class="material-icons">info</i> — show the metadata panel of the page
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

OMN-Go adds a link to the new page at the top of the page you started from. The link goes directly below the header block, or at the start of a page that has no header block.

## Editing pages

1. Press <i class="material-icons">edit</i> to open the editor.
2. Make your changes.
3. Press <i class="material-icons">save</i>.

When you save, OMN-Go updates the `Modified:` header line and recompiles the page. The editor is a page of its own. It loads the Markdown source of the note when it opens. Its small toolbar has an Emmet-style HTML expander and a select-current-line button. See [The editor and Emmet](Editor) for the toolbar and the abbreviation syntax.

**Internal vs. external editor.** If you disable *Use Internal Editor* on
the [Config](Config) page, the edit button sends the file to an external editor. On the desktop application, OMN-Go runs the command from *Desktop External Cmd*, for example `subl`. On the Android application, OMN-Go opens the system app-chooser. When you come back, the page reloads with your changes.

**Images.** Drag an image file onto the editor area. OMN-Go uploads the
file to `images/`. OMN-Go then puts a Markdown image reference at the cursor.

**Find and replace.** Press <i class="material-icons">search</i> in the
editor toolbar, or press **Ctrl-F**. To start with the replace field shown, press **Ctrl-H**. The find bar opens between the toolbar and the text. It pushes the note down and does not cover it.

The editor highlights every match. It puts a ring around the current match and shows a **3 / 17** counter. Press **Enter** to move to the next match. Press **Shift-Enter** to move to the previous match. Press **Esc** to close the find bar.

Three switches enable case sensitivity, whole-word matching and regular expressions. **Replace** changes one match, and **All** changes every match. One **Ctrl-Z** undoes either change. The find bar searches the Markdown source, which is the text you edit. It finds header lines and the contents of code blocks like any other text. For full details, see [The editor and Emmet](Editor#find-and-replace).

## Page format: the header block

Every page starts with a header block. The header block contains `Key: value` lines, and the first blank line ends it. Example:

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

You write pages in Markdown with GitHub-flavored extensions (tables, strikethrough, task lists):

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

Line breaks are literal. One newline in the editor makes a line break in the page. For all other syntax, use the [GitHub Markdown guide](https://docs.github.com/en/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax). It is close to the Markdown dialect of OMN-Go.

## Links: absolute, relative, external

When OMN-Go compiles a page, it normalizes the internal links. You can write them in a natural form:

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
  `.js`, ...). It also does not change a link that starts with a scheme:
  `http://`, `https://`, `mailto:`, `tel:`, `sms:`, `geo:`, `market:`,
  `whatsapp:` and every other one. On the Android application such a link
  goes to the application that owns the scheme — the browser, the mail
  client, the Messaging application, Maps. A scheme that no installed
  application accepts does nothing.
- Do not put a `:` in a page name. `[Draft](Notes:Draft)` looks like a link
  with the scheme `notes:`, and OMN-Go sends it to the system.

### Copy a link to this page

OMN-Go writes the Markdown link to the page for you.

1. Press <i class="material-icons">info</i> in the header to show the metadata panel.
2. Press <i class="material-icons">link</i> **Copy a link to this page**, in the column of buttons at the right of the panel.
3. Open the note where you want the link, and paste.

The link holds the title of the page and the address of the page: `[Trip planning](/projects/Trip%20planning.html?tag=go#day-two)`.

The address starts at the notes root and holds no device name. The link is thus correct on each device that opens your notes. A link that carries `127.0.0.1` is correct on one device only.

If the address of the page holds parameters or an anchor, the link holds them too. Open the [Bookmarks](Bookmarks) page with `?tag=go`, copy the link, and the link opens the same list of bookmarks.

This button shows on each page. The two buttons above it, [Send this note](#send-a-note) and Copy this note as text, show on a note only.

## Edit links for non-page files

To open any file that OMN-Go serves in the editor, add `?edit=true` to its URL. With this URL you can keep "edit me" shortcuts in your notes for static assets:

```
[Edit my stylesheet](/css/custom.css?edit=true)
[Edit shared data](/user_json/inventory.json?edit=true)
```

The link opens the raw file in the internal editor. If you configured an external editor, the link opens the file there. When you save, OMN-Go writes the file back in place. A file that comes with OMN-Go opens with its content, also when the storage directory does not hold that file yet. If the file does not exist, the editor opens an empty page. Your first save makes the file.

If the editor cannot read the file, it shows a red message and turns the
*Save* button off. This keeps an empty editor from replacing a file that
has content.

OMN-Go does not open a picture, a font, an audio file or a video file in an editor. An editor writes text, and a save would damage such a file. The link answers with a short page and a link to the file. The [file index](OMNGoFiles) gives no edit link for these files.

If you do not know the name of a file, open [the file index](OMNGoFiles). A file that OMN-Go can show as text has an **edit** link there, with the address already in it. An image, a font, an audio file, a video file and a compiled `.html` page have no edit link. See [The edit link](#the-edit-link).

## Text files beside your notes

You can keep a plain text file with the notes that use it. Put `log.txt` in the `md/` directory, beside `Log.md`, then link to it from the note:

```
[The raw log](log.txt)
```

OMN-Go serves your notes from `md/`, and each other file from `html/`. A link is a URL, and a URL finds its file in `html/`. Thus OMN-Go keeps the text file in the two directories and keeps the two copies the same. Each direction has its own moment:

- At each start, OMN-Go copies `md/log.txt` to `html/log.txt` when `html/` has no copy, or when the copy in `md/` is more recent. A git synchronization, a file manager and a desktop editor all make a change in this direction.
- After you save the file in the editor, OMN-Go copies it back to `md/`. The editor writes to the location that the URL points to, which is `html/`.

Each copy keeps the modification time of its source. Thus the pair becomes stable: a start that comes after a save finds nothing to do.

The copy in `md/` is the one that git synchronization carries with your notes. `.gitignore` keeps each `.txt` file below `html/` out of git, because it is only a copy: two copies of one text in git is one too many, and the second one is what a merge conflict looks for. After a download, OMN-Go makes the copy in `html/` again, thus the link operates immediately.

Subdirectories stay: `md/project/data.txt` becomes `html/project/data.txt`.

Three limits to know:

- Only a `.txt` file is copied. Put a file of a different type in `html/`, where OMN-Go serves it directly.
- OMN-Go deletes nothing. If you delete `md/log.txt`, the copy in `html/` stays and the link continues to operate. Delete the two files.
- An external editor writes only to `html/`, and OMN-Go does not copy such a change to `md/` by itself. OMN-Go does see it: the [file index](OMNGoFiles) marks the file *edited outside*. Save the file one time in the internal editor to copy it to `md/`.

## Math and code highlighting

OMN-Go renders formulas with KaTeX, fully offline. Math rendering is **opt-in per page**. To enable it, put this line anywhere on the page. A good habit is to put the line at the top of the body, directly after the header block:

```
<script>var OMN_GO_KATEX=true</script>
```

Without that line, `$...$` stays literal text. Pages about money (`$5 and $10`) thus do not change into formulas. This manual page has the flag set, thus the two examples below are live.

<script>var OMN_GO_KATEX=true</script>

Inline math goes between one `$` on each side. Write:

```
The mass-energy relation is $E = mc^2$.
```

Renders to:

The mass-energy relation is $E = mc^2$.

A display block goes between two `$` on each side, on lines of its own. Write:

```
$$
\frac{a}{b} = \sum_{i=1}^{n} x_i
$$
```

Renders to:

$$
\frac{a}{b} = \sum_{i=1}^{n} x_i
$$

OMN-Go protects underscores and other Markdown-sensitive characters inside `$...$` and `$$...$$`. Write normal TeX.

OMN-Go highlights the syntax of a fenced code block that has a language name:

````
```go
func main() { fmt.Println("hi") }
```
````

## Material icons in notes

OMN-Go ships with the Google Material Icons font. You can put any icon in a note with a small piece of inline HTML:

```
<i class="material-icons">home</i>
<i class="material-icons">lightbulb</i> An idea!
<i class="material-icons" style="font-size:48px;color:#28a745;">check_circle</i>
```

renders as: <i class="material-icons">home</i>
<i class="material-icons">lightbulb</i> An idea!
<i class="material-icons" style="font-size:48px;color:#28a745;">check_circle</i>

Find icon names at [fonts.google.com/icons](https://fonts.google.com/icons). Select the "Material Icons" style. Use the snake_case name.

## Buttons and shortcuts inside a page

The header buttons call JavaScript functions. These functions are available on every page. You can put your own buttons anywhere in a note:

```
<button onclick="document.getElementById('bmPanel').classList.remove('hidden')">
  <i class="material-icons">bookmark_add</i> Add bookmark
</button>

<button onclick="document.getElementById('quickPanel').classList.remove('hidden')">
  <i class="material-icons">insert_comment</i> Add quick note
</button>
```

For navigation, plain links are usually better than buttons. The header has no *Quick notes* button. The Quick Notes page is a normal page, so make a link to it:

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
a button can do more. It can open a system screen, start another Android application, run a Termux command, or scan a barcode into the Quick Notes page. OMN-Go disables this function by default. See [Android Intents & Termux](AndroidIntents).

## The Tags page

The Tags page (the note `OMNGoTags`) indexes every note that has a `Tags:` header line. The page shows an alphabetical cloud of all tags at the top. Below the cloud it shows one section per tag, with links to the notes that use the tag. To open the Tags page, press a tag pill in a page header. The pill opens the section of that tag.

OMN-Go **generates the page automatically**. It rebuilds the page at startup and each time the tags of a note change. Do not edit the page by hand. A "do not edit" comment at the top gives this warning, and the next rebuild overwrites your changes. The page is plain static HTML with no note scripts, so it also works when you open the compiled `html/` tree offline. OMN-Go omits the notes that have no tags.

### The tags of the pages that come with OMN-Go

Each page that comes with OMN-Go has the tag `OMN-Go`. A second tag says who owns the page:

- **`OMN-Go app`** — the application owns the page. An update writes the shipped text over your version of it. OMN-Go first copies your version to `asset_backups/<previous version>/md/`, and writes the path in the log. This manual and the other documentation pages have this tag.
- **`OMN-Go user`** — you own the page. OMN-Go writes the page one time: at the first start, or at the first view. After that an update keeps your text. The [Welcome](Welcome) page, the [Quick Notes](QuickNotes) page, the [Bookmarks](Bookmarks) page and the test pages have this tag.

Press a tag pill in the header of such a page to see the full list. The [Tags](OMNGoTags) page also holds the three sections.

Before you write in a page with the tag `OMN-Go app`, copy the text into a page of your own. Your notes are safe: OMN-Go touches only the pages that come with it.

The other files use the same two words. The [file index](OMNGoFiles) marks each file *app-owned* or *user-owned*. `css/omn-go-custom.css` and `js/omn-go-custom.js` are user-owned, like a page with the tag `OMN-Go user`.

## Searching

To open the search panel, press the magnifier in the page header. On a keyboard, press **Ctrl-K** or **/**.

The search panel searches your notes as OMN-Go publishes them. To search the text of the note that you edit, use the find bar of the editor. The find bar also searches the header lines and the contents of the code blocks. See [Find and replace](Editor#find-and-replace).

There are two searches. The chips at the top of the panel show which search you use:

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

### How to start the search

Typing does not search. Type your query, then press the **magnifier button** at the right of the input field, or press **Enter**. This is the same button that the [search page](OMNGoSearch) has, in the same position.

A search of all notes reads every note that the index holds. A search at each key that you press does that work five or six times for one word and shows you only the last answer.

While the search operates, a bar below the input field moves from side to side. It makes no promise about the time, because the time depends on the quantity of your notes. The panel also shows *Searching…*. The bar goes away when the results come. You get the same bar when you press **All notes**, which searches immediately.

**Enter** does one of two things, and the field tells you which:

- The field shows the query of the results below it. Enter opens the result
  that you selected.
- You changed the field. Enter searches for the new text. The panel shows
  *Press ↵ or the magnifier to search* while the two do not agree.

The search page shows the progress indicator of the application while the server makes the results.

### What it matches

Your query does not have to be exact. OMN-Go tries each term of the query on three rungs, best first:

1. the term as written — `json` finds `json`
2. the letters in order — `andint` finds **And**roid **Int**ents
3. a near miss — `fecth` finds `fetch`. A swapped pair of letters is the
   most common typing error.

All terms must match. The query `fetch json` finds the notes that contain both terms. To limit a term to one part of a note, use `title:`, `tag:` or `path:`. An example is `tag:recipe bread`.

OMN-Go ignores case and accents in every script. The query `елка` finds `Ёлка`.

### Reading the results

Each line shows where the match is, and highlights the match. A `‹/›` mark tells you that the line is in a note script or in a code block, and not in prose. OMN-Go does not rank such a line lower, because code in notes is a normal thing to look for.

If a note has sections, the result names the section and not the line. A quick note shows its timestamp. A bookmark shows its title. A note with headings shows the heading. When you open the result, OMN-Go opens that section, and not the top of a long page.

"Line 1842" does not answer the question *where*. "27 Jul, 07:23" answers it.

OMN-Go searches bookmarks by what you see: the title, the address, the tags and the notes. It does not search the stored form. This is important. OMN-Go writes a bookmark with the title *Cats & Dogs* to disk with the `&` encoded. Before this change, you could not find that bookmark by its own name.

When you press a result in **This page**, OMN-Go closes the search panel. It marks every occurrence in the page and moves to the first one. When you press a result in **All notes**, OMN-Go opens the note of the match. The note is already scrolled to the match, and the terms you typed are marked. You do not have to find them again by eye.

The marks include the matches in a fenced code block, together with the syntax colours of that block. Before, a match in a ```` ``` ```` block was in the result list but had no mark in the page, and only prose and an inline `code` span showed one.

The full [search page](OMNGoSearch) behaves the same way. The *See all results* link opens that page. The search page is easier to read for a long list. It is also an ordinary URL that you can bookmark or send to another person.

OMN-Go removes the highlight from the address bar directly after it applies the marks. The URL that you copy or bookmark is thus the plain one. A reload of the page does not put the marks back.

In one case OMN-Go marks nothing. A term that matched only on a lower rung, such as `fecth` for `fetch`, is not in the note in the form you typed. There is thus nothing to mark. The result list has already shown you the lines that matched.

### What it costs

Page search costs nothing. Global search keeps the search index in memory. The index is approximately half the size of the text it covers, which is about 3 MB for 2000 notes. The Config page shows the current figure. On a device with little memory, keep global search disabled. Page search works in both cases.

There are two limits. First, OMN-Go searches only the first 500 KiB of a very large file. The results say so when this happens. Second, OMN-Go cannot link directly to some sections. Examples are a heading in a non-Latin script, and a heading that contains a link, code or a formula.

These results still name the section. They open the page at the top. This behavior is deliberate. OMN-Go does not make a link when the link can send you to the wrong place.

## Theme

On the [Config](Config) page, *Theme* selects **Auto**, **Light**, or
**Dark**. **Auto** follows the light or dark setting of the system. When
you save, the choice applies immediately to every page.

---

# Advanced

## Configuration reference

The [Config](Config) page edits `config.json`. The page puts the settings into six groups, and this reference uses the same groups and the same order.

### General

| Setting | Meaning |
|---------|---------|
| Author Name | OMN-Go writes this name into the `Author:` line of the header block of a new page. |
| Theme | Auto / Light / Dark, see [Theme](#theme). |
| Use Internal Editor | If you disable it, OMN-Go sends the files to an external editor. |
| Desktop External Cmd | Editor command for the desktop application (for example `subl` or `code`). |

### Network & Access

| Setting | Meaning |
|---------|---------|
| Server Port | TCP port of the OMN-Go server (default `8080`). It takes effect after a restart. |
| Admin Password | Full-access password for remote callers. |
| Guest Password | Read-oriented password for remote callers. |
| Share on LAN | Serve other devices, see [Sharing on the LAN](#sharing-on-the-lan). A change of this setting restarts the application. |

### DB Backups

| Setting | Meaning |
|---------|---------|
| Hostname (device label) | Device label in the database backup filenames (see [Database backups](#database-backups)). The default is the OS hostname. On Android, set a short name like `phone`. |
| Backup Prune Depth | How many backups to keep per database (default `3`). When you create a backup, OMN-Go deletes the oldest backups above this count. |
| Max Upload Size (MB) | Largest image or JSON file that you can drag into the editor or share into the Quick Notes page (default `3`). |

### Android Integration

| Setting | Meaning |
|---------|---------|
| Fullscreen mode | Which system bars the Android application hides. *Off* shows the status bar. *Fullscreen* hides the status bar and is the default. *Immersive* hides the status bar and the navigation bar. Swipe from an edge to show them for a short time. The setting applies as soon as you save. |
| Enable intent: links | Lets a note start Android `intent:` URIs when you press the link. Disabled by default. See [Android Intents & Termux](AndroidIntents). |
| Enable Termux commands | Also lets notes run Termux shell commands. You must enable *Enable intent: links* too. Disabled by default. See [Android Intents & Termux](AndroidIntents). |

These three settings do nothing on the desktop application and in a LAN browser.

### Search

Page search always operates and needs no setting. These settings control the global index only. See [Searching](#searching).

| Setting | Meaning |
|---------|---------|
| Enable global search | Builds and holds the index of the whole storage. Disabled by default. The index is the one standing memory cost of OMN-Go, and it is about half the size of the text that it covers. |
| Include in the index | What the index covers. *Notes* and *Bookmarks* are on by default. *Scripts (html/js)*, *JSON (html/json)* and *Uploaded JSON (html/user_json)* are off. |
| Also index OMN-Go's own scripts | Adds the scripts that come with the application to the index. Disabled by default. They are several times the size of a normal note collection. |
| Search in | Where a search starts. *All notes* is the default. *The open page only* starts each search on the page that you read. |
| Index status | Not a setting. It tells what the index holds now. |

### Git Sync

| Setting | Meaning |
|---------|---------|
| Git Servers | Up to five git server slots, see [Git synchronization](#git-synchronization). |

### Settings that only the file holds

Two settings have no field on the page. `mime_types` holds the extension-to-content-type overrides. `force_pull_one_time` makes the next git sync do one forced pull. OMN-Go normally manages `force_pull_one_time`.

`config.json` belongs to one device. OMN-Go never commits or synchronizes it. Each device keeps its own passwords, port and git keys.

## Git synchronization

OMN-Go synchronizes the whole storage directory with a git remote over SSH.

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
  A dialog then offers **Force Push** or **Abort** (see below).

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

**A rejected push** opens a dialog with two choices:

- **Force Push** — overwrites the history on the remote to match this
  device. **Destructive:** the force push destroys changes that exist
  only on the remote. Use it only when you know the remote is behind on
  purpose. If the rejected push had no commit message, the dialog asks
  for one before the force push runs.
- **Abort** — cancels the push. Your local commit stays in place. Pull
  the remote changes first, then push again.

### Files that stay on this device

Give a file or a directory a name that starts with `local-`. OMN-Go then keeps it out of git. The file stays on this device, and it does not go to the other devices.

| Name | Result |
| --- | --- |
| `html/user_json/local-data.json` | The data file stays here |
| `md/local-drafts/Monday.md` | Each note in the directory stays here |
| a database with the name `local-notes` | Each backup of it stays here |

The rule looks at the name of the file and at the name of each directory above it. The name must **start** with `local-`, and the capitals count. A file with the name `mylocal-data.json` is thus a normal file that goes to the other devices.

A file with such a name is also safe from **Force Pull**. That command deletes a local file that git does not track, but it keeps a file that `.gitignore` matches.

If you give the `local-` name to a file that the other devices already have, the next upload removes that file from git. The file stays on this device. The next download deletes the copy on each other device. The upload dialog shows the name of such a file before you write the commit message.

The older rule `/md/local/` stays. Each note in that one directory also stays on this device.

## Send and receive one note

Git synchronization moves all of your notes between your own devices. This function is different. It moves **one note** to one other person, or to a device that has no access to your git remote.

The note travels as one Markdown file. Telegram, E-Mail, LocalSend and each other application that carries files can carry it. OMN-Go does not select that application and does not know which one you use.

You must be an admin to send a note and to receive a note.

### Send a note

1. Open the note.
2. Press <i class="material-icons">info</i> in the header to show the metadata panel.
3. Press <i class="material-icons">share</i> **Send this note**, at the top of the column of buttons at the right of the panel.

On Android the share sheet opens. Select the application that carries the note.

On the desktop the browser saves the file in your download directory. Attach the file to your message yourself.

Press <i class="material-icons">content_copy</i> **Copy this note as text** to put the Markdown on the clipboard instead. Use this when you want to paste the note into a message.

These two controls show only on a note. A view with no Markdown source behind it, the [Config](Config) page for example, has only [Copy a link to this page](#copy-a-link-to-this-page).

**The name of the file.** OMN-Go makes one flat name from the full name of the note. The note `project/Sub/WeeklyPlan` becomes `project-Sub-WeeklyPlan.md`. Two notes that have the same short name are thus two different attachments in one message.

**The `FileName:` line.** OMN-Go writes the full name of the note into the header of the file that it sends. The receiver reads this line and puts the note into the same directory structure. The line is only in the file that travels. Your own note does not change.

### A message with the file

Telegram shows a caption above an attachment, and a mail client has a body above it. Give the note a description, and OMN-Go puts it there for you.

The description is an HTML comment, thus it does not show on the page. Write it below the header block:

```
Title: Weekly plan
Tags: Test, Document

<!--- DESCRIPTION:
There is some
description
--->
```

Send the note on Android, and that text is in the message beside the file. An application with no place for text beside a file ignores it, thus a description is safe to write on each note you send.

Keep a description short. OMN-Go sends the first 1000 characters, because Telegram refuses a caption that is longer than 1024 and shows no caption at all.

A description stays in the note when the note travels. The person who receives it can send the note on with the same text.

Two small rules. `<!--` and `-->` name the same block as `<!---` and `--->`, and the word `DESCRIPTION` can be in small letters. A description cannot contain `-->`, because that is where an HTML comment ends.

The desktop does not use the description. A browser download has no message with it.

### Receive a note

**On Android.** Share the note into OMN-Go from the application that holds it, or open the `.md` file from a file manager. OMN-Go saves the note and shows it. If the editor is open, OMN-Go saves the note but stays in the editor, thus you do not lose your text.

Some applications, Telegram for example, give a `.md` attachment the type `application/octet-stream`. OMN-Go is thus in the share sheet for each unknown file type. If the file is not a note, OMN-Go refuses it and shows a message.

**On the desktop.** Open the [Incoming notes](incoming/incoming) note and press **Receive a note** to open the box. Select one or more `.md` files, or drop the files on the box. Then press **Import**.

The box is closed when the page opens, because the usual reason to open this page is to see what arrived. It does not show on Android at all: there the share sheet does this work.

The box is a part of OMN-Go and not a part of the note. The note itself holds only the list, thus you can write your own text at the top of it and OMN-Go keeps that text where you put it.

### Where a received note goes

Each received note goes below `md/incoming/`. That directory is the root for the name in the `FileName:` line. A note with the name `project/Sub/WeeklyPlan` on the device of the sender becomes `incoming/project/Sub/WeeklyPlan` on your device.

**A received note never writes over one of your notes.** If the name is already in use, OMN-Go adds an index to it: `WeeklyPlan-2`, then `WeeklyPlan-3`.

OMN-Go removes the `FileName:` line and adds an `Imported:` line that holds the date and the time of the import. The `Date:` and `Modified:` lines are facts about the note of the sender, thus they stay as they are.

OMN-Go puts a link to the new note at the top of the list on the [Incoming notes](incoming/incoming) note. The newest note is the first one in the list.

The text of the link is the title of the note. If the note has no title, OMN-Go uses the file name. When a note arrives a second time, the link text carries the same index as the file, thus `Weekly plan (2)` is the second copy of `Weekly plan`.

A received note is a normal note. Read it, edit it, or move the text into your own structure.

### Limits

Only the Markdown text travels. An image or an other file that the note uses stays behind. A link to that file does not work on the device of the receiver.

The **Max Upload Size** setting on the [Config](Config) page is also the limit for a received note.

OMN-Go never deletes a received note. Delete the notes in `md/incoming/` yourself when you do not need them.

`md/incoming/` is a normal part of the storage directory. Git synchronization thus sends each received note to your own other devices too.

## Database backups

A note script can store data in a real SQL database. See the [Database](Database) page for the API. These databases are outside git. OMN-Go does **not** synchronize them with your notes automatically. To move the contents of a database between devices, or to keep a copy, create a **backup**.

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

A colored dot shows the state of each database (in sync, not backed up, backup newer, no backups, ...). A new device has no database file. On such a device, OMN-Go restores the newest backup automatically the first time a note opens the database. Two related settings are on the [Config](Config) page under **DB Backups**. **Hostname** labels this device in the backup filenames. **Backup Prune Depth** sets how many backups to keep per database.

OMN-Go backs up a database whose name starts with `local-` on the device, but keeps it out of git.

To load data from an existing SQL dump, use the [SQL Import](SQLImport) note. Such a dump is a `sqlite3 .dump` file or old `websqldump.js` output. Then press **Backup now**. See the [Database](Database) page for the full reference.

## The file index

The [file index](OMNGoFiles) shows the files that OMN-Go holds. The page is **admin-only**. From another device on the network, you must log in as admin. From the device itself, you are always admin.

### Three trees

The first screen has three buttons. Each one answers one question:

- **Bundled** — the files inside the application. This is what the version that you run carries.
- **Served** — `html/`, the files that a URL finds. A page, a script, a stylesheet, an image, and each file that you sent to a note.
- **Source** — `md/`, your notes and the text files beside them.

A tree shows **one directory at a time**, like a browser that shows a `file:///` folder. The trail at the top leads up, and each subdirectory is a link down.

### One row for one file

Each name has one row, and each name shows one time. A row has two lines. The name is on the first line, and the facts about the file are on the second line.

**Most rows say nothing more.** A note that you wrote, a page that OMN-Go made from your note, an image that you sent to a note: each of them is yours, OMN-Go does not touch it, thus the row is quiet. A row speaks only when OMN-Go is involved:

| The word | What it says |
| --- | --- |
| *not extracted* | The file comes with the application, and the device has no copy yet. |
| *changed here* | The file comes with the application, and the copy on the device is different. |
| *edited outside* | An editor that is not the OMN-Go editor wrote the copy of a text file in `html/`. Save the file one time in the editor to copy it back to `md/`. See [Text files beside your notes](#text-files-beside-your-notes). |
| *waits for restart* | The text file in `md/` is more recent than its copy. The next start of OMN-Go makes the copy again. |
| *same size* | The file is too large to compare, and the two copies have the same size. |

**The colour says what happens to the file.**

| The colour | What happens to the file |
| --- | --- |
| orange | The next version of OMN-Go replaces this file. |
| red | The next version replaces it, and you changed it. OMN-Go copies your file to a backup, and then it does not use your changes. |
| green | You changed a file that OMN-Go does not replace. Your change stays. |
| blue-green | OMN-Go repairs this by itself at the next start. |
| grey | Nothing is at stake. |

The colour is a help only. Each row that the next version replaces also shows the word **app-owned** on its second line, in each of the three trees. The stylesheets and the scripts of the frontend are app-owned, and so are eight of the notes that come with the application.

Press **What the words mean** below the trail to see the words that the directory on the screen uses. The list is closed at the start, and it gives only those words. A directory that uses no word has no such list.

The date is on the row of a file that is on the device. The date says when OMN-Go last wrote the file. Point at a date to see the time also.

A row of the **Source** tree shows **local only** when the file stays on this device. See [Files that stay on this device](#files-that-stay-on-this-device).

### Directories

A directory row gives the number of files below the directory and their size. The numbers cover each file below the directory, and not only the files that you see. The numbers thus answer the question "how large is this whole folder".

A directory says **10 from the app** when OMN-Go delivered files into it, and **2 from the app, none extracted** when the device has no copy of any of them. A directory of your own files says nothing.

### The edit link

A file that OMN-Go can show as text has an **edit** link. The link uses the `?edit=true` form from [Edit links for non-page files](#edit-links-for-non-page-files), with the address already in it.

An image, a font, a sound and a video have no edit link, because the editor would only damage the file. A page with the `.html` extension also has no edit link. Open the page and press the Edit button of the page.

A file that says *not extracted* also has an edit link. Open the link. OMN-Go writes the file to the device at that moment, and the editor opens on it.

The **Bundled** tree has no edit link. An edit always operates on the copy on the device, and that copy has a row in the Served tree or in the Source tree.

### Limits

A very large directory shows its first 200 files. It also gives the number of files that it holds back, with a link to show each of them. A directory row is never held back.

Two things never come into the file index. The page templates are a part of OMN-Go and not a part of your data. The `db_backup/` folder is on the [Database backups](#database-backups) screen instead.

Your notes are not in the Served tree. A note is not served from `html/`. Its page is, and that page says *compiled*.

The page deletes nothing and moves nothing. One action on it creates a file. The edit link of a row that says *not extracted* writes that file to the device at the moment you open the link.

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

By default, the server answers **only this device**. With LAN sharing disabled, other devices cannot open a connection.

To share your notes on the local network:

1. Set your own **admin and guest passwords** first.
2. Enable **Share on LAN** on the [Config](Config) page.
3. Save the configuration.
4. Confirm the restart prompt.

The application restarts to re-bind the server. The Android application closes. Open it again. On the desktop application, the page reloads by itself.

On Android, a **persistent notification** shows while LAN sharing is active. It gives the exact address that other devices must open, for example `http://192.168.1.5:8080`. It also has a **Stop** button. The first start with LAN sharing enabled asks for the notification permission and for an exemption from battery optimization. Grant both. If you do not, the server may not answer when the screen stays locked for some time.

With LAN sharing disabled, OMN-Go shows no notification and asks for no permissions.

On another device, open the address in a browser. Log in with the guest password for read access, or with the admin password for full access.
**Security note:** Any person on your network who has a password can
access your notes. OMN-Go has no HTTPS. Use LAN sharing only on a trusted home network. Do not use it to publish on the internet.

## Raw HTML and JavaScript in pages

Markdown pages can contain raw HTML. The [icon](#material-icons-in-notes) and [button](#buttons-and-shortcuts-inside-a-page) examples above use raw HTML. Pages can also contain note scripts. The [Bookmarks](Bookmarks) page stores its data in a note script. Four rules keep note scripts correct. See [ScriptRules](ScriptRules) for details and examples:

- Put your code in a block scope (`{ ... }` or an IIFE). Then the variables of one page cannot collide with the note scripts of another page, or with the scripts of OMN-Go.
- Attach to `window` each function that an HTML `onclick` calls.
- Write no empty line inside a `<script>` block. An empty line ends the HTML element that holds the block, and Markdown then reads the rest of the code as text.
- The backend compiles the page one time and then caches it. The note scripts run on every view, so make them idempotent.

Note scripts run with full access to the page. Put only note scripts that you understand and trust in your notes.

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


## Disclaimer

OMN-Go is a personal tool for one person. It has no separate accounts and no separate data for a second person. The admin role and the guest role give access to one set of notes. A connection from the device itself is always admin. See [Login and roles](#login-and-roles).

A note script and the SQL API operate with full rights. A script in a note can change or delete any note, any file in the storage directory and any database. Put in your notes only the scripts that you wrote, or that you trust. See [Raw HTML and JavaScript in pages](#raw-html-and-javascript-in-pages).

LAN sharing sends plain HTTP with no encryption. Another person on the same network can read what OMN-Go sends. Use LAN sharing only on a network that you control. Do not put OMN-Go on a public network, and do not open a port to it from the internet. See [Sharing on the LAN](#sharing-on-the-lan).

OMN-Go is not an enterprise product. Keep your own backup of your notes. [Git synchronization](#git-synchronization) and the [database backups](#database-backups) help you, but the backup stays your responsibility. The author gives no warranty and takes no responsibility for lost data. The MIT license gives the legal form of this statement.
