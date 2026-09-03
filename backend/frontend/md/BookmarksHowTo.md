Title: How to use Bookmarks
Date: 2026-08-03 12:00:00
Modified: 2026-09-03 12:00:00
Category: System
Author: Mikhail Basov
Tags: Bookmarks, OMN-Go, OMN-Go app

Your links stay on the [Bookmarks](Bookmarks) page.

**To save a link on Android,** press *Share* in your browser and select OMN-Go. The bookmark form opens with the address in it. Press *Save*.

On each device you can also use the button. Press the title of the page. Then press <i class="material-icons">bookmark_add</i>, write the address and press *Save*.

**To find a link,** open the [Bookmarks](Bookmarks) page. Write a word in the search box and press *Search*. The page shows only the links with that word. Press *All* to show all links again.

More ways to save a link and to find it: [User Manual](UserManual#bookmarks).

- - -

# Reference

- [The bookmark form](#the-bookmark-form)
- [Search](#search)
- [The counter and the tags cloud](#the-counter-and-the-tags-cloud)
- [The control buttons](#the-control-buttons)
- [One bookmark in the list](#one-bookmark-in-the-list)
- [A link to one bookmark](#a-link-to-one-bookmark)
- [Address parameters](#address-parameters)
- [Options](#options)
- [The file format](#the-file-format)

## The bookmark form

The form has four fields. Only the address is necessary.

| Field | Contents |
| --- | --- |
| Address | The link. An entry with an empty address does not appear on the page. |
| Title | The text of the link. The page shows the address when the title is empty. |
| Tags | One or more tags. Write a comma between two tags. |
| Notes | One or more notes. Write a semicolon between two notes. |

OMN-Go trims each tag and each note. An empty tag and an empty note go away. A trailing comma or semicolon thus adds nothing.

The *Tags* field suggests a tag after two characters. Move with the arrow keys, take the tag with *Enter*, and close the list with *Escape*. You can also press a tag with the pointer.

The suggestions come from the file `json/bookmarker-tags.json`. This file belongs to you. Open it in the editor and write the tags that you use.

OMN-Go writes each new bookmark to the [Bookmarks](Bookmarks) note.

## Search

Write a pattern in the search box. Press *Search*, or press *Enter* in the box.

OMN-Go looks in the title, the address, the tags, the notes and the date of each bookmark. The page shows each bookmark with a match.

A search pattern is a JavaScript regular expression. See [How to Use Regular Expressions in JavaScript](https://www.freecodecamp.org/news/regular-expressions-for-beginners/).

### Useful patterns

| Pattern | Finds |
| --- | --- |
| `li(-)?ion` | `liion` or `li-ion` |
| `(?=.*omn)(?=.*go)` | `omn` and `go` |
| `OMN\|mqtt` | `omn` or `mqtt` |
| `\bOpen\b` | `Open Markdown`, but not `OpenFile` |
| `Open\w` | `OpenFile`, but not `Open Markdown` |
| `^https` | each address that starts with `https` |
| `2026-08` | each bookmark of August 2026 |

A tag has priority over a search. A press on a tag button clears the search box.

OMN-Go removes the `&si=` part of a YouTube address before the search. A shared address and a copied address thus give the same result.

## The counter and the tags cloud

The number in the top right corner is the count of the bookmarks on the page. Press the number to show or to hide the tags cloud.

The cloud holds the tags of the bookmarks on the page, in alphabetic order. Press a tag to show the bookmarks with that tag. The button of the current tag is inactive.

Two special buttons stand at the start of the cloud, on a light red background.

- **NoTag** shows the bookmarks without a tag.
- **Duplicates** shows the bookmarks that have the same address as one more bookmark. OMN-Go tells you the count of the duplicate sets first.

*Duplicates* compares the bookmarks of the list that the page showed before. Press *All* first to compare all bookmarks.

The cloud stays empty when no bookmark on the page has a tag.

## The control buttons

| Button | Action |
| --- | --- |
| *Expand* | Opens the details of each bookmark on the page. |
| *Collapse* | Closes the details of each bookmark on the page. |
| *All* | Shows all bookmarks and closes the details. |

## One bookmark in the list

The page shows the newest bookmark first.

- **ⓘ** shows or hides the tags and the notes of that bookmark.
- The title is the link. A press opens the address.
- The creation date stands below the title.
- A tag is a button. A press shows the bookmarks with that tag.
- The notes stand in double quotation marks, with a comma between two notes.

Give the title the focus to read the address below it. The address goes away when a different element takes the focus.

Hold the pointer on the title for one half second. OMN-Go then copies the address to the clipboard. The browser permits this on a local address and on HTTPS only.

## A link to one bookmark

Each bookmark has an anchor. The anchor is the creation date. Remove the two colons and write a hyphen for the space.

```
/Bookmarks.html#2026-06-15-200000
```

The page moves to that bookmark and marks it. A filter that hides the bookmark goes away first.

The header search (<i class="material-icons">search</i>) makes such a link for each bookmark that it finds. Bookmarks are part of the index by default. See the [User Manual](UserManual#search).

## Address parameters

The [Bookmarks](Bookmarks) page reads three parameters. Write each value as the browser writes it in the address.

| Parameter | Action |
| --- | --- |
| `tag` | Shows the bookmarks with that tag. The value `-1` shows the bookmarks without a tag. |
| `search` | Puts the pattern in the search box and searches. |
| `config` | Changes an option. See [Options](#options). |

`tag` has priority over `search`. OMN-Go uses `tag` alone when the address holds both. `config` works together with `tag` and with `search`.

Examples:

- [Search for "omn" and "go"](/Bookmarks.html?search=%28%3F%3D.*omn%29%28%3F%3D.*go%29)
- [Search for "OMN" or "mqtt"](/Bookmarks.html?search=OMN%7Cmqtt)
- [Show the bookmarks without a tag](/Bookmarks.html?tag=-1)
- [Show the bookmarks with the tag "YouTube channel"](/Bookmarks.html?tag=YouTube%20channel)

## Options

The [Bookmarks](Bookmarks) page has two options.

### `ignoreCase`

The default value is `true`. A search then finds `tiMer` for the pattern `timer`. The two strings are different when the value is `false`.

### `stripAccents`

The default value is `true`. A search then finds `ёжик` for the pattern `ежик`, and `café` for the pattern `cafe`. The two strings are different when the value is `false`.

This option works on the title, the tags and the notes. It does not work on the address and on the date.

### To change an option

Press a link below:

- [ignoreCase off](/Bookmarks.html?config=%7B%22ignoreCase%22%3Afalse%7D)
- [ignoreCase on](/Bookmarks.html?config=%7B%22ignoreCase%22%3Atrue%7D)
- [stripAccents off](/Bookmarks.html?config=%7B%22stripAccents%22%3Afalse%7D)
- [stripAccents on](/Bookmarks.html?config=%7B%22stripAccents%22%3Atrue%7D)
- [Show the options](/Bookmarks.html?config=show)

The last link shows the default values, the stored values, the values of the address and the values in use.

OMN-Go keeps the options in the browser under the key `OMNBookmarkerConfigG`. A value from the address also goes to that store. The value thus stays after you close the page. Press the opposite link to go back.

Each browser keeps its own copy. A synchronization does not move the options to a second device.

The options are not part of the application settings. The page thus also works as an exported `.html` file on a device without OMN-Go.

## The file format

OMN-Go adds each new bookmark to the [Bookmarks](Bookmarks) note. That note is the one target of the bookmark form.

You can make a second note in the same format for a different list. Write the bookmarks of that note by hand or with the editor.

A bookmark note is a note script. Read [ScriptRules](ScriptRules) first.

### The header of the note

These two lines must stand at the top of the body, below the header block.

```
<script>bookmarks = [
<!-- Don't edit body below this line -->
```

Keep the comment line without a change. OMN-Go looks for that exact text and writes each new bookmark directly below it. OMN-Go writes no bookmark when the line is absent.

### One bookmark

```
  {
    "date": "2026-08-03 12:00:00",
    "url": "https://github.com/mvbasov/OMN-Go",
    "title": "GitHub - mvbasov/OMN-Go",
    "tags": [
      "Reference",
      "Go lang"
    ],
    "notes": [
      "Note 1",
      "Note 2"
    ]
  },
```

Write the date as `YYYY-MM-DD HH:MM:SS`. Write an empty list for a bookmark without tags or without notes. A comma after the last bookmark is correct.

### The footer of the note

These lines must stand at the end of the note.

```
];
</script>

<!-- end of bookmarks definition -->

<link rel="stylesheet" type="text/css" href="/css/Bookmarker.css" />
<script type="text/javascript" src="/js/Bookmarker.js"></script>
```

The two paths point to the files of the application. Change them when you export the note to a different directory.

### One limit

Write no bullet list and no numbered list in a bookmark note. *Expand* and *Collapse* work on the first list of the page. A list above the bookmarks takes that place and stops the two buttons.
